# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
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

## [0.2.2] — 2026-07-28

### Fixed
- **The tunnel never came up (`too many colons in address`).** The chisel client
  decides whether an address already carries a scheme with
  `HasPrefix(server, "http")` — not by looking for `://`. A valid
  `wss://host` was therefore not recognised as a URL: chisel prepended its own
  scheme, producing `http://wss://host`, and dialed `wss::80` forever. The agent
  now normalises `wss://` → `https://` (and `ws://` → `http://`) before handing
  the address over, so both spellings work.

### Changed
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

[Unreleased]: https://github.com/backifyapp/bridge/compare/v0.2.2...HEAD
[0.2.2]: https://github.com/backifyapp/bridge/releases/tag/v0.2.2
[0.2.1]: https://github.com/backifyapp/bridge/releases/tag/v0.2.1
[0.2.0]: https://github.com/backifyapp/bridge/releases/tag/v0.2.0
[0.1.0]: https://github.com/backifyapp/bridge/releases/tag/v0.1.0
