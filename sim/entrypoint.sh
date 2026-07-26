#!/bin/sh
# Enroll no mock-cp e depois roda o daemon (mantém o túnel).
set -e
export BACKIFY_BRIDGE_CONFIG=/tmp/bridge.json

echo "[bridge] fazendo enroll no mock-cp..."
until backify-bridge enroll --token simtoken --url http://mock-cp:8080; do
  echo "[bridge] mock-cp indisponível, retry em 2s..."
  sleep 2
done

echo "[bridge] enrolled. iniciando daemon (abre o túnel reverso)..."
exec backify-bridge run
