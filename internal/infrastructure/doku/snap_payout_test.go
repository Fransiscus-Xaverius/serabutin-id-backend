package doku

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	snapTestClientID = "BRN-0001-TEST"
	snapTestSecret   = "snap-secret"
)

var snapTestNow = time.Date(2026, 11, 8, 14, 0, 0, 0, time.FixedZone("WIB", 7*60*60))

func snapTestPrivateKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pemText := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return key, string(pemText)
}

func newSnapTestClient(t *testing.T, serverURL, privateKey string) *SnapPayoutClient {
	t.Helper()
	client := NewSnapPayoutClient(PayoutConfig{
		ClientID:               snapTestClientID,
		SecretKey:              snapTestSecret,
		PrivateKey:             privateKey,
		BaseURL:                serverURL,
		ChannelID:              "11",
		SourceAccount:          "1234567890",
		SenderFirstName:        "Serabutin",
		SenderLastName:         "Operations",
		SenderPersonalID:       "3174000000000001",
		SenderPersonalIDType:   "KTP",
		SenderCountryCode:      "ID",
		CustomerNumberFallback: "628111111111",
	})
	client.now = func() time.Time { return snapTestNow }
	return client
}

func TestSnapPayoutClient_FullFlowSignsAndSendsSnapPayloads(t *testing.T) {
	key, pemText := snapTestPrivateKey(t)
	var tokenCalls, inquiryCalls, transferCalls, statusCalls, balanceCalls int32
	var tokenBody map[string]interface{}
	var inquiryBody, transferBody, statusBody, balanceBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(raw)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case snapTokenPath:
			atomic.AddInt32(&tokenCalls, 1)
			require.Equal(t, snapTestClientID, r.Header.Get("X-CLIENT-KEY"))
			require.NotEmpty(t, r.Header.Get("X-SIGNATURE"))
			require.NoError(t, json.Unmarshal(raw, &tokenBody))
			timestamp := r.Header.Get("X-TIMESTAMP")
			sum := sha256.Sum256([]byte(snapTestClientID + "|" + timestamp))
			signature, err := base64.StdEncoding.DecodeString(r.Header.Get("X-SIGNATURE"))
			require.NoError(t, err)
			require.NoError(t, rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], signature))
			_, _ = w.Write([]byte(`{"responseCode":"2007300","accessToken":"token-1","expiresIn":900}`))
		case snapInquiryPath:
			atomic.AddInt32(&inquiryCalls, 1)
			assertSnapSignedRequest(t, r, snapInquiryPath, raw, "token-1")
			require.NoError(t, json.Unmarshal(raw, &inquiryBody))
			_, _ = w.Write([]byte(`{"responseCode":"2000000","referenceNo":"INQ-1","partnerReferenceNo":"PO-42","beneficiaryAccountName":"SITI RAHAYU","sessionId":"SESS-1"}`))
		case snapTransferPath:
			atomic.AddInt32(&transferCalls, 1)
			assertSnapSignedRequest(t, r, snapTransferPath, raw, "token-1")
			require.NoError(t, json.Unmarshal(raw, &transferBody))
			_, _ = w.Write([]byte(`{"responseCode":"2000000","referenceNo":"PAY-1","partnerReferenceNo":"PO-42","referenceNumber":"REF-1"}`))
		case snapStatusPath:
			atomic.AddInt32(&statusCalls, 1)
			assertSnapSignedRequest(t, r, snapStatusPath, raw, "token-1")
			require.NoError(t, json.Unmarshal(raw, &statusBody))
			_, _ = w.Write([]byte(`{"responseCode":"2000000","originalPartnerReferenceNo":"PO-42","originalReferenceNo":"PAY-1","sessionId":"SESS-1","latestTransactionStatus":"00","transactionStatusDesc":"Success"}`))
		case snapBalancePath:
			atomic.AddInt32(&balanceCalls, 1)
			assertSnapSignedRequest(t, r, snapBalancePath, raw, "token-1")
			require.NoError(t, json.Unmarshal(raw, &balanceBody))
			_, _ = w.Write([]byte(`{"responseCode":"2000000","accountNo":"1234567890","name":"Serabutin","accountInfos":{"avaliableBalance":{"value":"500000.00","currency":"IDR"}}}`))
		default:
			t.Errorf("unexpected SNAP path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newSnapTestClient(t, server.URL, pemText)
	ctx := t.Context()

	inquiry, err := client.TransferInquiry(ctx, "420000001", TransferInquiryRequest{
		PartnerReferenceNo:       "PO-42",
		Amount:                   150000,
		BeneficiaryBankCode:      "014",
		BeneficiaryAccountNumber: "1234567890",
		CustomerNumber:           "6281234567890",
		BeneficiaryName:          "Siti Rahayu",
		BeneficiaryPhone:         "6281234567890",
		BeneficiaryBankName:      "Bank Central Asia",
		Remark:                   "Pencairan pesanan Serabutin #42",
	})
	require.NoError(t, err)
	assert.Equal(t, "SESS-1", inquiry.SessionID)
	assert.Equal(t, "INQ-1", inquiry.ReferenceNo)
	assert.Equal(t, "SITI RAHAYU", inquiry.BeneficiaryAccountName)

	payment, err := client.TransferPayment(ctx, "420000002", TransferPaymentRequest{
		TransferInquiryRequest: TransferInquiryRequest{
			PartnerReferenceNo:       "PO-42",
			Amount:                   150000,
			BeneficiaryBankCode:      "014",
			BeneficiaryAccountNumber: "1234567890",
			BeneficiaryName:          "Siti Rahayu",
			BeneficiaryPhone:         "6281234567890",
			BeneficiaryBankName:      "Bank Central Asia",
			Remark:                   "Pencairan pesanan Serabutin #42",
		},
		SessionID:              inquiry.SessionID,
		BeneficiaryAccountName: inquiry.BeneficiaryAccountName,
	})
	require.NoError(t, err)
	assert.Equal(t, "PAY-1", payment.ReferenceNo)
	assert.Equal(t, "REF-1", payment.ReferenceNumber)

	status, err := client.TransactionStatus(ctx, "420000003", TransactionStatusRequest{
		PartnerReferenceNo:       "PO-42",
		BeneficiaryAccountNumber: "1234567890",
		BeneficiaryBankCode:      "014",
	})
	require.NoError(t, err)
	assert.Equal(t, TransactionStatusSuccess, status.LatestStatus)
	assert.Equal(t, "PAY-1", status.OriginalReferenceNo)
	assert.Equal(t, "SESS-1", status.SessionID)

	balance, err := client.BalanceInquiry(ctx, "420000004", "1234567890")
	require.NoError(t, err)
	available, ok := balance.SettledAvailable()
	require.True(t, ok)
	assert.Equal(t, "500000.00", available)

	assert.Equal(t, int32(1), atomic.LoadInt32(&tokenCalls), "the access token is cached")
	assert.Equal(t, int32(1), atomic.LoadInt32(&inquiryCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&transferCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&statusCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&balanceCalls))

	assert.Equal(t, "client_credentials", tokenBody["grantType"])
	assert.Equal(t, "PO-42", inquiryBody["partnerReferenceNo"])
	assert.Equal(t, "6281234567890", inquiryBody["customerNumber"])
	assert.Equal(t, "014", inquiryBody["additionalInfo"].(map[string]interface{})["beneficiaryBankCode"])
	assert.Equal(t, "11", inquiryBody["additionalInfo"].(map[string]interface{})["channelCode"])
	assert.Equal(t, "150000.00", inquiryBody["amount"].(map[string]interface{})["value"])

	assert.Equal(t, "SESS-1", transferBody["sessionId"])
	assert.Equal(t, "014", transferBody["beneficiaryBankCode"])
	info := transferBody["additionalInfo"].(map[string]interface{})
	assert.Equal(t, "SITI", info["beneficiaryFirstName"])
	assert.Equal(t, "RAHAYU", info["beneficiaryLastName"])
	assert.Equal(t, "6281234567890", info["beneficiaryPhoneNumber"])
	assert.Equal(t, "Serabutin", info["senderFirstName"])
	assert.Equal(t, "Operations", info["senderLastName"])
	assert.Equal(t, "3174000000000001", info["senderPersonalId"])
	assert.Equal(t, "KTP", info["senderPersonalIdType"])

	assert.Equal(t, "PO-42", statusBody["originalPartnerReferenceNo"])
	assert.Equal(t, "43", statusBody["serviceCode"])
	assert.Equal(t, "1234567890", statusBody["beneficiaryAccountNumber"])
	assert.Equal(t, "014", statusBody["beneficiaryBankCode"])
	assert.Equal(t, "1234567890", balanceBody["accountNo"])
}

func assertSnapSignedRequest(t *testing.T, r *http.Request, path string, body []byte, token string) {
	t.Helper()
	assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
	assert.Equal(t, snapTestClientID, r.Header.Get("X-PARTNER-ID"))
	assert.Equal(t, "H2H", r.Header.Get("CHANNEL-ID"))
	assert.NotEmpty(t, r.Header.Get("X-EXTERNAL-ID"))
	timestamp := r.Header.Get("X-TIMESTAMP")
	_, err := time.Parse("2006-01-02T15:04:05-07:00", timestamp)
	require.NoError(t, err)
	expected := SignSHA512(snapTestSecret, http.MethodPost, path, token, string(body), timestamp)
	assert.Equal(t, expected, r.Header.Get("X-SIGNATURE"))
}

func TestSignSHA512_KnownVector(t *testing.T) {
	body := `{"partnerReferenceNo":"PO-1","amount":{"value":"150000.00","currency":"IDR"}}`
	signature := SignSHA512("secret", http.MethodPost, "/snap/v1.1/emoney/transfer-bank", "token", body, "2026-11-08T14:00:00+07:00")
	assert.Equal(t, "YvsXP5rd+IVlyhW9iaSIAuBwBXppCza4E0+an4sBvpGwoh2nhAuyID4pirfgMj/0yLgq7EZS+tjak0a50hK8ug==", signature)
}

func TestSnapPayoutClient_RequiresSenderIdentity(t *testing.T) {
	_, pemText := snapTestPrivateKey(t)
	client := newSnapTestClient(t, "https://api-sandbox.doku.com", pemText)
	client.cfg.SenderPersonalID = ""

	_, err := client.TransferPayment(t.Context(), "420000002", TransferPaymentRequest{
		TransferInquiryRequest: TransferInquiryRequest{
			PartnerReferenceNo:       "PO-42",
			Amount:                   150000,
			BeneficiaryBankCode:      "014",
			BeneficiaryAccountNumber: "1234567890",
			BeneficiaryPhone:         "6281234567890",
		},
		SessionID:              "SESS-1",
		BeneficiaryAccountName: "Siti Rahayu",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_PAYOUT_SENDER_PERSONAL_ID")
}

func TestSnapPayoutClient_TransferRequiresSessionID(t *testing.T) {
	_, pemText := snapTestPrivateKey(t)
	client := newSnapTestClient(t, "https://api-sandbox.doku.com", pemText)

	_, err := client.TransferPayment(t.Context(), "420000002", TransferPaymentRequest{
		TransferInquiryRequest: TransferInquiryRequest{
			PartnerReferenceNo:       "PO-42",
			Amount:                   150000,
			BeneficiaryBankCode:      "014",
			BeneficiaryAccountNumber: "1234567890",
		},
		BeneficiaryAccountName: "Siti Rahayu",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sessionId")
}

func TestSnapPayoutClient_RejectsNonNumericExternalID(t *testing.T) {
	_, pemText := snapTestPrivateKey(t)
	client := newSnapTestClient(t, "https://api-sandbox.doku.com", pemText)

	_, err := client.TransferInquiry(t.Context(), "ext-1", TransferInquiryRequest{
		PartnerReferenceNo:       "PO-42",
		Amount:                   150000,
		BeneficiaryBankCode:      "014",
		BeneficiaryAccountNumber: "1234567890",
		BeneficiaryName:          "Siti Rahayu",
		CustomerNumber:           "6281234567890",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "numeric external id")
}

func TestSplitBeneficiaryName_RepeatsASingleWord(t *testing.T) {
	first, last, err := splitBeneficiaryName("Siti")
	require.NoError(t, err)
	assert.Equal(t, "Siti", first)
	assert.Equal(t, "Siti", last, "SNAP requires both name fields")

	first, last, err = splitBeneficiaryName("  Siti   Rahayu  ")
	require.NoError(t, err)
	assert.Equal(t, "Siti", first)
	assert.Equal(t, "Rahayu", last)

	_, _, err = splitBeneficiaryName("   ")
	require.Error(t, err)
}

func TestSnapPayoutClient_StatusNotFoundIsDistinct(t *testing.T) {
	_, pemText := snapTestPrivateKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, snapTokenPath) {
			_, _ = w.Write([]byte(`{"responseCode":"2007300","accessToken":"token-1","expiresIn":900}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"responseCode":"4040000","responseMessage":"Transaction not found"}`))
	}))
	defer server.Close()

	client := newSnapTestClient(t, server.URL, pemText)
	_, err := client.TransactionStatus(t.Context(), "420000003", TransactionStatusRequest{PartnerReferenceNo: "PO-42"})
	assert.ErrorIs(t, err, ErrTransactionNotFound)
}

func TestSnapPayoutClient_AcceptsStringExpiresIn(t *testing.T) {
	_, pemText := snapTestPrivateKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"responseCode":"2007300","accessToken":"token-1","expiresIn":"900"}`))
	}))
	defer server.Close()

	client := newSnapTestClient(t, server.URL, pemText)
	token, err := client.accessToken(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "token-1", token)
}

func TestSnapPayoutClient_BareHTTP404IsNotANotFoundProof(t *testing.T) {
	_, pemText := snapTestPrivateKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, snapTokenPath) {
			_, _ = w.Write([]byte(`{"responseCode":"2007300","accessToken":"token-1","expiresIn":900}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"gateway not found"}`))
	}))
	defer server.Close()

	client := newSnapTestClient(t, server.URL, pemText)
	_, err := client.TransactionStatus(t.Context(), "420000003", TransactionStatusRequest{PartnerReferenceNo: "PO-42"})

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrTransactionNotFound)
}

func TestNormalizeBeneficiaryName_CountsRunes(t *testing.T) {
	name, err := normalizeBeneficiaryName(strings.Repeat("李", 64))
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("李", 64), name)

	_, err = normalizeBeneficiaryName(strings.Repeat("李", 65))
	require.Error(t, err)
}
