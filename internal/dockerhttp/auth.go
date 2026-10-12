package dockerhttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/backifyapp/bridge/internal/sign"
)

const (
	clockSkewSeconds = 300
	// A nonce must be remembered for as long as its timestamp is accepted:
	// first use at ts-300 and a replay at ts+300 are 600s apart.
	nonceTTLSeconds = 2*clockSkewSeconds + 60
)

var (
	tsRe    = regexp.MustCompile(`^[0-9]{1,12}$`)
	nonceRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)
	hexRe   = regexp.MustCompile(`^[0-9a-f]{64}$`)

	errBodyMismatch = errors.New("body does not match the signed hash")
)

// nonceCache holds already-used nonces (replay protection), with expiry.
type nonceCache struct {
	mu   sync.Mutex
	seen map[string]int64 // nonce → expiry unix
}

func newNonceCache() *nonceCache { return &nonceCache{seen: map[string]int64{}} }

// consume reserves the nonce; returns false if it was already used (replay).
func (c *nonceCache) consume(nonce string, now int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, exp := range c.seen {
		if exp < now {
			delete(c.seen, k)
		}
	}
	if _, dup := c.seen[nonce]; dup {
		return false
	}
	c.seen[nonce] = now + nonceTTLSeconds
	return true
}

// authMiddleware verifies signature protocol 2 (see sign.CanonicalStringV2).
//
// Order — nothing is stored and no body byte is acted on before the signature
// checks out:
//  1. headers present and well-formed; v1 requests (no body hash) get a clear
//     "protocol 1 refused" error;
//  2. ±300s clock window;
//  3. HMAC over method, escaped path, raw query, ts, nonce and the body hash
//     (constant-time compare);
//  4. only then the nonce is burned;
//  5. a request body is spooled to a temp file while hashed, and handed to
//     the handler only if the hash matches the signed one. The import used to
//     stream straight into `tar -x`, so a tampered body was applied before
//     anyone could notice.
func authMiddleware(secret string, nc *nonceCache, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sign.HeaderProtocol, strconv.Itoa(sign.HelperProtocol))

		ts := r.Header.Get(sign.HeaderTimestamp)
		nonce := r.Header.Get(sign.HeaderNonce)
		gotSig := strings.ToLower(r.Header.Get(sign.HeaderSignature))
		claim := strings.ToLower(r.Header.Get(sign.HeaderContentSHA256))
		if ts == "" || nonce == "" || gotSig == "" {
			http.Error(w, "assinatura ausente", http.StatusUnauthorized)
			return
		}
		if claim == "" {
			http.Error(w, "protocolo de assinatura 1 recusado: este Bridge exige o protocolo 2 (o worker do Backify precisa ser atualizado)", http.StatusUnauthorized)
			return
		}
		if !tsRe.MatchString(ts) || !nonceRe.MatchString(nonce) || !hexRe.MatchString(gotSig) || !hexRe.MatchString(claim) {
			http.Error(w, "assinatura malformada", http.StatusUnauthorized)
			return
		}
		now := time.Now().Unix()
		tsNum, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || abs64(now-tsNum) > clockSkewSeconds {
			http.Error(w, "timestamp fora da janela", http.StatusUnauthorized)
			return
		}

		hasBody := r.Method != http.MethodGet && r.Method != http.MethodHead
		bodyHash := sign.EmptySHA256
		if hasBody {
			bodyHash = claim
		}
		want := sign.SignatureV2(secret, r.Method, r.URL.EscapedPath(), r.URL.RawQuery, ts, nonce, bodyHash)
		if !hmac.Equal([]byte(want), []byte(gotSig)) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		if !nc.consume(nonce, now) {
			http.Error(w, "nonce reutilizado", http.StatusUnauthorized)
			return
		}

		if hasBody {
			body, cleanup, err := spoolVerified(r.Body, claim)
			if errors.Is(err, errBodyMismatch) {
				http.Error(w, "o corpo recebido não confere com o assinado", http.StatusBadRequest)
				return
			}
			if err != nil {
				http.Error(w, "falha ao receber o corpo: "+err.Error(), http.StatusInternalServerError)
				return
			}
			defer cleanup()
			r.Body = body
		}
		next(w, r)
	}
}

// spoolVerified copies the body to a temp file (os.TempDir — $TMPDIR, a
// private /tmp under the systemd unit) hashing it on the way, and returns the
// file rewound only when the digest equals wantHex. The file is always removed
// by cleanup, or right away on failure.
func spoolVerified(body io.Reader, wantHex string) (io.ReadCloser, func(), error) {
	f, err := os.CreateTemp("", "backify-bridge-body-*")
	if err != nil {
		return nil, nil, err
	}
	discard := func() { _ = f.Close(); _ = os.Remove(f.Name()) }
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), body); err != nil {
		discard()
		return nil, nil, err
	}
	if !hmac.Equal([]byte(hex.EncodeToString(h.Sum(nil))), []byte(wantHex)) {
		discard()
		return nil, nil, errBodyMismatch
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		discard()
		return nil, nil, err
	}
	return f, discard, nil
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
