package sign

import "testing"

// Vetor de referência gerado com o MESMO algoritmo do backend
// (apps/api/src/auth/agent.ts). Se este teste quebrar, o Bridge e o Backify
// deixaram de assinar igual — a auth do agent para de funcionar.
func TestSignatureMatchesBackendVector(t *testing.T) {
	got := Signature("s3cr3t-hmac-key", "POST", "/api/agents/heartbeat", "1700000000", "nonce-abc")
	const want = "46b571e5501aa5fad22ab04fbcc55a02b58926b00ddd00cd13b0d9f76ce5f5c2"
	if got != want {
		t.Fatalf("assinatura divergente do backend:\n got=%s\nwant=%s", got, want)
	}
}

func TestCanonicalStringUppercasesMethod(t *testing.T) {
	got := CanonicalString("post", "/x", "1", "n")
	const want = "POST\n/x\n1\nn"
	if got != want {
		t.Fatalf("canonical=%q want=%q", got, want)
	}
}

func TestNonceUniqueAndHexLength(t *testing.T) {
	a, err := Nonce()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Nonce()
	if a == b {
		t.Fatal("nonce repetido — não deveria")
	}
	if len(a) != 32 {
		t.Fatalf("nonce len=%d want 32 (16 bytes hex)", len(a))
	}
}
