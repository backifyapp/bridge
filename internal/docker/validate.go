package docker

import (
	"fmt"
	"regexp"
	"strings"
)

// Docker's own grammar for volume and container names ([a-zA-Z0-9][a-zA-Z0-9_.-]*).
// It is also what keeps a "name" from being something else: "/etc" in
// `-v <name>:/data` is a HOST bind mount, and "-f=..." in `docker inspect <id>`
// is a flag. The worker already checks this before calling; the helper checks
// again because it is the one running the command on the customer's host.
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,254}$`)

// ValidName reports whether s is a valid Docker volume/container name or id.
func ValidName(s string) bool { return nameRe.MatchString(s) }

var (
	containerPathRe = regexp.MustCompile(`^/[^\x00]*$`)
	envRe           = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*=`)
	portRe          = regexp.MustCompile(`^(\d{1,5}:)?(\d{1,5}:)?\d{1,5}(/(tcp|udp|sctp))?$|^[0-9.:\[\]a-fA-F]+:\d{1,5}:\d{1,5}(/(tcp|udp|sctp))?$`)
	restartRe       = regexp.MustCompile(`^(|no|always|unless-stopped|on-failure(:\d{1,4})?)$`)
	imageRe         = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/@-]*$`)
)

// ValidateRunSpec accepts only what the worker builds from a backed-up
// `docker inspect`: NAMED volumes (never a host path), plain env/port strings
// and a known restart policy. No value may start with "-" (it would become a
// docker flag). Without this, a run spec could bind-mount the host's "/".
func ValidateRunSpec(s RunSpec) error {
	if s.Name != "" && !ValidName(s.Name) {
		return fmt.Errorf("nome de container inválido: %q", s.Name)
	}
	if !imageRe.MatchString(s.Image) {
		return fmt.Errorf("imagem inválida: %q", s.Image)
	}
	if !restartRe.MatchString(s.RestartPolicy) {
		return fmt.Errorf("restart policy inválida: %q", s.RestartPolicy)
	}
	for _, e := range s.Env {
		if !envRe.MatchString(e) {
			return fmt.Errorf("variável de ambiente inválida")
		}
	}
	for _, p := range s.Ports {
		if !portRe.MatchString(p) {
			return fmt.Errorf("porta inválida: %q", p)
		}
	}
	for _, v := range s.Volumes {
		parts := strings.Split(v, ":")
		if len(parts) < 2 || len(parts) > 3 || !ValidName(parts[0]) || !containerPathRe.MatchString(parts[1]) {
			return fmt.Errorf("volume inválido (só volume nomeado → caminho absoluto): %q", v)
		}
		if len(parts) == 3 && parts[2] != "ro" && parts[2] != "rw" {
			return fmt.Errorf("modo de volume inválido: %q", v)
		}
	}
	return nil
}
