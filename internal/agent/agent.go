// Package agent roda o loop do daemon: heartbeat periódico no Backify e, a cada
// resposta, aplica a config no transport (quais serviços expor). Mantém o túnel
// vivo — o AGENDAMENTO dos backups é do Backify, não do agent.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/backifyapp/bridge/internal/api"
	"github.com/backifyapp/bridge/internal/config"
	"github.com/backifyapp/bridge/internal/docker"
	"github.com/backifyapp/bridge/internal/dockerhttp"
	"github.com/backifyapp/bridge/internal/sysinfo"
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

	dockerHelperUp := false
	for {
		// Inventário de frota (barato); versão do Docker só se estiver em modo docker.
		si := sysinfo.Collect()
		if dockerHelperUp {
			si.DockerVersion = docker.Version(ctx)
		}
		// Heartbeat imediato na 1ª volta, depois no intervalo.
		if acfg, err := client.Heartbeat(ctx, version, hostname, si); err != nil {
			log.Printf("[agent] heartbeat falhou: %v", err)
		} else {
			if err := t.Sync(ctx, acfg); err != nil {
				log.Printf("[agent] sync do transport falhou: %v", err)
			}
			// Capability Docker: sobe o helper HTTP local (uma vez) na porta do serviço.
			if !dockerHelperUp {
				if svc := findDockerService(acfg.Services); svc != nil {
					dockerHelperUp = true
					addr := fmt.Sprintf("127.0.0.1:%d", svc.LocalPort)
					go func() {
						log.Printf("[agent] helper docker ouvindo em %s", addr)
						if err := dockerhttp.Serve(ctx, addr, cfg.HMACSecret); err != nil {
							log.Printf("[agent] helper docker parou: %v", err)
						}
					}()
				}
			}
		}

		select {
		case <-ctx.Done():
			return t.Close()
		case <-tick.C:
		}
	}
}

// findDockerService devolve o serviço com a capability DOCKER, se houver.
func findDockerService(services []api.Service) *api.Service {
	for i := range services {
		if services[i].Type == "DOCKER" {
			return &services[i]
		}
	}
	return nil
}
