package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/backifyapp/bridge/internal/sign"
)

const testSecret = "segredo-do-agent"

// fakeAPI verifies protocol 3 the way the backend does (requireAgent in
// apps/api/src/auth/agent.ts): X-Backify-Protocol: 3, signature with the API
// key derived from the secret over the declared hash, then the body actually
// read must match it. It records the body it accepted.
func fakeAPI(t *testing.T, accepted *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sign.HeaderProtocol, "3")
		claim := r.Header.Get(sign.HeaderContentSHA256)
		want := sign.SignatureAPIV3(sign.APIKey(testSecret), r.Method, r.URL.EscapedPath(), r.URL.RawQuery,
			r.Header.Get(sign.HeaderTimestamp), r.Header.Get(sign.HeaderNonce), claim)
		body, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(body)
		if claim == "" || r.Header.Get(sign.HeaderProtocol) != "3" || !hmac.Equal([]byte(want), []byte(r.Header.Get(sign.HeaderSignature))) ||
			hex.EncodeToString(sum[:]) != claim || r.Header.Get(sign.HeaderAgentID) != "agent-1" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHENTICATED","message":"Assinatura inválida"}}`))
			return
		}
		_ = json.Unmarshal(body, accepted)
		_, _ = w.Write([]byte(`{"data":{"tunnel":{"server":"https://tunnel.example","fingerprint":"fp"},"services":[{"type":"POSTGRES","localPort":5432,"remotePort":40001}]}}`))
	}))
}

func TestHeartbeatSignsWithProtocol3(t *testing.T) {
	var got map[string]any
	srv := fakeAPI(t, &got)
	defer srv.Close()

	cfg, err := New(srv.URL, "agent-1", testSecret).Heartbeat(context.Background(), "0.4.0", "srv-01", map[string]any{"os": "linux"})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if got["version"] != "0.4.0" || got["hostname"] != "srv-01" {
		t.Fatalf("API received %v", got)
	}
	if cfg.Tunnel.Server != "https://tunnel.example" || len(cfg.Services) != 1 || cfg.Services[0].RemotePort != 40001 {
		t.Fatalf("config=%+v", cfg)
	}
}

func TestHeartbeatWrongSecretIsAPIError(t *testing.T) {
	var got map[string]any
	srv := fakeAPI(t, &got)
	defer srv.Close()

	_, err := New(srv.URL, "agent-1", "outro-segredo").Heartbeat(context.Background(), "0.4.0", "h", nil)
	if err == nil || errors.Is(err, ErrAPIOutdated) || !strings.Contains(err.Error(), "UNAUTHENTICATED") {
		t.Fatalf("err=%v (want the API's 401, not ErrAPIOutdated)", err)
	}
}

// An API older than protocol 2 answers 401 without X-Backify-Protocol: the
// agent says so instead of looking like a wrong secret.
func TestHeartbeat401WithoutProtocolHeaderIsAPIOutdated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHENTICATED","message":"Assinatura inválida"}}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "agent-1", testSecret).Heartbeat(context.Background(), "0.4.0", "h", nil)
	if !errors.Is(err, ErrAPIOutdated) {
		t.Fatalf("err=%v want ErrAPIOutdated", err)
	}
}

// Enroll is not signed: its 401 is never read as "API outdated".
func TestEnroll401IsNotAPIOutdated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(sign.HeaderSignature) != "" {
			t.Error("enroll must not be signed")
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHENTICATED","message":"Token inválido"}}`))
	}))
	defer srv.Close()

	_, _, err := Enroll(context.Background(), srv.URL, "tok", "h", "0.4.0")
	if err == nil || errors.Is(err, ErrAPIOutdated) {
		t.Fatalf("err=%v", err)
	}
}

// The API before per-channel keys (protocol 2) answers X-Backify-Protocol: 2
// and refuses the protocol-3 signature: still "API outdated", not a wrong secret.
func TestHeartbeat401FromProtocol2APIIsAPIOutdated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sign.HeaderProtocol, "2")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHENTICATED","message":"Assinatura inválida"}}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "agent-1", testSecret).Heartbeat(context.Background(), "0.5.0", "h", nil)
	if !errors.Is(err, ErrAPIOutdated) {
		t.Fatalf("err=%v want ErrAPIOutdated", err)
	}
}

// The heartbeat must never be signed with the enroll secret itself.
func TestHeartbeatNeverSignsWithTheRootSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sign.HeaderProtocol, "3")
		claim := r.Header.Get(sign.HeaderContentSHA256)
		ts, nonce, got := r.Header.Get(sign.HeaderTimestamp), r.Header.Get(sign.HeaderNonce), r.Header.Get(sign.HeaderSignature)
		for _, k := range []string{testSecret, sign.HelperKey(testSecret), sign.ChiselPassword(testSecret)} {
			if got == sign.SignatureAPIV3(k, r.Method, r.URL.EscapedPath(), r.URL.RawQuery, ts, nonce, claim) ||
				got == sign.SignatureAPIV2(k, r.Method, r.URL.EscapedPath(), r.URL.RawQuery, ts, nonce, claim) {
				t.Error("heartbeat signed with a key other than the API one")
			}
		}
		_, _ = w.Write([]byte(`{"data":{"tunnel":{},"services":[]}}`))
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "agent-1", testSecret).Heartbeat(context.Background(), "0.5.0", "h", nil); err != nil {
		t.Fatal(err)
	}
}
