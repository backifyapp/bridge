package transport

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	chclient "github.com/jpillora/chisel/client"

	"github.com/backifyapp/bridge/internal/api"
)

// ChiselTransport mantém um client Chisel (TCP sobre HTTPS, cripto SSH) que abre
// túneis reversos: o chisel-server do Backify passa a alcançar os serviços locais
// do cliente. A config de conexão (server/fingerprint) vem no heartbeat e pode
// rotacionar; a auth reusa a identidade do agent (agentID:secret), validada no
// server por um plugin contra a API do Backify (nenhum segredo novo).
type ChiselTransport struct {
	agentID string
	secret  string

	mu      sync.Mutex
	client  *chclient.Client
	current string // server+fingerprint+remotes atuais (detecta mudança)
}

// NewChisel cria o transporte com a identidade do agent (pra auth no túnel).
func NewChisel(agentID, secret string) *ChiselTransport {
	return &ChiselTransport{agentID: agentID, secret: secret}
}

// buildRemotes traduz os serviços autorizados em remotes reversos do Chisel:
//
//	R:<bind no server>:<alvo no cliente>
//
// O bind (RemotePort) é atribuído pelo control plane; o worker conecta nele.
// Serviços sem porta atribuída ainda são ignorados.
func buildRemotes(services []api.Service) []string {
	remotes := make([]string, 0, len(services))
	for _, s := range services {
		if s.RemotePort == 0 || s.LocalPort == 0 {
			continue
		}
		// Bind em 0.0.0.0 no chisel-server (não loopback) — senão a porta reversa
		// fica inalcançável fora do container (worker / publish de portas).
		remotes = append(remotes, fmt.Sprintf("R:0.0.0.0:%d:127.0.0.1:%d", s.RemotePort, s.LocalPort))
	}
	return remotes
}

// Sync (re)conecta o client Chisel quando a config muda; caso contrário, no-op.
func (t *ChiselTransport) Sync(ctx context.Context, cfg *api.AgentConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	remotes := buildRemotes(cfg.Services)
	key := cfg.Tunnel.Server + "|" + cfg.Tunnel.Fingerprint + "|" + strings.Join(remotes, ",")
	if key == t.current && t.client != nil {
		return nil // nada mudou
	}

	// Chisel não troca remotes ao vivo — reinicia o client com a nova config.
	if t.client != nil {
		_ = t.client.Close()
		t.client = nil
	}
	t.current = key

	// Sem servidor informado ou sem nada autorizado ainda: espera o próximo
	// heartbeat (control plane provisiona o túnel na Fase 2).
	if cfg.Tunnel.Server == "" || len(remotes) == 0 {
		return nil
	}

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
	// Start conecta e mantém o túnel vivo em background (com retry); não bloqueia.
	if err := cl.Start(ctx); err != nil {
		return err
	}
	t.client = cl
	return nil
}

// Close derruba o túnel.
func (t *ChiselTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client != nil {
		return t.client.Close()
	}
	return nil
}
