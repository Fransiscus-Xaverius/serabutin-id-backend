package usecase

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/idempotency"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/logger"
)

const (
	payoutCurrency         = "IDR"
	payoutChannel          = "BI_FAST"
	payoutRemark           = "Serabutin payout"
	bookingStatusConfirmed = "confirmed"
	bookingStatusCompleted = "completed"
	// payoutReconcileBatch bounds one reconciliation pass.
	payoutReconcileBatch = 50
	// payoutReconcileInterval is the least time between two DOKU status
	// checks of the same payout.
	payoutReconcileInterval = 5 * time.Minute
	// payoutInquiryStaleAfter is how long a payout may sit in inquiry before
	// the worker re-drives it.
	payoutInquiryStaleAfter = 5 * time.Minute
	payoutStatusCheckFailed = "transaction status check failed"
)

// The money-moving steps of a payout. Each has its own X-EXTERNAL-ID, derived
// from the payout row, so DOKU deduplicates a retry of the same step.
const (
	externalStepInquiry = 1
	externalStepPayment = 2
)

var _ TransferGateway = (*doku.SnapPayoutClient)(nil)

// SecretBox is the encryption boundary the payout usecase depends on.
type SecretBox interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
	Fingerprint(plaintext string) string
}

// UserNameLookup is the part of the user repository the payout usecase needs
// to compare the submitted holder name with the registered name.
type UserNameLookup interface {
	FindByID(ctx context.Context, id uint) (*entity.User, error)
}

// PayoutAccountService is what the payout HTTP handler depends on. It exposes
// no decryption: plaintext is reachable only through the concrete
// *PayoutUsecase, which the future transfer path holds.
type PayoutAccountService interface {
	Upsert(ctx context.Context, userID uint, req dto.PayoutAccountRequest) (*entity.PayoutAccount, error)
	Get(ctx context.Context, userID uint) (*entity.PayoutAccount, error)
}

// PayoutDestination is the decrypted form, used only when paying out.
type PayoutDestination struct {
	BankCode          string
	BankName          string
	AccountHolderName string
	AccountNumber     string
	Phone             string
}

// PayoutBookingLookup is the part of the booking repository a release reads.
type PayoutBookingLookup interface {
	FindByID(ctx context.Context, id uint) (*entity.Booking, error)
}

// PayoutPaymentLookup is the part of the payment repository a release reads.
type PayoutPaymentLookup interface {
	FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payment, error)
}

// TransferGateway is the money-moving boundary of the payout usecase.
type TransferGateway interface {
	TransferInquiry(ctx context.Context, externalID string, req doku.TransferInquiryRequest) (*doku.TransferInquiryResult, error)
	TransferPayment(ctx context.Context, externalID string, req doku.TransferPaymentRequest) (*doku.TransferPaymentResult, error)
	TransactionStatus(ctx context.Context, externalID string, req doku.TransactionStatusRequest) (*doku.TransactionStatusResult, error)
	BalanceInquiry(ctx context.Context, externalID string, profileID string) (*doku.BalanceResult, error)
}

// PayoutRelease holds what only the money-moving path needs. It is attached
// with WithRelease; without it Release and SyncTransferStatus refuse to run.
type PayoutRelease struct {
	Payouts  repository.PayoutRepository
	Bookings PayoutBookingLookup
	Payments PayoutPaymentLookup
	Gateway  TransferGateway
	// ProfileID is the merchant payout account number used by the SNAP
	// balance-inquiry call. It gates every release.
	ProfileID string
}

type PayoutUsecase struct {
	repo            repository.PayoutAccountRepository
	users           UserNameLookup
	secret          SecretBox
	encryptionKeyID string
	release         *PayoutRelease
	// settleMu makes the settled-balance gate and the reservation of the
	// payout one step, so two releases cannot both pass on the same balance.
	settleMu sync.Mutex
}

func NewPayoutUsecase(repo repository.PayoutAccountRepository, users UserNameLookup, secret SecretBox, encryptionKeyID string) *PayoutUsecase {
	if encryptionKeyID == "" {
		encryptionKeyID = "v1"
	}
	return &PayoutUsecase{repo: repo, users: users, secret: secret, encryptionKeyID: encryptionKeyID}
}

func (uc *PayoutUsecase) Upsert(ctx context.Context, userID uint, req dto.PayoutAccountRequest) (*entity.PayoutAccount, error) {
	if err := req.Validate(); err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_PAYOUT_ACCOUNT", err.Error())
	}

	fingerprint := uc.secret.Fingerprint(req.AccountNumber)
	// Two Mitras must not share one bank account: the payout would go to
	// one of them. The fingerprint detects this without decrypting anything.
	existing, err := uc.repo.FindByFingerprint(ctx, fingerprint)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return nil, err
	}
	if err == nil && existing != nil && existing.UserID != userID {
		return nil, apperror.NewDomainError(apperror.ErrAlreadyExists, "PAYOUT_ACCOUNT_DUPLICATE", "this bank account is already used by another mitra")
	}

	user, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	holderName := strings.TrimSpace(req.AccountHolderName)
	encryptedNumber, err := uc.secret.Encrypt(req.AccountNumber)
	if err != nil {
		return nil, err
	}
	encryptedHolder, err := uc.secret.Encrypt(holderName)
	if err != nil {
		return nil, err
	}
	phone := doku.NormalizePhone(req.Phone)
	encryptedPhone, err := uc.secret.Encrypt(phone)
	if err != nil {
		return nil, err
	}

	// A re-save always starts unverified: a changed account number must be
	// validated again.
	account := &entity.PayoutAccount{
		UserID:                     userID,
		BankCode:                   req.BankCode,
		BankName:                   strings.TrimSpace(req.BankName),
		AccountHolderNameEncrypted: encryptedHolder,
		AccountNumberEncrypted:     encryptedNumber,
		PhoneEncrypted:             encryptedPhone,
		AccountLast4:               lastFour(req.AccountNumber),
		PhoneLast4:                 lastFour(phone),
		AccountFingerprint:         fingerprint,
		EncryptionKeyID:            uc.encryptionKeyID,
		Status:                     entity.PayoutAccountStatusUnverified,
		// Flagged, not blocked: the production entity (PT/CV) is unresolved.
		NameMismatch: !sameName(holderName, user.Name),
	}
	if err := uc.repo.Upsert(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// Get returns the stored account (still encrypted) so the handler can mask it.
func (uc *PayoutUsecase) Get(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	return uc.repo.FindByUserID(ctx, userID)
}

// DecryptDestination returns the plain destination for a payout. Only the
// future transfer path may call it; it never reaches a handler.
func (uc *PayoutUsecase) DecryptDestination(ctx context.Context, userID uint) (PayoutDestination, error) {
	account, err := uc.repo.FindByUserID(ctx, userID)
	if err != nil {
		return PayoutDestination{}, err
	}
	number, err := uc.secret.Decrypt(account.AccountNumberEncrypted)
	if err != nil {
		return PayoutDestination{}, err
	}
	holder, err := uc.secret.Decrypt(account.AccountHolderNameEncrypted)
	if err != nil {
		return PayoutDestination{}, err
	}
	phone, err := uc.secret.Decrypt(account.PhoneEncrypted)
	if err != nil {
		return PayoutDestination{}, err
	}
	return PayoutDestination{
		BankCode:          account.BankCode,
		BankName:          account.BankName,
		AccountHolderName: holder,
		AccountNumber:     number,
		Phone:             phone,
	}, nil
}

// sameName compares two names case-insensitively, ignoring spaces and
// punctuation, so "Siti Rahayu" matches "SITI RAHAYU" but "PT Contoh" does not
// match "Contoh".
func sameName(a, b string) bool {
	return normalizeName(a) == normalizeName(b)
}

func normalizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// firstNonEmptyString returns the first non-blank value.
func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// lastFour returns the last four characters of an account number for masked
// display. The full value is never stored in plaintext.
func lastFour(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return accountNumber
	}
	return accountNumber[len(accountNumber)-4:]
}

// WithRelease attaches the money-moving dependencies.
func (uc *PayoutUsecase) WithRelease(release PayoutRelease) *PayoutUsecase {
	uc.release = &release
	return uc
}

// Release pays the Mitra of a booking. The payout row is created once per
// booking and reused by every later call, so a retry continues the same
// transfer under the same partnerReferenceNo, which DOKU answers as a replay.
func (uc *PayoutUsecase) Release(ctx context.Context, bookingID uint) (*entity.Payout, error) {
	release, err := uc.releaseDeps()
	if err != nil {
		return nil, err
	}

	booking, err := release.Bookings.FindByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if booking.Status != bookingStatusConfirmed && booking.Status != bookingStatusCompleted {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYOUT_BOOKING_NOT_RELEASABLE", "booking must be confirmed or completed before its payout")
	}
	payment, err := release.Payments.FindByBookingID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if payment.Status != entity.PaymentStatusPaid {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYOUT_BOOKING_NOT_PAID", "booking has no paid payment")
	}

	account, err := uc.repo.FindByUserID(ctx, booking.ProviderID)
	if err != nil {
		return nil, err
	}
	holder, err := uc.secret.Decrypt(account.AccountHolderNameEncrypted)
	if err != nil {
		return nil, err
	}
	fingerprint := uc.destinationFingerprint(booking, account, holder)

	payout, err := release.Payouts.FindByBookingID(ctx, bookingID)
	if errors.Is(err, apperror.ErrNotFound) {
		payout, err = uc.createPayout(ctx, release, booking, account, fingerprint)
	}
	if err != nil {
		return nil, err
	}
	if payout.BeneficiaryFingerprint != fingerprint {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYOUT_DESTINATION_CHANGED", "the bank account or holder name changed after this booking's payout was created")
	}
	switch payout.Status {
	case entity.PayoutStatusSubmitted, entity.PayoutStatusPaid:
		return payout, nil
	case entity.PayoutStatusInquiry:
		// A failed inquiry or a crash can leave the payout mid-flow. Ask DOKU
		// first: only its typed not-found answer allows the inquiry and the
		// payment to be sent again under the same partnerReferenceNo.
		_, err := uc.redriveInquiry(ctx, release, payout)
		if err != nil {
			return nil, err
		}
		return payout, nil
	case entity.PayoutStatusFailed:
		// The provider may already hold this transfer, so its state is read
		// before anything is sent. A failed payout that DOKU still reports as
		// failed is attempted again under the same key. Real DOKU may replay
		// the inquiry, which makes a new PO-<bookingID>-R<n> reference
		// necessary; that scheme is not implemented yet.
		from := payout.Status
		if err := uc.reconcile(ctx, release, payout); err != nil {
			return nil, uc.failTransfer(ctx, release, payout, "transaction status")
		}
		if payout.Status != entity.PayoutStatusFailed {
			if err := uc.save(ctx, release, payout, from); err != nil {
				return nil, err
			}
			return payout, nil
		}
	}
	return uc.transfer(ctx, release, payout)
}

// reconcile reads DOKU's answer for the payout and applies the state that
// answer describes to the in-memory payout. The caller saves it. An error or
// an unknown code changes nothing: the caller must not send a transfer on an
// unknown state.
func (uc *PayoutUsecase) reconcile(ctx context.Context, release *PayoutRelease, payout *entity.Payout) error {
	readID, err := uc.nextReadID(ctx, release, payout)
	if err != nil {
		return err
	}
	number, err := uc.secret.Decrypt(payout.BeneficiaryAccountNumberEncrypted)
	if err != nil {
		return err
	}
	status, err := release.Gateway.TransactionStatus(ctx, readID, doku.TransactionStatusRequest{
		PartnerReferenceNo:       payout.PartnerReferenceNo,
		BeneficiaryAccountNumber: number,
		BeneficiaryBankCode:      payout.BeneficiaryBankCode,
	})
	if err != nil {
		return err
	}
	return applyTransactionStatus(payout, status)
}

// applyTransactionStatus moves the payout to the state DOKU's answer describes.
// An unknown code is an error and changes nothing.
func applyTransactionStatus(payout *entity.Payout, status *doku.TransactionStatusResult) error {
	// A transfer response can be lost after DOKU accepted the payment. The
	// check-status answer then carries the same identifiers, so store them
	// before the payout can become paid. A refund notification is resolved by
	// this DOKU reference.
	if payout.DokuReferenceNo == "" {
		payout.DokuReferenceNo = status.OriginalReferenceNo
	}
	if payout.DokuSessionID == "" {
		payout.DokuSessionID = status.SessionID
	}
	switch status.LatestStatus {
	case doku.TransactionStatusSuccess:
		now := time.Now()
		payout.Status = entity.PayoutStatusPaid
		payout.PaidAt = &now
		payout.LastError = ""
	case doku.TransactionStatusPending:
		payout.Status = entity.PayoutStatusSubmitted
		payout.LastError = ""
	case doku.TransactionStatusFailed, doku.TransactionStatusRefunded:
		payout.Status = entity.PayoutStatusFailed
		payout.LastError = transferFailureReason(status)
	default:
		return errors.New("unknown DOKU transaction status " + status.LatestStatus)
	}
	return nil
}

// transferFailureReason names why DOKU did not deliver the money. It keeps the
// status code only: provider text can echo beneficiary data.
func transferFailureReason(status *doku.TransactionStatusResult) string {
	label := "failed"
	if status.LatestStatus == doku.TransactionStatusRefunded {
		label = "refunded"
	}
	return "doku transfer " + label + " (status " + status.LatestStatus + ")"
}

// checkSettlement refuses a release unless the merchant payout balance covers
// the payout and every payout already in flight. A held balance never counts.
func (uc *PayoutUsecase) checkSettlement(ctx context.Context, release *PayoutRelease, externalID string, amount int64, inFlight int64) error {
	balance, err := release.Gateway.BalanceInquiry(ctx, externalID, release.ProfileID)
	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_SETTLEMENT_UNAVAILABLE", "cannot read the merchant payout balance, the payout was not released")
	}
	available, ok := balance.SettledAvailable()
	if !ok {
		return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_SETTLEMENT_UNAVAILABLE", "the merchant payout account has no available balance, the payout was not released")
	}
	cents, ok := settledCents(available)
	if !ok {
		return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_SETTLEMENT_UNAVAILABLE", "the merchant payout balance is not a valid amount, the payout was not released")
	}
	if cents < (amount+inFlight)*100 {
		return apperror.NewDomainError(apperror.ErrValidation, "PAYOUT_INSUFFICIENT_SETTLED_BALANCE", "the merchant payout balance is below the payout amount and the payouts in flight")
	}
	return nil
}

// settledCents parses a DOKU IDR amount such as "150000" or "150000.50" into
// whole rupiah and cents, without floating point. Anything else is rejected.
func settledCents(value string) (int64, bool) {
	whole, frac, _ := strings.Cut(value, ".")
	if whole == "" || len(frac) > 2 {
		return 0, false
	}
	for _, r := range whole + frac {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	frac += strings.Repeat("0", 2-len(frac))
	cents, err := strconv.ParseInt(whole+frac, 10, 64)
	return cents, err == nil
}

// SyncTransferStatus records the outcome of a payout's transfer as the SNAP
// check-status API reports it. The caller supplies only the partner
// reference: the outcome is never taken from a notification body. A paid
// payout returns unchanged without asking DOKU; a refund reopens it only
// through SyncRefundStatus. Every attempt is stamped, so the reconciliation
// waits before asking again; a failed attempt also keeps a LastError, and the
// state is never changed by it.
func (uc *PayoutUsecase) SyncTransferStatus(ctx context.Context, partnerReferenceNo string) (*entity.Payout, error) {
	release, err := uc.releaseDeps()
	if err != nil {
		return nil, err
	}
	payout, err := release.Payouts.FindByPartnerReferenceNo(ctx, partnerReferenceNo)
	if err != nil {
		return nil, err
	}
	return uc.syncStatus(ctx, release, payout, false)
}

// SyncRefundStatus reconciles a refund notification. A refund notification
// carries only transactionId, which DOKU fills with its own transfer
// reference. The outcome still comes from check-status: a paid payout stays
// paid unless DOKU answers refunded or failed.
func (uc *PayoutUsecase) SyncRefundStatus(ctx context.Context, transactionID string) (*entity.Payout, error) {
	release, err := uc.releaseDeps()
	if err != nil {
		return nil, err
	}
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYOUT_NOTIFICATION_INVALID", "transaction id is required")
	}
	payout, err := release.Payouts.FindByDokuReferenceNo(ctx, transactionID)
	if err != nil {
		return nil, err
	}
	return uc.syncStatus(ctx, release, payout, true)
}

// syncStatus applies DOKU's answer. A paid payout is terminal for ordinary
// status checks. The refund path may reopen it, but only when DOKU reports
// refunded or failed; a late success or pending answer leaves it paid and
// keeps its original paid_at.
func (uc *PayoutUsecase) syncStatus(ctx context.Context, release *PayoutRelease, payout *entity.Payout, allowPaidRefund bool) (*entity.Payout, error) {
	if payout.Status == entity.PayoutStatusPaid && !allowPaidRefund {
		return payout, nil
	}
	from := payout.Status
	paidAt := payout.PaidAt
	now := time.Now()
	payout.LastReconciledAt = &now
	if err := uc.reconcile(ctx, release, payout); err != nil {
		payout.LastError = payoutStatusCheckFailed
		if updateErr := uc.save(ctx, release, payout, from); updateErr != nil {
			return nil, updateErr
		}
		return nil, err
	}
	if from == entity.PayoutStatusPaid && payout.Status != entity.PayoutStatusFailed {
		payout.Status = entity.PayoutStatusPaid
		payout.PaidAt = paidAt
		payout.LastError = ""
	}
	if err := uc.save(ctx, release, payout, from); err != nil {
		return nil, err
	}
	return payout, nil
}

// ReconcileSubmitted asks DOKU for the outcome of up to limit submitted
// payouts that were not reconciled in the last payoutReconcileInterval, and
// records each answer through SyncTransferStatus. A row that fails is logged
// and skipped, so one bad row cannot stall the rest. It returns how many
// payouts left the submitted state.
func (uc *PayoutUsecase) ReconcileSubmitted(ctx context.Context, limit int) (int, error) {
	release, err := uc.releaseDeps()
	if err != nil {
		return 0, err
	}
	payouts, err := release.Payouts.FindSubmitted(ctx, limit, time.Now().Add(-payoutReconcileInterval))
	if err != nil {
		return 0, err
	}
	resolved := 0
	for _, payout := range payouts {
		alreadyFailing := payout.LastError != ""
		synced, err := uc.SyncTransferStatus(ctx, payout.PartnerReferenceNo)
		if err != nil {
			logUnreconciled(payout, alreadyFailing, err)
			continue
		}
		if synced.Status != entity.PayoutStatusSubmitted {
			resolved++
		}
	}
	return resolved, nil
}

// logUnreconciled reports a payout the reconciliation could not resolve. The
// first failure is a warning; a payout that keeps failing logs at debug level,
// so a permanently unknown transfer does not flood the log every pass.
func logUnreconciled(payout *entity.Payout, alreadyFailing bool, err error) {
	event := logger.Log.Warn()
	message := "payout not reconciled, will retry"
	if alreadyFailing {
		event = logger.Log.Debug()
		message = "payout still not reconciled"
	}
	event.Uint("payout_id", payout.ID).Str("partner_reference_no", payout.PartnerReferenceNo).Err(err).Msg(message)
}

// ReconcileStalledInquiries re-drives up to limit payouts stuck in inquiry for
// longer than payoutInquiryStaleAfter. It returns how many moved out of
// inquiry. See redriveInquiry for the rule that makes a re-drive safe.
func (uc *PayoutUsecase) ReconcileStalledInquiries(ctx context.Context, limit int) (int, error) {
	release, err := uc.releaseDeps()
	if err != nil {
		return 0, err
	}
	payouts, err := release.Payouts.FindStalledInquiry(ctx, limit, time.Now().Add(-payoutInquiryStaleAfter))
	if err != nil {
		return 0, err
	}
	resolved := 0
	for _, payout := range payouts {
		moved, err := uc.redriveInquiry(ctx, release, payout)
		if err != nil {
			logger.Log.Warn().Uint("payout_id", payout.ID).Str("partner_reference_no", payout.PartnerReferenceNo).Err(err).Msg("stalled payout not re-driven, will retry")
			continue
		}
		if moved {
			resolved++
		}
	}
	return resolved, nil
}

// redriveInquiry settles a payout stuck in inquiry. If DOKU holds the partner
// reference, its answer is adopted and nothing is sent again. Only DOKU's
// typed not-found error means DOKU has no record: no money has moved, so the
// inquiry and the payment are sent again under the same partnerReferenceNo,
// which DOKU deduplicates. Any other failure says nothing about the transfer,
// so the payout is skipped for this pass and its attempt refreshes updated_at.
func (uc *PayoutUsecase) redriveInquiry(ctx context.Context, release *PayoutRelease, payout *entity.Payout) (bool, error) {
	before := payout.Status
	readID, err := uc.nextReadID(ctx, release, payout)
	if err != nil {
		return false, err
	}
	number, err := uc.secret.Decrypt(payout.BeneficiaryAccountNumberEncrypted)
	if err != nil {
		return false, err
	}
	status, err := release.Gateway.TransactionStatus(ctx, readID, doku.TransactionStatusRequest{
		PartnerReferenceNo:       payout.PartnerReferenceNo,
		BeneficiaryAccountNumber: number,
		BeneficiaryBankCode:      payout.BeneficiaryBankCode,
	})
	switch {
	case err == nil:
		if err := applyTransactionStatus(payout, status); err != nil {
			return false, err
		}
		if err := uc.save(ctx, release, payout, before); err != nil {
			return false, err
		}
		return payout.Status != before, nil
	case errors.Is(err, doku.ErrTransactionNotFound):
		// DOKU has no record of the reference: the re-drive below sends it.
	default:
		return false, uc.failTransfer(ctx, release, payout, "transaction status")
	}

	redriven, err := uc.transfer(ctx, release, payout)
	if err != nil {
		return false, err
	}
	return redriven.Status != before, nil
}

// StartPayoutReconcileWorker reconciles submitted payouts and re-drives stalled
// inquiries every interval until ctx is cancelled. It is the guarantee behind
// the transfer webhook: a missed or unroutable notification is still resolved
// here, from DOKU's status API. A deployment without the release configuration
// runs no pass at all.
func StartPayoutReconcileWorker(ctx context.Context, uc *PayoutUsecase, interval time.Duration) {
	if uc == nil {
		return
	}
	if err := uc.releaseGate(); err != nil {
		logger.Log.Info().Err(err).Msg("payout reconcile worker not started: payout release is not configured")
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}

	run := func() {
		resolved, err := uc.ReconcileSubmitted(ctx, payoutReconcileBatch)
		if err != nil {
			logger.Log.Error().Err(err).Msg("payout reconcile: submitted pass failed")
		} else if resolved > 0 {
			logger.Log.Info().Int("resolved", resolved).Msg("payout reconcile: resolved submitted payouts")
		}
		moved, err := uc.ReconcileStalledInquiries(ctx, payoutReconcileBatch)
		if err != nil {
			logger.Log.Error().Err(err).Msg("payout reconcile: stalled inquiry pass failed")
		} else if moved > 0 {
			logger.Log.Info().Int("moved", moved).Msg("payout reconcile: moved stalled inquiries")
		}
	}

	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (uc *PayoutUsecase) releaseDeps() (*PayoutRelease, error) {
	if uc.release == nil {
		return nil, errors.New("payout release is not configured")
	}
	return uc.release, nil
}

// releaseGate is the configuration a release needs: the merchant payout
// source account. It is the same condition the admin release is gated on.
func (uc *PayoutUsecase) releaseGate() error {
	release, err := uc.releaseDeps()
	if err != nil {
		return err
	}
	if release.ProfileID == "" {
		return errors.New("DOKU_PAYOUT_SOURCE_ACCOUNT must be set")
	}
	return nil
}

// destinationFingerprint is the encryption-keyed digest of the payout
// destination. The account number enters it through its stored keyed digest,
// so deciding whether the destination changed never decrypts the number.
func (uc *PayoutUsecase) destinationFingerprint(booking *entity.Booking, account *entity.PayoutAccount, holder string) string {
	return uc.secret.Fingerprint(idempotency.CanonicalRequest(
		strconv.FormatUint(uint64(booking.ID), 10),
		strconv.FormatInt(booking.SubTotal, 10),
		payoutCurrency,
		account.BankCode,
		account.AccountFingerprint,
		holder,
	))
}

// createPayout stores the one payout row of a booking. The beneficiary is
// copied in encrypted form only. A concurrent release that wins the unique
// index is continued instead of failed.
func (uc *PayoutUsecase) createPayout(ctx context.Context, release *PayoutRelease, booking *entity.Booking, account *entity.PayoutAccount, fingerprint string) (*entity.Payout, error) {
	key := idempotency.PayoutKey(booking.ID)
	payout := &entity.Payout{
		BookingID:          booking.ID,
		ProviderID:         booking.ProviderID,
		Amount:             booking.SubTotal,
		Currency:           payoutCurrency,
		Channel:            payoutChannel,
		Status:             entity.PayoutStatusPending,
		PartnerReferenceNo: key,
		IdempotencyKey:     key,
		RequestFingerprint: idempotency.Fingerprint(idempotency.CanonicalRequest(
			strconv.FormatUint(uint64(booking.ID), 10),
			strconv.FormatInt(booking.SubTotal, 10),
			payoutCurrency,
			account.BankCode,
		)),

		BeneficiaryBankCode:                   account.BankCode,
		BeneficiaryAccountHolderNameEncrypted: account.AccountHolderNameEncrypted,
		BeneficiaryAccountNumberEncrypted:     account.AccountNumberEncrypted,
		BeneficiaryLast4:                      account.AccountLast4,
		BeneficiaryFingerprint:                fingerprint,
	}
	err := release.Payouts.Create(ctx, payout)
	if errors.Is(err, apperror.ErrAlreadyExists) {
		return release.Payouts.FindByBookingID(ctx, booking.ID)
	}
	if err != nil {
		return nil, err
	}
	return payout, nil
}

// externalIDFor is the X-EXTERNAL-ID of one money-moving DOKU request of a
// payout: payoutID*10 + step. It is derived from the persisted row, so a retry
// of the same step sends the same id, which DOKU answers as a duplicate.
func externalIDFor(payout *entity.Payout, step int64) string {
	return strconv.FormatInt(int64(payout.ID)*10+step, 10)
}

// nextReadID takes the X-EXTERNAL-ID of the next read-only DOKU call from the
// persisted read sequence, which never reaches the money-moving band. The
// attempt is saved before the call goes out, and saving it also refreshes
// updated_at, which spaces out the next pass for this payout.
func (uc *PayoutUsecase) nextReadID(ctx context.Context, release *PayoutRelease, payout *entity.Payout) (string, error) {
	readID, err := release.Payouts.NextReadExternalID(ctx)
	if err != nil {
		return "", err
	}
	payout.ReadAttempts++
	if err := uc.save(ctx, release, payout, payout.Status); err != nil {
		return "", err
	}
	return readID, nil
}

// reserve passes the settled-balance gate and records the payout as in flight,
// as one step under settleMu. The in-flight total is read before the balance:
// a payout that completes between the two reads only makes the gate stricter.
func (uc *PayoutUsecase) reserve(ctx context.Context, release *PayoutRelease, payout *entity.Payout) error {
	uc.settleMu.Lock()
	defer uc.settleMu.Unlock()

	inFlight, err := release.Payouts.SumInFlight(ctx, payout.ID)
	if err != nil {
		return err
	}
	from := payout.Status
	balanceID, err := uc.nextReadID(ctx, release, payout)
	if err != nil {
		return err
	}
	if err := uc.checkSettlement(ctx, release, balanceID, payout.Amount, inFlight); err != nil {
		return err
	}
	payout.Status = entity.PayoutStatusInquiry
	return uc.save(ctx, release, payout, from)
}

// transfer runs the SNAP account inquiry and then the bank transfer, each
// under the payout's stored partnerReferenceNo. A retry always starts again at
// the inquiry, which DOKU answers as a replay with the original session.
func (uc *PayoutUsecase) transfer(ctx context.Context, release *PayoutRelease, payout *entity.Payout) (*entity.Payout, error) {
	number, err := uc.secret.Decrypt(payout.BeneficiaryAccountNumberEncrypted)
	if err != nil {
		return nil, err
	}
	holder, err := uc.secret.Decrypt(payout.BeneficiaryAccountHolderNameEncrypted)
	if err != nil {
		return nil, err
	}
	account, err := uc.repo.FindByUserID(ctx, payout.ProviderID)
	if err != nil {
		return nil, err
	}
	// Accounts saved before the SNAP migration have no phone column value.
	// Leave it empty so the gateway can use DOKU_PAYOUT_CUSTOMER_NUMBER.
	var phone string
	if account.PhoneEncrypted != "" {
		phone, err = uc.secret.Decrypt(account.PhoneEncrypted)
		if err != nil {
			return nil, err
		}
	}
	inquiryReq := doku.TransferInquiryRequest{
		PartnerReferenceNo:       payout.PartnerReferenceNo,
		Amount:                   payout.Amount,
		BeneficiaryBankCode:      payout.BeneficiaryBankCode,
		BeneficiaryAccountNumber: number,
		Remark:                   payoutRemark,
		CustomerNumber:           doku.NormalizePhone(phone),
		BeneficiaryName:          holder,
		BeneficiaryPhone:         doku.NormalizePhone(phone),
		BeneficiaryBankName:      account.BankName,
	}
	if err := uc.reserve(ctx, release, payout); err != nil {
		return nil, err
	}

	inquiry, err := release.Gateway.TransferInquiry(ctx, externalIDFor(payout, externalStepInquiry), inquiryReq)
	if err != nil {
		return nil, uc.failTransfer(ctx, release, payout, "transfer inquiry")
	}
	payout.DokuInquiryReferenceNo = firstNonEmptyString(inquiry.ReferenceNo, payout.DokuInquiryReferenceNo)
	payout.DokuSessionID = firstNonEmptyString(inquiry.SessionID, payout.DokuSessionID)
	payout.Replayed = inquiry.Replayed
	if inquiry.Replayed {
		// DOKU already holds the inquiry. Adopt the state it reports; only its
		// typed not-found answer means no transfer was ever recorded, so the
		// payment below is safe to send under the same partner reference.
		switch err := uc.reconcile(ctx, release, payout); {
		case err == nil:
			if err := uc.save(ctx, release, payout, entity.PayoutStatusInquiry); err != nil {
				return nil, err
			}
			return payout, nil
		case errors.Is(err, doku.ErrTransactionNotFound):
			// Continue to the payment.
		default:
			return nil, uc.failTransfer(ctx, release, payout, "transaction status")
		}
	}
	if err := uc.save(ctx, release, payout, entity.PayoutStatusInquiry); err != nil {
		return nil, err
	}

	paymentExternalID := externalIDFor(payout, externalStepPayment)
	sessionID := firstNonEmptyString(inquiry.SessionID, payout.DokuSessionID, inquiry.ReferenceNo)
	paid, err := release.Gateway.TransferPayment(ctx, paymentExternalID, doku.TransferPaymentRequest{
		TransferInquiryRequest: inquiryReq,
		ReferenceNo:            sessionID,
		SessionID:              sessionID,
		BeneficiaryAccountName: inquiry.BeneficiaryAccountName,
	})
	if err != nil {
		return nil, uc.failTransfer(ctx, release, payout, "transfer payment")
	}
	payout.DokuReferenceNo = paid.ReferenceNo
	payout.DokuReferenceNumber = paid.ReferenceNumber
	payout.ExternalID = paymentExternalID
	payout.Replayed = payout.Replayed || paid.Replayed
	payout.Status = entity.PayoutStatusSubmitted
	payout.LastError = ""
	if err := uc.save(ctx, release, payout, entity.PayoutStatusInquiry); err != nil {
		return nil, err
	}

	// A 409 on the payment means DOKU already holds this transfer. It is
	// mapped from DOKU's status and never sent again as a new transfer. A
	// failed reconciliation keeps the payout submitted: DOKU has the transfer.
	if paid.Replayed {
		if err := uc.reconcile(ctx, release, payout); err != nil {
			payout.LastError = payoutStatusCheckFailed
		}
		if err := uc.save(ctx, release, payout, entity.PayoutStatusSubmitted); err != nil {
			return nil, err
		}
	}
	return payout, nil
}

// save writes a payout's state. The status moves only while the row is still
// in from, the status this copy was read with. The guarded update then writes
// the other columns only while the row still carries the resulting status, so
// a stale copy cannot clear a newer paid_at or doku_session_id.
func (uc *PayoutUsecase) save(ctx context.Context, release *PayoutRelease, payout *entity.Payout, from string) error {
	moved, err := release.Payouts.Transition(ctx, payout.ID, from, payout.Status)
	if err != nil {
		return err
	}
	if !moved {
		return payoutStateChangedError()
	}
	return release.Payouts.UpdateIfStatus(ctx, payout, payout.Status)
}

// failTransfer records a failed provider step without the provider's raw
// response, which can echo the beneficiary account number. The payout keeps
// its state, so the next release retries the same step with the same key.
// A failed step never changes the in-memory status, so the state saved is the
// one the payout was read with.
func (uc *PayoutUsecase) failTransfer(ctx context.Context, release *PayoutRelease, payout *entity.Payout, step string) error {
	payout.LastError = step + " failed"
	if err := uc.save(ctx, release, payout, payout.Status); err != nil {
		return err
	}
	return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_TRANSFER_FAILED", step+" failed, retry the release")
}

func payoutStateChangedError() error {
	return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_STATE_CHANGED", "the payout changed while it was being processed, retry the release")
}
