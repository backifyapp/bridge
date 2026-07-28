// Package config reads and writes the Bridge's local identity (API URL + agent
// agent + segredo HMAC). Diferente do modelo "config-file de backup": aqui o
// file only holds the agent's IDENTITY — WHAT to expose is decided by Backify
// and arrives in the heartbeat. The secret rests with mode 0600.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const defaultPath = "/etc/backify-bridge/bridge.json"

// Config is the agent's persisted identity.
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

// Enrolled reports whether the agent already traded the enrollment token for credentials.
func (c *Config) Enrolled() bool {
	return c.AgentID != "" && c.HMACSecret != ""
}

// Load reads the config from disk.
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

// Save writes the config with mode 0600 (secret at rest) and directory 0700.
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
