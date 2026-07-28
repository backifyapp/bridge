package transport

import (
	"testing"

	"github.com/backifyapp/bridge/internal/api"
)

func TestBuildRemotesFormatsAndSkips(t *testing.T) {
	got := buildRemotes([]api.Service{
		{Type: "POSTGRES", LocalPort: 5432, RemotePort: 40001},
		{Type: "SSH", LocalPort: 22, RemotePort: 0},      // no bind assigned → skipped
		{Type: "REDIS", LocalPort: 0, RemotePort: 40002}, // sem porta local → ignorado
	})
	if len(got) != 1 {
		t.Fatalf("esperava 1 remote, veio %d: %v", len(got), got)
	}
	const want = "R:0.0.0.0:40001:127.0.0.1:5432"
	if got[0] != want {
		t.Fatalf("remote=%q want=%q", got[0], want)
	}
}

// A half-set CHISEL_TUNNEL_SERVER on the control plane used to reach the chisel
// client as "wss://", which only failed there with "address wss::80: too many
// colons in address" — after retrying forever. Reject it up front.
func TestValidateServer(t *testing.T) {
	valid := []string{
		"https://tunnel.backify.app",
		"wss://tunnel.backify.app:443",
		"https://tunnel.backify.app",
		"tunnel.backify.app:8080",
		"chisel:8080",
	}
	for _, s := range valid {
		if err := validateServer(s); err != nil {
			t.Errorf("validateServer(%q) = %v, want nil", s, err)
		}
	}
	invalid := []string{"https://", "://"}
	for _, s := range invalid {
		if err := validateServer(s); err == nil {
			t.Errorf("validateServer(%q) = nil, want error", s)
		}
	}
}

// Regression: chisel applies its default scheme with HasPrefix(server, "http"),
// so it does not recognise "wss://host" as a URL at all — it prepends its own
// scheme and ends up dialing "wss::80" forever. The control plane published
// wss:// (that is what the docs said), so normalise it here.
func TestNormalizeServer(t *testing.T) {
	cases := map[string]string{
		"wss://tunnel.backify.app":   "https://tunnel.backify.app",
		"ws://chisel:8080":           "http://chisel:8080",
		"https://tunnel.backify.app": "https://tunnel.backify.app",
		"http://chisel:8080":         "http://chisel:8080",
		"chisel:8080":                "chisel:8080",
	}
	for in, want := range cases {
		if got := normalizeServer(in); got != want {
			t.Errorf("normalizeServer(%q) = %q, want %q", in, got, want)
		}
	}
}
