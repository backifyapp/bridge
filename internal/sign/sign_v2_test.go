package sign

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Protocol 2 vector shared with the worker (prhost/backify,
// apps/worker/src/engine/bridge-helper-sign.test.ts). If either side changes the
// canonical string, both tests break.
func TestSignatureV2SharedVector(t *testing.T) {
	got := SignatureV2("chave-hmac-do-agent-de-teste", "POST", "/docker/volume/pgdata/import", "",
		"1700000000", "0123456789abcdef0123456789abcdef", sha256Hex("conteudo"))
	const want = "1f90de9a3cd2b99cbf50a0e89c38814312323e1fd8d1cd5425c35573d08a6cfe"
	if got != want {
		t.Fatalf("assinatura v2 divergente do worker:\n got=%s\nwant=%s", got, want)
	}
}

func TestCanonicalStringV2Layout(t *testing.T) {
	got := CanonicalStringV2("post", "/docker/x", "a=1", "1", "n", "ABC")
	const want = "BACKIFY-BRIDGE-HMAC-SHA256-V2\nPOST\n/docker/x\na=1\n1\nn\nabc"
	if got != want {
		t.Fatalf("canonical=%q want=%q", got, want)
	}
	if EmptySHA256 != sha256Hex("") {
		t.Fatal("EmptySHA256 is not the digest of the empty string")
	}
}

// A v2 string can never equal a v1 string (same secret signs both): the
// scheme line comes first.
func TestV2NeverCollidesWithV1(t *testing.T) {
	if Signature("k", "GET", "/docker/ping", "1", "n") == SignatureV2("k", "GET", "/docker/ping", "", "1", "n", EmptySHA256) {
		t.Fatal("v1 and v2 signatures collide")
	}
}
