#!/usr/bin/env sh
# Backify Bridge installer.
#   curl -fsSL https://raw.githubusercontent.com/backifyapp/bridge/main/install.sh | sh -s -- --token <TOKEN>
#
# Downloads the binary from GitHub Releases, installs it into /usr/local/bin,
# creates the dedicated user + systemd service and, if --token is given,
# enrolls this server.
set -eu

REPO="backifyapp/bridge"
BIN="/usr/local/bin/backify-bridge"
API_URL="${BACKIFY_API_URL:-https://api.backify.app}"
TOKEN=""

while [ $# -gt 0 ]; do
  case "$1" in
    --token) TOKEN="$2"; shift 2 ;;
    --url)   API_URL="$2"; shift 2 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "please run as root (sudo)." >&2
  exit 1
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
install -d -o backify-bridge -g backify-bridge -m 0700 /etc/backify-bridge

echo "==> installing systemd service…"
curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/packaging/systemd/backify-bridge.service" \
  -o /etc/systemd/system/backify-bridge.service
systemctl daemon-reload

if [ -n "$TOKEN" ]; then
  echo "==> enrolling this server with Backify…"
  sudo -u backify-bridge "$BIN" enroll --token "$TOKEN" --url "$API_URL"
  systemctl enable --now backify-bridge
  echo "==> done. status: systemctl status backify-bridge"
else
  echo "==> installed. Enroll with:"
  echo "    sudo -u backify-bridge $BIN enroll --token <TOKEN>"
  echo "    systemctl enable --now backify-bridge"
fi
