// Package transport mantém os túneis reversos que expõem os serviços locais do
// cliente (localhost:5432, localhost:22…) pro worker do Backify.
//
// A implementação v1 é Chisel (ver chisel.go), embutida como client. Ela fica
// ATRÁS desta interface de propósito: trocar por FRP depois é só uma nova
// implementação, sem mexer no daemon. Ver README > Transporte.
package transport

import (
	"context"
	"log"

	"github.com/backifyapp/bridge/internal/api"
)

// Transport ajusta os túneis para expor exatamente o que o Backify autorizou.
// Sync deve ser idempotente (chamado a cada heartbeat) e não bloquear.
type Transport interface {
	Sync(ctx context.Context, cfg *api.AgentConfig) error
	Close() error
}

// Stub só registra o que exporia — útil pra rodar enroll → heartbeat ponta a
// ponta sem um chisel-server (BACKIFY_BRIDGE_STUB=1).
type Stub struct{}

// Sync apenas loga os serviços que seriam expostos.
func (Stub) Sync(_ context.Context, cfg *api.AgentConfig) error {
	for _, s := range cfg.Services {
		log.Printf("[transport:stub] exporia %s localhost:%d (bind remoto :%d) via %s",
			s.Type, s.LocalPort, s.RemotePort, cfg.Tunnel.Server)
	}
	return nil
}

// Close não faz nada no stub.
func (Stub) Close() error { return nil }
