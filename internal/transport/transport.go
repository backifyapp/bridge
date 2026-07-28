// Package transport keeps the reverse tunnels that expose the customer's local
// services (localhost:5432, localhost:22…) to the Backify worker.
//
// The v1 implementation is Chisel (see chisel.go), embedded as a client. It sits
// BEHIND this interface on purpose: swapping in FRP later is just a new
// implementation, with no changes to the daemon. See README > Transport.
package transport

import (
	"context"
	"log"

	"github.com/backifyapp/bridge/internal/api"
)

// Transport adjusts the tunnels to expose exactly what Backify authorized.
// Sync must be idempotent (called on every heartbeat) and must not block.
type Transport interface {
	Sync(ctx context.Context, cfg *api.AgentConfig) error
	Close() error
}

// Stub only logs what it would expose — handy to run enroll → heartbeat end to
// end without a chisel-server (BACKIFY_BRIDGE_STUB=1).
type Stub struct{}

// Sync just logs the services that would be exposed.
func (Stub) Sync(_ context.Context, cfg *api.AgentConfig) error {
	for _, s := range cfg.Services {
		log.Printf("[transport:stub] exporia %s localhost:%d (bind remoto :%d) via %s",
			s.Type, s.LocalPort, s.RemotePort, cfg.Tunnel.Server)
	}
	return nil
}

// Close is a no-op in the stub.
func (Stub) Close() error { return nil }
