// Package api fala com a API do Backify: enroll (troca do enrollment token) e
// heartbeat (assinado por HMAC). Respeita o envelope { data } / { error } do
// backend.
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

// Service é um serviço local que o agent deve expor pelo túnel. RemotePort é o
// bind atribuído pelo control plane no chisel-server (onde o worker conecta);
// fica 0 enquanto o Backify ainda não provisionou o túnel (Fase 2).
type Service struct {
	Type       string `json:"type"`
	LocalPort  int    `json:"localPort"`
	RemotePort int    `json:"remotePort"`
}

// TunnelInfo diz onde e como o agent disca o túnel. Vem no heartbeat (pode
// rotacionar). Fingerprint faz pinning do host key do chisel-server.
type TunnelInfo struct {
	Server      string `json:"server"`
	Fingerprint string `json:"fingerprint"`
}

// AgentConfig é a config devolvida no heartbeat (túnel + o que expor).
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

// Client é o cliente autenticado por HMAC (heartbeat e futuras chamadas).
type Client struct {
	baseURL string
	agentID string
	secret  string
	http    *http.Client
}

// New cria um cliente já com a identidade do agent.
func New(baseURL, agentID, secret string) *Client {
	return &Client{
		baseURL: baseURL,
		agentID: agentID,
		secret:  secret,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// do executa a requisição e desserializa `data` em out (ou devolve o erro da API).
func do(hc *http.Client, req *http.Request, out any) error {
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("resposta inválida (HTTP %d): %s", resp.StatusCode, string(body))
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

// Enroll troca o enrollment token de uso único pelas credenciais de máquina.
// Não é assinado (o próprio token é a prova). Devolve o segredo HMAC uma vez.
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

// Heartbeat reporta estado (versão/hostname) e recebe a config a aplicar.
func (c *Client) Heartbeat(ctx context.Context, version, hostname string) (*AgentConfig, error) {
	payload, _ := json.Marshal(map[string]string{"version": version, "hostname": hostname})
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
