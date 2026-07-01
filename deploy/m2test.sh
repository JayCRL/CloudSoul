#!/bin/zsh
# M2 上传链路自测：build+重启服务 → 构造 codex 样本 → 上传 → 查库验证。
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

cat > /tmp/sample.jsonl <<'EOF'
{"timestamp":"2026-06-18T05:30:20.589Z","type":"session_meta","payload":{"id":"test-sess-1","cwd":"/Users/wust_lh/Continuum"}}
{"timestamp":"2026-06-18T05:30:20.631Z","type":"turn_context","payload":{"model":"gpt-5.5"}}
{"timestamp":"2026-06-18T05:30:20.630Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions>secret injected</permissions>"}]}}
{"timestamp":"2026-06-18T05:30:20.668Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi from continuum test"}]}}
{"timestamp":"2026-06-18T05:30:25.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello back"}]}}
EOF

python3 -c 'import json; print(json.dumps({"tool":"codex","raw":open("/tmp/sample.jsonl").read()}))' > /tmp/body.json

echo "== UPLOAD codex session =="
curl -s -m5 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d @/tmp/body.json http://localhost:8088/api/sessions/upload; echo

echo "== DB sessions =="
psql -d continuum -A -t -c "SELECT id, source_tool, source_session_id, coalesce(model,''), message_count FROM sessions"
echo "== DB messages (developer 注入应被过滤，只剩 user+assistant) =="
psql -d continuum -A -t -c "SELECT seq, role, content FROM session_messages ORDER BY seq"
echo "== re-upload 幂等（message_count 应不变） =="
curl -s -m5 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d @/tmp/body.json http://localhost:8088/api/sessions/upload; echo
psql -d continuum -A -t -c "SELECT count(*) as total_messages FROM session_messages"
