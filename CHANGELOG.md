# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Security
- **A failed `docker run` no longer returns environment variable values.** The
  error of the restore's `docker run` (the helper's `POST /docker/container/run`)
  carried the whole command line, `-e MYSQL_ROOT_PASSWORD=<value>` included,
  and the backend wrote it into the restore log the customer sees. Errors now
  keep the variable NAME and hide the value (`-e KEY=***`) — on the command
  line and, as defense in depth, wherever docker's own output repeats a value
  (of 4 bytes or more).
- **`install.sh` installs the systemd unit from the release it installs.** The
  binary came from the resolved release tag, but the unit came from `main`, so
  a fresh install could pair a binary with a unit from a different (even
  unreleased) version. Both now come from the same tag.

## [0.5.0] — 2026-10-15

Requires a Backify backend that speaks protocol 3 (deploy
the backend first; it keeps accepting older Bridges during the transition).

**No downgrade after the first protocol 3 call.** On its first protocol 3
heartbeat the backend stores the three per-channel keys and **deletes the
enroll secret**. Bridge 0.4.x signs with that secret, so putting it back on
the host gets 401 on every call and the tunnel stops. To roll back a host,
use **Reinstall** in the dashboard and enroll again with the older binary.

### Security
- **One key per channel (protocol 3).** Up to 0.4.0 the enroll secret was used
  as is for three things: the chisel password (stored in clear in the
  chisel-server's `users.json`, an internet-facing service), the agent → API
  HMAC and the worker → Docker helper HMAC. Reading that file, or code
  execution on the chisel-server, handed over every agent's secret — enough to
  sign Docker helper calls on any customer host. The enroll secret is now only
  a root: the chisel password, the API key and the helper key are derived from
  it with HKDF-SHA256 (`backify/chisel-auth/v1`, `backify/agent-api/v1`,
  `backify/docker-helper/v1`). The API and helper signatures use new scheme
  lines (`BACKIFY-AGENT-API-HMAC-SHA256-V3`, `BACKIFY-BRIDGE-HMAC-SHA256-V3`)
  and say `X-Backify-Protocol: 3`; the helper only accepts protocol 3. Shared
  vectors pin the derivation and both signatures to the backend.
- **A failed volume export is no longer a complete one.** The helper streams
  the archive with a 200 before `docker run … tar` finishes; when tar died
  midway, the error text was written into the gzip body and the stream ended
  cleanly, so the backup looked successful. The exit status now goes in the
  `X-Backify-Exit-Code` HTTP trailer (`X-Backify-Exit-Error` on failure), and
  the error is never written into the archive.
- Exporting or measuring a volume that does not exist answers 404 (`docker run
  -v <name>:/data` used to create it empty and back that up).
- **Safe defaults for restores, with no configuration:** the container spec
  refuses published ports, restart policies and unknown fields (host network,
  privileged, capabilities, devices…); the image must already be on the host
  (`docker run --pull=never`); a volume import only creates new volumes (409 on
  an existing one).
- **Optional local allowlist** (`"docker": {"volumes", "containers", "images"}`
  in `bridge.json`), enforced when present; listings show only what is allowed;
  an invalid pattern keeps the helper down. Re-enrolling preserves it.
- Body limits before anything is spooled: 1 MiB for the container spec; the
  import requires `Content-Length` and enough free space in `$TMPDIR` (256 MiB
  reserve), checked before writing a byte.
- `update` downloads from the exact tag it resolved (never `latest/download`
  twice), accepts only `vX.Y.Z` tags, refuses downgrades and caps the download
  size. `install.sh` resolves the tag once and checks the published SHA-256.
  Both are integrity checks, not authenticity: see SECURITY.md.

## [0.4.0] — 2026-10-13

Requires a Backify API that accepts agent signature protocol 2 (deploy the API
first; it keeps accepting protocol 1 during the transition).

### Security
- **Agent → API calls are signed with protocol 2 (body in the HMAC).** The
  heartbeat used protocol 1 (`METHOD\nPATH\nTS\nNONCE`): whoever could tamper
  with a request inside the time window could change what the agent reports
  (version, hostname, inventory) while keeping the signature valid. The
  canonical string is now
  `BACKIFY-AGENT-API-HMAC-SHA256-V2\nMETHOD\nPATH\nQUERY\nTS\nNONCE\nSHA256(body)`,
  with the body hash in `X-Backify-Content-SHA256`. The scheme line differs from
  the Docker helper's, so a signature from one channel is never valid on the
  other. A shared vector pins the Go and backend implementations.
- A signed call refused with 401 and no `X-Backify-Protocol` header now reports
  "the Backify API predates protocol 2" instead of looking like a wrong secret.

## [0.3.0] — 2026-10-12

The Docker helper protocol changes incompatibly: update the agents BEFORE the
Backify worker that speaks protocol 2 is deployed.

### Security
- **The Docker helper signature now covers the request body (protocol 2).**
  Protocol 1 signed only `METHOD\nPATH\nTS\nNONCE`: whoever could tamper with a
  request between the worker and the helper kept the signature valid while
  swapping the body — a volume import (`tar -x` into the volume) or a container
  spec (`docker run`, which could bind-mount the host's `/`). The canonical
  string is now `BACKIFY-BRIDGE-HMAC-SHA256-V2\nMETHOD\nPATH\nQUERY\nTS\nNONCE\nSHA256(body)`,
  with the body hash in `X-Backify-Content-SHA256`. The body is spooled to a
  private temp file while hashed and handed to the handler only if it matches;
  the file is always removed. Every helper response carries
  `X-Backify-Protocol: 2`; protocol-1 requests are refused with an explicit
  version error.
- Header formats are checked (nonce 16–64 of `[A-Za-z0-9_-]`, numeric timestamp,
  hex signature/hash) and the nonce TTL now covers the whole ±300s window.
- **Names and container specs are validated on the helper.** A volume name of
  `/etc` used to become a host bind mount in `-v <name>:/data`, and a container
  spec could carry `"/:/host"` or values starting with `-` (docker flags). Names
  must follow Docker's grammar and a spec may only mount named volumes.

### Note
- A volume import now needs free space in `$TMPDIR` (a private `/tmp` under the
  systemd unit) for the compressed archive while it is verified.

## [0.2.2] — 2026-07-28

### Added
- **Volume size** (`GET /docker/volume/{name}/size`): measures the volume with
  `du` in an ephemeral read-only container, so the dashboard can estimate the
  backup size without pulling the whole tar through the tunnel.
- `install.sh --docker` enables the Docker capability on a binary install: it
  adds the agent to the `docker` group and drops in the systemd overrides the
  socket needs. Off by default — socket access is root-equivalent on the host.
- **`install.sh --uninstall`** removes everything the installer created: the
  service and its drop-ins, the binary, the credentials in `/etc/backify-bridge`
  and the dedicated user. It is idempotent, works on hosts without systemd, and
  reminds you to revoke the server in the dashboard — deleting the local
  credentials does not revoke the identity.

### Fixed
- **The Docker capability could not work on a binary install.** The hardened unit
  allows only `AF_INET`/`AF_INET6`, so the agent could not open a unix socket at
  all, whatever the file permissions said — `docker ps` failed with "Cannot
  connect to the Docker daemon". The `--docker` drop-in adds `AF_UNIX`,
  `SupplementaryGroups=docker` and a `DOCKER_CONFIG` that is not under the
  `ProtectHome`-masked `/home`.
- **The tunnel never came up (`too many colons in address`).** The chisel client
  decides whether an address already carries a scheme with
  `HasPrefix(server, "http")` — not by looking for `://`. A valid
  `wss://host` was therefore not recognised as a URL: chisel prepended its own
  scheme, producing `http://wss://host`, and dialed `wss::80` forever. The agent
  now normalises `wss://` → `https://` (and `ws://` → `http://`) before handing
  the address over, so both spellings work.

### Changed
- The update/security docs now use the **absolute path**
  (`sudo /usr/local/bin/backify-bridge update`): on RHEL-family distros sudo
  resets `PATH` to a `secure_path` without `/usr/local/bin`, so the short form
  failed with "command not found".
- The vendored chisel version is now stamped into the binary (it used to report
  `0.0.0-src`, which the server logged as a version mismatch on a matched pair).
- The transport used to go **silent** when it had no tunnel address or no remote
  port: a misconfigured control plane looked exactly like a healthy idle agent.
  It now logs why it is idle, and logs the address when it connects.

## [0.2.1] — 2026-07-28

### Fixed
- **Enroll returned 401.** The default API URL pointed at `api.backify.app`
  (the auth provider) instead of `srv.backify.app` (the Backify API), so every
  enroll was rejected. Affects both the binary default and `install.sh`.
  Existing installs can pass `--url https://srv.backify.app` or set
  `BACKIFY_API_URL`.

## [0.2.0] — 2026-07-27

### Added
- **Docker backup and restore** (opt-in mode): volumes and containers.
  - Local HTTP helper authenticated with HMAC, reachable only through the tunnel
    (`/docker/volumes`, `/docker/volume/{n}/export|import`,
    `/docker/container/{id}/inspect`, `/docker/container/run`, pause/unpause).
  - Volume export through an ephemeral container mounting the volume `:ro`.
  - Restore creates the **volume/container under a new name** by default (it
    never overwrites what is in use).
- **Official image** `ghcr.io/backifyapp/bridge` (bundles `docker-cli`), to run
  the agent in a container with the Docker socket mounted.
- **Fleet inventory** in the heartbeat: OS, architecture, kernel, CPU count,
  total memory, IPs and Docker version — surfaced in the dashboard.
- **`backify-bridge update`**: downloads the latest release, **verifies the
  SHA-256** and replaces the binary.

### Changed
- Requires Go 1.25 to build (Chisel client dependency).

## [0.1.0] — 2026-07-26

### Added
- First release of the **Backify Bridge**: a **reverse tunnel** (Chisel) access
  agent for backing up databases and files without opening inbound ports.
- `enroll` (single-use token → local `0600` credentials), `run` (daemon with
  heartbeat), `status`, `version`.
- **HMAC-SHA256** authentication on every API call (the secret never travels),
  with a test vector locked against the backend.
- `install.sh` installer + hardened `systemd` service (dedicated, unprivileged
  user), Linux amd64/arm64 binaries on GitHub Releases.

### Fixed
- The reverse tunnel now binds to `0.0.0.0` (it was `127.0.0.1`, which made the
  port unreachable from outside the container).

[Unreleased]: https://github.com/backifyapp/bridge/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/backifyapp/bridge/releases/tag/v0.5.0
[0.4.0]: https://github.com/backifyapp/bridge/releases/tag/v0.4.0
[0.3.0]: https://github.com/backifyapp/bridge/releases/tag/v0.3.0
[0.2.2]: https://github.com/backifyapp/bridge/releases/tag/v0.2.2
[0.2.1]: https://github.com/backifyapp/bridge/releases/tag/v0.2.1
[0.2.0]: https://github.com/backifyapp/bridge/releases/tag/v0.2.0
[0.1.0]: https://github.com/backifyapp/bridge/releases/tag/v0.1.0
