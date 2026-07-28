<p align="center">
  <a href="https://backify.app">
    <img src=".github/assets/backify-icon.svg" alt="Backify" width="96" height="96">
  </a>
</p>

<h1 align="center">Backify Bridge</h1>

<p align="center">
  <strong>Secure access broker for <a href="https://backify.app">Backify</a> backups.</strong><br>
  Back up databases and files behind a closed firewall — no inbound ports, no shared credentials.
</p>

<p align="center">
  <a href="https://github.com/backifyapp/bridge/releases"><img src="https://img.shields.io/github/v/release/backifyapp/bridge?color=2563EB" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-2563EB" alt="License: MIT"></a>
  <a href="https://backify.app"><img src="https://img.shields.io/badge/backify.app-Backup%20as%20a%20Service-10b981" alt="Backify"></a>
</p>

---

The Bridge is a lightweight agent that runs on your Linux server and gives Backify
secure access to your local services (databases, files) **without opening a single
inbound port on your firewall and without handing your production credentials to
anyone**.

It dials **outbound** (443/TLS), keeps a reverse tunnel up, and exposes **only**
the services you authorized in the dashboard. The Bridge **does not run backups**:
the Backify worker does the work, reaching your database/files *through* the
tunnel. That's why the Bridge is tiny and your server **needs nothing else** — no
Docker, no `pg_dump`, no `restic`.

```
  Your server                               Backify
 ┌────────────────────┐   outbound 443    ┌───────────────────────┐
 │ backify-bridge     │ ────────────────► │ tunnel (private net)  │
 │  exposes localhost:│   reverse tunnel  │        │              │
 │   5432 (postgres)  │ ◄──────────────── │        ▼              │
 │   22   (files)     │                   │   worker → pg_dump    │
 └────────────────────┘                   └───────────────────────┘
     ▲ no inbound port open
```

## Why use it

- **Firewall stays closed.** Outbound only; nothing exposed to the internet.
- **Credentials stay with you.** The worker connects through the tunnel — no need
  to expose your database to the world.
- **`localhost-only` databases.** Back up databases that bind to `127.0.0.1`.
- **Zero dependencies.** A single static binary. No Docker, no database clients.

## Installation

Linux **amd64/arm64**, runs as a `systemd` service.

```sh
curl -fsSL https://raw.githubusercontent.com/backifyapp/bridge/main/install.sh | sudo sh -s -- --token <YOUR_TOKEN>
```

`<YOUR_TOKEN>` (the enrollment token) is generated in the Backify dashboard when
you create a Bridge. Or do it manually:

```sh
sudo -u backify-bridge backify-bridge enroll --token <YOUR_TOKEN>
sudo systemctl enable --now backify-bridge
backify-bridge status
```

### Updating

```sh
sudo backify-bridge update && sudo systemctl restart backify-bridge
```

Downloads the latest release, **verifies the SHA-256** and replaces the binary.
Per-version changes live in the [CHANGELOG](CHANGELOG.md); security fixes in
[SECURITY.md](SECURITY.md).

### Uninstalling

```sh
curl -fsSL https://raw.githubusercontent.com/backifyapp/bridge/main/install.sh | sudo sh -s -- --uninstall
```

Stops and removes the service, the binary, the credentials in
`/etc/backify-bridge` and the dedicated user. Running it as a container instead?
`docker rm -f backify-bridge && docker rmi ghcr.io/backifyapp/bridge`.

> Deleting the local credentials does **not** revoke the identity. Finish by
> revoking the server in the dashboard (**Servers → Revoke**).

## How it works

1. **Enroll** — the Bridge trades a single-use enrollment token for a machine
   identity plus an HMAC secret, stored in `/etc/backify-bridge/bridge.json`
   (mode `0600`).
2. **Heartbeat** — periodically reports that it's alive and receives **which
   services to expose** (the policy lives in Backify, not in the agent).
3. **Tunnel** — keeps the outbound connection up and exposes only the authorized
   ports.

Every API call (except enroll) is signed with **HMAC-SHA256** — the secret
**never travels**, it only signs. See [`internal/sign`](internal/sign).

## Docker mode (volumes and containers)

Beyond databases and files, the Bridge backs up and restores Docker **volumes**
and **containers** (opt-in `Docker` capability in the dashboard). The agent
exposes a local HTTP helper (tunnel-only, HMAC-authenticated) that exports the
volume as `tar.gz` (through an ephemeral `:ro` container) and reads the container
config (`docker inspect`); restic runs on the worker. Restore recreates the
volume/container under a **new name** by default.

It needs access to the Docker socket. Either install with `--docker`:

```sh
curl -fsSL https://raw.githubusercontent.com/backifyapp/bridge/main/install.sh | sudo sh -s -- --token <YOUR_TOKEN> --docker
```

or run the official image with the socket mounted:

```sh
docker run -d --name backify-bridge \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /etc/backify-bridge:/etc/backify-bridge \
  ghcr.io/backifyapp/bridge
```

> ⚠️ Access to the Docker socket is **root-equivalent** on the host. `docker
> inspect` may expose secrets (env vars) — they end up in the encrypted snapshot.
> Only enable it where you trust Backify. For a database running in a container,
> prefer the database source over the tunnel.

## Transport

The tunnel transport sits behind the [`transport.Transport`](internal/transport/transport.go)
interface. The v1 implementation is **Chisel** ([`chisel.go`](internal/transport/chisel.go);
TCP over HTTPS, SSH crypto), embedded as a client — your server only runs the
Bridge binary. The interface is deliberate: moving to **FRP** later is a new
implementation, with no changes to the daemon.

The agent authenticates against the chisel-server with its own identity
(`agentID:secret`), validated there by a plugin against the Backify API. Each
service becomes a reverse tunnel `R:<server bind>:localhost:<local port>` — the
bind is assigned by the control plane and delivered in the heartbeat.

> **Status (phase 1):** the Chisel client is implemented. It waits for Backify to
> provision the tunnel — until the heartbeat carries `tunnel.server` plus each
> service's remote port (phase 2: `chisel-server` + port allocation in the control
> plane), the transport stays idle without erroring. To exercise the flow without
> a server, use `BACKIFY_BRIDGE_STUB=1`. Run `go mod tidy` after cloning.

## Security

- Outbound only on the client; no inbound port is opened.
- HMAC secret at rest with `0600`; per-agent identity, revocable from the dashboard.
- Tunnel endpoints are private (only the worker reaches them), never public.
- The systemd service runs as a dedicated, unprivileged user (hardened).

Found a vulnerability? Please read [SECURITY.md](SECURITY.md) — do **not** open a
public issue.

## Development

```sh
go test ./...      # includes the vector proving HMAC compatibility with the backend
go build ./...
go run ./cmd/backify-bridge status

# Docker acceptance test (real volume roundtrip) — on a host with docker:
go test -tags e2e ./internal/docker
```

Dev config elsewhere: `BACKIFY_BRIDGE_CONFIG=./bridge.json`.

---

## About Backify

<p align="center">
  <a href="https://backify.app">
    <img src=".github/assets/backify-icon.svg" alt="Backify" width="56" height="56">
  </a>
</p>

The Bridge is one piece of **[Backify](https://backify.app)** — Backup as a Service
for databases, apps, files, email and clouds.

- **Set it and forget it.** Schedule once; Backify runs, encrypts, versions and
  monitors — and alerts you the moment something fails.
- **20+ connectors.** PostgreSQL, MySQL, MariaDB, SQL Server, MongoDB, Redis,
  WordPress, Git, Kubernetes, IMAP email, S3 / Azure / Google Cloud, Google Drive,
  OneDrive, Dropbox, SFTP / FTP / WebDAV.
- **Restores that prove themselves.** Periodic integrity checks and automatic
  restore drills — because a backup you can't restore isn't a backup.
- **AES-256 encryption**, deduplicated versioned snapshots, and streaming straight
  to the destination (nothing ever touches disk).
- **Your storage or ours.** Bring your own bucket, or use Backify Cloud and pay
  per GB.

**Free plan available — no credit card required.** → **[backify.app](https://backify.app)**

## License

[MIT](LICENSE).
