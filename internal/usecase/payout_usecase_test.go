package usecase

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
)

type fakeSecretBox struct{}

func (f *fakeSecretBox) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (f *fakeSecretBox) Decrypt(ciphertext string) (string, error) {
	plaintext, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", errors.New("bad ciphertext")
	}
	return plaintext, nil
}

func (f *fakeSecretBox) Fingerprint(plaintext string) string { return "fp:" + plaintext }

type fakePayoutAccountRepo struct {
	byUser         map[uint]*entity.PayoutAccount
	fingerprintErr error
}

func (f *fakePayoutAccountRepo) Upsert(ctx context.Context, a *entity.PayoutAccount) error {
	if f.byUser == nil {
		f.byUser = map[uint]*entity.PayoutAccount{}
	}
	a.ID = 1
	f.byUser[a.UserID] = a
	return nil
}

func (f *fakePayoutAccountRepo) FindByUserID(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	a, ok := f.byUser[userID]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "payout account not found")
	}
	return a, nil
}

func (f *fakePayoutAccountRepo) FindByFingerprint(ctx context.Context, fingerprint string) (*entity.PayoutAccount, error) {
	if f.fingerprintErr != nil {
		return nil, f.fingerprintErr
	}
	for _, a := range f.byUser {
		if a.AccountFingerprint == fingerprint {
			return a, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "payout account not found")
}

type fakeUserNameLookup struct {
	names map[uint]string
}

func (f *fakeUserNameLookup) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	name, ok := f.names[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}
	return &entity.User{ID: id, Name: name}, nil
}

func newTestPayoutUsecase(repo *fakePayoutAccountRepo) *PayoutUsecase {
	users := &fakeUserNameLookup{names: map[uint]string{71: "Siti Rahayu", 72: "Rudi Hartono"}}
	return NewPayoutUsecase(repo, users, &fakeSecretBox{}, "v1")
}

func TestPayoutUsecase_Upsert_EncryptsAndNeverReturnsPlaintext(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode:          "BCAIDJA",
		BankName:          "Bank Central Asia",
		AccountHolderName: "Siti Rahayu",
		AccountNumber:     "1234567890",
	})
	require.NoError(t, err)

	assert.Equal(t, "enc:1234567890", account.AccountNumberEncrypted)
	assert.Equal(t, "enc:Siti Rahayu", account.AccountHolderNameEncrypted)
	assert.Equal(t, "7890", account.AccountLast4)
	assert.Equal(t, "fp:1234567890", account.AccountFingerprint)
	assert.Equal(t, "v1", account.EncryptionKeyID)
	assert.Equal(t, entity.PayoutAccountStatusUnverified, account.Status)
	assert.False(t, account.NameMismatch)
}

func TestPayoutUsecase_Upsert_RejectsAnAccountAlreadyUsedByAnotherMitra(t *testing.T) {
	repo := &fakePayoutAccountRepo{}
	uc := newTestPayoutUsecase(repo)

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	_, err = uc.Upsert(context.Background(), 72, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Rudi Hartono", AccountNumber: "1234567890",
	})
	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "PAYOUT_ACCOUNT_DUPLICATE", domainErr.Code)
	assert.ErrorIs(t, err, apperror.ErrAlreadyExists)
	assert.NotContains(t, repo.byUser, uint(72), "the second user's row must not be stored")
}

func TestPayoutUsecase_Upsert_AllowsTheSameUserToResaveTheirAccount(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank Central Asia", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bank Central Asia", account.BankName)
}

func TestPayoutUsecase_Upsert_PropagatesFingerprintLookupErrors(t *testing.T) {
	dbErr := errors.New("connection reset")
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{fingerprintErr: dbErr})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	assert.ErrorIs(t, err, dbErr)
}

func TestPayoutUsecase_Upsert_RejectsInvalidBankCodeAndAccountNumber(t *testing.T) {
	cases := []struct {
		name string
		req  dto.PayoutAccountRequest
	}{
		{name: "bank code too short", req: dto.PayoutAccountRequest{BankCode: "BC", BankName: "Bank", AccountHolderName: "Siti", AccountNumber: "1234567890"}},
		{name: "account number not digits", req: dto.PayoutAccountRequest{BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti", AccountNumber: "12-34"}},
		{name: "bank name empty", req: dto.PayoutAccountRequest{BankCode: "BCAIDJA", BankName: "   ", AccountHolderName: "Siti", AccountNumber: "1234567890"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakePayoutAccountRepo{}
			uc := newTestPayoutUsecase(repo)

			_, err := uc.Upsert(context.Background(), 71, tc.req)
			require.Error(t, err)
			var domainErr *apperror.DomainError
			require.ErrorAs(t, err, &domainErr)
			assert.Equal(t, "INVALID_PAYOUT_ACCOUNT", domainErr.Code)
			assert.Empty(t, repo.byUser, "a rejected request must not reach the repository")
		})
	}
}

func TestPayoutRequest_Validate_BankNameLength(t *testing.T) {
	const bankNameMessage = "bank_name must be 2 to 100 characters"
	cases := []struct {
		name     string
		bankName string
		wantErr  string
	}{
		{name: "empty is rejected", bankName: "", wantErr: bankNameMessage},
		{name: "whitespace only is rejected", bankName: "   ", wantErr: bankNameMessage},
		{name: "one character after trimming is rejected", bankName: "  B  ", wantErr: bankNameMessage},
		{name: "two characters is accepted", bankName: "BR"},
		{name: "100 characters is accepted", bankName: strings.Repeat("a", 100)},
		{name: "101 characters is rejected", bankName: strings.Repeat("a", 101), wantErr: bankNameMessage},
		{name: "100 multi-byte characters is accepted", bankName: strings.Repeat("é", 100)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := dto.PayoutAccountRequest{
				BankCode: "BCAIDJA", BankName: tc.bankName, AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
			}
			err := req.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestPayoutRequest_Validate_HolderNameLength(t *testing.T) {
	const holderNameMessage = "account_holder_name must be 2 to 100 characters"
	cases := []struct {
		name       string
		holderName string
		wantErr    string
	}{
		{name: "empty is rejected", holderName: "", wantErr: holderNameMessage},
		{name: "one character after trimming is rejected", holderName: "  S  ", wantErr: holderNameMessage},
		{name: "exactly 2 characters is accepted", holderName: "Si"},
		{name: "exactly 100 characters is accepted", holderName: strings.Repeat("a", 100)},
		{name: "101 characters is rejected", holderName: strings.Repeat("a", 101), wantErr: holderNameMessage},
		{name: "100 multi-byte characters is accepted", holderName: strings.Repeat("é", 100)},
		{name: "100 CJK characters is accepted", holderName: strings.Repeat("李", 100)},
		{name: "101 multi-byte characters is rejected", holderName: strings.Repeat("é", 101), wantErr: holderNameMessage},
		{name: "100 characters with surrounding spaces is accepted after trimming", holderName: "  " + strings.Repeat("a", 100) + "  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := dto.PayoutAccountRequest{
				BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: tc.holderName, AccountNumber: "1234567890",
			}
			err := req.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestPayoutAccountService_ExposesNoDecryption(t *testing.T) {
	var _ PayoutAccountService = (*PayoutUsecase)(nil)

	iface := reflect.TypeOf((*PayoutAccountService)(nil)).Elem()
	var methods []string
	for i := 0; i < iface.NumMethod(); i++ {
		methods = append(methods, iface.Method(i).Name)
	}
	assert.ElementsMatch(t, []string{"Upsert", "Get"}, methods, "the handler-facing interface must not expose decryption")
}

func TestPayoutUsecase_Upsert_ResaveResetsStatusToUnverified(t *testing.T) {
	repo := &fakePayoutAccountRepo{}
	uc := newTestPayoutUsecase(repo)

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	repo.byUser[71].Status = entity.PayoutAccountStatusVerified

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "9876543210",
	})
	require.NoError(t, err)
	assert.Equal(t, entity.PayoutAccountStatusUnverified, account.Status)
	assert.Equal(t, "fp:9876543210", account.AccountFingerprint)
}

func TestPayoutUsecase_Upsert_StoresTheTrimmedHolderName(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "  Bank Central Asia  ", AccountHolderName: "  Siti Rahayu  ", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	assert.Equal(t, "enc:Siti Rahayu", account.AccountHolderNameEncrypted)
	assert.Equal(t, "Bank Central Asia", account.BankName)
	assert.False(t, account.NameMismatch, "the trimmed name must match the registered name")
}

func TestPayoutRequest_Validate_BankCodeLength(t *testing.T) {
	const bankCodeMessage = "bank_code must be 4 to 11 uppercase letters or digits"
	cases := []struct {
		name     string
		bankCode string
		wantErr  string
	}{
		{name: "7-character BIC is accepted", bankCode: "BCAIDJA"},
		{name: "8-character BIC is accepted", bankCode: "CENAIDJA"},
		{name: "8-character BRI BIC is accepted", bankCode: "BRIDIDJA"},
		{name: "lowercase is rejected", bankCode: "bcaidja", wantErr: bankCodeMessage},
		{name: "3 characters is rejected", bankCode: "BCA", wantErr: bankCodeMessage},
		{name: "12 characters is rejected", bankCode: "BCAIDJAXYZAB", wantErr: bankCodeMessage},
		{name: "embedded space is rejected", bankCode: "BCA IDJA", wantErr: bankCodeMessage},
		{name: "hyphen is rejected", bankCode: "BCA-IDJA", wantErr: bankCodeMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := dto.PayoutAccountRequest{
				BankCode: tc.bankCode, BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
			}
			err := req.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestPayoutUsecase_Upsert_ValidationErrorsNeverEchoTheAccountNumber(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "12-34",
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "12-34")
}

func TestPayoutUsecase_Upsert_FlagsNameMismatchWithoutFailing(t *testing.T) {
	repo := &fakePayoutAccountRepo{}
	uc := newTestPayoutUsecase(repo)

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "PT Contoh", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	assert.True(t, account.NameMismatch)
	assert.Equal(t, entity.PayoutAccountStatusUnverified, account.Status)
	assert.Equal(t, account, repo.byUser[71])
}

func TestPayoutUsecase_Upsert_MatchesNameIgnoringCaseSpacesAndPunctuation(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "SITI-RAHAYU.", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	assert.False(t, account.NameMismatch)
}

func TestPayoutUsecase_Upsert_FailsWhenTheUserIsUnknown(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 99, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	assert.Error(t, err)
}

func TestPayoutUsecase_DecryptDestination_ReturnsPlaintextOnlyHere(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	destination, err := uc.DecryptDestination(context.Background(), 71)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", destination.AccountNumber)
	assert.Equal(t, "Siti Rahayu", destination.AccountHolderName)
	assert.Equal(t, "BCAIDJA", destination.BankCode)
}

func TestPayoutUsecase_Get_ReturnsStoredAccountWithoutDecrypting(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	account, err := uc.Get(context.Background(), 71)
	require.NoError(t, err)
	assert.Equal(t, "7890", account.AccountLast4)
	assert.Equal(t, "enc:1234567890", account.AccountNumberEncrypted)
}

type fakePayoutRepo struct {
	rows   map[uint]*entity.Payout
	nextID uint
}

func (f *fakePayoutRepo) find(match func(*entity.Payout) bool) (*entity.Payout, error) {
	for _, row := range f.rows {
		if match(row) {
			copied := *row
			return &copied, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_NOT_FOUND", "payout not found")
}

func (f *fakePayoutRepo) Create(ctx context.Context, p *entity.Payout) error {
	if f.rows == nil {
		f.rows = map[uint]*entity.Payout{}
	}
	for _, row := range f.rows {
		if row.BookingID == p.BookingID || row.PartnerReferenceNo == p.PartnerReferenceNo {
			return apperror.NewDomainError(apperror.ErrAlreadyExists, "PAYOUT_DUPLICATE", "a payout already exists for this booking or reference")
		}
	}
	f.nextID++
	p.ID = f.nextID
	p.UpdatedAt = time.Now()
	copied := *p
	f.rows[p.ID] = &copied
	return nil
}

func (f *fakePayoutRepo) FindByID(ctx context.Context, id uint) (*entity.Payout, error) {
	return f.find(func(p *entity.Payout) bool { return p.ID == id })
}

func (f *fakePayoutRepo) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payout, error) {
	return f.find(func(p *entity.Payout) bool { return p.BookingID == bookingID })
}

func (f *fakePayoutRepo) FindByPartnerReferenceNo(ctx context.Context, partnerReferenceNo string) (*entity.Payout, error) {
	return f.find(func(p *entity.Payout) bool { return p.PartnerReferenceNo == partnerReferenceNo })
}

// FindSubmitted mirrors the gorm query: unreconciled rows first, then the
// least recently reconciled, then the oldest update.
func (f *fakePayoutRepo) FindSubmitted(ctx context.Context, limit int, reconciledBefore time.Time) ([]*entity.Payout, error) {
	var candidates []*entity.Payout
	for _, row := range f.rows {
		if row.Status != entity.PayoutStatusSubmitted {
			continue
		}
		if row.LastReconciledAt != nil && !row.LastReconciledAt.Before(reconciledBefore) {
			continue
		}
		copied := *row
		candidates = append(candidates, &copied)
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if (a.LastReconciledAt == nil) != (b.LastReconciledAt == nil) {
			return a.LastReconciledAt == nil
		}
		if a.LastReconciledAt != nil && !a.LastReconciledAt.Equal(*b.LastReconciledAt) {
			return a.LastReconciledAt.Before(*b.LastReconciledAt)
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.Before(b.UpdatedAt)
		}
		return a.ID < b.ID
	})
	return capPayouts(candidates, limit), nil
}

// FindStalledInquiry mirrors the gorm query: inquiry rows not updated since
// updatedBefore, oldest update first.
func (f *fakePayoutRepo) FindStalledInquiry(ctx context.Context, limit int, updatedBefore time.Time) ([]*entity.Payout, error) {
	var candidates []*entity.Payout
	for _, row := range f.rows {
		if row.Status != entity.PayoutStatusInquiry || !row.UpdatedAt.Before(updatedBefore) {
			continue
		}
		copied := *row
		candidates = append(candidates, &copied)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].UpdatedAt.Equal(candidates[j].UpdatedAt) {
			return candidates[i].UpdatedAt.Before(candidates[j].UpdatedAt)
		}
		return candidates[i].ID < candidates[j].ID
	})
	return capPayouts(candidates, limit), nil
}

func capPayouts(rows []*entity.Payout, limit int) []*entity.Payout {
	if limit <= 0 {
		return []*entity.Payout{}
	}
	if len(rows) > limit {
		return rows[:limit]
	}
	return rows
}

func (f *fakePayoutRepo) Update(ctx context.Context, p *entity.Payout) error {
	if _, ok := f.rows[p.ID]; !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_NOT_FOUND", "payout not found")
	}
	p.UpdatedAt = time.Now()
	copied := *p
	f.rows[p.ID] = &copied
	return nil
}

type fakeBookingLookup struct {
	rows map[uint]*entity.Booking
}

func (f *fakeBookingLookup) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	b, ok := f.rows[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	copied := *b
	return &copied, nil
}

type fakePaymentLookup struct {
	byBooking map[uint]*entity.Payment
}

func (f *fakePaymentLookup) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payment, error) {
	p, ok := f.byBooking[bookingID]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYMENT_NOT_FOUND", "payment not found")
	}
	copied := *p
	return &copied, nil
}

type fakeTransferGateway struct {
	inquiries    []doku.TransferInquiryRequest
	payments     []doku.TransferPaymentRequest
	statuses     []string
	balanceCalls int
	// The X-EXTERNAL-ID each request kind was sent with, in call order.
	inquiryIDs []string
	paymentIDs []string
	statusIDs  []string
	balanceIDs []string

	inquiryResult *doku.TransferInquiryResult
	inquiryErr    error
	paymentResult *doku.TransferPaymentResult
	paymentErr    error
	statusResult  *doku.TransactionStatusResult
	statusErr     error
	// statusByRef and statusErrByRef answer one reference, ahead of the
	// shared statusResult and statusErr.
	statusByRef    map[string]*doku.TransactionStatusResult
	statusErrByRef map[string]error
	balance        *doku.BalanceResult
	balanceErr     error
}

func (f *fakeTransferGateway) TransferInquiry(ctx context.Context, externalID string, req doku.TransferInquiryRequest) (*doku.TransferInquiryResult, error) {
	f.inquiries = append(f.inquiries, req)
	f.inquiryIDs = append(f.inquiryIDs, externalID)
	if f.inquiryErr != nil {
		return nil, f.inquiryErr
	}
	if f.inquiryResult != nil {
		return f.inquiryResult, nil
	}
	return &doku.TransferInquiryResult{ReferenceNo: "INQ-1", PartnerReferenceNo: req.PartnerReferenceNo, BeneficiaryAccountName: "SITI RAHAYU"}, nil
}

func (f *fakeTransferGateway) TransferPayment(ctx context.Context, externalID string, req doku.TransferPaymentRequest) (*doku.TransferPaymentResult, error) {
	f.payments = append(f.payments, req)
	f.paymentIDs = append(f.paymentIDs, externalID)
	if f.paymentErr != nil {
		return nil, f.paymentErr
	}
	if f.paymentResult != nil {
		return f.paymentResult, nil
	}
	return &doku.TransferPaymentResult{ReferenceNo: "PAY-1", PartnerReferenceNo: req.PartnerReferenceNo}, nil
}

func (f *fakeTransferGateway) TransactionStatus(ctx context.Context, externalID string, partnerReferenceNo string) (*doku.TransactionStatusResult, error) {
	f.statusIDs = append(f.statusIDs, externalID)
	f.statuses = append(f.statuses, partnerReferenceNo)
	if err, ok := f.statusErrByRef[partnerReferenceNo]; ok {
		return nil, err
	}
	if result, ok := f.statusByRef[partnerReferenceNo]; ok {
		return result, nil
	}
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	if f.statusResult != nil {
		return f.statusResult, nil
	}
	return &doku.TransactionStatusResult{PartnerReferenceNo: partnerReferenceNo, LatestStatus: doku.TransactionStatusPending}, nil
}

func (f *fakeTransferGateway) BalanceInquiry(ctx context.Context, externalID string, profileID string) (*doku.BalanceResult, error) {
	f.balanceIDs = append(f.balanceIDs, externalID)
	f.balanceCalls++
	if f.balanceErr != nil {
		return nil, f.balanceErr
	}
	if f.balance != nil {
		return f.balance, nil
	}
	return &doku.BalanceResult{ProfileID: profileID, Accounts: []doku.BalanceAccount{
		{Type: doku.BalanceAccountMerchantIDR, Currency: "IDR", Available: "1000000000.00"},
		{Type: doku.BalanceAccountMerchantPendingIDR, Currency: "IDR", Available: "0.00"},
	}}, nil
}

type releaseFixture struct {
	uc       *PayoutUsecase
	payouts  *fakePayoutRepo
	accounts *fakePayoutAccountRepo
	bookings *fakeBookingLookup
	payments *fakePaymentLookup
	gateway  *fakeTransferGateway
}

// newReleaseFixture seeds booking 42: a completed booking of provider 7 with a
// paid payment, service amount 150000 and platform fee 15000, and a verified
// bank account for provider 7.
func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	f := &releaseFixture{
		payouts: &fakePayoutRepo{},
		bookings: &fakeBookingLookup{rows: map[uint]*entity.Booking{
			42: {ID: 42, ProviderID: 7, Status: bookingStatusCompleted, SubTotal: 150000, PlatformFee: 15000, Total: 165000},
		}},
		payments: &fakePaymentLookup{byBooking: map[uint]*entity.Payment{
			42: {BookingID: 42, Status: entity.PaymentStatusPaid, Amount: 165000, Currency: payoutCurrency},
		}},
		accounts: &fakePayoutAccountRepo{byUser: map[uint]*entity.PayoutAccount{
			7: {
				UserID:                     7,
				BankCode:                   "BCAIDJA",
				BankName:                   "Bank Central Asia",
				AccountHolderNameEncrypted: "enc:Siti Rahayu",
				AccountNumberEncrypted:     "enc:1234567890",
				AccountLast4:               "7890",
				AccountFingerprint:         "fp:1234567890",
				Status:                     entity.PayoutAccountStatusVerified,
			},
		}},
		gateway: &fakeTransferGateway{},
	}
	users := &fakeUserNameLookup{names: map[uint]string{7: "Siti Rahayu"}}
	f.uc = NewPayoutUsecase(f.accounts, users, &fakeSecretBox{}, "v1").WithRelease(PayoutRelease{
		Payouts:     f.payouts,
		Bookings:    f.bookings,
		Payments:    f.payments,
		Gateway:     f.gateway,
		FromAccount: "SRB-POOL",
		ProfileID:   "HUB-1",
	})
	return f
}

func requireDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, code, domainErr.Code)
}

func TestPayoutUsecase_Release_TwiceCreatesOnePayoutAndPaysOnce(t *testing.T) {
	f := newReleaseFixture(t)

	first, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	second, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Len(t, f.payouts.rows, 1)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, entity.PayoutStatusSubmitted, second.Status)
	assert.Len(t, f.gateway.inquiries, 1)
	assert.Len(t, f.gateway.payments, 1)
	assert.Equal(t, "PO-42", first.PartnerReferenceNo)
	assert.Equal(t, "PO-42", f.gateway.inquiries[0].PartnerReferenceNo)
	assert.Equal(t, "PO-42", f.gateway.payments[0].PartnerReferenceNo)
	assert.Equal(t, "INQ-1", f.gateway.payments[0].ReferenceNo)
}

func TestPayoutUsecase_Release_PaymentConflictMarksSubmittedAndNeverResends(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.paymentResult = &doku.TransferPaymentResult{ReferenceNo: "PAY-ORIG", PartnerReferenceNo: "PO-42", Replayed: true}

	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusSubmitted, payout.Status)
	assert.Equal(t, "PAY-ORIG", payout.DokuReferenceNo)
	assert.True(t, payout.Replayed)
	assert.Equal(t, []string{"PO-42"}, f.gateway.statuses)

	_, err = f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	assert.Len(t, f.gateway.inquiries, 1)
	assert.Len(t, f.gateway.payments, 1)
}

func TestPayoutUsecase_Release_PaymentErrorIsReconciledNotResent(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.paymentErr = errors.New("connection reset")

	_, err := f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_TRANSFER_FAILED")
	stored, err := f.payouts.FindByBookingID(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusInquiry, stored.Status)
	assert.Equal(t, "transfer payment failed", stored.LastError)

	// The payment outcome is unknown. DOKU reports the transfer as pending,
	// so the retry reconciles and does not send a second payment.
	f.gateway.paymentErr = nil
	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusSubmitted, payout.Status)
	assert.Empty(t, payout.LastError)
	assert.Equal(t, []string{"PO-42"}, f.gateway.statuses)
	assert.Len(t, f.gateway.payments, 1)
	assert.Len(t, f.payouts.rows, 1)
}

func TestPayoutUsecase_Release_UnreadableStatusBlocksAnyResend(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.paymentErr = errors.New("connection reset")
	_, err := f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_TRANSFER_FAILED")

	f.gateway.paymentErr = nil
	f.gateway.statusErr = errors.New("status endpoint unavailable")
	inquiriesBefore := len(f.gateway.inquiries)
	_, err = f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_TRANSFER_FAILED")

	assert.Len(t, f.gateway.inquiries, inquiriesBefore)
	assert.Len(t, f.gateway.payments, 1)
	stored, err := f.payouts.FindByBookingID(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusInquiry, stored.Status)
	assert.Equal(t, "transaction status failed", stored.LastError)
}

func TestPayoutUsecase_Release_SubmittedPayoutIsReturnedUnchangedWithoutAnyProviderCall(t *testing.T) {
	f := newReleaseFixture(t)
	first, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	f.gateway.inquiries = nil
	f.gateway.payments = nil

	again, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, entity.PayoutStatusSubmitted, again.Status)
	assert.Empty(t, f.gateway.inquiries)
	assert.Empty(t, f.gateway.payments)
	assert.Empty(t, f.gateway.statuses)
}

func TestPayoutUsecase_Release_PaidPayoutIsReturnedUnchangedWithoutAnyProviderCall(t *testing.T) {
	f := newReleaseFixture(t)
	first, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}
	_, err = f.uc.SyncTransferStatus(context.Background(), "PO-42")
	require.NoError(t, err)
	f.gateway.inquiries = nil

	again, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, entity.PayoutStatusPaid, again.Status)
	assert.Empty(t, f.gateway.inquiries)
}

func TestPayoutUsecase_Release_RefusesAChangedDestinationForTheSameBooking(t *testing.T) {
	f := newReleaseFixture(t)
	_, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	inquiriesBefore := len(f.gateway.inquiries)

	f.accounts.byUser[7].AccountFingerprint = "fp:0987654321"
	f.accounts.byUser[7].AccountLast4 = "4321"

	_, err = f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_DESTINATION_CHANGED")
	assert.Len(t, f.gateway.inquiries, inquiriesBefore)
	assert.Len(t, f.gateway.payments, 1)
}

func TestPayoutUsecase_Release_RefusesAChangedHolderNameForTheSameBooking(t *testing.T) {
	f := newReleaseFixture(t)
	_, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	f.accounts.byUser[7].AccountHolderNameEncrypted = "enc:Rudi Hartono"

	_, err = f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_DESTINATION_CHANGED")
}

func TestPayoutUsecase_SyncTransferStatus_PaidPayoutIsNeverChangedByALaterAnswer(t *testing.T) {
	f, payout := syncFixture(t)
	ctx := context.Background()
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}
	_, err := f.uc.SyncTransferStatus(ctx, "PO-42")
	require.NoError(t, err)
	paidAt := f.payouts.rows[payout.ID].PaidAt
	require.NotNil(t, paidAt)
	before := *f.payouts.rows[payout.ID]

	for _, code := range []string{doku.TransactionStatusPending, doku.TransactionStatusFailed, doku.TransactionStatusCanceled} {
		f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: code}
		_, err = f.uc.SyncTransferStatus(ctx, "PO-42")
		require.NoError(t, err)
	}

	after := f.payouts.rows[payout.ID]
	assert.Equal(t, entity.PayoutStatusPaid, after.Status)
	assert.Equal(t, before, *after)
	assert.Equal(t, paidAt, after.PaidAt)
}

func TestPayoutUsecase_SyncTransferStatus_PendingThenSuccessMarksPaid(t *testing.T) {
	f, _ := syncFixture(t)
	ctx := context.Background()

	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusPending}
	_, err := f.uc.SyncTransferStatus(ctx, "PO-42")
	require.NoError(t, err)
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}
	_, err = f.uc.SyncTransferStatus(ctx, "PO-42")
	require.NoError(t, err)

	stored, err := f.payouts.FindByBookingID(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusPaid, stored.Status)
	assert.NotNil(t, stored.PaidAt)
}

func TestPayoutUsecase_SyncTransferStatus_UnknownCodeIsRejectedAndChangesNothing(t *testing.T) {
	f, payout := syncFixture(t)
	before := *f.payouts.rows[payout.ID]
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: "bogus"}

	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.Error(t, err)
	requireStateUnchangedButAttempted(t, before, *f.payouts.rows[payout.ID])
}

func TestPayoutUsecase_Release_PaysTheServiceAmountNotThePlatformFee(t *testing.T) {
	f := newReleaseFixture(t)

	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Equal(t, int64(150000), payout.Amount)
	require.Len(t, f.gateway.inquiries, 1)
	assert.Equal(t, int64(150000), f.gateway.inquiries[0].Amount)
	assert.Equal(t, int64(150000), f.gateway.payments[0].Amount)
}

func TestPayoutUsecase_Release_StoresTheAccountNumberOnlyEncrypted(t *testing.T) {
	f := newReleaseFixture(t)

	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	require.Len(t, f.gateway.inquiries, 1)
	assert.Equal(t, "1234567890", f.gateway.inquiries[0].BeneficiaryAccountNumber)
	assert.Equal(t, f.accounts.byUser[7].AccountNumberEncrypted, payout.BeneficiaryAccountNumberEncrypted)
	assert.NotEqual(t, "1234567890", payout.BeneficiaryAccountNumberEncrypted)
	assert.Equal(t, "7890", payout.BeneficiaryLast4)
}

func TestPayoutUsecase_Release_RefusesAnUnpaidOrUnfinishedBooking(t *testing.T) {
	cases := []struct {
		name    string
		booking string
		payment string
		code    string
	}{
		{name: "pending booking", booking: entity.BookingStatusPending, payment: entity.PaymentStatusPaid, code: "PAYOUT_BOOKING_NOT_RELEASABLE"},
		{name: "unpaid payment", booking: bookingStatusCompleted, payment: entity.PaymentStatusPending, code: "PAYOUT_BOOKING_NOT_PAID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			f.bookings.rows[42].Status = tc.booking
			f.payments.byBooking[42].Status = tc.payment

			_, err := f.uc.Release(context.Background(), 42)
			requireDomainCode(t, err, tc.code)
			assert.Empty(t, f.gateway.inquiries)
			assert.Empty(t, f.payouts.rows)
		})
	}
}

func TestPayoutUsecase_Release_PaymentConflictMapsTheDOKUStatus(t *testing.T) {
	cases := []struct {
		code       string
		want       string
		reasonWith string
	}{
		{code: doku.TransactionStatusSuccess, want: entity.PayoutStatusPaid},
		{code: doku.TransactionStatusPending, want: entity.PayoutStatusSubmitted},
		{code: doku.TransactionStatusRefunded, want: entity.PayoutStatusFailed, reasonWith: "refunded"},
		{code: doku.TransactionStatusCanceled, want: entity.PayoutStatusFailed, reasonWith: "canceled"},
		{code: doku.TransactionStatusFailed, want: entity.PayoutStatusFailed, reasonWith: "failed"},
	}
	for _, tc := range cases {
		t.Run("status "+tc.code, func(t *testing.T) {
			f := newReleaseFixture(t)
			f.gateway.paymentResult = &doku.TransferPaymentResult{ReferenceNo: "PAY-ORIG", ReferenceNumber: "REF-7", PartnerReferenceNo: "PO-42", Replayed: true}
			f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: tc.code, LatestStatusDesc: "desc " + tc.code}

			payout, err := f.uc.Release(context.Background(), 42)
			require.NoError(t, err)

			assert.Equal(t, tc.want, payout.Status)
			assert.Equal(t, "PAY-ORIG", payout.DokuReferenceNo)
			assert.Equal(t, "REF-7", payout.DokuReferenceNumber)
			assert.Equal(t, []string{"PO-42"}, f.gateway.statuses)
			assert.Len(t, f.gateway.payments, 1)
			switch tc.want {
			case entity.PayoutStatusPaid:
				assert.NotNil(t, payout.PaidAt)
				assert.Empty(t, payout.LastError)
			case entity.PayoutStatusFailed:
				assert.Nil(t, payout.PaidAt)
				assert.Contains(t, payout.LastError, tc.reasonWith)
				assert.Contains(t, payout.LastError, "status "+tc.code)
			default:
				assert.Nil(t, payout.PaidAt)
				assert.Empty(t, payout.LastError)
			}
			if tc.want != entity.PayoutStatusFailed {
				_, err := f.uc.Release(context.Background(), 42)
				require.NoError(t, err)
				assert.Len(t, f.gateway.payments, 1)
				assert.Len(t, f.gateway.inquiries, 1)
			}
		})
	}
}

func TestPayoutUsecase_Release_ConflictWithAnUnknownStatusStaysSubmitted(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.paymentResult = &doku.TransferPaymentResult{ReferenceNo: "PAY-ORIG", PartnerReferenceNo: "PO-42", Replayed: true}
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: "99"}

	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Equal(t, entity.PayoutStatusSubmitted, payout.Status)
	assert.Equal(t, "transaction status check failed", payout.LastError)
	assert.Len(t, f.gateway.payments, 1)
}

func TestPayoutUsecase_Release_FailedPayoutStillFailedIsResentUnderTheSameKey(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.paymentResult = &doku.TransferPaymentResult{ReferenceNo: "PAY-ORIG", PartnerReferenceNo: "PO-42", Replayed: true}
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusFailed, LatestStatusDesc: "rejected"}
	_, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	f.gateway.paymentResult = nil
	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Equal(t, entity.PayoutStatusSubmitted, payout.Status)
	assert.Empty(t, payout.LastError)
	require.Len(t, f.gateway.inquiries, 2)
	require.Len(t, f.gateway.payments, 2)
	assert.Equal(t, "PO-42", f.gateway.inquiries[1].PartnerReferenceNo)
	assert.Equal(t, "PO-42", f.gateway.payments[1].PartnerReferenceNo)
	assert.Len(t, f.payouts.rows, 1)
}

func TestPayoutUsecase_Release_FailedPayoutReconciledToPendingIsNotResent(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.paymentResult = &doku.TransferPaymentResult{ReferenceNo: "PAY-ORIG", PartnerReferenceNo: "PO-42", Replayed: true}
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusFailed}
	_, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	f.gateway.paymentResult = nil
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusPending}
	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Equal(t, entity.PayoutStatusSubmitted, payout.Status)
	assert.Len(t, f.gateway.inquiries, 1)
	assert.Len(t, f.gateway.payments, 1)
}

func TestPayoutUsecase_Release_FailedPayoutWithAnUnreadableStatusIsNotResent(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.paymentResult = &doku.TransferPaymentResult{ReferenceNo: "PAY-ORIG", PartnerReferenceNo: "PO-42", Replayed: true}
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusFailed}
	_, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	f.gateway.paymentResult = nil
	f.gateway.statusResult = nil
	f.gateway.statusErr = errors.New("status endpoint unavailable")
	_, err = f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_TRANSFER_FAILED")

	assert.Len(t, f.gateway.inquiries, 1)
	assert.Len(t, f.gateway.payments, 1)
}

func TestPayoutUsecase_Release_ReplayedInquiryWithAFailedStatusSendsNoPayment(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.inquiryResult = &doku.TransferInquiryResult{ReferenceNo: "INQ-1", PartnerReferenceNo: "PO-42", BeneficiaryAccountName: "SITI RAHAYU", Replayed: true}
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusFailed, LatestStatusDesc: "rejected"}

	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Equal(t, entity.PayoutStatusFailed, payout.Status)
	assert.Contains(t, payout.LastError, "status 06")
	assert.Empty(t, f.gateway.payments)
}

func TestPayoutUsecase_Release_ReplayedInquiryWithAPendingStatusSendsNoPayment(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.inquiryResult = &doku.TransferInquiryResult{ReferenceNo: "INQ-1", PartnerReferenceNo: "PO-42", BeneficiaryAccountName: "SITI RAHAYU", Replayed: true}

	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)

	assert.Equal(t, entity.PayoutStatusSubmitted, payout.Status)
	assert.Empty(t, f.gateway.payments)
}

func TestPayoutUsecase_Release_RefusesWhenTheSettledBalanceIsBelowTheAmount(t *testing.T) {
	cases := []struct {
		name      string
		available string
		paid      bool
	}{
		{name: "one cent short", available: "149999.99", paid: false},
		{name: "exactly the amount", available: "150000.00", paid: true},
		{name: "without decimals", available: "150000", paid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			f.gateway.balance = settledBalance(tc.available, "0.00")

			_, err := f.uc.Release(context.Background(), 42)
			if tc.paid {
				require.NoError(t, err)
				assert.Len(t, f.gateway.payments, 1)
				return
			}
			requireDomainCode(t, err, "PAYOUT_INSUFFICIENT_SETTLED_BALANCE")
			assert.Empty(t, f.gateway.inquiries)
			assert.Empty(t, f.gateway.payments)
		})
	}
}

func TestPayoutUsecase_Release_HeldBalanceNeverCountsAsSettled(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.balance = settledBalance("0.00", "999999999.00")

	_, err := f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_INSUFFICIENT_SETTLED_BALANCE")
	assert.Empty(t, f.gateway.inquiries)
}

func TestPayoutUsecase_Release_RefusesWhenTheSettledAccountIsMissing(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.balance = &doku.BalanceResult{ProfileID: "HUB-1", Accounts: []doku.BalanceAccount{
		{Type: doku.BalanceAccountMerchantPendingIDR, Currency: "IDR", Available: "999999999.00"},
	}}

	_, err := f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_SETTLEMENT_UNAVAILABLE")
	assert.Empty(t, f.gateway.inquiries)
	assert.Empty(t, f.gateway.payments)
}

func TestPayoutUsecase_Release_RefusesWhenTheBalanceCannotBeRead(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.balanceErr = errors.New("balance endpoint unavailable")

	_, err := f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_SETTLEMENT_UNAVAILABLE")
	assert.Empty(t, f.gateway.inquiries)
}

func TestPayoutUsecase_Release_RefusesAMalformedSettledBalance(t *testing.T) {
	f := newReleaseFixture(t)
	f.gateway.balance = settledBalance("1,000,000.00", "0.00")

	_, err := f.uc.Release(context.Background(), 42)
	requireDomainCode(t, err, "PAYOUT_SETTLEMENT_UNAVAILABLE")
	assert.Empty(t, f.gateway.inquiries)
}

func TestSettledCents(t *testing.T) {
	cases := map[string]struct {
		cents int64
		ok    bool
	}{
		"150000":    {15000000, true},
		"150000.5":  {15000050, true},
		"149999.99": {14999999, true},
		"0.00":      {0, true},
		"":          {0, false},
		"-1":        {0, false},
		"1,000":     {0, false},
		"1.2.3":     {0, false},
		"1.000":     {0, false},
		".50":       {0, false},
		"12abc":     {0, false},
	}
	for value, want := range cases {
		cents, ok := settledCents(value)
		assert.Equal(t, want.ok, ok, value)
		assert.Equal(t, want.cents, cents, value)
	}
}

func settledBalance(settled, pending string) *doku.BalanceResult {
	return &doku.BalanceResult{ProfileID: "HUB-1", Accounts: []doku.BalanceAccount{
		{Type: doku.BalanceAccountMerchantIDR, Currency: "IDR", Available: settled},
		{Type: doku.BalanceAccountMerchantPendingIDR, Currency: "IDR", Available: pending},
	}}
}
