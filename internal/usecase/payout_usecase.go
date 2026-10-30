package usecase

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/idempotency"
)

const (
	payoutCurrency         = "IDR"
	payoutChannel          = "BI_FAST"
	payoutRemark           = "Serabutin payout"
	bookingStatusConfirmed = "confirmed"
	bookingStatusCompleted = "completed"
)

var _ TransferGateway = (*doku.SubAccountClient)(nil)

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
	TransferInquiry(ctx context.Context, req doku.TransferInquiryRequest) (*doku.TransferInquiryResult, error)
	TransferPayment(ctx context.Context, req doku.TransferPaymentRequest) (*doku.TransferPaymentResult, error)
	TransactionStatus(ctx context.Context, partnerReferenceNo string) (*doku.TransactionStatusResult, error)
	BalanceInquiry(ctx context.Context, profileID string) (*doku.BalanceResult, error)
}

// PayoutRelease holds what only the money-moving path needs. It is attached
// with WithRelease; without it Release and ApplyTransferStatus refuse to run.
type PayoutRelease struct {
	Payouts     repository.PayoutRepository
	Bookings    PayoutBookingLookup
	Payments    PayoutPaymentLookup
	Gateway     TransferGateway
	FromAccount string
	// ProfileID is the hub profile whose settled balance gates every release.
	ProfileID string
}

type PayoutUsecase struct {
	repo            repository.PayoutAccountRepository
	users           UserNameLookup
	secret          SecretBox
	encryptionKeyID string
	release         *PayoutRelease
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

	// A re-save always starts unverified: a changed account number must be
	// validated again.
	account := &entity.PayoutAccount{
		UserID:                     userID,
		BankCode:                   req.BankCode,
		BankName:                   strings.TrimSpace(req.BankName),
		AccountHolderNameEncrypted: encryptedHolder,
		AccountNumberEncrypted:     encryptedNumber,
		AccountLast4:               lastFour(req.AccountNumber),
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
	return PayoutDestination{
		BankCode:          account.BankCode,
		BankName:          account.BankName,
		AccountHolderName: holder,
		AccountNumber:     number,
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
	case entity.PayoutStatusInquiry, entity.PayoutStatusFailed:
		// The provider may already hold this transfer, so its state is read
		// before anything is sent. An unreadable state is never re-sent.
		wasFailed := payout.Status == entity.PayoutStatusFailed
		if err := uc.reconcile(ctx, release, payout); err != nil {
			return nil, uc.failTransfer(ctx, release, payout, "transaction status")
		}
		// A failed payout that DOKU still reports as failed is re-sent under
		// the same key, which DOKU deduplicates. Any other answer is final.
		if !wasFailed || payout.Status != entity.PayoutStatusFailed {
			if err := release.Payouts.Update(ctx, payout); err != nil {
				return nil, err
			}
			return payout, nil
		}
	}
	return uc.transfer(ctx, release, payout)
}

// reconcile reads DOKU's answer for the payout and moves the payout to the
// state that answer describes. An error or an unknown code changes nothing:
// the caller must not send a transfer on an unknown state.
func (uc *PayoutUsecase) reconcile(ctx context.Context, release *PayoutRelease, payout *entity.Payout) error {
	status, err := release.Gateway.TransactionStatus(ctx, payout.PartnerReferenceNo)
	if err != nil {
		return err
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
	case doku.TransactionStatusFailed, doku.TransactionStatusRefunded, doku.TransactionStatusCanceled:
		payout.Status = entity.PayoutStatusFailed
		payout.LastError = transferFailureReason(status)
	default:
		return errors.New("unknown DOKU transaction status " + status.LatestStatus)
	}
	return nil
}

// transferFailureReason names why DOKU did not deliver the money. It keeps the
// status code and DOKU's description, never the beneficiary details.
func transferFailureReason(status *doku.TransactionStatusResult) string {
	var label string
	switch status.LatestStatus {
	case doku.TransactionStatusRefunded:
		label = "refunded"
	case doku.TransactionStatusCanceled:
		label = "canceled"
	default:
		label = "failed"
	}
	reason := "doku transfer " + label + " (status " + status.LatestStatus + ")"
	if desc := strings.TrimSpace(status.LatestStatusDesc); desc != "" {
		reason += ": " + desc
	}
	if len(reason) > 255 {
		reason = reason[:255]
	}
	return reason
}

// checkSettlement refuses a release unless the hub's settled balance covers
// the amount. The held DOKU_MERCHANT_PENDING_IDR balance is not withdrawable,
// so it never counts.
func (uc *PayoutUsecase) checkSettlement(ctx context.Context, release *PayoutRelease, amount int64) error {
	balance, err := release.Gateway.BalanceInquiry(ctx, release.ProfileID)
	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_SETTLEMENT_UNAVAILABLE", "cannot read the settled balance, the payout was not released")
	}
	available, ok := balance.SettledAvailable()
	if !ok {
		return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_SETTLEMENT_UNAVAILABLE", "the hub has no settled DOKU_MERCHANT_IDR balance, the payout was not released")
	}
	cents, ok := settledCents(available)
	if !ok {
		return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_SETTLEMENT_UNAVAILABLE", "the settled balance is not a valid amount, the payout was not released")
	}
	if cents < amount*100 {
		return apperror.NewDomainError(apperror.ErrValidation, "PAYOUT_INSUFFICIENT_SETTLED_BALANCE", "the settled balance is below the payout amount")
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

// ApplyTransferStatus moves a payout to the status DOKU reports for its
// partnerReferenceNo. Repeating a status is a no-op, and a paid payout is
// never changed again.
func (uc *PayoutUsecase) ApplyTransferStatus(ctx context.Context, partnerReferenceNo, status string) error {
	release, err := uc.releaseDeps()
	if err != nil {
		return err
	}
	payout, err := release.Payouts.FindByPartnerReferenceNo(ctx, partnerReferenceNo)
	if err != nil {
		return err
	}

	switch status {
	case entity.PayoutStatusSubmitted:
		if payout.Status != entity.PayoutStatusPending && payout.Status != entity.PayoutStatusInquiry {
			return nil
		}
		payout.Status = entity.PayoutStatusSubmitted
	case entity.PayoutStatusPaid:
		if payout.Status == entity.PayoutStatusPaid {
			return nil
		}
		now := time.Now()
		payout.Status = entity.PayoutStatusPaid
		payout.PaidAt = &now
	case entity.PayoutStatusFailed:
		if payout.Status == entity.PayoutStatusPaid || payout.Status == entity.PayoutStatusFailed {
			return nil
		}
		payout.Status = entity.PayoutStatusFailed
	default:
		return apperror.NewDomainError(apperror.ErrValidation, "INVALID_TRANSFER_STATUS", "unknown transfer status")
	}
	return release.Payouts.Update(ctx, payout)
}

func (uc *PayoutUsecase) releaseDeps() (*PayoutRelease, error) {
	if uc.release == nil {
		return nil, errors.New("payout release is not configured")
	}
	return uc.release, nil
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

// transfer runs the inquiry and then the payment, each under the payout's
// stored partnerReferenceNo. A retry always starts again at the inquiry, which
// DOKU answers as a replay with the original referenceNo.
func (uc *PayoutUsecase) transfer(ctx context.Context, release *PayoutRelease, payout *entity.Payout) (*entity.Payout, error) {
	number, err := uc.secret.Decrypt(payout.BeneficiaryAccountNumberEncrypted)
	if err != nil {
		return nil, err
	}
	inquiryReq := doku.TransferInquiryRequest{
		PartnerReferenceNo:       payout.PartnerReferenceNo,
		Amount:                   payout.Amount,
		FromAccount:              release.FromAccount,
		BeneficiaryBankCode:      payout.BeneficiaryBankCode,
		BeneficiaryAccountNumber: number,
		Remark:                   payoutRemark,
	}
	if err := uc.checkSettlement(ctx, release, payout.Amount); err != nil {
		return nil, err
	}

	inquiry, err := release.Gateway.TransferInquiry(ctx, inquiryReq)
	if err != nil {
		return nil, uc.failTransfer(ctx, release, payout, "transfer inquiry")
	}
	payout.DokuInquiryReferenceNo = inquiry.ReferenceNo
	payout.Replayed = inquiry.Replayed
	payout.Status = entity.PayoutStatusInquiry
	if inquiry.Replayed {
		// DOKU already holds this transfer, so the state it reports is final
		// for this release: no payment is sent after a replayed inquiry.
		if err := uc.reconcile(ctx, release, payout); err != nil {
			return nil, uc.failTransfer(ctx, release, payout, "transaction status")
		}
		if err := release.Payouts.Update(ctx, payout); err != nil {
			return nil, err
		}
		return payout, nil
	}
	if err := release.Payouts.Update(ctx, payout); err != nil {
		return nil, err
	}

	paid, err := release.Gateway.TransferPayment(ctx, doku.TransferPaymentRequest{
		TransferInquiryRequest: inquiryReq,
		ReferenceNo:            inquiry.ReferenceNo,
		BeneficiaryAccountName: inquiry.BeneficiaryAccountName,
	})
	if err != nil {
		return nil, uc.failTransfer(ctx, release, payout, "transfer payment")
	}
	payout.DokuReferenceNo = paid.ReferenceNo
	payout.DokuReferenceNumber = paid.ReferenceNumber
	payout.Replayed = payout.Replayed || paid.Replayed
	payout.Status = entity.PayoutStatusSubmitted
	payout.LastError = ""
	if err := release.Payouts.Update(ctx, payout); err != nil {
		return nil, err
	}

	// A 409 on the payment means DOKU already holds this transfer. It is
	// mapped from DOKU's status and never sent again as a new transfer. A
	// failed reconciliation keeps the payout submitted: DOKU has the transfer.
	if paid.Replayed {
		if err := uc.reconcile(ctx, release, payout); err != nil {
			payout.LastError = "transaction status check failed"
			if err := release.Payouts.Update(ctx, payout); err != nil {
				return nil, err
			}
		} else if err := release.Payouts.Update(ctx, payout); err != nil {
			return nil, err
		}
	}
	return payout, nil
}

// failTransfer records a failed provider step without the provider's raw
// response, which can echo the beneficiary account number. The payout keeps
// its state, so the next release retries the same step with the same key.
func (uc *PayoutUsecase) failTransfer(ctx context.Context, release *PayoutRelease, payout *entity.Payout, step string) error {
	payout.LastError = step + " failed"
	if err := release.Payouts.Update(ctx, payout); err != nil {
		return err
	}
	return apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_TRANSFER_FAILED", step+" failed, retry the release")
}
