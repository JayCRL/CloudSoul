#!/bin/zsh
# M3 handoff 自测：上传自动生成 handoff → 取回 → agent on-start 注入(习惯+handoff) → 手动覆盖。
export PATH=/opt/homebrew/opt/postgresql@16/bin:/opt/homebrew/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
export GOTOOLCHAIN=local
cd /Users/wust_lh/Continuum || exit 1

TOKEN=cont_dev_8f3a9e2b1d8c
export CONTINUUM_DB_DSN="postgres://wust_lh@localhost:5432/continuum?sslmode=disable"
export CONTINUUM_BEARER_TOKEN="$TOKEN"
export CONTINUUM_LISTEN_ADDR=":8088"
export CONTINUUM_SERVER_URL=http://localhost:8088
export CONTINUUM_TOKEN="$TOKEN"

echo "== BUILD server + agent =="
go build -o continuum-server ./cmd/continuum-server || { echo BUILD_FAILED; exit 1; }
go build -o continuum-agent ./cmd/continuum-agent || { echo BUILD_FAILED; exit 1; }
pkill -f continuum-server 2>/dev/null
sleep 1
nohup ./continuum-server > /tmp/continuum.log 2>&1 &
sleep 3

cat > /tmp/m3_sess.jsonl <<'EOF'
{"timestamp":"2026-06-21T09:00:00.000Z","type":"session_meta","payload":{"id":"m3-sess-1","cwd":"/Users/wust_lh/Continuum"}}
{"timestamp":"2026-06-21T09:00:01.000Z","type":"turn_context","payload":{"model":"gpt-5.5"}}
{"timestamp":"2026-06-21T09:00:02.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"帮我把 handoff 的自动生成接到 upload 上"}]}}
{"timestamp":"2026-06-21T09:00:30.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"已在 uploadSession 里加了同步生成，改动 handlers.go 和 generate.go，下一步测端到端"}]}}
EOF

python3 -c 'import json; print(json.dumps({"tool":"codex","raw":open("/tmp/m3_sess.jsonl").read()}))' > /tmp/m3_body.json

echo "== UPLOAD (应 handoff_generated:true) =="
curl -s -m10 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d @/tmp/m3_body.json http://localhost:8088/api/sessions/upload; echo

echo "== GET handoff (workspace=continuum) =="
curl -s -m5 -H "Authorization: Bearer $TOKEN" "http://localhost:8088/api/handoff?workspace=continuum"; echo

echo "== agent on-start (习惯 + handoff 一起注入，这就是开工自动接上的内容) =="
./continuum-agent on-start --tool codex --model gpt-5.5 --workspace continuum

echo "== 手动 save_handoff 覆盖 + 再取 =="
curl -s -m5 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"workspace":"continuum","content":"手动交接：M3 完成，下一步 M4 检索。"}' http://localhost:8088/api/handoff; echo
curl -s -m5 -H "Authorization: Bearer $TOKEN" "http://localhost:8088/api/handoff?workspace=continuum"; echo
