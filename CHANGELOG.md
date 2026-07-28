# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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

[Unreleased]: https://github.com/backifyapp/bridge/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/backifyapp/bridge/releases/tag/v0.2.1
[0.2.0]: https://github.com/backifyapp/bridge/releases/tag/v0.2.0
[0.1.0]: https://github.com/backifyapp/bridge/releases/tag/v0.1.0
