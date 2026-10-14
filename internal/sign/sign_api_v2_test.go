package sign

import "testing"

// Agent → API protocol-2 vector shared with the backend (prhost/backify,
// apps/api/src/auth/agent.test.ts, "vetor compartilhado com o Go"). The Bridge
// no longer signs with protocol 2 (0.5.0+ uses protocol 3), but the API still
// accepts it during the transition, so the vector stays pinned on both sides.
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

// Protocol 2 used the same secret on both channels: a signature from one must
// never be valid on the other, nor as protocol 1.
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
