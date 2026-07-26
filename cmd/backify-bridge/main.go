// Command backify-bridge — Backify Bridge: broker de acesso via túnel reverso.
//
// Instalado no servidor Linux do cliente, disca pra fora (443/TLS), mantém um
// túnel reverso e expõe SÓ os serviços locais que o Backify autorizou. Não faz
// backup: o worker do Backify roda os dumps ALCANÇANDO os serviços pelo túnel.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/backifyapp/bridge/internal/agent"
	"github.com/backifyapp/bridge/internal/api"
	"github.com/backifyapp/bridge/internal/config"
	"github.com/backifyapp/bridge/internal/transport"
)

// version é injetada no build: -ldflags "-X main.version=v1.2.3".
var version = "dev"

const defaultAPIURL = "https://api.backify.app"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "enroll":
		cmdEnroll(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("backify-bridge", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `backify-bridge — Backify Bridge (broker de acesso via túnel reverso)

Uso:
  backify-bridge enroll --token <TOKEN> [--url <API_URL>]   registra este servidor no Backify
  backify-bridge run                                        roda o daemon (mantém o túnel)
  backify-bridge status                                     mostra o estado do registro
  backify-bridge version                                    versão do binário
`)
}

func cmdEnroll(args []string) {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	token := fs.String("token", "", "enrollment token gerado no painel (obrigatório)")
	url := fs.String("url", defaultAPIURL, "URL da API do Backify")
	_ = fs.Parse(args)

	if *token == "" {
		fmt.Fprintln(os.Stderr, "erro: --token é obrigatório")
		os.Exit(2)
	}

	hostname, _ := os.Hostname()
	agentID, secret, err := api.Enroll(context.Background(), *url, *token, hostname, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "enroll falhou:", err)
		os.Exit(1)
	}

	cfg := &config.Config{APIURL: *url, AgentID: agentID, HMACSecret: secret}
	if err := config.Save(config.Path(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, "erro ao salvar o config:", err)
		os.Exit(1)
	}
	fmt.Printf("Registrado com sucesso.\n  agentId: %s\n  config:  %s\nAgora rode: backify-bridge run\n", agentID, config.Path())
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg, err := config.Load(config.Path())
	if err != nil {
		fmt.Fprintln(os.Stderr, "não foi possível ler o config (rode `enroll` primeiro):", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Túnel real via Chisel; BACKIFY_BRIDGE_STUB=1 usa o stub (dev sem servidor).
	var t transport.Transport = transport.NewChisel(cfg.AgentID, cfg.HMACSecret)
	if os.Getenv("BACKIFY_BRIDGE_STUB") == "1" {
		t = transport.Stub{}
	}
	if err := agent.Run(ctx, cfg, version, t); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "agent parou:", err)
		os.Exit(1)
	}
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg, err := config.Load(config.Path())
	if err != nil {
		fmt.Println("status: NÃO configurado (rode `backify-bridge enroll`)")
		return
	}
	fmt.Printf("status:  %s\nagentId: %s\napi:     %s\nconfig:  %s\n",
		enrolledLabel(cfg.Enrolled()), cfg.AgentID, cfg.APIURL, config.Path())
}

func enrolledLabel(b bool) string {
	if b {
		return "ENROLLED"
	}
	return "PENDENTE"
}
