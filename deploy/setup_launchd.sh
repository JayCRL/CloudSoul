#!/bin/zsh
# 安装/重载 continuum-server 的 launchd 服务（开机自启 + 崩溃重启）。
PLIST=/Users/wust_lh/Library/LaunchAgents/com.continuum.server.plist
echo "== 停掉 nohup 的旧进程 =="
pkill -f continuum-server 2>/dev/null
sleep 1
echo "== 重载 launchd =="
launchctl unload "$PLIST" 2>/dev/null
launchctl load -w "$PLIST"
sleep 3
echo -n "healthz: "; curl -s -m5 http://localhost:8088/healthz; echo
echo "== launchd 状态（PID 非 - 即在跑）=="
launchctl list | grep continuum
