// Package docker envolve o Docker via CLI (os/exec) — mantém o binário leve
// (sem o SDK oficial). A construção de comandos e o parsing são funções puras
// (testáveis); o exec é uma casca fina.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Imagem efêmera usada para tarar/destarar volumes.
const helperImage = "alpine:3"

// Volume é um volume nomeado do Docker.
type Volume struct {
	Name string `json:"name"`
}

// Container é um container (em execução ou não).
type Container struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

// RunSpec é a config mínima para recriar um container no restore. O worker deriva
// isto do `docker inspect` guardado no backup.
type RunSpec struct {
	Name          string   `json:"name"`
	Image         string   `json:"image"`
	Env           []string `json:"env"`           // "KEY=value"
	Ports         []string `json:"ports"`         // "hostPort:containerPort"
	Volumes       []string `json:"volumes"`       // "volName:/path[:ro]"
	RestartPolicy string   `json:"restartPolicy"` // ex.: "unless-stopped"
}

// ── Funções puras (testáveis) ───────────────────────────────────────────────

func parseVolumeList(out string) []Volume {
	vs := []Volume{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			vs = append(vs, Volume{Name: line})
		}
	}
	return vs
}

func parseContainerList(out string) []Container {
	cs := []Container{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		p := strings.SplitN(line, "\t", 3)
		c := Container{ID: p[0]}
		if len(p) > 1 {
			c.Name = p[1]
		}
		if len(p) > 2 {
			c.Image = p[2]
		}
		cs = append(cs, c)
	}
	return cs
}

func exportVolumeArgs(volume string) []string {
	return []string{"run", "--rm", "-v", volume + ":/data:ro", helperImage, "tar", "-C", "/data", "-czf", "-", "."}
}

func importVolumeArgs(volume string) []string {
	return []string{"run", "--rm", "-i", "-v", volume + ":/data", helperImage, "tar", "-C", "/data", "-xzf", "-"}
}

func runContainerArgs(s RunSpec) []string {
	args := []string{"run", "-d"}
	if s.Name != "" {
		args = append(args, "--name", s.Name)
	}
	if s.RestartPolicy != "" {
		args = append(args, "--restart", s.RestartPolicy)
	}
	for _, e := range s.Env {
		args = append(args, "-e", e)
	}
	for _, p := range s.Ports {
		args = append(args, "-p", p)
	}
	for _, v := range s.Volumes {
		args = append(args, "-v", v)
	}
	return append(args, s.Image)
}

// ── Casca de exec ────────────────────────────────────────────────────────────

func run(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Available indica se o docker CLI + daemon respondem.
func Available(ctx context.Context) bool {
	_, err := run(ctx, "version", "--format", "{{.Server.Version}}")
	return err == nil
}

// Version devolve a versão do Docker server (vazio se indisponível).
func Version(ctx context.Context) string {
	out, err := run(ctx, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// ListVolumes lista os volumes nomeados.
func ListVolumes(ctx context.Context) ([]Volume, error) {
	out, err := run(ctx, "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil, err
	}
	return parseVolumeList(out), nil
}

// ListContainers lista os containers (inclui parados).
func ListContainers(ctx context.Context) ([]Container, error) {
	out, err := run(ctx, "ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}")
	if err != nil {
		return nil, err
	}
	return parseContainerList(out), nil
}

// ExportVolume tara o volume (:ro) e escreve o tar.gz no writer (stream, sem disco).
func ExportVolume(ctx context.Context, volume string, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", exportVolumeArgs(volume)...)
	cmd.Stdout = w
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("export volume %s: %v: %s", volume, err, stderr.String())
	}
	return nil
}

// ImportVolume cria o volume (idempotente) e destara o stream dentro dele.
func ImportVolume(ctx context.Context, volume string, r io.Reader) error {
	if _, err := run(ctx, "volume", "create", volume); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", importVolumeArgs(volume)...)
	cmd.Stdin = r
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("import volume %s: %v: %s", volume, err, stderr.String())
	}
	return nil
}

// InspectContainer devolve o JSON cru do `docker inspect`.
func InspectContainer(ctx context.Context, id string) (json.RawMessage, error) {
	out, err := run(ctx, "inspect", id)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

// RunContainer recria um container a partir da spec mínima; devolve o id.
func RunContainer(ctx context.Context, s RunSpec) (string, error) {
	out, err := run(ctx, runContainerArgs(s)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Pause / Unpause para consistência durante o backup do volume.
func Pause(ctx context.Context, id string) error   { _, err := run(ctx, "pause", id); return err }
func Unpause(ctx context.Context, id string) error { _, err := run(ctx, "unpause", id); return err }
