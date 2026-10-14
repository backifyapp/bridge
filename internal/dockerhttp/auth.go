package dockerhttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

// bodyRule caps what a route may spool before the signature of its body is
// checked. Without a cap, a signed request could fill $TMPDIR (and the disk
// it lives on) before anything looked at it.
type bodyRule struct {
	// max is the fixed ceiling in bytes (0 = no body).
	max int64
	// disk: the ceiling is the free space in os.TempDir() minus diskReserve
	// instead, and Content-Length is mandatory (the volume import, which can
	// be large but must never exhaust the disk).
	disk bool
}

var (
	noBody      = bodyRule{}
	jsonBody    = bodyRule{max: 1 << 20}
	archiveBody = bodyRule{disk: true}
)

// diskReserve is left free on the temp filesystem when spooling an import.
const diskReserve = 256 << 20

// freeSpace reports the bytes available to an unprivileged writer in dir
// (ok=false when unknown: the check is skipped, the fixed caps still apply).
var freeSpace = diskFree

// authMiddleware verifies signature protocol 3 (see sign.CanonicalStringV3):
// key is sign.HelperKey(secret), never the enroll secret itself.
//
// Order — nothing is stored and no body byte is acted on before the signature
// checks out:
//  1. headers present and well-formed; protocol-1 (no body hash) and
//     protocol-2 (enroll secret) requests get a clear "update the worker" error;
//  2. ±300s clock window;
//  3. HMAC over method, escaped path, raw query, ts, nonce and the body hash
//     (constant-time compare);
//  4. only then the nonce is burned;
//  5. a request body is size-checked (rule), spooled to a temp file while
//     hashed, and handed to the handler only if the hash matches the signed
//     one. The import used to stream straight into `tar -x`, so a tampered
//     body was applied before anyone could notice.
func authMiddleware(key string, nc *nonceCache, rule bodyRule, next http.HandlerFunc) http.HandlerFunc {
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
			http.Error(w, "protocolo de assinatura 1 recusado: este Bridge exige o protocolo 3 (o worker do Backify precisa ser atualizado)", http.StatusUnauthorized)
			return
		}
		if r.Header.Get(sign.HeaderProtocol) != strconv.Itoa(sign.HelperProtocol) {
			http.Error(w, "protocolo de assinatura 2 recusado: este Bridge (0.5.0+) só aceita o protocolo 3, assinado com a chave do helper derivada do segredo (o worker do Backify precisa ser atualizado)", http.StatusUnauthorized)
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
		want := sign.SignatureV3(key, r.Method, r.URL.EscapedPath(), r.URL.RawQuery, ts, nonce, bodyHash)
		if !hmac.Equal([]byte(want), []byte(gotSig)) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		if !nc.consume(nonce, now) {
			http.Error(w, "nonce reutilizado", http.StatusUnauthorized)
			return
		}

		if hasBody {
			limit, status, msg := bodyLimit(r, rule)
			if status != 0 {
				http.Error(w, msg, status)
				return
			}
			body, cleanup, err := spoolVerified(http.MaxBytesReader(w, r.Body, limit), claim)
			var tooLarge *http.MaxBytesError
			switch {
			case errors.As(err, &tooLarge):
				http.Error(w, "corpo maior que o permitido para esta rota", http.StatusRequestEntityTooLarge)
				return
			case errors.Is(err, errBodyMismatch):
				http.Error(w, "o corpo recebido não confere com o assinado", http.StatusBadRequest)
				return
			case err != nil:
				http.Error(w, "falha ao receber o corpo: "+err.Error(), http.StatusInternalServerError)
				return
			}
			defer cleanup()
			r.Body = body
		}
		next(w, r)
	}
}

// bodyLimit returns the byte ceiling for this request's body, or a non-zero
// status (and message) when it must be refused before reading anything.
func bodyLimit(r *http.Request, rule bodyRule) (int64, int, string) {
	if !rule.disk {
		if r.ContentLength > rule.max {
			return 0, http.StatusRequestEntityTooLarge, "corpo maior que o permitido para esta rota"
		}
		return rule.max, 0, ""
	}
	if r.ContentLength < 0 {
		return 0, http.StatusLengthRequired, "Content-Length obrigatório no import"
	}
	if free, ok := freeSpace(os.TempDir()); ok {
		if free < diskReserve || uint64(r.ContentLength) > free-diskReserve {
			return 0, http.StatusInsufficientStorage, fmt.Sprintf(
				"espaço insuficiente em %s para conferir o arquivo (%d bytes; livres %d, reserva %d)",
				os.TempDir(), r.ContentLength, free, diskReserve)
		}
	}
	return r.ContentLength, 0, ""
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
