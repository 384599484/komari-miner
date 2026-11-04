#!/usr/bin/env bash
# Build the whole project:
#   web-src (React) -> dist.tar.zst -> embedded theme -> server binary
# and optionally the agent binary.
#
# Usage:
#   scripts/build.sh              # server + agent
#   scripts/build.sh server       # server only
#   scripts/build.sh agent        # agent only
#   SKIP_WEB=1 scripts/build.sh   # reuse existing dist.tar.zst
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${1:-all}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: $1 not found in PATH" >&2
    exit 1
  }
}

build_web() {
  echo "==> building frontend (web-src)"
  need npm
  pushd "$ROOT/web-src" >/dev/null
  if [ ! -d node_modules ]; then
    npm ci --no-fund --no-audit
  fi
  # Low-memory hosts (2 GB) need a capped heap or vite gets OOM-killed.
  NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=1536}" npm run build
  popd >/dev/null
}

pack_theme() {
  echo "==> packing theme archive"
  pushd "$ROOT/server" >/dev/null
  mkdir -p web/public/defaultTheme
  go run ./tools/packdist \
    -src "$ROOT/web-src/dist" \
    -dst web/public/defaultTheme/dist.tar.zst
  cp "$ROOT/web-src/komari-theme.json" web/public/defaultTheme/
  popd >/dev/null
}

build_server() {
  if [ "${SKIP_WEB:-0}" != "1" ]; then
    build_web
    pack_theme
  else
    echo "==> SKIP_WEB=1, reusing existing theme archive"
  fi
  echo "==> building server"
  need go
  pushd "$ROOT/server" >/dev/null
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o komari-server .
  popd >/dev/null
  echo "    $ROOT/server/komari-server"
}

build_agent() {
  echo "==> building agent"
  need go
  pushd "$ROOT/agent" >/dev/null
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o komari-miner-probe .
  popd >/dev/null
  echo "    $ROOT/agent/komari-miner-probe"
}

case "$TARGET" in
  all)    build_server; build_agent ;;
  server) build_server ;;
  agent)  build_agent ;;
  web)    build_web; pack_theme ;;
  *)      echo "usage: $0 [all|server|agent|web]" >&2; exit 2 ;;
esac

echo "done."
