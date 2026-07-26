// Package sign implementa a assinatura HMAC-SHA256 que autentica o Bridge na
// API do Backify. Espelha EXATAMENTE o verificador do backend
// (apps/api/src/auth/agent.ts) e o do plugin WordPress: o segredo NUNCA trafega,
// só assina a string canônica.
//
//	canonical = METHOD\nPATH\nTIMESTAMP\nNONCE   (PATH = pathname da requisição)
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

// Headers HMAC — nomes idênticos aos esperados pelo backend.
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

// Nonce gera um nonce de uso único (16 bytes → 32 hex chars).
func Nonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Request assina a requisição in-place: usa o método e o pathname da URL, o
// timestamp atual e um nonce novo. secret é o segredo HMAC obtido no enroll.
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
