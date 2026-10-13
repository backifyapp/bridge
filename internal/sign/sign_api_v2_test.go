package sign

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Agent → API protocol-2 vector shared with the backend (prhost/backify,
// apps/api/src/auth/agent.test.ts, "vetor compartilhado com o Go"). If either
// side changes the canonical string, both tests break — and the heartbeat
// stops authenticating.
func TestSignatureAPIV2SharedVector(t *testing.T) {
	body := `{"version":"0.4.0","hostname":"srv-01"}`
	bodyHash := sha256Hex(body)
	if bodyHash != "c29f4006321b97aa62b679328dfd1748817a3faf4128f1fb9b7ebfd79b44e4a4" {
		t.Fatalf("body hash=%s", bodyHash)
	}
	got := SignatureAPIV2("chave-hmac-do-agent-de-teste", "POST", "/api/agents/heartbeat", "",
		"1700000000", "0123456789abcdef0123456789abcdef", bodyHash)
	const want = "6c42448f3873b0dd0ffc8d62d7912f046db1d315963190b9687ede46f7206c78"
	if got != want {
		t.Fatalf("assinatura agent → API v2 divergente do backend:\n got=%s\nwant=%s", got, want)
	}
}

func TestCanonicalStringAPIV2Layout(t *testing.T) {
	got := CanonicalStringAPIV2("post", "/api/x", "a=1", "1", "n", "ABC")
	const want = "BACKIFY-AGENT-API-HMAC-SHA256-V2\nPOST\n/api/x\na=1\n1\nn\nabc"
	if got != want {
		t.Fatalf("canonical=%q want=%q", got, want)
	}
}

// The agent's secret signs both channels (agent → API and worker → helper).
// A signature from one must never be valid on the other, nor as protocol 1.
func TestAPIV2NeverCollidesWithHelperV2OrV1(t *testing.T) {
	h := sha256Hex("x")
	api := SignatureAPIV2("k", "POST", "/p", "", "1", "n", h)
	if api == SignatureV2("k", "POST", "/p", "", "1", "n", h) {
		t.Fatal("agent → API and helper signatures collide")
	}
	if api == Signature("k", "POST", "/p", "1", "n") {
		t.Fatal("agent → API v2 and v1 signatures collide")
	}
}

func TestRequestAPIV2SignsBodyPathAndQuery(t *testing.T) {
	body := []byte(`{"version":"0.4.0"}`)
	req := httptest.NewRequest(http.MethodPost, "https://srv.backify.app/api/agents/heartbeat?x=1", bytes.NewReader(body))
	if err := RequestAPIV2(req, "agent-1", "segredo", body); err != nil {
		t.Fatal(err)
	}
	ts, nonce := req.Header.Get(HeaderTimestamp), req.Header.Get(HeaderNonce)
	if req.Header.Get(HeaderAgentID) != "agent-1" || ts == "" || len(nonce) != 32 {
		t.Fatalf("headers incompletos: %v", req.Header)
	}
	if got := req.Header.Get(HeaderContentSHA256); got != sha256Hex(string(body)) {
		t.Fatalf("content hash=%s", got)
	}
	want := SignatureAPIV2("segredo", "POST", "/api/agents/heartbeat", "x=1", ts, nonce, sha256Hex(string(body)))
	if got := req.Header.Get(HeaderSignature); got != want {
		t.Fatalf("signature=%s want=%s", got, want)
	}

	// Another body → another signature (the body is in the HMAC).
	other := SignatureAPIV2("segredo", "POST", "/api/agents/heartbeat", "x=1", ts, nonce, sha256Hex(`{"version":"9.9.9"}`))
	if other == want {
		t.Fatal("signature does not depend on the body")
	}
}

func TestRequestAPIV2NilBodyIsEmptyHash(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://srv.backify.app/api/agents/config", nil)
	if err := RequestAPIV2(req, "a", "k", nil); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get(HeaderContentSHA256); got != EmptySHA256 {
		t.Fatalf("content hash=%s want empty-string digest", got)
	}
}
