package dockerhttp

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/backifyapp/bridge/internal/sign"
)

const secret = "s3cr3t"

func signedReq(method, path, ts, nonce string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(sign.HeaderTimestamp, ts)
	req.Header.Set(sign.HeaderNonce, nonce)
	req.Header.Set(sign.HeaderSignature, sign.Signature(secret, method, path, ts, nonce))
	return req
}

func newHandler() http.HandlerFunc {
	nc := newNonceCache()
	return authMiddleware(secret, nc, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
}

func now() string { return strconv.FormatInt(time.Now().Unix(), 10) }

func TestAuthValidPasses(t *testing.T) {
	rr := httptest.NewRecorder()
	newHandler()(rr, signedReq("GET", "/docker/ping", now(), "n1"))
	if rr.Code != 200 {
		t.Fatalf("assinatura válida devia passar, veio %d", rr.Code)
	}
}

func TestAuthMissingHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/docker/ping", nil)
	newHandler()(rr, req)
	if rr.Code != 401 {
		t.Fatalf("sem headers devia dar 401, veio %d", rr.Code)
	}
}

func TestAuthBadSignature(t *testing.T) {
	rr := httptest.NewRecorder()
	req := signedReq("GET", "/docker/ping", now(), "n2")
	req.Header.Set(sign.HeaderSignature, "deadbeef")
	newHandler()(rr, req)
	if rr.Code != 401 {
		t.Fatalf("assinatura errada devia dar 401, veio %d", rr.Code)
	}
}

func TestAuthClockSkew(t *testing.T) {
	old := strconv.FormatInt(time.Now().Unix()-1000, 10)
	rr := httptest.NewRecorder()
	newHandler()(rr, signedReq("GET", "/docker/ping", old, "n3"))
	if rr.Code != 401 {
		t.Fatalf("timestamp velho devia dar 401, veio %d", rr.Code)
	}
}

func TestAuthReplay(t *testing.T) {
	h := newHandler()
	ts := now()
	// mesmo request (ts+nonce+sig) duas vezes: 1ª passa, 2ª é replay.
	rr1 := httptest.NewRecorder()
	h(rr1, signedReq("GET", "/docker/ping", ts, "dup"))
	rr2 := httptest.NewRecorder()
	h(rr2, signedReq("GET", "/docker/ping", ts, "dup"))
	if rr1.Code != 200 || rr2.Code != 401 {
		t.Fatalf("replay: 1ª=%d (quer 200), 2ª=%d (quer 401)", rr1.Code, rr2.Code)
	}
}
