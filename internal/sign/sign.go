// Package sign implements the HMAC-SHA256 signatures between the Bridge and
// Backify. The secret NEVER travels: it only signs a canonical string. Two
// channels use the agent's secret, each with its own protocol-2 scheme line:
//
//   - agent → API (heartbeat): SchemeAPIV2, see RequestAPIV2. Mirrors the
//     backend verifier (apps/api/src/auth/agent.ts in prhost/backify).
//   - worker → local Docker helper: SchemeV2, verified in internal/dockerhttp.
//
// Protocol 1 (METHOD\nPATH\nTIMESTAMP\nNONCE) did not cover the body. The
// Bridge no longer sends it; CanonicalString/Signature stay only to pin the
// backend vector and the v1/v2 separation in tests.
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

// CanonicalString builds the protocol-1 string: METHOD\nPATH\nTIMESTAMP\nNONCE.
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
	// worker can tell "outdated agent" from "wrong secret". The API sets it on
	// its responses to the agent too (see APIProtocol).
	HeaderProtocol = "X-Backify-Protocol"
	HelperProtocol = 2
	SchemeV2       = "BACKIFY-BRIDGE-HMAC-SHA256-V2"
	EmptySHA256    = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// ── Protocol 2 (agent → Backify API) ───────────────────────────────────────
//
// Same layout as the helper's, with its own scheme line: the agent's secret
// signs both channels, and a signature from one must never be valid on the
// other. Protocol 1 left the heartbeat body (version, hostname, inventory) out
// of the HMAC, so whoever could tamper with a request inside the time window
// could change what the agent reports.
//
//	canonical = BACKIFY-AGENT-API-HMAC-SHA256-V2\nMETHOD\nPATH\nQUERY\nTIMESTAMP\nNONCE\nSHA256(body)
//
// The API marks protocol 2 by the presence of X-Backify-Content-SHA256, reads
// the body, checks it against the signed hash and only then burns the nonce.
// It answers with X-Backify-Protocol: 2. TestSignatureAPIV2SharedVector pins
// this to apps/api/src/auth/agent.test.ts in prhost/backify.
const (
	SchemeAPIV2 = "BACKIFY-AGENT-API-HMAC-SHA256-V2"
	// APIProtocol is the signature protocol this Bridge speaks to the API.
	APIProtocol = 2
)

func canonicalV2(scheme, method, path, query, timestamp, nonce, bodySHA256 string) string {
	return strings.Join([]string{
		scheme, strings.ToUpper(method), path, query, timestamp, nonce, strings.ToLower(bodySHA256),
	}, "\n")
}

func hmacHex(secret, canonical string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(canonical))
	return hex.EncodeToString(m.Sum(nil))
}

// CanonicalStringV2 builds the protocol-2 string the Docker helper verifies.
func CanonicalStringV2(method, path, query, timestamp, nonce, bodySHA256 string) string {
	return canonicalV2(SchemeV2, method, path, query, timestamp, nonce, bodySHA256)
}

// SignatureV2 is the hex HMAC-SHA256 of CanonicalStringV2.
func SignatureV2(secret, method, path, query, timestamp, nonce, bodySHA256 string) string {
	return hmacHex(secret, CanonicalStringV2(method, path, query, timestamp, nonce, bodySHA256))
}

// CanonicalStringAPIV2 builds the protocol-2 string for agent → API calls.
func CanonicalStringAPIV2(method, path, query, timestamp, nonce, bodySHA256 string) string {
	return canonicalV2(SchemeAPIV2, method, path, query, timestamp, nonce, bodySHA256)
}

// SignatureAPIV2 is the hex HMAC-SHA256 of CanonicalStringAPIV2.
func SignatureAPIV2(secret, method, path, query, timestamp, nonce, bodySHA256 string) string {
	return hmacHex(secret, CanonicalStringAPIV2(method, path, query, timestamp, nonce, bodySHA256))
}

// Signature is the hex HMAC-SHA256 of the protocol-1 string.
func Signature(secret, method, path, timestamp, nonce string) string {
	return hmacHex(secret, CanonicalString(method, path, timestamp, nonce))
}

// Nonce generates a single-use nonce (16 bytes → 32 hex chars).
func Nonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RequestAPIV2 signs an agent → API request in place (protocol 2). body must
// be exactly the bytes the request sends (nil = empty body); its SHA-256 goes
// into the signature and into X-Backify-Content-SHA256. PATH is the escaped
// request path and QUERY the raw query, as the API receives them.
func RequestAPIV2(req *http.Request, agentID, secret string, body []byte) error {
	nonce, err := Nonce()
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sum := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(sum[:])
	sig := SignatureAPIV2(secret, req.Method, req.URL.EscapedPath(), req.URL.RawQuery, ts, nonce, bodyHash)
	req.Header.Set(HeaderAgentID, agentID)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderContentSHA256, bodyHash)
	req.Header.Set(HeaderSignature, sig)
	return nil
}
