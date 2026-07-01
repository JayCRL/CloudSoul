#!/bin/zsh
# M4 自测：搜索会话 + AI 提炼习惯建议 + accept/reject。
export PATH=/opt/homebrew/opt/postgresql@16/bin:/opt/homebrew/bin:$PATH
TOKEN=cont_dev_8f3a9e2b1d8c
echo "== SEARCH for JWT =="
curl -s -m5 -H "Authorization: Bearer $TOKEN" "http://localhost:8088/api/search?q=JWT&workspace=continuum"
echo
echo "== AI SUGGEST habits =="
curl -s -m35 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"workspace":"continuum"}' http://localhost:8088/api/suggestions/ai
echo
echo "== LIST suggestions =="
curl -s -m5 -H "Authorization: Bearer $TOKEN" http://localhost:8088/api/suggestions
echo
echo "== DONE =="
