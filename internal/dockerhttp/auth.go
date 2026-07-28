package dockerhttp

import (
	"crypto/hmac"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/backifyapp/bridge/internal/sign"
)

const (
	clockSkewSeconds = 300
	nonceTTLSeconds  = 600
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

// authMiddleware verifies the HMAC signature (same scheme as the rest of the agent):
// ±300s clock window, signature over METHOD\nPATH\nTS\nNONCE, and a single-use
// nonce. The nonce is only burned after the signature checks out.
func authMiddleware(secret string, nc *nonceCache, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ts := r.Header.Get(sign.HeaderTimestamp)
		nonce := r.Header.Get(sign.HeaderNonce)
		gotSig := r.Header.Get(sign.HeaderSignature)
		if ts == "" || nonce == "" || gotSig == "" {
			http.Error(w, "assinatura ausente", http.StatusUnauthorized)
			return
		}
		now := time.Now().Unix()
		tsNum, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || abs64(now-tsNum) > clockSkewSeconds {
			http.Error(w, "timestamp fora da janela", http.StatusUnauthorized)
			return
		}
		want := sign.Signature(secret, r.Method, r.URL.Path, ts, nonce)
		if !hmac.Equal([]byte(want), []byte(gotSig)) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		if !nc.consume(nonce, now) {
			http.Error(w, "nonce reutilizado", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
