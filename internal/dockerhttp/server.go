// Package dockerhttp é o helper HTTP LOCAL do agent (modo docker): expõe, só pelo
// túnel e autenticado por HMAC, operações de backup/restore de volumes e
// containers Docker. O restic continua no worker — aqui só exportamos/importamos.
package dockerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/backifyapp/bridge/internal/docker"
)

// Handler monta o roteador do helper, todo autenticado por HMAC.
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

	mux.HandleFunc("GET /docker/containers", auth(func(w http.ResponseWriter, r *http.Request) {
		cs, err := docker.ListContainers(r.Context())
		if fail(w, err) {
			return
		}
		writeJSON(w, cs)
	}))

	mux.HandleFunc("GET /docker/volume/{name}/export", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		if err := docker.ExportVolume(r.Context(), r.PathValue("name"), w); err != nil {
			// headers podem já ter ido; o worker detecta o stream truncado.
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))

	mux.HandleFunc("POST /docker/volume/{name}/import", auth(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := docker.ImportVolume(r.Context(), r.PathValue("name"), r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"volume": r.PathValue("name")})
	}))

	mux.HandleFunc("GET /docker/container/{id}/inspect", auth(func(w http.ResponseWriter, r *http.Request) {
		raw, err := docker.InspectContainer(r.Context(), r.PathValue("id"))
		if fail(w, err) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))

	mux.HandleFunc("POST /docker/container/run", auth(func(w http.ResponseWriter, r *http.Request) {
		var spec docker.RunSpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			http.Error(w, "spec inválida", http.StatusBadRequest)
			return
		}
		id, err := docker.RunContainer(r.Context(), spec)
		if fail(w, err) {
			return
		}
		writeJSON(w, map[string]string{"id": id})
	}))

	mux.HandleFunc("POST /docker/container/{id}/pause", auth(func(w http.ResponseWriter, r *http.Request) {
		if fail(w, docker.Pause(r.Context(), r.PathValue("id"))) {
			return
		}
		writeJSON(w, map[string]bool{"paused": true})
	}))

	mux.HandleFunc("POST /docker/container/{id}/unpause", auth(func(w http.ResponseWriter, r *http.Request) {
		if fail(w, docker.Unpause(r.Context(), r.PathValue("id"))) {
			return
		}
		writeJSON(w, map[string]bool{"paused": false})
	}))

	return mux
}

// Serve sobe o helper em addr (127.0.0.1:<porta>) até o contexto cancelar.
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

// fail escreve 500 e retorna true se err != nil.
func fail(w http.ResponseWriter, err error) bool {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	return false
}
