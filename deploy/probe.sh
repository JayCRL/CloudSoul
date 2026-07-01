#!/bin/zsh
# 探查 Mac mini 的中转/隧道机制与监听端口。
echo "=SSH PROCS (反向隧道会是 ssh -R ... 8.162.0.88)="
ps aux | grep -i ssh | grep -v grep | grep -v sshd
echo "=OTHER TUNNELS="
ps aux | grep -iE 'frp|ngrok|tunnel|tailscale|autossh' | grep -v grep
echo "=LAUNCHD (含开机自启的隧道)="
launchctl list 2>/dev/null | grep -iE 'ssh|frp|tunnel|continuum'
echo "=~/.ssh/config="
cat ~/.ssh/config 2>/dev/null
echo "=LISTEN ports (8088/8000段/22)="
lsof -iTCP -sTCP:LISTEN -n -P 2>/dev/null | grep -E ':808|:800|:22 '
echo "=TOOLS on mac mini="
which codex claude 2>/dev/null
echo "=DONE="
