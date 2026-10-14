package sign

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/hex"
)

// ── Per-channel keys (protocol 3, Bridge ≥ 0.5.0) ──────────────────────────
//
// Up to 0.4.0 the secret returned by enroll was used AS IS on three channels:
// the chisel password (written in clear into the chisel-server's users.json),
// the agent → API HMAC and the worker → Docker helper HMAC. Whoever read
// users.json — or got code execution on the internet-facing chisel-server —
// held every agent's secret, and could sign Docker helper calls on any
// customer host.
//
// Since protocol 3 the enroll secret is only the root: each channel uses its
// own key, derived with HKDF-SHA256 (RFC 5869, empty salt, 32-byte output,
// lowercase hex). HKDF is one-way, so a leaked chisel password says nothing
// about the API or helper keys. The backend derives the same values
// (apps/api/src/auth/agent-keys.ts in prhost/backify) and
// TestChannelKeysSharedVector pins both sides.
const (
	InfoChiselAuth   = "backify/chisel-auth/v1"
	InfoAgentAPI     = "backify/agent-api/v1"
	InfoDockerHelper = "backify/docker-helper/v1"
)

// DeriveKey returns hex(HKDF-SHA256(secret, salt="", info, 32 bytes)).
func DeriveKey(secret, info string) string {
	k, err := hkdf.Key(sha256.New, []byte(secret), nil, info, 32)
	if err != nil {
		// Only possible for an output longer than 255*32 bytes.
		panic("hkdf: " + err.Error())
	}
	return hex.EncodeToString(k)
}

// ChiselPassword is what the Bridge sends as the chisel password (the backend
// writes only this value into users.json).
func ChiselPassword(secret string) string { return DeriveKey(secret, InfoChiselAuth) }

// APIKey signs agent → API calls (protocol 3).
func APIKey(secret string) string { return DeriveKey(secret, InfoAgentAPI) }

// HelperKey verifies worker → Docker helper calls (protocol 3).
func HelperKey(secret string) string { return DeriveKey(secret, InfoDockerHelper) }
