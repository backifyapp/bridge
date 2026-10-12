// Package dockerhttp is the agent's LOCAL HTTP helper (docker mode): it exposes,
// tunnel-only and HMAC-authenticated, backup/restore operations for Docker
// volumes and containers. restic stays on the worker — here we only export and
// import.
package dockerhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/backifyapp/bridge/internal/docker"
)

// Handler monta o roteador do helper, todo autenticado por HMAC (protocolo 2:
// a assinatura cobre o corpo — ver authMiddleware).
func Handler(secret string) http.Handler {
	nc := newNonceCache()
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc { return authMiddleware(secret, nc, h) }

	mux.HandleFunc("GET /docker/ping", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": docker.Available(r.Context())})
	}))

	mux.HandleFunc("GET /docker/volumes", auth(func(w http.ResponseWriter, r *http.Request) {
		vs, err := docker.ListVolumes(r.Context())
		if fail(w, err) {
			return
		}
		writeJSON(w, vs)
	}))

	// Tamanho de um volume (estimativa do painel). Medido no host com `du`,
	// sem trafegar dados pelo túnel.
	mux.HandleFunc("GET /docker/volume/{name}/size", auth(func(w http.ResponseWriter, r *http.Request) {
		name, ok := pathName(w, r, "name")
		if !ok {
			return
		}
		n, err := docker.VolumeSize(r.Context(), name)
		if fail(w, err) {
			return
		}
		writeJSON(w, map[string]int64{"bytes": n})
	}))

	mux.HandleFunc("GET /docker/containers", auth(func(w http.ResponseWriter, r *http.Request) {
		cs, err := docker.ListContainers(r.Context())
		if fail(w, err) {
			return
		}
		writeJSON(w, cs)
	}))

	mux.HandleFunc("GET /docker/volume/{name}/export", auth(func(w http.ResponseWriter, r *http.Request) {
		name, ok := pathName(w, r, "name")
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		if err := docker.ExportVolume(r.Context(), name, w); err != nil {
			// headers may already be sent; the worker detects the truncated stream.
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))

	mux.HandleFunc("POST /docker/volume/{name}/import", auth(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		name, ok := pathName(w, r, "name")
		if !ok {
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

	mux.HandleFunc("GET /docker/container/{id}/inspect", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathName(w, r, "id")
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

	mux.HandleFunc("POST /docker/container/run", auth(func(w http.ResponseWriter, r *http.Request) {
		var spec docker.RunSpec
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&spec); err != nil {
			http.Error(w, "invalid spec", http.StatusBadRequest)
			return
		}
		if err := docker.ValidateRunSpec(spec); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id, err := docker.RunContainer(r.Context(), spec)
		if fail(w, err) {
			return
		}
		writeJSON(w, map[string]string{"id": id})
	}))

	mux.HandleFunc("POST /docker/container/{id}/pause", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathName(w, r, "id")
		if !ok {
			return
		}
		if fail(w, docker.Pause(r.Context(), id)) {
			return
		}
		writeJSON(w, map[string]bool{"paused": true})
	}))

	mux.HandleFunc("POST /docker/container/{id}/unpause", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathName(w, r, "id")
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
func Serve(ctx context.Context, addr, secret string) error {
	srv := &http.Server{Addr: addr, Handler: Handler(secret)}
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

// fail escreve 500 e retorna true se err != nil.
func fail(w http.ResponseWriter, err error) bool {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	return false
}
