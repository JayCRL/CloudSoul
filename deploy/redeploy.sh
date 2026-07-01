#!/bin/zsh
# 重新部署：build+重启 server，交叉编译 Windows agent.exe。
export PATH=/opt/homebrew/opt/postgresql@16/bin:/opt/homebrew/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
export GOTOOLCHAIN=local
cd /Users/wust_lh/Continuum || exit 1
export CONTINUUM_DB_DSN="postgres://wust_lh@localhost:5432/continuum?sslmode=disable"
export CONTINUUM_BEARER_TOKEN=cont_dev_8f3a9e2b1d8c
export CONTINUUM_LISTEN_ADDR=":8088"

echo "== BUILD server =="
go build -o continuum-server ./cmd/continuum-server || { echo BUILD_FAILED; exit 1; }
pkill -f continuum-server 2>/dev/null
sleep 1
nohup ./continuum-server > /tmp/continuum.log 2>&1 &
sleep 2

echo "== CROSS-BUILD agent.exe (windows/amd64) =="
GOOS=windows GOARCH=amd64 go build -o continuum-agent.exe ./cmd/continuum-agent || { echo AGENT_BUILD_FAILED; exit 1; }
echo BUILT_ALL
echo -n "healthz: "; curl -s -m5 http://localhost:8088/healthz; echo
