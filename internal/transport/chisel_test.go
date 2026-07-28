package transport

import (
	"testing"

	"github.com/backifyapp/bridge/internal/api"
)

func TestBuildRemotesFormatsAndSkips(t *testing.T) {
	got := buildRemotes([]api.Service{
		{Type: "POSTGRES", LocalPort: 5432, RemotePort: 40001},
		{Type: "SSH", LocalPort: 22, RemotePort: 0}, // no bind assigned → skipped
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
		"wss://tunnel.backify.app",
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
	invalid := []string{"wss://", "https://", "://"}
	for _, s := range invalid {
		if err := validateServer(s); err == nil {
			t.Errorf("validateServer(%q) = nil, want error", s)
		}
	}
}
