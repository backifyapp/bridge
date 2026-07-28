// Package sign implementa a assinatura HMAC-SHA256 que autentica o Bridge na
// API do Backify. Espelha EXATAMENTE o verificador do backend
// (apps/api/src/auth/agent.ts) e o do plugin WordPress: o segredo NUNCA trafega,
// it only signs the canonical string.
//
//	canonical = METHOD\nPATH\nTIMESTAMP\nNONCE   (PATH = the request pathname)
package sign

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HMAC headers — names identical to the ones the backend expects.
const (
	HeaderAgentID   = "X-Backify-Agent-Id"
	HeaderTimestamp = "X-Backify-Timestamp"
	HeaderNonce     = "X-Backify-Nonce"
	HeaderSignature = "X-Backify-Signature"
)

// CanonicalString monta a string assinada: METHOD\nPATH\nTIMESTAMP\nNONCE.
func CanonicalString(method, path, timestamp, nonce string) string {
	return strings.ToUpper(method) + "\n" + path + "\n" + timestamp + "\n" + nonce
}

// Signature devolve o HMAC-SHA256 hex da string canônica.
func Signature(secret, method, path, timestamp, nonce string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(CanonicalString(method, path, timestamp, nonce)))
	return hex.EncodeToString(m.Sum(nil))
}

// Nonce generates a single-use nonce (16 bytes → 32 hex chars).
func Nonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Request signs the request in place: it uses the method and the URL pathname,
// the current timestamp and a fresh nonce. secret is the HMAC secret obtained
// during enroll.
func Request(req *http.Request, agentID, secret string) error {
	nonce, err := Nonce()
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := Signature(secret, req.Method, req.URL.Path, ts, nonce)
	req.Header.Set(HeaderAgentID, agentID)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, sig)
	return nil
}
