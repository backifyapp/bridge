// Package api talks to the Backify API: enroll (trading the enrollment token)
// and heartbeat (HMAC-signed). It honors the backend's { data } / { error }
// envelope.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/backifyapp/bridge/internal/sign"
)

// Service is a local service the agent must expose through the tunnel.
// RemotePort is the bind assigned by the control plane on the chisel-server
// (where the worker connects); it stays 0 while Backify hasn't provisioned the
// tunnel yet (phase 2).
type Service struct {
	Type       string `json:"type"`
	LocalPort  int    `json:"localPort"`
	RemotePort int    `json:"remotePort"`
}

// TunnelInfo says where and how the agent dials the tunnel. It arrives in the
// heartbeat (and may rotate). Fingerprint pins the chisel-server host key.
type TunnelInfo struct {
	Server      string `json:"server"`
	Fingerprint string `json:"fingerprint"`
}

// AgentConfig is the config returned by the heartbeat (tunnel + what to expose).
type AgentConfig struct {
	Tunnel   TunnelInfo `json:"tunnel"`
	Services []Service  `json:"services"`
}

// envelope espelha as respostas do backend (ok → data; fail → error).
type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Client is the HMAC-authenticated client (heartbeat and future calls).
type Client struct {
	baseURL string
	agentID string
	secret  string
	http    *http.Client
}

// New creates a client already carrying the agent's identity.
func New(baseURL, agentID, secret string) *Client {
	return &Client{
		baseURL: baseURL,
		agentID: agentID,
		secret:  secret,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// do performs the request and unmarshals `data` into out (or returns the API error).
func do(hc *http.Client, req *http.Request, out any) error {
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("invalid response (HTTP %d): %s", resp.StatusCode, string(body))
	}
	if env.Error != nil {
		return fmt.Errorf("API %d %s: %s", resp.StatusCode, env.Error.Code, env.Error.Message)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("API status %d", resp.StatusCode)
	}
	if out != nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

// Enroll trades the single-use enrollment token for machine credentials. It is
// not signed (the token itself is the proof). It returns the HMAC secret once.
func Enroll(ctx context.Context, baseURL, token, hostname, version string) (agentID, secret string, err error) {
	payload, _ := json.Marshal(map[string]string{
		"enrollmentToken": token,
		"hostname":        hostname,
		"version":         version,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/agents/enroll", bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	var res struct {
		AgentID    string `json:"agentId"`
		HMACSecret string `json:"hmacSecret"`
	}
	if err := do(&http.Client{Timeout: 30 * time.Second}, req, &res); err != nil {
		return "", "", err
	}
	return res.AgentID, res.HMACSecret, nil
}

// Heartbeat reports state (version/hostname + inventory) and receives the config to apply.
func (c *Client) Heartbeat(ctx context.Context, version, hostname string, systemInfo any) (*AgentConfig, error) {
	payload, _ := json.Marshal(map[string]any{"version": version, "hostname": hostname, "systemInfo": systemInfo})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/agents/heartbeat", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := sign.Request(req, c.agentID, c.secret); err != nil {
		return nil, err
	}

	var cfg AgentConfig
	if err := do(c.http, req, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
