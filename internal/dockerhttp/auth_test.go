package dockerhttp

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/backifyapp/bridge/internal/sign"
)

const secret = "s3cr3t"

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func now() string { return strconv.FormatInt(time.Now().Unix(), 10) }

var nonceSeq int

func freshNonce() string {
	nonceSeq++
	return "nonce-de-teste-" + strconv.Itoa(1000+nonceSeq)
}

// signedReq builds a protocol-2 request exactly as the worker does: the body
// hash goes into the signature and into X-Backify-Content-SHA256.
func signedReq(method, target, body, ts, nonce string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	hash := sha(body)
	if method == http.MethodGet {
		hash = sign.EmptySHA256
	}
	req.Header.Set(sign.HeaderTimestamp, ts)
	req.Header.Set(sign.HeaderNonce, nonce)
	req.Header.Set(sign.HeaderContentSHA256, hash)
	req.Header.Set(sign.HeaderSignature, sign.SignatureV2(secret, method, req.URL.EscapedPath(), req.URL.RawQuery, ts, nonce, hash))
	return req
}

// recorder handler: what the route would have acted on.
type seen struct {
	called bool
	body   string
}

func newHandler(s *seen) http.HandlerFunc {
	nc := newNonceCache()
	return authMiddleware(secret, nc, func(w http.ResponseWriter, r *http.Request) {
		s.called = true
		b, _ := io.ReadAll(r.Body)
		s.body = string(b)
		w.WriteHeader(200)
	})
}

func TestAuthValidGetPasses(t *testing.T) {
	var s seen
	rr := httptest.NewRecorder()
	newHandler(&s)(rr, signedReq("GET", "/docker/ping", "", now(), freshNonce()))
	if rr.Code != 200 || !s.called {
		t.Fatalf("a valid signature should pass, got %d", rr.Code)
	}
	if rr.Header().Get(sign.HeaderProtocol) != "2" {
		t.Fatalf("every response must carry %s: 2", sign.HeaderProtocol)
	}
}

func TestAuthValidPostDeliversVerifiedBody(t *testing.T) {
	var s seen
	body := strings.Repeat("tar-bytes;", 300_000) // > 1 MiB, goes through the temp file
	rr := httptest.NewRecorder()
	newHandler(&s)(rr, signedReq("POST", "/docker/volume/pgdata/import", body, now(), freshNonce()))
	if rr.Code != 200 || s.body != body {
		t.Fatalf("valid POST: code=%d, body delivered intact=%v", rr.Code, s.body == body)
	}
}

// The defect: a tampered body kept the v1 signature and reached `tar -x` /
// `docker run`. Now the handler is never called.
func TestAuthTamperedBodyRefusedBeforeHandler(t *testing.T) {
	var s seen
	req := signedReq("POST", "/docker/container/run", `{"name":"app","image":"postgres:16"}`, now(), freshNonce())
	req.Body = io.NopCloser(strings.NewReader(`{"name":"x","image":"alpine","volumes":["/:/host"]}`))
	rr := httptest.NewRecorder()
	newHandler(&s)(rr, req)
	if rr.Code != 400 || s.called {
		t.Fatalf("tampered body: code=%d (want 400), handler called=%v (want false)", rr.Code, s.called)
	}
}

func TestAuthTamperedClaimRefused(t *testing.T) {
	var s seen
	req := signedReq("POST", "/docker/volume/pgdata/import", "legit", now(), freshNonce())
	req.Header.Set(sign.HeaderContentSHA256, sha("evil"))
	req.Body = io.NopCloser(strings.NewReader("evil"))
	rr := httptest.NewRecorder()
	newHandler(&s)(rr, req)
	if rr.Code != 401 || s.called {
		t.Fatalf("hash header swapped together with the body must break the signature, got %d", rr.Code)
	}
}

func TestAuthPathAndQueryAreSigned(t *testing.T) {
	var s seen
	h := newHandler(&s)
	// signature for /docker/ping replayed on another route
	req := signedReq("GET", "/docker/ping", "", now(), freshNonce())
	req.URL.Path = "/docker/volumes"
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != 401 {
		t.Fatalf("signature for another path must fail, got %d", rr.Code)
	}
	req = signedReq("GET", "/docker/ping", "", now(), freshNonce())
	req.URL.RawQuery = "x=1"
	rr = httptest.NewRecorder()
	h(rr, req)
	if rr.Code != 401 {
		t.Fatalf("an added query must fail, got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h(rr, signedReq("GET", "/docker/ping?x=1", "", now(), freshNonce()))
	if rr.Code != 200 {
		t.Fatalf("a signed query must pass, got %d", rr.Code)
	}
}

func TestAuthV1RequestRefusedWithVersionError(t *testing.T) {
	var s seen
	ts, nonce := now(), freshNonce()
	req := httptest.NewRequest("GET", "/docker/ping", nil)
	req.Header.Set(sign.HeaderTimestamp, ts)
	req.Header.Set(sign.HeaderNonce, nonce)
	req.Header.Set(sign.HeaderSignature, sign.Signature(secret, "GET", "/docker/ping", ts, nonce))
	rr := httptest.NewRecorder()
	newHandler(&s)(rr, req)
	if rr.Code != 401 || !strings.Contains(rr.Body.String(), "protocolo de assinatura 1") {
		t.Fatalf("v1 request: code=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestAuthMissingHeaders(t *testing.T) {
	var s seen
	rr := httptest.NewRecorder()
	newHandler(&s)(rr, httptest.NewRequest("GET", "/docker/ping", nil))
	if rr.Code != 401 {
		t.Fatalf("sem headers devia dar 401, veio %d", rr.Code)
	}
}

func TestAuthBadSignature(t *testing.T) {
	var s seen
	req := signedReq("GET", "/docker/ping", "", now(), freshNonce())
	req.Header.Set(sign.HeaderSignature, strings.Repeat("0", 64))
	rr := httptest.NewRecorder()
	newHandler(&s)(rr, req)
	if rr.Code != 401 {
		t.Fatalf("assinatura errada devia dar 401, veio %d", rr.Code)
	}
}

func TestAuthMalformedHeaders(t *testing.T) {
	var s seen
	h := newHandler(&s)
	for name, mutate := range map[string]func(*http.Request){
		"nonce curto":      func(r *http.Request) { r.Header.Set(sign.HeaderNonce, "abc") },
		"nonce com \\n":    func(r *http.Request) { r.Header.Set(sign.HeaderNonce, "abcdefgh\nijklmnop") },
		"nonce enorme":     func(r *http.Request) { r.Header.Set(sign.HeaderNonce, strings.Repeat("a", 65)) },
		"ts não numérico":  func(r *http.Request) { r.Header.Set(sign.HeaderTimestamp, "1e9") },
		"hash não hex":     func(r *http.Request) { r.Header.Set(sign.HeaderContentSHA256, strings.Repeat("z", 64)) },
		"assinatura curta": func(r *http.Request) { r.Header.Set(sign.HeaderSignature, "deadbeef") },
	} {
		req := signedReq("GET", "/docker/ping", "", now(), freshNonce())
		mutate(req)
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code != 401 {
			t.Errorf("%s: want 401, got %d", name, rr.Code)
		}
	}
}

func TestAuthClockSkew(t *testing.T) {
	var s seen
	old := strconv.FormatInt(time.Now().Unix()-1000, 10)
	rr := httptest.NewRecorder()
	newHandler(&s)(rr, signedReq("GET", "/docker/ping", "", old, freshNonce()))
	if rr.Code != 401 {
		t.Fatalf("timestamp velho devia dar 401, veio %d", rr.Code)
	}
}

func TestAuthReplay(t *testing.T) {
	var s seen
	h := newHandler(&s)
	ts, nonce := now(), freshNonce()
	rr1 := httptest.NewRecorder()
	h(rr1, signedReq("GET", "/docker/ping", "", ts, nonce))
	rr2 := httptest.NewRecorder()
	h(rr2, signedReq("GET", "/docker/ping", "", ts, nonce))
	if rr1.Code != 200 || rr2.Code != 401 {
		t.Fatalf("replay: 1ª=%d (quer 200), 2ª=%d (quer 401)", rr1.Code, rr2.Code)
	}
}

// Nonces are only remembered after the signature checks out: garbage
// requests do not grow the cache.
func TestAuthNonceOnlyAfterSignature(t *testing.T) {
	nc := newNonceCache()
	h := authMiddleware(secret, nc, func(w http.ResponseWriter, r *http.Request) {})
	for i := 0; i < 50; i++ {
		req := signedReq("GET", "/docker/ping", "", now(), freshNonce())
		req.Header.Set(sign.HeaderSignature, strings.Repeat("0", 64))
		h(httptest.NewRecorder(), req)
	}
	if len(nc.seen) != 0 {
		t.Fatalf("%d nonces stored by unauthenticated requests", len(nc.seen))
	}
}

// No spool file is left behind, accepted or refused.
func TestAuthNoTempFilesLeft(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	var s seen
	h := newHandler(&s)
	h(httptest.NewRecorder(), signedReq("POST", "/docker/volume/v/import", "ok", now(), freshNonce()))
	req := signedReq("POST", "/docker/volume/v/import", "legit", now(), freshNonce())
	req.Body = io.NopCloser(strings.NewReader("evil"))
	h(httptest.NewRecorder(), req)
	left, _ := filepath.Glob(filepath.Join(dir, "backify-bridge-body-*"))
	if len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
	_ = os.Remove(dir)
}
