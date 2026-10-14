package docker

import (
	"fmt"
	"path"
)

// Policy is the OPTIONAL local allowlist for docker mode, read from the
// "docker" section of /etc/backify-bridge/bridge.json. It is the customer's
// last word over what Backify may touch through the helper, independent of
// anything Backify sends:
//
//	"docker": {
//	  "volumes":    ["app-data", "app-*"],
//	  "containers": ["web", "db"],
//	  "images":     ["postgres:16-alpine", "ghcr.io/acme/*"]
//	}
//
// Each list is enforced only when present (a missing list = no restriction;
// an empty list = nothing allowed). Entries are exact names or path.Match
// globs. Restores create volumes named "<volume>-restored-<timestamp>", so a
// volume allowlist that should accept restores needs a glob such as "app-*".
//
// "images" changes how restores get their image: without it the helper only
// runs images already present on this host (docker run --pull=never); with it,
// only the listed images run, and those may be pulled if missing.
type Policy struct {
	Volumes    []string `json:"volumes,omitempty"`
	Containers []string `json:"containers,omitempty"`
	Images     []string `json:"images,omitempty"`
}

// Validate refuses malformed globs, so a typo fails at startup instead of
// silently denying (or allowing) everything.
func (p *Policy) Validate() error {
	if p == nil {
		return nil
	}
	for kind, list := range map[string][]string{"volumes": p.Volumes, "containers": p.Containers, "images": p.Images} {
		for _, pat := range list {
			if pat == "" {
				return fmt.Errorf("docker.%s: entrada vazia", kind)
			}
			if _, err := path.Match(pat, ""); err != nil {
				return fmt.Errorf("docker.%s: padrão inválido %q: %w", kind, pat, err)
			}
		}
	}
	return nil
}

func matchAny(list []string, name string) bool {
	for _, pat := range list {
		if ok, _ := path.Match(pat, name); ok {
			return true
		}
	}
	return false
}

// AllowVolume reports whether the volume may be exported, measured, imported
// into or mounted.
func (p *Policy) AllowVolume(name string) bool {
	return p == nil || p.Volumes == nil || matchAny(p.Volumes, name)
}

// AllowContainer reports whether the container (by any of the given names —
// the value Backify sent and, when different, its resolved name) may be
// inspected or paused.
func (p *Policy) AllowContainer(names ...string) bool {
	if p == nil || p.Containers == nil {
		return true
	}
	for _, n := range names {
		if n != "" && matchAny(p.Containers, n) {
			return true
		}
	}
	return false
}

// RestrictsImages reports whether an image allowlist is in force.
func (p *Policy) RestrictsImages() bool { return p != nil && p.Images != nil }

// AllowImage reports whether a restore may run (and, with an allowlist, pull)
// the image.
func (p *Policy) AllowImage(image string) bool {
	return !p.RestrictsImages() || matchAny(p.Images, image)
}
