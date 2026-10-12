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

// ── Protocol 2 (worker → local Docker helper) ───────────────────────────────
//
// The v1 string above does not cover the request body: on the helper, a
// tampered POST /docker/volume/{name}/import or /docker/container/run kept a
// valid signature. Protocol 2 signs the body hash too, and starts with a scheme
// line so a v2 string can never be confused with a v1 one (same secret).
//
//	canonical = BACKIFY-BRIDGE-HMAC-SHA256-V2\nMETHOD\nPATH\nQUERY\nTIMESTAMP\nNONCE\nSHA256(body)
//
// PATH is the escaped request path, QUERY the raw query string (no "?"),
// SHA256(body) the lowercase hex digest of the exact body bytes (the empty-string
// digest for GET). The worker side is apps/worker/src/engine/bridge-helper-sign.ts
// in prhost/backify; TestSignatureV2SharedVector pins both sides to one vector.

const (
	HeaderContentSHA256 = "X-Backify-Content-SHA256"
	// HeaderProtocol is set on every helper response (401s included), so the
	// worker can tell "outdated agent" from "wrong secret".
	HeaderProtocol = "X-Backify-Protocol"
	HelperProtocol = 2
	SchemeV2       = "BACKIFY-BRIDGE-HMAC-SHA256-V2"
	EmptySHA256    = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// CanonicalStringV2 builds the protocol-2 string to sign.
func CanonicalStringV2(method, path, query, timestamp, nonce, bodySHA256 string) string {
	return strings.Join([]string{
		SchemeV2, strings.ToUpper(method), path, query, timestamp, nonce, strings.ToLower(bodySHA256),
	}, "\n")
}

// SignatureV2 is the hex HMAC-SHA256 of CanonicalStringV2.
func SignatureV2(secret, method, path, query, timestamp, nonce, bodySHA256 string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(CanonicalStringV2(method, path, query, timestamp, nonce, bodySHA256)))
	return hex.EncodeToString(m.Sum(nil))
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
