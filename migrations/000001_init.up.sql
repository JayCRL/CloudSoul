-- Continuum 初始 schema
-- 数据归属主干：用户(单用户，隐含) → 项目(workspaces) → 会话(sessions) → 消息(session_messages)
-- 习惯分层：habit_layers(user/workspace/tool/model/ws_tool/ws_model)

CREATE TABLE workspaces (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    path_globs  TEXT[] NOT NULL DEFAULT '{}',   -- cwd → workspace 的匹配规则
    git_remote  TEXT,
    git_branch  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE habit_layers (
    id          BIGSERIAL PRIMARY KEY,
    scope_type  TEXT NOT NULL CHECK (scope_type IN ('user','workspace','tool','model','ws_tool','ws_model')),
    scope_key   TEXT NOT NULL DEFAULT '',       -- ''(user) / "lingxi" / "codex" / "opus" / "lingxi|codex"
    content     TEXT NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_type, scope_key)
);

CREATE TABLE sessions (
    id                BIGSERIAL PRIMARY KEY,
    source_tool       TEXT NOT NULL,            -- 'claude-code' | 'codex'
    source_session_id TEXT NOT NULL,
    workspace_id      BIGINT REFERENCES workspaces(id) ON DELETE SET NULL,
    cwd               TEXT,
    model             TEXT,
    started_at        TIMESTAMPTZ,
    ended_at          TIMESTAMPTZ,
    message_count     INT NOT NULL DEFAULT 0,
    raw_blob          JSONB,                    -- 原始 JSONL 记录数组（全量归档）
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_tool, source_session_id)     -- 幂等上传
);
CREATE INDEX idx_sessions_workspace ON sessions(workspace_id);
CREATE INDEX idx_sessions_started   ON sessions(started_at DESC);

CREATE TABLE session_messages (
    id          BIGSERIAL PRIMARY KEY,
    session_id  BIGINT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq         INT NOT NULL,
    role        TEXT NOT NULL,                  -- user | assistant | tool
    content     JSONB NOT NULL,                 -- 中立 blocks
    ts          TIMESTAMPTZ,
    UNIQUE (session_id, seq)
);
CREATE INDEX idx_msgs_session ON session_messages(session_id);

CREATE TABLE handoffs (
    id                BIGSERIAL PRIMARY KEY,
    workspace_id      BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    content           TEXT NOT NULL,
    source_session_id BIGINT REFERENCES sessions(id) ON DELETE SET NULL,
    is_manual         BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()  -- 同 workspace 最新一条 = 当前交接
);
CREATE INDEX idx_handoffs_ws_created ON handoffs(workspace_id, created_at DESC);

CREATE TABLE habit_suggestions (
    id                BIGSERIAL PRIMARY KEY,
    workspace_id      BIGINT REFERENCES workspaces(id) ON DELETE CASCADE,
    scope_type        TEXT NOT NULL,
    scope_key         TEXT NOT NULL DEFAULT '',
    suggested_content TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','rejected')),
    source_session_id BIGINT REFERENCES sessions(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_suggestions_status ON habit_suggestions(status);
