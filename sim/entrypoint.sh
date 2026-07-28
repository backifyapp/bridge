#!/bin/sh
# Enroll against mock-cp, then run the daemon (keeps the tunnel up).
set -e
export BACKIFY_BRIDGE_CONFIG=/tmp/bridge.json

echo "[bridge] fazendo enroll no mock-cp..."
until backify-bridge enroll --token simtoken --url http://mock-cp:8080; do
  echo "[bridge] mock-cp unavailable, retrying in 2s..."
  sleep 2
done

echo "[bridge] enrolled. starting daemon (opens the reverse tunnel)..."
exec backify-bridge run
