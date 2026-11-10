//go:build ignore

// Prints a ready-to-paste DOKU Checkout (non-SNAP) cURL for the sandbox.
// Usage: set DOKU_CLIENT_ID + DOKU_SECRET_KEY, then
//
//	go run scripts/doku-curl.go
//
// The signature is bound to the timestamp (±3600s window at DOKU) and to the
// exact body bytes, so regenerate the cURL instead of editing it.
package main

import (
	"strings"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const checkoutPath = "/checkout/v1/payment"

func main() {
	clientID := os.Getenv("DOKU_CLIENT_ID")
	secret := os.Getenv("DOKU_SECRET_KEY")
	baseURL := os.Getenv("DOKU_BASE_URL")
	if clientID == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "DOKU_CLIENT_ID and DOKU_SECRET_KEY are required")
		os.Exit(1)
	}
	if baseURL == "" {
		baseURL = "https://api-sandbox.doku.com"
	}

	method := os.Getenv("DOKU_METHOD")
	if method == "" {
		method = "QRIS"
	}
	methods := []string{}
	for _, m := range strings.Split(method, ",") {
		if m = strings.TrimSpace(m); m != "" {
			methods = append(methods, m)
		}
	}
	invoice := "INV-STAGING-" + time.Now().UTC().Format("20060102150405")

	body := `{"order":{"amount":150000,"invoice_number":"` + invoice + `","currency":"IDR","callback_url":"http://localhost:5173/payment/return?booking_id=1","callback_url_result":"http://localhost:5173/payment/return?booking_id=1","auto_redirect":true,"language":"ID"},"payment":{"payment_due_date":60,"payment_method_types":` + jsonMarshal(methods) + `,"type":"SALE"},"customer":{"id":"1","name":"Budi Santoso","email":"budi@example.com","phone":"6281234567890"}}`
	requestID := randomID()
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	sum := sha256.Sum256([]byte(body))
	digest := base64.StdEncoding.EncodeToString(sum[:])

	stringToSign := "Client-Id:" + clientID + "\n" +
		"Request-Id:" + requestID + "\n" +
		"Request-Timestamp:" + timestamp + "\n" +
		"Request-Target:" + checkoutPath + "\n" +
		"Digest:" + digest
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	fmt.Println("curl -sS -X POST " + baseURL + checkoutPath + " \\")
	fmt.Println("  -H 'Content-Type: application/json' \\")
	fmt.Println("  -H 'Client-Id: " + clientID + "' \\")
	fmt.Println("  -H 'Request-Id: " + requestID + "' \\")
	fmt.Println("  -H 'Request-Timestamp: " + timestamp + "' \\")
	fmt.Println("  -H 'Signature: HMACSHA256=" + signature + "' \\")
	fmt.Println("  --data-binary '" + body + "'")
}

func jsonMarshal(v []string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
