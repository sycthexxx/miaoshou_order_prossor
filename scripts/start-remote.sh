#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
export ORDER_LISTEN="${ORDER_LISTEN:-0.0.0.0:17381}"
export ORDER_BROWSER_IMAGE="${ORDER_BROWSER_IMAGE:-order-processor-browser:1.63.0}"
# Prepare before Docker COPY: recover missing/incomplete locks without
# borrowing this host's node_modules into the Linux image.
node scripts/prepare-lock.mjs
if ! command -v docker >/dev/null 2>&1; then
  echo '未找到 Docker：Mac 请先安装并启动 Docker Desktop。' >&2
  exit 1
fi
if ! docker info --format '{{.ServerVersion}}' >/dev/null; then
  echo 'Docker 服务不可用。请启动 Docker Desktop，并在当前终端确认 docker info 可执行。' >&2
  exit 1
fi
if [ ! -d node_modules ]; then npm ci; fi
npm run build
if ! docker image inspect "$ORDER_BROWSER_IMAGE" >/dev/null 2>&1; then
  docker build -f deploy/browser/Dockerfile -t "$ORDER_BROWSER_IMAGE" .
fi
exec go run .
