// Package config lê e grava a identidade local do Bridge (URL da API + id do
// agent + segredo HMAC). Diferente do modelo "config-file de backup": aqui o
// arquivo só guarda a IDENTIDADE do agent — O QUE expor é decidido pelo Backify
// e vem no heartbeat. O segredo fica em repouso com permissão 0600.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const defaultPath = "/etc/backify-bridge/bridge.json"

// Config é a identidade persistida do agent.
type Config struct {
	APIURL     string `json:"apiUrl"`
	AgentID    string `json:"agentId"`
	HMACSecret string `json:"hmacSecret"`
}

// Path devolve o caminho do config (override por BACKIFY_BRIDGE_CONFIG).
func Path() string {
	if p := os.Getenv("BACKIFY_BRIDGE_CONFIG"); p != "" {
		return p
	}
	return defaultPath
}

// Enrolled indica se o agent já trocou o enrollment token pelas credenciais.
func (c *Config) Enrolled() bool {
	return c.AgentID != "" && c.HMACSecret != ""
}

// Load lê o config do disco.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save grava o config com permissão 0600 (segredo em repouso) e diretório 0700.
func Save(path string, c *Config) error {
	if path == "" {
		return errors.New("caminho de config vazio")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
