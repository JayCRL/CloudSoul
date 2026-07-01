#!/bin/zsh
# Continuum server wrapper — 从外部 env 文件加载密钥，不写入 git 仓库。
ENV_FILE="${CONTINUUM_ENV:-$HOME/.continuum.env}"
if [ -f "$ENV_FILE" ]; then
  set -a; source "$ENV_FILE"; set +a
fi
exec /Users/wust_lh/Continuum/continuum-server
