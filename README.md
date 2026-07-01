# CloudSoul

> **你的 AI 工作区灵魂，跨机器、跨工具，如影随形。**

在 A 机器用 Claude Code 干到一半，换 B 机器用 Codex 打开——CloudSoul 让你的习惯、会话记忆、工作进度自动跟上，不需要手动交代任何事。

---

## 这是什么

你用 Claude Code 写代码，用 Codex 做设计，在不同机器上切来切去。每次换工具或换机器，都要重新交代"我是谁、我在做什么、做到哪了"。

CloudSoul 在你自己的服务器上建了一个**云端灵魂**，记住三样东西：

| 层 | 内容 | 例子 |
|---|---|---|
| **习惯** | 你的决策风格、每个工具/模型的偏好 | "偏好简体中文"、"Codex 用 commentary 报进度" |
| **记忆** | 所有会话的完整归档，可全文搜索 | "上个月那个 JWT 中间件怎么写的来着？" |
| **进度** | AI 自动压缩的交接上下文 | 目标 / 当前进度 / 改了什么 / 下一步 / 踩的坑 |

**下次打开 Claude Code 或 Codex，这些自动注入到会话里——无感接上。**

---

## 怎么工作的

```
你打开 AI 工具
    ↓ SessionStart hook 自动触发
    ↓ cloudsoul-agent 从云端查 git remote → git pull 拉最新代码
    ↓ 拉取：合成习惯 + 最新交接上下文
    ↓ 自动注入会话（Claude Code additionalContext / Codex AGENTS.md）
你开始干活——代码、习惯、进度全接上了
    ↓
    ... 干活中也能调 MCP 工具：搜历史会话、存档、接受 AI 习惯建议 ...
    ↓
你结束会话
    ↓ Stop hook 自动触发
    ↓ cloudsoul-agent 读本地会话 JSONL → 上传归档
    ↓ 记录当前 git remote/branch/commit 到云端
    ↓ AI 自动压缩成结构化交接文档（目标/进度/下一步/坑）
    ↓ 下次打开任意工具任意机器 → 代码自动拉 + 上下文注入
```

**不需要手敲任何命令。** Hook 全自动触发。

---

## 架构

```
你的服务器（Mac mini / 云主机）
┌────────────────────────────────────────┐
│  cloudshift（单个 Go 二进制）            │
│   ├── /api/*   REST API                │
│   ├── /mcp     MCP Server (Streamable HTTP) │
│   └── PostgreSQL（JSONB + 全文搜索）     │
└────────────────────────────────────────┘
       ▲ HTTPS + bearer token
       │
┌──────┴──────┐  ┌──────────────┐
│ 机器 A       │  │ 机器 B        │
│ Claude Code  │  │ Codex         │
│  hooks → agent│  │  hooks → agent│
│  MCP → /mcp  │  │  MCP → /mcp   │
└─────────────┘  └──────────────┘
```

---

## 快速开始

### 1. 在你的服务器上

需要 **Go 1.23+** 与 **PostgreSQL 16+**。

```bash
git clone git@github.com:JayCRL/CloudSoul.git
cd CloudSoul

# 配置环境变量
cp deploy/.env.example .env
# 编辑 .env：填 PostgreSQL DSN、Bearer token、可选的 AI endpoint

# 建库 & 迁移
createdb cloudsoul
psql -d cloudsoul -f migrations/000001_init.up.sql

# 编译 & 启动
go build -o cloudshift ./cmd/cloudshift
./cloudshift
```

### 2. 在每台机器上

```bash
# 编译机器端 agent（或从 releases 下载）
GOOS=windows GOARCH=amd64 go build -o cloudagent.exe ./cmd/cloudagent

# 创建配置
echo '{"server_url":"http://localhost:8088","token":"你的token"}' > ~/.cloudsoul.json
```

### 3. 配 AI 工具的 hook

**Claude Code** (`~/.claude/settings.json`)：
```json
"hooks": {
  "SessionStart": [{ "hooks": [{ "type": "command", "command": "D:\\CloudSoul\\cloudagent.exe on-start --tool claude-code --format claude" }] }],
  "Stop": [{ "hooks": [{ "type": "command", "command": "D:\\CloudSoul\\cloudagent.exe on-stop --tool claude-code" }] }]
}
```

**Codex** (`~/.codex/config.toml`)：
```toml
[mcp_servers.cloudsoul]
type = "http"
url = "http://localhost:8088/mcp"
headers = { Authorization = "Bearer 你的token" }

[hooks]
[[hooks.SessionStart]]
[[hooks.SessionStart.hooks]]
type = "command"
command = 'D:\\CloudSoul\\cloudagent.exe'
args = ["on-start", "--tool", "codex", "--format", "text"]
```

---

## MCP 工具集（模型对话中可调用）

| 工具 | 用途 |
|---|---|
| `get_habits` | 查看当前项目/工具/模型下的合成习惯 |
| `set_habit` | 写入或更新一层习惯 |
| `get_handoff` | 取最新交接上下文（换机换工具接续用） |
| `save_handoff` | 手动存档当前进度 |
| `search_sessions` | 全文搜索历史会话 |
| `list_workspaces` | 列出所有项目 |
| `suggest_habits` | AI 从历史会话提炼习惯候选 |
| `list_habit_suggestions` | 列出待确认的 AI 建议 |
| `accept_habit` / `reject_habit` | 确认或拒绝候选（accept 后自动写入对应分层） |
| `ping` | 健康检查 |

---

## 跨机器代码同步

不需要手动 `git pull`。收工时 agent 自动把 `(remote, branch, commit)` 记到云端；开工时自动比对并拉取：

- **机器 B 上已有代码** → 自动 `git fetch && git merge --ff-only` 拉到最新
- **机器 B 上还没有代码** → 自动 `git clone` 到相同路径
- **换了仓库** → 不自动操作（remote 不匹配时安全跳过）

代码同步走的是标准 Git，不改工作区、不强制覆盖、不自动处理冲突。

## 习惯分层模型

习惯不只是一坨全局配置——它**分层叠加**，启动时按你此刻的「项目×工具×模型」合成最终配置。

**覆盖链**（后覆盖前，工具/模型优先于项目）：
```
user → workspace → tool → model → ws_tool → ws_model
```

例：user 层"偏好中文" → tool:codex 层"用 commentary 报进度" → 最终习惯里两条都有。

---

## 项目结构

```
cmd/
  cloudshift/      # 服务端：REST API + MCP Server + PostgreSQL
  cloudagent/      # 机器端：hook 桥接（SessionStart取习惯+handoff，Stop上传会话）
internal/
  api/             # REST handlers + bearer 认证中间件
  mcp/             # MCP Server 工具注册与实现
  habits/          # 分层习惯合成引擎
  sessions/        # 中立会话格式 + Claude/Codex adapter
  handoff/         # AI 交接上下文生成
  ai/              # OpenAI 兼容 chat client + 习惯提炼
  store/           # pgx 数据访问层
  config/          # 环境变量配置
migrations/        # PostgreSQL 迁移 SQL
deploy/            # launchd plist / Caddyfile / 隧道脚本 / 测试脚本
testdata/          # 脱敏会话样本（adapter 单测用）
```

---

## 技术栈

| 层 | 选择 |
|---|---|
| 语言 | Go 1.23+（单二进制，无运行时依赖） |
| MCP SDK | `modelcontextprotocol/go-sdk` v1.6.x（官方，Streamable HTTP） |
| HTTP | 标准库 `net/http`（Go 1.22+ 增强路由） |
| 数据库 | PostgreSQL + `jackc/pgx` v5 + JSONB |
| 迁移 | `golang-migrate` |
| 反代/TLS | Caddy（自动 HTTPS） |
| AI 压缩 | 任意 OpenAI 兼容 API（DeepSeek / OpenAI / Anthropic） |

---

## 许可

MIT
