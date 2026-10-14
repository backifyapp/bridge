package sign

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

const vectorSecret = "chave-hmac-do-agent-de-teste"

// Per-channel keys shared with the backend (prhost/backify,
// apps/api/src/auth/agent-keys.test.ts, "vetor compartilhado com o Go"). If
// either side changes the derivation, the tunnel, the heartbeat and the Docker
// helper all stop authenticating at once.
func TestChannelKeysSharedVector(t *testing.T) {
	for name, c := range map[string]struct{ got, want string }{
		"chisel": {ChiselPassword(vectorSecret), "4946d480dabb80c77898bc6ec0a0eacc0048429414070fa2bd4db5d058b44ab8"},
		"api":    {APIKey(vectorSecret), "204748ffb075b193f2f184b1806963a15d71f2413fc9d54384f6ef1cb3abeabb"},
		"helper": {HelperKey(vectorSecret), "5a977b40d83eee303488f4cc0692fca50ba4319d1426009d84dd20cbe4c58e69"},
	} {
		if c.got != c.want {
			t.Errorf("%s key diverges from the backend:\n got=%s\nwant=%s", name, c.got, c.want)
		}
	}
}

// Each channel key is distinct from the others and from the root: leaking one
// (users.json holds the chisel one in clear) must not hand over another.
func TestChannelKeysAreIndependent(t *testing.T) {
	keys := []string{vectorSecret, ChiselPassword(vectorSecret), APIKey(vectorSecret), HelperKey(vectorSecret)}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] == keys[j] {
				t.Fatalf("keys %d and %d are equal", i, j)
			}
		}
	}
	if ChiselPassword("outro") == ChiselPassword(vectorSecret) {
		t.Fatal("the derivation ignores the secret")
	}
}

// Agent → API protocol-3 vector shared with apps/api/src/auth/agent.test.ts.
func TestSignatureAPIV3SharedVector(t *testing.T) {
	bodyHash := sha256Hex(`{"version":"0.5.0","hostname":"srv-01"}`)
	got := SignatureAPIV3(APIKey(vectorSecret), "POST", "/api/agents/heartbeat", "",
		"1700000000", "0123456789abcdef0123456789abcdef", bodyHash)
	const want = "685f1e0f0ccf5374338c4c481ed8dd8ac47dc3a799596f94bd5ead60cd1de406"
	if got != want {
		t.Fatalf("agent → API v3 signature diverges from the backend:\n got=%s\nwant=%s", got, want)
	}
}

// Worker → helper protocol-3 vector shared with
// apps/worker/src/engine/bridge-helper-sign.test.ts.
func TestSignatureV3SharedVector(t *testing.T) {
	got := SignatureV3(HelperKey(vectorSecret), "POST", "/docker/volume/pgdata/import", "",
		"1700000000", "0123456789abcdef0123456789abcdef", sha256Hex("tar-bytes"))
	const want = "00bc7de5c50f7cfe4f8bc46f32a67af10378cb5d9d76bb9f19bfe4427c2c6150"
	if got != want {
		t.Fatalf("helper v3 signature diverges from the worker:\n got=%s\nwant=%s", got, want)
	}
}

func TestCanonicalStringV3Layouts(t *testing.T) {
	if got := CanonicalStringAPIV3("post", "/api/x", "a=1", "1", "n", "ABC"); got != "BACKIFY-AGENT-API-HMAC-SHA256-V3\nPOST\n/api/x\na=1\n1\nn\nabc" {
		t.Fatalf("api canonical=%q", got)
	}
	if got := CanonicalStringV3("get", "/docker/ping", "", "1", "n", EmptySHA256); got != "BACKIFY-BRIDGE-HMAC-SHA256-V3\nGET\n/docker/ping\n\n1\nn\n"+EmptySHA256 {
		t.Fatalf("helper canonical=%q", got)
	}
}

// Same key, same request: no protocol's signature is valid on another one.
func TestV3NeverCollidesWithOtherSchemes(t *testing.T) {
	h := sha256Hex("x")
	sigs := map[string]string{
		"apiV3":    SignatureAPIV3("k", "POST", "/p", "", "1", "n", h),
		"helperV3": SignatureV3("k", "POST", "/p", "", "1", "n", h),
		"apiV2":    SignatureAPIV2("k", "POST", "/p", "", "1", "n", h),
		"helperV2": SignatureV2("k", "POST", "/p", "", "1", "n", h),
		"v1":       Signature("k", "POST", "/p", "1", "n"),
	}
	seen := map[string]string{}
	for name, s := range sigs {
		if other, dup := seen[s]; dup {
			t.Fatalf("%s and %s collide", name, other)
		}
		seen[s] = name
	}
}

func TestRequestAPIV3SignsBodyPathAndQueryWithTheAPIKey(t *testing.T) {
	body := []byte(`{"version":"0.5.0"}`)
	req := httptest.NewRequest(http.MethodPost, "https://srv.backify.app/api/agents/heartbeat?x=1", bytes.NewReader(body))
	if err := RequestAPIV3(req, "agent-1", APIKey("segredo"), body); err != nil {
		t.Fatal(err)
	}
	ts, nonce := req.Header.Get(HeaderTimestamp), req.Header.Get(HeaderNonce)
	if req.Header.Get(HeaderAgentID) != "agent-1" || ts == "" || len(nonce) != 32 {
		t.Fatalf("incomplete headers: %v", req.Header)
	}
	if req.Header.Get(HeaderProtocol) != "3" {
		t.Fatalf("%s=%q want 3", HeaderProtocol, req.Header.Get(HeaderProtocol))
	}
	bodyHash := sha256Hex(string(body))
	if got := req.Header.Get(HeaderContentSHA256); got != bodyHash {
		t.Fatalf("content hash=%s", got)
	}
	want := SignatureAPIV3(APIKey("segredo"), "POST", "/api/agents/heartbeat", "x=1", ts, nonce, bodyHash)
	if got := req.Header.Get(HeaderSignature); got != want {
		t.Fatalf("signature=%s want=%s", got, want)
	}
	other := SignatureAPIV3(APIKey("segredo"), "POST", "/api/agents/heartbeat", "x=1", ts, nonce, sha256Hex(`{"version":"9.9.9"}`))
	if other == want {
		t.Fatal("signature does not depend on the body")
	}
}

func TestRequestAPIV3NilBodyIsEmptyHash(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://srv.backify.app/api/agents/config", nil)
	if err := RequestAPIV3(req, "a", "k", nil); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get(HeaderContentSHA256); got != EmptySHA256 {
		t.Fatalf("content hash=%s want empty-string digest", got)
	}
}
