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
	imageRe         = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/@-]*$`)
)

// ValidateRunSpec accepts only what a restore needs: a name, an image, plain
// env strings and NAMED volumes (never a host path). No value may start with
// "-" (it would become a docker flag). Without this, a run spec could
// bind-mount the host's "/".
//
// Published ports and restart policies are REFUSED (since 0.5.0): whoever
// signs helper calls could otherwise start a container listening on the
// internet that comes back after every reboot. The restored container is a
// copy next to the original (which usually still holds those host ports
// anyway); the operator publishes ports / sets the policy afterwards. Host
// network/pid/ipc, capabilities and devices have no field in RunSpec at all,
// and the handler refuses unknown fields, so they cannot be smuggled in.
func ValidateRunSpec(s RunSpec) error {
	if s.Name != "" && !ValidName(s.Name) {
		return fmt.Errorf("nome de container inválido: %q", s.Name)
	}
	if !imageRe.MatchString(s.Image) {
		return fmt.Errorf("imagem inválida: %q", s.Image)
	}
	if len(s.Ports) > 0 {
		return fmt.Errorf("publicar portas no restore não é permitido: recrie o container com as portas depois de conferir a cópia")
	}
	if s.RestartPolicy != "" && s.RestartPolicy != "no" {
		return fmt.Errorf("restart policy no restore não é permitida (%q): ajuste com docker update --restart depois", s.RestartPolicy)
	}
	for _, e := range s.Env {
		if !envRe.MatchString(e) {
			return fmt.Errorf("variável de ambiente inválida")
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
