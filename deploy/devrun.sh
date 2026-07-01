#!/bin/zsh
# Continuum 开发期：编译 + 重启服务 + 冒烟测试。一切在脚本内完成，规避 SSH 多层引号传输问题。
export PATH=/opt/homebrew/opt/postgresql@16/bin:/opt/homebrew/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
export GOTOOLCHAIN=local
cd /Users/wust_lh/Continuum || exit 1

TOKEN=cont_dev_8f3a9e2b1d8c
export CONTINUUM_DB_DSN="postgres://wust_lh@localhost:5432/continuum?sslmode=disable"
export CONTINUUM_BEARER_TOKEN="$TOKEN"
export CONTINUUM_LISTEN_ADDR=":8088"

echo "== BUILD =="
go build -o continuum-server ./cmd/continuum-server || { echo BUILD_FAILED; exit 1; }

pkill -f continuum-server 2>/dev/null
sleep 1
nohup ./continuum-server > /tmp/continuum.log 2>&1 &
sleep 3

echo "== HEALTH (no auth needed) =="
curl -s -m5 http://localhost:8088/healthz; echo

echo "== /api/ping NO token (expect 401) =="
curl -s -m5 -o /dev/null -w "code=%{http_code}" http://localhost:8088/api/ping; echo

echo "== /api/ping WITH token (expect {ok:true}) =="
curl -s -m5 -H "Authorization: Bearer $TOKEN" http://localhost:8088/api/ping; echo

echo "== M1 set_habit (user + tool:codex) =="
curl -s -m5 -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"scope_type":"user","scope_key":"","content":"我是 JayCRL，偏好简体中文、直接。"}' http://localhost:8088/api/habits; echo
curl -s -m5 -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"scope_type":"tool","scope_key":"codex","content":"Codex 专属：用 commentary 频道更新进度。"}' http://localhost:8088/api/habits; echo
echo "== M1 get_habits (tool=codex, model=gpt-5.5) =="
curl -s -m5 -H "Authorization: Bearer $TOKEN" "http://localhost:8088/api/habits?tool=codex&model=gpt-5.5"; echo
echo "== M1 workspaces upsert + list =="
curl -s -m5 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"name":"continuum","path_globs":["Continuum"],"git_branch":"main"}' http://localhost:8088/api/workspaces; echo
curl -s -m5 -H "Authorization: Bearer $TOKEN" http://localhost:8088/api/workspaces; echo
echo "== SERVER LOG =="
tail -8 /tmp/continuum.log
echo "== TOKEN (for reference) =="
echo "$TOKEN"
