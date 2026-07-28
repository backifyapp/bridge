#!/usr/bin/env sh
# Backify Bridge installer.
#   curl -fsSL https://raw.githubusercontent.com/backifyapp/bridge/main/install.sh | sh -s -- --token <TOKEN>
#
# Downloads the binary from GitHub Releases, installs it into /usr/local/bin,
# creates the dedicated user + systemd service and, if --token is given,
# enrolls this server.
#
# Add --docker to enable the Docker capability (volume/container backups). It
# grants the agent access to the Docker socket, which is ROOT-EQUIVALENT on this
# host — off by default, and only worth it if you enabled Docker in the panel.
#
# Use --uninstall to remove everything this script installed (service, binary,
# credentials and the dedicated user).
set -eu

REPO="backifyapp/bridge"
BIN="/usr/local/bin/backify-bridge"
CONFIG_DIR="/etc/backify-bridge"
UNIT="/etc/systemd/system/backify-bridge.service"
API_URL="${BACKIFY_API_URL:-https://srv.backify.app}"
TOKEN=""
DOCKER=0
UNINSTALL=0

while [ $# -gt 0 ]; do
  case "$1" in
    --token) TOKEN="$2"; shift 2 ;;
    --url)   API_URL="$2"; shift 2 ;;
    --docker) DOCKER=1; shift ;;
    --uninstall) UNINSTALL=1; shift ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "please run as root (sudo)." >&2
  exit 1
fi

if [ "$UNINSTALL" -eq 1 ]; then
  # Every step tolerates "already gone", so re-running is safe and a partial
  # install still cleans up.
  # The unit files go regardless of systemd being present — only the systemctl
  # calls are guarded, otherwise a host without systemd keeps them forever.
  if command -v systemctl >/dev/null 2>&1; then
    echo "==> stopping the service…"
    systemctl disable --now backify-bridge >/dev/null 2>&1 || true
  fi
  echo "==> removing the systemd unit…"
  rm -f "$UNIT"
  rm -rf "${UNIT}.d"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl reset-failed backify-bridge >/dev/null 2>&1 || true
  fi

  echo "==> removing the binary…"
  rm -f "$BIN" "$(dirname "$BIN")/.backify-bridge.new"

  echo "==> removing credentials…"
  rm -rf "$CONFIG_DIR"

  echo "==> removing the dedicated user…"
  userdel backify-bridge >/dev/null 2>&1 || true
  groupdel backify-bridge >/dev/null 2>&1 || true

  echo
  echo "Backify Bridge removed from this server."
  echo
  echo "One thing left, in the dashboard: revoke this server (Servers → Revoke)."
  echo "Deleting the local credentials does not revoke the identity — Backify"
  echo "still considers it valid until you revoke it there."
  echo
  echo "Running it as a container instead? Remove it with:"
  echo "  docker rm -f backify-bridge && docker rmi ghcr.io/backifyapp/bridge"
  exit 0
fi

# Detect OS/arch (v1: Linux amd64/arm64 only).
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
if [ "$OS" != "linux" ]; then
  echo "Bridge v1 only runs on Linux (detected: $OS)." >&2
  exit 1
fi

echo "==> downloading backify-bridge ($OS/$ARCH)…"
URL="https://github.com/${REPO}/releases/latest/download/backify-bridge_${OS}_${ARCH}"
curl -fsSL "$URL" -o "$BIN"
chmod +x "$BIN"

echo "==> creating dedicated user backify-bridge…"
id backify-bridge >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin backify-bridge
install -d -o backify-bridge -g backify-bridge -m 0700 "$CONFIG_DIR"

echo "==> installing systemd service…"
curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/packaging/systemd/backify-bridge.service" \
  -o "$UNIT"

if [ "$DOCKER" -eq 1 ]; then
  if ! getent group docker >/dev/null 2>&1; then
    echo "--docker was given but there is no 'docker' group on this host." >&2
    exit 1
  fi
  echo "==> enabling the Docker capability (root-equivalent access to the socket)…"
  usermod -aG docker backify-bridge
  # A drop-in, so upgrading the unit file never drops these.
  install -d -m 0755 "${UNIT}.d"
  cat > "${UNIT}.d/docker.conf" <<EOF
[Service]
# The Docker socket is a unix socket: without AF_UNIX in the allow list the
# agent cannot open it at all, whatever the file permissions say.
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
SupplementaryGroups=docker
# ProtectHome=true masks /home, and the docker CLI looks for ~/.docker/config.json.
Environment=DOCKER_CONFIG=/etc/backify-bridge
EOF
fi

systemctl daemon-reload

if [ -n "$TOKEN" ]; then
  echo "==> enrolling this server with Backify…"
  sudo -u backify-bridge "$BIN" enroll --token "$TOKEN" --url "$API_URL"
  systemctl enable --now backify-bridge
  echo "==> done. status: systemctl status backify-bridge"
  [ "$DOCKER" -eq 1 ] || echo "    (Docker volumes/containers? reinstall with --docker)"
else
  echo "==> installed. Enroll with:"
  echo "    sudo -u backify-bridge $BIN enroll --token <TOKEN>"
  echo "    systemctl enable --now backify-bridge"
fi
