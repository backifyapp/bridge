#!/usr/bin/env sh
# Instalador do Backify Bridge.
#   curl -fsSL https://raw.githubusercontent.com/backifyapp/bridge/main/install.sh | sh -s -- --token <TOKEN>
#
# Baixa o binário do GitHub Releases, instala em /usr/local/bin, cria o usuário
# dedicado + o serviço systemd, e (se --token for passado) registra o servidor.
set -eu

REPO="backifyapp/bridge"
BIN="/usr/local/bin/backify-bridge"
API_URL="${BACKIFY_API_URL:-https://api.backify.app}"
TOKEN=""

while [ $# -gt 0 ]; do
  case "$1" in
    --token) TOKEN="$2"; shift 2 ;;
    --url)   API_URL="$2"; shift 2 ;;
    *) echo "flag desconhecida: $1" >&2; exit 2 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "rode como root (sudo)." >&2
  exit 1
fi

# Detecta OS/arch (v1: só Linux amd64/arm64).
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "arquitetura não suportada: $ARCH" >&2; exit 1 ;;
esac
if [ "$OS" != "linux" ]; then
  echo "o Bridge v1 só roda em Linux (detectado: $OS)." >&2
  exit 1
fi

echo "==> baixando backify-bridge ($OS/$ARCH)…"
URL="https://github.com/${REPO}/releases/latest/download/backify-bridge_${OS}_${ARCH}"
curl -fsSL "$URL" -o "$BIN"
chmod +x "$BIN"

echo "==> criando usuário dedicado backify-bridge…"
id backify-bridge >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin backify-bridge
install -d -o backify-bridge -g backify-bridge -m 0700 /etc/backify-bridge

echo "==> instalando serviço systemd…"
curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/packaging/systemd/backify-bridge.service" \
  -o /etc/systemd/system/backify-bridge.service
systemctl daemon-reload

if [ -n "$TOKEN" ]; then
  echo "==> registrando este servidor no Backify…"
  sudo -u backify-bridge "$BIN" enroll --token "$TOKEN" --url "$API_URL"
  systemctl enable --now backify-bridge
  echo "==> pronto. status: systemctl status backify-bridge"
else
  echo "==> instalado. Registre com:"
  echo "    sudo -u backify-bridge $BIN enroll --token <TOKEN>"
  echo "    systemctl enable --now backify-bridge"
fi
