#!/bin/zsh
# 验证 UpsertWorkspace 空值不覆盖 + cwd 匹配。
export PATH=/opt/homebrew/opt/postgresql@16/bin:/opt/homebrew/bin:$PATH
TOKEN=cont_dev_8f3a9e2b1d8c

echo "== 重设 workspace path_globs=[Continuum] =="
curl -s -m5 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"name":"continuum","path_globs":["Continuum"]}' http://localhost:8088/api/workspaces; echo

echo "== save_handoff（只传 name）后，path_globs 应仍在 =="
curl -s -m5 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"workspace":"continuum","content":"cwd 测试交接"}' http://localhost:8088/api/handoff; echo
curl -s -m5 -H "Authorization: Bearer $TOKEN" http://localhost:8088/api/workspaces; echo

echo "== handoff by cwd（应返回，不再为空）=="
curl -s -m5 -H "Authorization: Bearer $TOKEN" "http://localhost:8088/api/handoff?cwd=/Users/wust_lh/Continuum"; echo
