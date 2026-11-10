package usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
)

// TestPayoutUsecase_SnapStubEndToEnd drives the real SNAP client against a
// local stub: release -> balance -> inquiry -> transfer -> submitted ->
// check-status -> paid. No DOKU credential or network is involved.
func TestPayoutUsecase_SnapStubEndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	var mu sync.Mutex
	var paths []string
	var inquiryBody, transferBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/authorization/v1/access-token/b2b":
			_, _ = w.Write([]byte(`{"responseCode":"2007300","accessToken":"stub-token","expiresIn":900}`))
		case "/snap/v1.1/balance-inquiry":
			_, _ = w.Write([]byte(`{"responseCode":"2000000","accountNo":"1234567890","accountInfos":{"avaliableBalance":{"value":"10000000.00","currency":"IDR"}}}`))
		case "/snap/v1.1/emoney/bank-account-inquiry":
			_ = json.NewDecoder(r.Body).Decode(&inquiryBody)
			_, _ = w.Write([]byte(`{"responseCode":"2000000","referenceNo":"INQ-1","partnerReferenceNo":"PO-42","beneficiaryAccountName":"SITI RAHAYU","sessionId":"SESS-1"}`))
		case "/snap/v1.1/emoney/transfer-bank":
			_ = json.NewDecoder(r.Body).Decode(&transferBody)
			_, _ = w.Write([]byte(`{"responseCode":"2000000","referenceNo":"PAY-1","partnerReferenceNo":"PO-42","referenceNumber":"REF-1"}`))
		case "/snap/v1.1/qr/qr-mpm-status":
			_, _ = w.Write([]byte(`{"responseCode":"2000000","originalPartnerReferenceNo":"PO-42","latestTransactionStatus":"00","transactionStatusDesc":"Success"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	f := newReleaseFixture(t)
	f.uc.release.Gateway = doku.NewSnapPayoutClient(doku.PayoutConfig{
		ClientID:             "BRN-TEST",
		SecretKey:            "snap-secret",
		PrivateKey:           privatePEM,
		BaseURL:              server.URL,
		ChannelID:            "11",
		SenderFirstName:      "Serabutin",
		SenderLastName:       "Operations",
		SenderPersonalID:     "3174000000000001",
		SenderPersonalIDType: "KTP",
		SenderCountryCode:    "ID",
	})

	released, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusSubmitted, released.Status)
	assert.Equal(t, "SESS-1", released.DokuSessionID)
	assert.Equal(t, "INQ-1", released.DokuInquiryReferenceNo)
	assert.Equal(t, "PAY-1", released.DokuReferenceNo)

	paid, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")
	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusPaid, paid.Status)
	assert.NotNil(t, paid.PaidAt)

	assert.Equal(t, "SESS-1", transferBody["sessionId"])
	assert.Equal(t, "014", inquiryBody["additionalInfo"].(map[string]interface{})["beneficiaryBankCode"])
	assert.Equal(t, "6281234567890", inquiryBody["customerNumber"])
	assert.Contains(t, paths, "/snap/v1.1/emoney/bank-account-inquiry")
	assert.Contains(t, paths, "/snap/v1.1/emoney/transfer-bank")
	assert.Contains(t, paths, "/snap/v1.1/qr/qr-mpm-status")
}
