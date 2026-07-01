#!/bin/zsh
# 机器端 agent 自测：build → on-stop 上传 → on-start 拉习惯 → 查库。
export PATH=/opt/homebrew/opt/postgresql@16/bin:/opt/homebrew/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
export GOTOOLCHAIN=local
cd /Users/wust_lh/Continuum || exit 1

export CONTINUUM_SERVER_URL=http://localhost:8088
export CONTINUUM_TOKEN=cont_dev_8f3a9e2b1d8c

echo "== BUILD agent =="
go build -o continuum-agent ./cmd/continuum-agent || { echo BUILD_FAILED; exit 1; }

if ! curl -s -m3 http://localhost:8088/healthz >/dev/null; then
  export CONTINUUM_DB_DSN="postgres://wust_lh@localhost:5432/continuum?sslmode=disable"
  export CONTINUUM_BEARER_TOKEN=cont_dev_8f3a9e2b1d8c
  export CONTINUUM_LISTEN_ADDR=":8088"
  nohup ./continuum-server > /tmp/continuum.log 2>&1 &
  sleep 3
fi

cat > /tmp/agent_sess.jsonl <<'EOF'
{"timestamp":"2026-06-20T10:00:00.000Z","type":"session_meta","payload":{"id":"agent-test-1","cwd":"/Users/wust_lh/Continuum"}}
{"timestamp":"2026-06-20T10:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"agent on-stop test"}]}}
EOF

echo "== agent on-stop (上传会话) =="
./continuum-agent on-stop --tool codex --session /tmp/agent_sess.jsonl

echo "== agent on-start (拉取合成习惯，将来由 SessionStart hook 注入) =="
./continuum-agent on-start --tool codex --model gpt-5.5

echo "== DB: 所有会话 =="
psql -d continuum -A -t -c "SELECT source_session_id, cwd, message_count FROM sessions ORDER BY id"
