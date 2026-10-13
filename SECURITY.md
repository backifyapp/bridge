# Security policy

## Reporting a vulnerability

**Do not open a public issue** for security problems.

- Preferably use GitHub's [Private vulnerability reporting](https://github.com/backifyapp/bridge/security/advisories/new) (*Security* tab → *Report a vulnerability*).
- Or email **security@backify.app**.

We reply within **72 hours** with an initial assessment. When the fix ships, we
credit the reporter (if they want to be credited).

## Supported versions

Security fixes always ship in the **latest** version. Update with:

```sh
sudo /usr/local/bin/backify-bridge update && sudo systemctl restart backify-bridge
```

| Version | Supported |
|---|---|
| 0.2.x | ✅ |
| 0.1.x | ❌ (please update) |

## Security model (summary)

- The agent makes **outbound connections only** (443/TLS). No inbound port is
  opened on your server.
- Every API call is signed with **HMAC-SHA256** — the secret **never travels**;
  there is a clock window (±300s) and a single-use nonce (replay protection).
  Since 0.4.0 the signature (protocol 2,
  `BACKIFY-AGENT-API-HMAC-SHA256-V2\nMETHOD\nPATH\nQUERY\nTS\nNONCE\nSHA256(body)`)
  also covers the request body; the API checks the body it read against the
  signed hash before burning the nonce. Its scheme line differs from the Docker
  helper's, so a signature made for one channel is never valid on the other.
- The agent secret lives in `/etc/backify-bridge/bridge.json` with mode `0600`;
  the systemd service runs as a dedicated, unprivileged user.
- Tunnel-exposed ports are **whitelisted per agent** on the server — one agent
  cannot reach another agent's ports.
- `update` verifies the published **SHA-256** before replacing the binary.

### Docker mode (opt-in)

When the **Docker** capability is enabled, the agent uses the host's Docker
socket — which gives it control over that server's containers and volumes. This
is optional and only relevant if you want volume/container backups. The HTTP
helper that does this listens **on `127.0.0.1` only**, requires an HMAC
signature, and is reachable exclusively through the tunnel.

The helper speaks signature **protocol 2**: the signature covers the method,
the path, the query, the timestamp, a single-use nonce and the **SHA-256 of the
request body**. A request body (volume import, container spec) is spooled to a
private temp file and only acted on after its hash matches the signed one; the
nonce is only stored after the signature checks out. Volume/container names
must follow Docker's name grammar (never a host path or a flag), and a
container spec may only mount **named volumes**. Requests signed with the old
protocol 1 are refused with an explicit version error. Container config
(`docker inspect`) may contain environment variables holding secrets; they go
into the backup, always encrypted.
