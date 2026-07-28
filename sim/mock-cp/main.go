// mock-cp is a FAKE control plane for the simulation only: it answers enroll and
// heartbeat with a fixed config pointing the tunnel at the sim's chisel-server.
// It ignores the HMAC signature (the real backend validates it; here we only
// prove the transport works).
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func main() {
	http.HandleFunc("/api/agents/enroll", func(w http.ResponseWriter, r *http.Request) {
		log.Println("enroll <-", r.RemoteAddr)
		writeData(w, map[string]string{"agentId": "sim-agent", "hmacSecret": "sim-secret"})
	})

	http.HandleFunc("/api/agents/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		log.Println("heartbeat <-", r.RemoteAddr)
		writeData(w, map[string]any{
			"tunnel": map[string]string{"server": "http://chisel:8080", "fingerprint": ""},
			"services": []map[string]any{
				{"type": "POSTGRES", "localPort": 5432, "remotePort": 40001},
			},
		})
	})

	log.Println("mock-cp ouvindo em :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
