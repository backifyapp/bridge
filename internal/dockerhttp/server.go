// Package dockerhttp is the agent's LOCAL HTTP helper (docker mode): it exposes,
// tunnel-only and HMAC-authenticated, backup/restore operations for Docker
// volumes and containers. restic stays on the worker — here we only export and
// import.
package dockerhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/backifyapp/bridge/internal/docker"
	"github.com/backifyapp/bridge/internal/sign"
)

// Trailers of GET /docker/volume/{name}/export. The archive is streamed, so the
// 200 is on the wire before `docker run … tar` has finished: the only place
// left to say how it ended is a trailer. The worker treats a missing trailer,
// or a code other than "0", as a failed export (and discards the snapshot).
const (
	TrailerExitCode  = "X-Backify-Exit-Code"
	TrailerExitError = "X-Backify-Exit-Error"
)

// Handler monta o roteador do helper, todo autenticado por HMAC (protocolo 3:
// a assinatura cobre o corpo e usa a chave derivada para o helper — ver
// authMiddleware). secret is the enroll secret; only sign.HelperKey(secret)
// is kept. pol is the optional local allowlist (nil = none).
func Handler(secret string, pol *docker.Policy) http.Handler {
	key := sign.HelperKey(secret)
	nc := newNonceCache()
	mux := http.NewServeMux()
	auth := func(rule bodyRule, h http.HandlerFunc) http.HandlerFunc { return authMiddleware(key, nc, rule, h) }

	mux.HandleFunc("GET /docker/ping", auth(noBody, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": docker.Available(r.Context())})
	}))

	mux.HandleFunc("GET /docker/volumes", auth(noBody, func(w http.ResponseWriter, r *http.Request) {
		vs, err := docker.ListVolumes(r.Context())
		if fail(w, err) {
			return
		}
		allowed := vs[:0]
		for _, v := range vs {
			if pol.AllowVolume(v.Name) {
				allowed = append(allowed, v)
			}
		}
		writeJSON(w, allowed)
	}))

	// Tamanho de um volume (estimativa do painel). Medido no host com `du`,
	// sem trafegar dados pelo túnel.
	mux.HandleFunc("GET /docker/volume/{name}/size", auth(noBody, func(w http.ResponseWriter, r *http.Request) {
		name, ok := existingVolume(w, r, pol)
		if !ok {
			return
		}
		n, err := docker.VolumeSize(r.Context(), name)
		if fail(w, err) {
			return
		}
		writeJSON(w, map[string]int64{"bytes": n})
	}))

	mux.HandleFunc("GET /docker/containers", auth(noBody, func(w http.ResponseWriter, r *http.Request) {
		cs, err := docker.ListContainers(r.Context())
		if fail(w, err) {
			return
		}
		allowed := cs[:0]
		for _, c := range cs {
			if pol.AllowContainer(c.Name, c.ID) {
				allowed = append(allowed, c)
			}
		}
		writeJSON(w, allowed)
	}))

	mux.HandleFunc("GET /docker/volume/{name}/export", auth(noBody, func(w http.ResponseWriter, r *http.Request) {
		name, ok := existingVolume(w, r, pol)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Trailer", TrailerExitCode+", "+TrailerExitError)
		cw := &countingWriter{w: w}
		err := docker.ExportVolume(r.Context(), name, cw)
		code := docker.ExitCode(err)
		if err != nil {
			log.Printf("[docker] export de %s falhou (exit %d) depois de %d bytes: %v", name, code, cw.n, err)
			w.Header().Set(TrailerExitError, headerSafe(err.Error()))
		}
		w.Header().Set(TrailerExitCode, strconv.Itoa(code))
		if err != nil && cw.n == 0 {
			// Nothing streamed yet: a plain error status is still possible.
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		// Otherwise the body just ends: the trailer above is what says it is
		// incomplete. (Writing the error text here, as before 0.5.0, appended
		// it to the gzip stream and the export still looked complete.)
	}))

	mux.HandleFunc("POST /docker/volume/{name}/import", auth(archiveBody, func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		name, ok := pathName(w, r, "name")
		if !ok {
			return
		}
		if !pol.AllowVolume(name) {
			http.Error(w, "volume fora da allowlist local (bridge.json → docker.volumes)", http.StatusForbidden)
			return
		}
		// A restore never writes over an existing volume: whoever signs helper
		// calls could otherwise replace the data of a live one (a database's,
		// say). Restores always target a new name.
		if docker.VolumeExists(r.Context(), name) {
			http.Error(w, fmt.Sprintf("o volume %q já existe: o restore só cria volumes novos", name), http.StatusConflict)
			return
		}
		// r.Body is the spooled, hash-verified copy (authMiddleware): nothing
		// reaches `tar -x` before the whole body matched the signature.
		if err := docker.ImportVolume(r.Context(), name, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"volume": name})
	}))

	mux.HandleFunc("GET /docker/container/{id}/inspect", auth(noBody, func(w http.ResponseWriter, r *http.Request) {
		id, ok := allowedContainer(w, r, pol)
		if !ok {
			return
		}
		raw, err := docker.InspectContainer(r.Context(), id)
		if fail(w, err) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))

	mux.HandleFunc("POST /docker/container/run", auth(jsonBody, func(w http.ResponseWriter, r *http.Request) {
		var spec docker.RunSpec
		dec := json.NewDecoder(r.Body)
		// An unknown field (network, privileged, capAdd, devices…) is refused,
		// not silently ignored: RunSpec has no way to express them on purpose.
		dec.DisallowUnknownFields()
		if err := dec.Decode(&spec); err != nil {
			http.Error(w, "invalid spec: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := docker.ValidateRunSpec(spec); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, v := range spec.Volumes {
			if vol := strings.SplitN(v, ":", 2)[0]; !pol.AllowVolume(vol) {
				http.Error(w, fmt.Sprintf("volume %q fora da allowlist local (bridge.json → docker.volumes)", vol), http.StatusForbidden)
				return
			}
		}
		if !pol.AllowImage(spec.Image) {
			http.Error(w, fmt.Sprintf("imagem %q fora da allowlist local (bridge.json → docker.images)", spec.Image), http.StatusForbidden)
			return
		}
		// Without an image allowlist, only images already on this host run
		// (--pull=never): a signed request cannot make it fetch and start
		// evil.example.com/pwn. Checked here too, for a clear message.
		allowPull := pol.RestrictsImages()
		if !allowPull && !docker.ImageExists(r.Context(), spec.Image) {
			http.Error(w, fmt.Sprintf("a imagem %q não está neste servidor e o Bridge não baixa imagens por conta própria: rode `docker pull %s` e tente de novo (ou liste-a em bridge.json → docker.images)", spec.Image, spec.Image), http.StatusConflict)
			return
		}
		id, err := docker.RunContainer(r.Context(), spec, allowPull)
		if fail(w, err) {
			return
		}
		writeJSON(w, map[string]string{"id": id})
	}))

	mux.HandleFunc("POST /docker/container/{id}/pause", auth(noBody, func(w http.ResponseWriter, r *http.Request) {
		id, ok := allowedContainer(w, r, pol)
		if !ok {
			return
		}
		if fail(w, docker.Pause(r.Context(), id)) {
			return
		}
		writeJSON(w, map[string]bool{"paused": true})
	}))

	mux.HandleFunc("POST /docker/container/{id}/unpause", auth(noBody, func(w http.ResponseWriter, r *http.Request) {
		id, ok := allowedContainer(w, r, pol)
		if !ok {
			return
		}
		if fail(w, docker.Unpause(r.Context(), id)) {
			return
		}
		writeJSON(w, map[string]bool{"paused": false})
	}))

	return mux
}

// Serve starts the helper on addr (127.0.0.1:<port>) until the context is canceled.
func Serve(ctx context.Context, addr, secret string, pol *docker.Policy) error {
	srv := &http.Server{Addr: addr, Handler: Handler(secret, pol)}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// pathName reads a {name}/{id} path value and refuses (400) anything outside
// Docker's name grammar: "/etc" would be a host bind mount, "-f=…" a flag.
func pathName(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	v := r.PathValue(key)
	if !docker.ValidName(v) {
		http.Error(w, "nome inválido", http.StatusBadRequest)
		return "", false
	}
	return v, true
}

// existingVolume validates the {name}, the local allowlist, and that the
// volume EXISTS: `docker run -v <name>:/data` would create a missing one and
// the backup would be an empty archive reported as a success.
func existingVolume(w http.ResponseWriter, r *http.Request, pol *docker.Policy) (string, bool) {
	name, ok := pathName(w, r, "name")
	if !ok {
		return "", false
	}
	if !pol.AllowVolume(name) {
		http.Error(w, "volume fora da allowlist local (bridge.json → docker.volumes)", http.StatusForbidden)
		return "", false
	}
	if !docker.VolumeExists(r.Context(), name) {
		http.Error(w, fmt.Sprintf("volume %q não existe neste servidor", name), http.StatusNotFound)
		return "", false
	}
	return name, true
}

// allowedContainer validates the {id} and, with a container allowlist, matches
// it by the value sent and by its resolved name.
func allowedContainer(w http.ResponseWriter, r *http.Request, pol *docker.Policy) (string, bool) {
	id, ok := pathName(w, r, "id")
	if !ok {
		return "", false
	}
	if !pol.AllowContainer(id) && !pol.AllowContainer(docker.ContainerName(r.Context(), id)) {
		http.Error(w, "container fora da allowlist local (bridge.json → docker.containers)", http.StatusForbidden)
		return "", false
	}
	return id, true
}

// fail escreve 500 e retorna true se err != nil.
func fail(w http.ResponseWriter, err error) bool {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	return false
}

// countingWriter counts what reached the response, to know whether the status
// line is already on the wire.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// headerSafe turns an error message into a single short header value.
func headerSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300])
	}
	return strings.TrimSpace(s)
}
