#!/bin/bash
set -euo pipefail
mkdir -p "$HOME"
pids=()
cleanup(){ trap - TERM INT EXIT; kill -TERM "${pids[@]}" 2>/dev/null || true; wait || true; }
trap cleanup TERM INT EXIT
Xvfb :99 -screen 0 1728x1000x24 -nolisten tcp & pids+=("$!")
for i in {1..100}; do if xdpyinfo -display :99 >/dev/null 2>&1; then break; fi; sleep .1; done
xdpyinfo -display :99 >/dev/null
openbox >/tmp/openbox.log 2>&1 & pids+=("$!")
# VNC stays inside the container. Only the authenticated worker and Go's
# account-scoped desktop proxy are published to the host loopback interface.
x11vnc -display :99 -localhost -rfbport 5900 -forever -nevershared -dontdisconnect -nopw -noxdamage >/tmp/vnc.log 2>&1 & pids+=("$!")
websockify --web=/usr/share/novnc 6080 localhost:5900 >/tmp/websockify.log 2>&1 & pids+=("$!")
node /app/browser/worker.mjs & pids+=("$!")
wait -n "${pids[@]}"
exit 1
