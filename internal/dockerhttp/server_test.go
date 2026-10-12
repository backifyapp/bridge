package dockerhttp

import (
	"net/http/httptest"
	"testing"
)

// Names outside Docker's grammar never reach the docker CLI: "%2Fetc" decodes to
// "/etc", which `-v /etc:/data` would mount from the HOST.
func TestRoutesRefuseNamesOutsideDockerGrammar(t *testing.T) {
	h := Handler(secret)
	for _, c := range []struct{ method, target, body string }{
		{"POST", "/docker/volume/%2Fetc/import", "x"},
		{"GET", "/docker/volume/%2Froot/export", ""},
		{"GET", "/docker/volume/%2Fvar%2Flib/size", ""},
		{"GET", "/docker/container/-f=x/inspect", ""},
		{"POST", "/docker/container/--help/pause", ""},
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, signedReq(c.method, c.target, c.body, now(), freshNonce()))
		if rr.Code != 400 {
			t.Errorf("%s %s: want 400, got %d (%s)", c.method, c.target, rr.Code, rr.Body.String())
		}
	}
}

func TestRunRefusesHostBindMount(t *testing.T) {
	h := Handler(secret)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedReq("POST", "/docker/container/run", `{"name":"x","image":"alpine","volumes":["/:/host"]}`, now(), freshNonce()))
	if rr.Code != 400 {
		t.Fatalf("run spec with a host bind mount: want 400, got %d (%s)", rr.Code, rr.Body.String())
	}
}
