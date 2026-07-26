// Package agent roda o loop do daemon: heartbeat periódico no Backify e, a cada
// resposta, aplica a config no transport (quais serviços expor). Mantém o túnel
// vivo — o AGENDAMENTO dos backups é do Backify, não do agent.
package agent

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/backifyapp/bridge/internal/api"
	"github.com/backifyapp/bridge/internal/config"
	"github.com/backifyapp/bridge/internal/transport"
)

const heartbeatInterval = 30 * time.Second

// ErrNotEnrolled é devolvido quando o config não tem credenciais (rode `enroll`).
var ErrNotEnrolled = errors.New("agent não registrado — rode `backify-bridge enroll` primeiro")

// Run executa o loop até o contexto ser cancelado (SIGINT/SIGTERM).
func Run(ctx context.Context, cfg *config.Config, version string, t transport.Transport) error {
	if !cfg.Enrolled() {
		return ErrNotEnrolled
	}
	client := api.New(cfg.APIURL, cfg.AgentID, cfg.HMACSecret)
	hostname, _ := os.Hostname()

	tick := time.NewTicker(heartbeatInterval)
	defer tick.Stop()

	for {
		// Heartbeat imediato na 1ª volta, depois no intervalo.
		if acfg, err := client.Heartbeat(ctx, version, hostname); err != nil {
			log.Printf("[agent] heartbeat falhou: %v", err)
		} else if err := t.Sync(ctx, acfg); err != nil {
			log.Printf("[agent] sync do transport falhou: %v", err)
		}

		select {
		case <-ctx.Done():
			return t.Close()
		case <-tick.C:
		}
	}
}
