// Command backify-bridge — Backify Bridge: reverse-tunnel access broker.
//
// Installed on the customer's Linux server, it dials outbound (443/TLS), keeps a
// reverse tunnel up and exposes ONLY the local services Backify authorized. It
// does not run backups: the Backify worker runs the dumps, REACHING the services
// through the tunnel.
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
	"github.com/backifyapp/bridge/internal/updater"
)

// version is injected at build time: -ldflags "-X main.version=v1.2.3".
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
	case "update":
		cmdUpdate()
	case "version", "-v", "--version":
		fmt.Println("backify-bridge", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `backify-bridge — Backify Bridge (reverse-tunnel access broker)

Usage:
  backify-bridge enroll --token <TOKEN> [--url <API_URL>]   enroll this server with Backify
  backify-bridge run                                        run the daemon (keeps the tunnel up)
  backify-bridge status                                     show the enrollment state
  backify-bridge update                                     update to the latest version
  backify-bridge version                                    binary version
`)
}

func cmdEnroll(args []string) {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	token := fs.String("token", "", "enrollment token generated in the dashboard (required)")
	url := fs.String("url", defaultAPIURL, "Backify API URL")
	_ = fs.Parse(args)

	if *token == "" {
		fmt.Fprintln(os.Stderr, "error: --token is required")
		os.Exit(2)
	}

	hostname, _ := os.Hostname()
	agentID, secret, err := api.Enroll(context.Background(), *url, *token, hostname, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "enroll failed:", err)
		os.Exit(1)
	}

	cfg := &config.Config{APIURL: *url, AgentID: agentID, HMACSecret: secret}
	if err := config.Save(config.Path(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, "failed to save the config:", err)
		os.Exit(1)
	}
	fmt.Printf("Enrolled successfully.\n  agentId: %s\n  config:  %s\nNow run: backify-bridge run\n", agentID, config.Path())
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg, err := config.Load(config.Path())
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not read the config (run `enroll` first):", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Real tunnel over Chisel; BACKIFY_BRIDGE_STUB=1 uses the stub (dev without a server).
	var t transport.Transport = transport.NewChisel(cfg.AgentID, cfg.HMACSecret)
	if os.Getenv("BACKIFY_BRIDGE_STUB") == "1" {
		t = transport.Stub{}
	}
	if err := agent.Run(ctx, cfg, version, t); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "agent stopped:", err)
		os.Exit(1)
	}
}

func cmdUpdate() {
	tag, updated, err := updater.Run(version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "update failed:", err)
		os.Exit(1)
	}
	if !updated {
		fmt.Printf("Already on the latest version (%s).\n", tag)
		return
	}
	fmt.Printf("Updated to %s. Restart the service:\n  sudo systemctl restart backify-bridge\n", tag)
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg, err := config.Load(config.Path())
	if err != nil {
		fmt.Println("status: NOT configured (run `backify-bridge enroll`)")
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
