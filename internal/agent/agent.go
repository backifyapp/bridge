// Package agent runs the daemon loop: a periodic heartbeat to Backify and, on
// each response, applies the config to the transport (which services to expose).
// It keeps the tunnel alive — backup SCHEDULING belongs to Backify, not to the
// agent.
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

// ErrNotEnrolled is returned when the config has no credentials (run `enroll`).
var ErrNotEnrolled = errors.New("agent not enrolled — run `backify-bridge enroll` first")

// Run executes the loop until the context is canceled (SIGINT/SIGTERM).
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
		// Fleet inventory (cheap); Docker version only when in docker mode.
		si := sysinfo.Collect()
		if dockerHelperUp {
			si.DockerVersion = docker.Version(ctx)
		}
		// Heartbeat imediato na 1ª volta, depois no intervalo.
		if acfg, err := client.Heartbeat(ctx, version, hostname, si); err != nil {
			log.Printf("[agent] heartbeat falhou: %v", err)
		} else {
			if err := t.Sync(ctx, acfg); err != nil {
				log.Printf("[agent] transport sync failed: %v", err)
			}
			// Docker capability: starts the local HTTP helper (once) on the service port.
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

// findDockerService returns the service with the DOCKER capability, if any.
func findDockerService(services []api.Service) *api.Service {
	for i := range services {
		if services[i].Type == "DOCKER" {
			return &services[i]
		}
	}
	return nil
}
