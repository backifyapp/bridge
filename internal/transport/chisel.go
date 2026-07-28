package transport

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	chclient "github.com/jpillora/chisel/client"

	"github.com/backifyapp/bridge/internal/api"
)

// ChiselTransport keeps a Chisel client (TCP over HTTPS, SSH crypto) that opens
// reverse tunnels: Backify's chisel-server can then reach the customer's local
// services. The connection config (server/fingerprint) arrives in the heartbeat
// and may rotate; auth reuses the agent identity (agentID:secret), validated on
// the server by a plugin against the Backify API (no new secret).
type ChiselTransport struct {
	agentID string
	secret  string

	mu      sync.Mutex
	client  *chclient.Client
	current string // current server+fingerprint+remotes (detects changes)
}

// NewChisel creates the transport with the agent identity (for tunnel auth).
func NewChisel(agentID, secret string) *ChiselTransport {
	return &ChiselTransport{agentID: agentID, secret: secret}
}

// buildRemotes translates the authorized services into Chisel reverse remotes:
//
//	R:<bind on server>:<target on client>
//
// The bind (RemotePort) is assigned by the control plane; the worker connects to
// it. Services without an assigned port are skipped.
func buildRemotes(services []api.Service) []string {
	remotes := make([]string, 0, len(services))
	for _, s := range services {
		if s.RemotePort == 0 || s.LocalPort == 0 {
			continue
		}
		// Bind on 0.0.0.0 on the chisel-server (not loopback) — otherwise the reverse
		// port is unreachable outside the container (worker / published ports).
		remotes = append(remotes, fmt.Sprintf("R:0.0.0.0:%d:127.0.0.1:%d", s.RemotePort, s.LocalPort))
	}
	return remotes
}

// validateServer rejects addresses that would only blow up inside the chisel
// client — most importantly a scheme with no host ("wss://"), which the control
// plane can hand out when its own env var is half-set.
func validateServer(server string) error {
	s := server
	if !strings.Contains(s, "://") {
		s = "wss://" + s // chisel accepts a bare host:port
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if u.Hostname() == "" {
		return fmt.Errorf("no host")
	}
	return nil
}

// Sync (re)connects the Chisel client when the config changes; otherwise it's a no-op.
func (t *ChiselTransport) Sync(ctx context.Context, cfg *api.AgentConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	remotes := buildRemotes(cfg.Services)
	key := cfg.Tunnel.Server + "|" + cfg.Tunnel.Fingerprint + "|" + strings.Join(remotes, ",")
	if key == t.current && t.client != nil {
		return nil // nada mudou
	}

	// Chisel can't swap remotes live — restart the client with the new config.
	if t.client != nil {
		_ = t.client.Close()
		t.client = nil
	}
	// While idle, key stays equal but client is nil, so Sync re-runs on every
	// heartbeat — only log when something actually changed, not every 30s.
	changed := key != t.current
	t.current = key

	// No server given, or nothing authorized yet: wait for the next heartbeat.
	// This used to be silent, which made a misconfigured control plane look
	// exactly like a healthy idle agent — say what is missing.
	if cfg.Tunnel.Server == "" {
		if changed {
			log.Printf("[tunnel] idle: the control plane sent no tunnel address (check CHISEL_TUNNEL_SERVER on the API)")
		}
		return nil
	}
	if len(remotes) == 0 {
		if changed {
			log.Printf("[tunnel] idle: no service has a remote port assigned yet (%d authorized)", len(cfg.Services))
		}
		return nil
	}
	// A malformed address fails deep inside the chisel client with a cryptic
	// message ("address wss::80: too many colons"). Catch it here instead.
	if err := validateServer(cfg.Tunnel.Server); err != nil {
		return fmt.Errorf("invalid tunnel address %q: %w", cfg.Tunnel.Server, err)
	}
	log.Printf("[tunnel] connecting to %s (%d remotes)", cfg.Tunnel.Server, len(remotes))

	cl, err := chclient.NewClient(&chclient.Config{
		Server:           cfg.Tunnel.Server,
		Fingerprint:      cfg.Tunnel.Fingerprint,
		Auth:             t.agentID + ":" + t.secret,
		Remotes:          remotes,
		KeepAlive:        25 * time.Second,
		MaxRetryCount:    -1,
		MaxRetryInterval: 30 * time.Second,
	})
	if err != nil {
		return err
	}
	// Start connects and keeps the tunnel alive in the background (with retry); non-blocking.
	if err := cl.Start(ctx); err != nil {
		return err
	}
	t.client = cl
	return nil
}

// Close tears the tunnel down.
func (t *ChiselTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client != nil {
		return t.client.Close()
	}
	return nil
}
