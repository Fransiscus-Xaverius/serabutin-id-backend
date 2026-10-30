package doku

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	subAccountTestToken      = "tok-test-1"
	subAccountTestTimestamp  = "2026-10-31T10:00:00+07:00"
	subAccountTestRemoteRef  = "REF-900001"
	subAccountTestBankCode   = "014"
	subAccountTestAccountNum = "1234567890"
)

var numericExternalID = regexp.MustCompile(`^[0-9]+$`)

type subAccountServer struct {
	tokenCalls   atomic.Int32
	lastHeader   http.Header
	lastBody     []byte
	lastPath     string
	transferResp func(w http.ResponseWriter)
}

func newSubAccountServer(t *testing.T, transfer func(w http.ResponseWriter)) (*httptest.Server, *subAccountServer) {
	t.Helper()
	state := &subAccountServer{transferResp: transfer}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == subAccountTokenPath {
			state.tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"` + subAccountTestToken + `"}`))
			return
		}
		state.lastPath = r.URL.Path
		state.lastHeader = r.Header.Clone()
		state.lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		state.transferResp(w)
	}))
	t.Cleanup(server.Close)
	return server, state
}

func okTransfer(w http.ResponseWriter) {
	_, _ = w.Write([]byte(`{"responseCode":"2002500","referenceNo":"` + subAccountTestRemoteRef + `","partnerReferenceNo":"PR-1","beneficiaryAccountName":"Budi"}`))
}

func newTestSubAccountClient(baseURL string) *SubAccountClient {
	return NewSubAccountClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: baseURL})
}

func testInquiryRequest() TransferInquiryRequest {
	return TransferInquiryRequest{
		PartnerReferenceNo:       "PR-1",
		Amount:                   155000,
		FromAccount:              "9999999999",
		BeneficiaryBankCode:      subAccountTestBankCode,
		BeneficiaryAccountNumber: subAccountTestAccountNum,
		Remark:                   "payout",
	}
}

func TestSubAccount_SignSHA512MatchesIndependentComputation(t *testing.T) {
	body := `{ "a" : 1 }`

	sum := sha256.Sum256([]byte(`{"a":1}`))
	stringToSign := "POST:" + subAccountInquiryPath + ":" + subAccountTestToken + ":" +
		hex.EncodeToString(sum[:]) + ":" + subAccountTestTimestamp
	mac := hmac.New(sha512.New, []byte(testSecret))
	mac.Write([]byte(stringToSign))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	got := SignSHA512(testSecret, http.MethodPost, subAccountInquiryPath, subAccountTestToken, body, subAccountTestTimestamp)

	assert.Equal(t, expected, got)
}

func TestSubAccount_SignatureCoversTheBody(t *testing.T) {
	first := SignSHA512(testSecret, http.MethodPost, subAccountInquiryPath, subAccountTestToken, `{"amount":"1.00"}`, subAccountTestTimestamp)
	second := SignSHA512(testSecret, http.MethodPost, subAccountInquiryPath, subAccountTestToken, `{"amount":"2.00"}`, subAccountTestTimestamp)

	assert.NotEqual(t, first, second)
}

func TestSubAccount_TransferInquirySendsSignedHeaders(t *testing.T) {
	server, state := newSubAccountServer(t, okTransfer)
	client := newTestSubAccountClient(server.URL)

	result, err := client.TransferInquiry(context.Background(), testInquiryRequest())

	require.NoError(t, err)
	assert.Equal(t, subAccountTestRemoteRef, result.ReferenceNo)
	assert.Equal(t, "Budi", result.BeneficiaryAccountName)
	assert.False(t, result.Replayed)

	assert.Equal(t, subAccountInquiryPath, state.lastPath)
	assert.Equal(t, "Bearer "+subAccountTestToken, state.lastHeader.Get("Authorization"))
	assert.Equal(t, testClientID, state.lastHeader.Get("X-PARTNER-ID"))
	assert.Regexp(t, numericExternalID, state.lastHeader.Get("X-EXTERNAL-ID"))
	assert.NotEmpty(t, state.lastHeader.Get("X-TIMESTAMP"))
	_, parseErr := time.Parse(time.RFC3339, state.lastHeader.Get("X-TIMESTAMP"))
	assert.NoError(t, parseErr, "X-TIMESTAMP must be ISO 8601 with a zone offset")

	expectedSignature := SignSHA512(testSecret, http.MethodPost, subAccountInquiryPath,
		subAccountTestToken, string(state.lastBody), state.lastHeader.Get("X-TIMESTAMP"))
	assert.Equal(t, expectedSignature, state.lastHeader.Get("X-SIGNATURE"))

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(state.lastBody, &sent))
	assert.Equal(t, "PR-1", sent["partnerReferenceNo"])
	assert.Equal(t, "BANK_ACCOUNT", sent["type"])
	assert.Equal(t, "BI_FAST", sent["channel"])
	assert.Equal(t, "9999999999", sent["fromAccount"])
	assert.Equal(t, subAccountTestBankCode, sent["beneficiaryBankCode"])
	assert.Equal(t, subAccountTestAccountNum, sent["beneficiaryAccountNumber"])
	assert.Equal(t, "payout", sent["remark"])
	amount := sent["amount"].(map[string]interface{})
	assert.Equal(t, "155000.00", amount["value"])
	assert.Equal(t, "IDR", amount["currency"])
}

func TestSubAccount_TransferPaymentCarriesTheInquiryReferences(t *testing.T) {
	server, state := newSubAccountServer(t, okTransfer)
	client := newTestSubAccountClient(server.URL)

	result, err := client.TransferPayment(context.Background(), TransferPaymentRequest{
		TransferInquiryRequest: testInquiryRequest(),
		ReferenceNo:            subAccountTestRemoteRef,
		BeneficiaryAccountName: "Budi",
	})

	require.NoError(t, err)
	assert.Equal(t, subAccountTestRemoteRef, result.ReferenceNo)
	assert.False(t, result.Replayed)
	assert.Equal(t, subAccountPaymentPath, state.lastPath)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(state.lastBody, &sent))
	assert.Equal(t, "PR-1", sent["partnerReferenceNo"])
	assert.Equal(t, subAccountTestRemoteRef, sent["referenceNo"])
	assert.Equal(t, "Budi", sent["beneficiaryAccountName"])
	assert.Equal(t, "BI_FAST", sent["channel"])
}

func TestSubAccount_SignatureChangesWithTheBody(t *testing.T) {
	server, state := newSubAccountServer(t, okTransfer)
	client := newTestSubAccountClient(server.URL)

	_, err := client.TransferInquiry(context.Background(), testInquiryRequest())
	require.NoError(t, err)
	firstSignature := state.lastHeader.Get("X-SIGNATURE")

	changed := testInquiryRequest()
	changed.Amount = 155001
	_, err = client.TransferInquiry(context.Background(), changed)
	require.NoError(t, err)

	assert.NotEqual(t, firstSignature, state.lastHeader.Get("X-SIGNATURE"))
}

func TestSubAccount_Replays409WithAReferenceNo(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"responseCode":"4092500","referenceNo":"` + subAccountTestRemoteRef + `"}`))
	})
	client := newTestSubAccountClient(server.URL)

	result, err := client.TransferInquiry(context.Background(), testInquiryRequest())

	require.NoError(t, err, "a 409 carrying a reference is a replay, not a failure")
	assert.True(t, result.Replayed)
	assert.Equal(t, subAccountTestRemoteRef, result.ReferenceNo)
}

func TestSubAccount_Replays409OnPaymentWithAReferenceNo(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"referenceNo":"` + subAccountTestRemoteRef + `"}`))
	})
	client := newTestSubAccountClient(server.URL)

	result, err := client.TransferPayment(context.Background(), TransferPaymentRequest{
		TransferInquiryRequest: testInquiryRequest(),
		ReferenceNo:            subAccountTestRemoteRef,
		BeneficiaryAccountName: "Budi",
	})

	require.NoError(t, err)
	assert.True(t, result.Replayed)
	assert.Equal(t, subAccountTestRemoteRef, result.ReferenceNo)
}

func TestSubAccount_Treats409WithoutAReferenceAsAnError(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"responseCode":"4092500","responseMessage":"Conflict"}`))
	})
	client := newTestSubAccountClient(server.URL)

	result, err := client.TransferInquiry(context.Background(), testInquiryRequest())

	require.Error(t, err)
	assert.Nil(t, result)
	assert.NotContains(t, err.Error(), testSecret)
	assert.NotContains(t, err.Error(), subAccountTestToken)
}

func TestSubAccount_FetchesTheTokenOnceAndReusesIt(t *testing.T) {
	server, state := newSubAccountServer(t, okTransfer)
	client := newTestSubAccountClient(server.URL)

	_, err := client.TransferInquiry(context.Background(), testInquiryRequest())
	require.NoError(t, err)
	_, err = client.TransferInquiry(context.Background(), testInquiryRequest())
	require.NoError(t, err)
	_, err = client.BalanceInquiry(context.Background(), "profile-1")
	require.NoError(t, err)

	assert.Equal(t, int32(1), state.tokenCalls.Load())
}

func TestSubAccount_RefreshesTheTokenBeforeItExpires(t *testing.T) {
	server, state := newSubAccountServer(t, okTransfer)
	client := newTestSubAccountClient(server.URL)
	now := time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }

	_, err := client.TransferInquiry(context.Background(), testInquiryRequest())
	require.NoError(t, err)

	now = now.Add(time.Minute)
	_, err = client.TransferInquiry(context.Background(), testInquiryRequest())
	require.NoError(t, err)
	assert.Equal(t, int32(1), state.tokenCalls.Load(), "a token is cached until 60s before its 360s valid_time")

	now = now.Add(5 * time.Minute)
	_, err = client.TransferInquiry(context.Background(), testInquiryRequest())
	require.NoError(t, err)
	assert.Equal(t, int32(2), state.tokenCalls.Load(), "a token past its refresh point is fetched again")
}

func TestSubAccount_TokenRequestIsSignedWithTheNonSNAPScheme(t *testing.T) {
	var tokenHeader http.Header
	var tokenBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == subAccountTokenPath {
			tokenHeader = r.Header.Clone()
			tokenBody, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"token":"` + subAccountTestToken + `"}`))
			return
		}
		_, _ = w.Write([]byte(`{"referenceNo":"R"}`))
	}))
	defer server.Close()
	client := newTestSubAccountClient(server.URL)

	_, err := client.TransferInquiry(context.Background(), testInquiryRequest())
	require.NoError(t, err)

	assert.Equal(t, `{"grant_type":"client_credentials","valid_time":"360"}`, string(tokenBody))
	assert.Equal(t, testClientID, tokenHeader.Get("Client-Id"))
	assert.NotEmpty(t, tokenHeader.Get("Request-Id"))
	assert.NotEmpty(t, tokenHeader.Get("Request-Timestamp"))
	assert.Equal(t, Sign(testClientID, tokenHeader.Get("Request-Id"), tokenHeader.Get("Request-Timestamp"),
		subAccountTokenPath, Digest(tokenBody), testSecret), strings.TrimPrefix(tokenHeader.Get("Signature"), signaturePrefix))
}

const subAccountStatusDocumentedResponse = `{"responseCode":"2000000","responseMessage":"Successful","partnerReferenceNo":"PR-1","transactionType":"TRANSFER_P2P","latestTransactionStatus":"00","latestTransactionDesc":"success","transactionDate":"2026-10-31T09:00:00Z","amount":{"value":"155000.00","currency":"IDR"},"refundHistory":[{"refundNo":"RF-1","refundStatus":"SUCCESS","transactionDate":"2026-10-31T10:00:00Z","reason":"reversal","refundAmount":{"value":"0.00","currency":"IDR"}}]}`

func TestSubAccount_TransactionStatusSendsTheDocumentedRequestBody(t *testing.T) {
	server, state := newSubAccountServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(subAccountStatusDocumentedResponse))
	})
	client := newTestSubAccountClient(server.URL)

	_, err := client.TransactionStatus(context.Background(), "PR-1")

	require.NoError(t, err)
	assert.Equal(t, subAccountStatusPath, state.lastPath)
	assert.Equal(t, `{"partnerReferenceNo":"PR-1"}`, string(state.lastBody))
}

func TestSubAccount_TransactionStatusReadsTheDocumentedResponse(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(subAccountStatusDocumentedResponse))
	})
	client := newTestSubAccountClient(server.URL)

	result, err := client.TransactionStatus(context.Background(), "PR-1")

	require.NoError(t, err)
	assert.Equal(t, "PR-1", result.PartnerReferenceNo)
	assert.Equal(t, "TRANSFER_P2P", result.TransactionType)
	assert.Equal(t, TransactionStatusSuccess, result.LatestStatus)
	assert.Equal(t, "00", result.LatestStatus)
	assert.Equal(t, "success", result.LatestStatusDesc)
	assert.Equal(t, "2026-10-31T09:00:00Z", result.TransactionDate)
	assert.Equal(t, Money{Value: "155000.00", Currency: "IDR"}, result.Amount)
	require.Len(t, result.RefundHistory, 1)
	assert.Equal(t, "RF-1", result.RefundHistory[0].RefundNo)
	assert.Equal(t, "SUCCESS", result.RefundHistory[0].RefundStatus)
	assert.Equal(t, "reversal", result.RefundHistory[0].Reason)
	assert.Equal(t, Money{Value: "0.00", Currency: "IDR"}, result.RefundHistory[0].RefundAmount)
}

func TestSubAccount_TransactionStatusFailsClosedWithoutAMatchingReference(t *testing.T) {
	cases := map[string]string{
		"missing partner reference": `{"latestTransactionStatus":"00"}`,
		"other partner reference":   `{"partnerReferenceNo":"PR-OTHER","latestTransactionStatus":"00"}`,
		"missing status":            `{"partnerReferenceNo":"PR-1"}`,
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
				_, _ = w.Write([]byte(response))
			})
			client := newTestSubAccountClient(server.URL)

			result, err := client.TransactionStatus(context.Background(), "PR-1")

			require.Error(t, err)
			assert.Nil(t, result)
			assert.NotContains(t, err.Error(), testSecret)
			assert.NotContains(t, err.Error(), subAccountTestToken)
		})
	}
}

func TestSubAccount_BalanceInquirySendsTheDocumentedRequestBody(t *testing.T) {
	server, state := newSubAccountServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{}`))
	})
	client := newTestSubAccountClient(server.URL)

	_, err := client.BalanceInquiry(context.Background(), "hub-profile-1")

	require.NoError(t, err)
	assert.Equal(t, subAccountBalancePath, state.lastPath)
	assert.Equal(t, `{"profileId":"hub-profile-1"}`, string(state.lastBody), "accounts is optional and omitted, so all accounts come back")
}

func TestSubAccount_BalanceInquiryReadsTheDocumentedResponse(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"responseCode":"2000000","responseMessage":"Successful","name":"Mitra Budi","profileId":"hub-profile-1","accounts":[` +
			`{"type":"DOKU_MERCHANT_IDR","currency":"IDR","accountNo":"ACC-1","balance":{"available":"1000000.00","reserved":"0.00"}},` +
			`{"type":"DOKU_MERCHANT_PENDING_IDR","currency":"IDR","accountNo":"ACC-2","balance":{"available":"0.00","reserved":"250000.00"}},` +
			`{"type":"DOKU_MERCHANT_POINT","currency":"POINT","accountNo":"ACC-3","balance":{"available":"5.00","reserved":"0.00"}}]}`))
	})
	client := newTestSubAccountClient(server.URL)

	result, err := client.BalanceInquiry(context.Background(), "hub-profile-1")

	require.NoError(t, err)
	assert.Equal(t, "Mitra Budi", result.Name)
	assert.Equal(t, "hub-profile-1", result.ProfileID)
	require.Len(t, result.Accounts, 3)
	assert.Equal(t, BalanceAccount{
		Type:      "DOKU_MERCHANT_PENDING_IDR",
		Currency:  "IDR",
		AccountNo: "ACC-2",
		Available: "0.00",
		Reserved:  "250000.00",
	}, result.Accounts[1])
}

func TestSubAccount_SettledAvailableIsTheIDRAccountNotThePendingOne(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"accounts":[` +
			`{"type":"DOKU_MERCHANT_PENDING_IDR","currency":"IDR","accountNo":"ACC-2","balance":{"available":"9.00","reserved":"250000.00"}},` +
			`{"type":"DOKU_MERCHANT_IDR","currency":"IDR","accountNo":"ACC-1","balance":{"available":"1000000.00","reserved":"0.00"}}]}`))
	})
	client := newTestSubAccountClient(server.URL)

	result, err := client.BalanceInquiry(context.Background(), "hub-profile-1")
	require.NoError(t, err)

	available, ok := result.SettledAvailable()
	require.True(t, ok)
	assert.Equal(t, "1000000.00", available)
}

func TestSubAccount_SettledAvailableIsAbsentWithoutTheIDRAccount(t *testing.T) {
	result := &BalanceResult{Accounts: []BalanceAccount{{Type: BalanceAccountMerchantPendingIDR, Available: "9.00"}}}

	_, ok := result.SettledAvailable()

	assert.False(t, ok)
}

func TestSubAccount_TransferInquiryReadsTheBeneficiaryNameFromTheDocumentedResponse(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"responseCode":"2002400","responseMessage":"Successful","referenceNo":"REF-900001","partnerReferenceNo":"PR-1","type":"BANK_ACCOUNT","channel":"BI_FAST","fromAccount":"9999999999","beneficiaryAccountNumber":"1234567890","beneficiaryAccountName":"BUDI SANTOSO","beneficiaryBankCode":"014","amount":{"value":"155000.00","currency":"IDR"}}`))
	})
	client := newTestSubAccountClient(server.URL)

	result, err := client.TransferInquiry(context.Background(), testInquiryRequest())

	require.NoError(t, err)
	assert.Equal(t, "BUDI SANTOSO", result.BeneficiaryAccountName)
	assert.Equal(t, subAccountTestRemoteRef, result.ReferenceNo)
}

func TestSubAccount_TransferPaymentReadsTheDocumentedResponse(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"responseCode":"2002500","responseMessage":"Successful","referenceNo":"REF-900001","partnerReferenceNo":"PR-1","type":"BANK_ACCOUNT","channel":"BI_FAST","amount":{"value":"155000.00","currency":"IDR"},"referenceNumber":"RN-77","transactionDate":"2026-10-31T09:01:00Z","beneficiaryBankCode":"014","beneficiaryAccountNumber":"1234567890","beneficiaryAccountName":"BUDI SANTOSO","fromAccount":"9999999999"}`))
	})
	client := newTestSubAccountClient(server.URL)

	result, err := client.TransferPayment(context.Background(), TransferPaymentRequest{
		TransferInquiryRequest: testInquiryRequest(),
		ReferenceNo:            subAccountTestRemoteRef,
		BeneficiaryAccountName: "BUDI SANTOSO",
	})

	require.NoError(t, err)
	assert.Equal(t, "RN-77", result.ReferenceNumber)
	assert.Equal(t, "2026-10-31T09:01:00Z", result.TransactionDate)
}

func TestSubAccount_TransferFailsClosedOnA2xxWithoutAReferenceNo(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"responseCode":"2002500","partnerReferenceNo":"PR-1"}`))
	})
	client := newTestSubAccountClient(server.URL)

	inquiry, err := client.TransferInquiry(context.Background(), testInquiryRequest())
	require.Error(t, err)
	assert.Nil(t, inquiry)

	payment, err := client.TransferPayment(context.Background(), TransferPaymentRequest{
		TransferInquiryRequest: testInquiryRequest(),
		ReferenceNo:            subAccountTestRemoteRef,
		BeneficiaryAccountName: "BUDI SANTOSO",
	})
	require.Error(t, err)
	assert.Nil(t, payment)
}

func TestSubAccount_ErrorBodiesNeverCarryTheTokenOrSecret(t *testing.T) {
	server, _ := newSubAccountServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"echo":"Bearer ` + subAccountTestToken + ` secret ` + testSecret + `"}`))
	})
	client := newTestSubAccountClient(server.URL)

	_, err := client.TransactionStatus(context.Background(), "PR-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 400")
	assert.NotContains(t, err.Error(), testSecret)
	assert.NotContains(t, err.Error(), subAccountTestToken)
	assert.Contains(t, err.Error(), "[redacted]")
}

func TestSubAccount_RequiresConfiguration(t *testing.T) {
	client := NewSubAccountClient(Config{BaseURL: "https://api-sandbox.doku.com"})

	_, err := client.TransferInquiry(context.Background(), testInquiryRequest())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_CLIENT_ID")
}
