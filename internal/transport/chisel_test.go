package transport

import (
	"testing"

	"github.com/backifyapp/bridge/internal/api"
)

func TestBuildRemotesFormatsAndSkips(t *testing.T) {
	got := buildRemotes([]api.Service{
		{Type: "POSTGRES", LocalPort: 5432, RemotePort: 40001},
		{Type: "SSH", LocalPort: 22, RemotePort: 0}, // sem bind atribuído → ignorado
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
