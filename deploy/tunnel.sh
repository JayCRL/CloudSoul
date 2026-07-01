#!/bin/bash
# Continuum 隧道保活：本机 localhost:8088 -> Mac mini:8088（经 8.162.0.88:22176），断线自动重连。
# 用专用密钥 id_continuum 免密。开机自启见 deploy/install-tunnel-autostart.md。
while true; do
  ssh -i "$HOME/.ssh/id_continuum" -N \
    -L 8088:localhost:8088 -p 22176 \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    -o IdentitiesOnly=yes \
    -o ServerAliveInterval=30 -o ServerAliveCountMax=3 \
    -o ExitOnForwardFailure=yes \
    wust_lh@8.162.0.88
  echo "[continuum-tunnel] dropped, reconnecting in 5s..." >&2
  sleep 5
done
