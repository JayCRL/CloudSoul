// cloudagent：机器端桥接，由工具的 hook 自动调用（非用户手敲）。
//
//	on-stop  --tool <t> [--session <jsonl>]     收工：上传会话 + 同步 git 指针到云端
//	on-start --tool <t> [--cwd dir] [--model m] [--format text|claude]
//	         开工：git pull 拉最新代码 → 拉取习惯+handoff → 注入上下文
//
// 配置：优先读 ~/.cloudsoul.json {"server_url","token"}，可被环境变量覆盖。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type agentConfig struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
}

var cfg agentConfig

func loadConfig() {
	if home, err := os.UserHomeDir(); err == nil {
		for _, name := range []string{".cloudsoul.json", ".continuum.json"} {
			if b, err := os.ReadFile(filepath.Join(home, name)); err == nil {
				_ = json.Unmarshal(b, &cfg)
				break
			}
		}
	}
	if v := os.Getenv("CONTINUUM_SERVER_URL"); v != "" {
		cfg.ServerURL = v
	}
	if v := os.Getenv("CONTINUUM_TOKEN"); v != "" {
		cfg.Token = v
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = "http://localhost:8088"
	}
}

type hookInput struct {
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	HookEventName  string `json:"hook_event_name"`
}

func readHookInput() *hookInput {
	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return nil
	}
	b, _ := io.ReadAll(os.Stdin)
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	var h hookInput
	if json.Unmarshal(b, &h) != nil {
		return nil
	}
	return &h
}

func httpDo(method, path string, body []byte) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, cfg.ServerURL+path, r)
	if err != nil {
		return nil, err
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return b, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}

// --------------- git helpers ---------------

type gitInfo struct {
	Remote string // origin URL
	Branch string // current branch
	Commit string // HEAD hash
}

// findGit 在 Windows/Linux/macOS 上搜 git 可执行文件。
func findGit() string {
	if g, err := exec.LookPath("git"); err == nil {
		return g
	}
	for _, p := range []string{
		`D:\Git\bin\git.exe`,
		`D:\Git\mingw64\bin\git.exe`,
		`C:\Program Files\Git\bin\git.exe`,
		`C:\Program Files\Git\cmd\git.exe`,
		`/usr/bin/git`,
		`/opt/homebrew/bin/git`,
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// readGit 在 dir 下读 git 信息（dir 为空则跳过）。git 不可用或非 git 目录时返回 nil。
func readGit(dir string) *gitInfo {
	if dir == "" {
		return nil
	}
	g := findGit()
	if g == "" {
		return nil
	}
	run := func(args ...string) string {
		c := exec.Command(g, args...)
		c.Dir = dir
		out, _ := c.Output()
		return strings.TrimSpace(string(out))
	}
	remote := run("remote", "get-url", "origin")
	if remote == "" {
		return nil
	}
	return &gitInfo{
		Remote: remote,
		Branch: run("rev-parse", "--abbrev-ref", "HEAD"),
		Commit: run("rev-parse", "HEAD"),
	}
}

// syncGitToServer 把 git 指针写入服务器对应 workspace。
func syncGitToServer(gi *gitInfo, cwd string) {
	if gi == nil {
		return
	}
	// 按 cwd 匹配 workspace 并更新 git 字段
	body, _ := json.Marshal(map[string]string{
		"cwd":        cwd,
		"git_remote": gi.Remote,
		"git_branch": gi.Branch,
	})
	httpDo("POST", "/api/workspaces/sync-git", body)
}

// syncCodePull 开工时拉取最新代码。返回状态描述（注入上下文用）。
// 逻辑：目录不存在且 workspace 有 git_remote → clone；存在且 remote 匹配 → pull。
func syncCodePull(cwd string) string {
	if cwd == "" {
		return ""
	}
	g := findGit()
	if g == "" {
		return ""
	}

	resp, err := httpDo("GET", "/api/workspaces/match?cwd="+url.QueryEscape(cwd), nil)
	if err != nil {
		return ""
	}
	var ws struct {
		Name      string `json:"name"`
		GitRemote string `json:"git_remote"`
		GitBranch string `json:"git_branch"`
	}
	if json.Unmarshal(resp, &ws) != nil || ws.GitRemote == "" {
		return ""
	}

	// 目录存在 → pull
	if fi, err := os.Stat(cwd); err == nil && fi.IsDir() {
		localRemote := runCmd(g, cwd, "remote", "get-url", "origin")
		if localRemote == "" || !remoteMatch(localRemote, ws.GitRemote) {
			return "" // 不同 repo，不自动操作
		}
		before := runCmd(g, cwd, "rev-parse", "--short", "HEAD")
		// 先 fetch 再 merge，处理本地有未推送提交的情况
		runCmd(g, cwd, "fetch", "origin")
		runCmd(g, cwd, "merge", "--ff-only", "origin/"+ws.GitBranch)
		after := runCmd(g, cwd, "rev-parse", "--short", "HEAD")
		if before != after && after != "" {
			return fmt.Sprintf("代码已更新: %s → %s (%s)", before, after, ws.GitBranch)
		}
		return fmt.Sprintf("代码已是最新: %s @ %s (%s)", after, ws.GitBranch, ws.Name)
	}

	// 目录不存在 → clone
	parent := filepath.Dir(cwd)
	os.MkdirAll(parent, 0755)
	branch := ws.GitBranch
	if branch == "" {
		branch = "main"
	}
	out := runCmd(g, parent, "clone", "-b", branch, ws.GitRemote, filepath.Base(cwd))
	if strings.Contains(out, "Cloning into") {
		commit := runCmd(g, cwd, "rev-parse", "--short", "HEAD")
		return fmt.Sprintf("代码已克隆: %s @ %s (%s)", commit, branch, ws.Name)
	}
	return ""
}

func runCmd(exe, dir string, args ...string) string {
	c := exec.Command(exe, args...)
	c.Dir = dir
	out, _ := c.Output()
	return strings.TrimSpace(string(out))
}

func remoteMatch(a, b string) bool {
	// 容忍 https:// 与 git@ 的差异，只比 path 部分
	a = strings.TrimSuffix(strings.TrimSuffix(a, ".git"), "/")
	b = strings.TrimSuffix(strings.TrimSuffix(b, ".git"), "/")
	// 提取冒号或斜杠之后的 repo 路径
	for _, s := range []string{a, b} {
		if idx := strings.LastIndex(s, ":"); idx > 0 {
			s = s[idx+1:]
		}
		if idx := strings.Index(s, "github.com/"); idx >= 0 {
			s = s[idx+11:]
		}
	}
	ap := extractRepoPath(a)
	bp := extractRepoPath(b)
	return ap == bp
}

func extractRepoPath(u string) string {
	// github.com/user/repo → user/repo
	for _, prefix := range []string{"https://github.com/", "git@github.com:", "github.com/"} {
		if after, ok := strings.CutPrefix(u, prefix); ok {
			return strings.TrimSuffix(after, ".git")
		}
	}
	return u
}

// --------------- commands ---------------

func main() {
	loadConfig()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cloudagent <on-stop|on-start> [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "on-stop":
		onStop(os.Args[2:])
	case "on-start":
		onStart(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		os.Exit(2)
	}
}

func onStop(args []string) {
	fs := flag.NewFlagSet("on-stop", flag.ExitOnError)
	tool := fs.String("tool", "", "claude-code | codex")
	session := fs.String("session", "", "会话 JSONL 文件路径（无则取 hook stdin）")
	_ = fs.Parse(args)

	hi := readHookInput()
	path := *session
	cwd := ""
	if hi != nil {
		if hi.TranscriptPath != "" {
			path = hi.TranscriptPath
		}
		cwd = hi.CWD
	}
	if *tool == "" || path == "" {
		fmt.Fprintln(os.Stderr, "on-stop: 需要 --tool 且 --session 或 hook 的 transcript_path")
		os.Exit(2)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read session:", err)
		os.Exit(1)
	}
	body, _ := json.Marshal(map[string]string{"tool": *tool, "raw": string(raw)})
	resp, err := httpDo("POST", "/api/sessions/upload", body)
	if err != nil {
		fmt.Fprintln(os.Stderr, "upload:", err)
		os.Exit(1)
	}
	fmt.Println(string(resp))

	// 同步 git 指针到云端
	if gi := readGit(cwd); gi != nil {
		syncGitToServer(gi, cwd)
	}
}

func onStart(args []string) {
	fs := flag.NewFlagSet("on-start", flag.ExitOnError)
	tool := fs.String("tool", "", "claude-code | codex")
	model := fs.String("model", "", "模型标识（可选）")
	workspace := fs.String("workspace", "", "项目名（可选）")
	cwd := fs.String("cwd", "", "当前目录（无则取 hook stdin）")
	format := fs.String("format", "text", "text | claude")
	_ = fs.Parse(args)

	hi := readHookInput()
	curCwd := *cwd
	if curCwd == "" && hi != nil {
		curCwd = hi.CWD
	}

	var buf strings.Builder

	// 0) 拉取最新代码
	if codeStatus := syncCodePull(curCwd); codeStatus != "" {
		buf.WriteString("<!-- cloudsoul:code -->\n")
		buf.WriteString(codeStatus)
	}

	// 1) 合成习惯
	hq := url.Values{}
	hq.Set("tool", *tool)
	hq.Set("model", *model)
	if *workspace != "" {
		hq.Set("workspace", *workspace)
	}
	if curCwd != "" {
		hq.Set("cwd", curCwd)
	}
	if resp, err := httpDo("GET", "/api/habits?"+hq.Encode(), nil); err == nil {
		var out struct {
			Habits string `json:"habits"`
		}
		_ = json.Unmarshal(resp, &out)
		if strings.TrimSpace(out.Habits) != "" {
			if buf.Len() > 0 {
				buf.WriteString("\n\n")
			}
			buf.WriteString(out.Habits)
		}
	} else {
		fmt.Fprintln(os.Stderr, "get habits:", err)
	}

	// 2) 最新 handoff
	if *workspace != "" || curCwd != "" {
		oq := url.Values{}
		if *workspace != "" {
			oq.Set("workspace", *workspace)
		}
		if curCwd != "" {
			oq.Set("cwd", curCwd)
		}
		if resp, err := httpDo("GET", "/api/handoff?"+oq.Encode(), nil); err == nil {
			var ho struct {
				Handoff string `json:"handoff"`
			}
			_ = json.Unmarshal(resp, &ho)
			if strings.TrimSpace(ho.Handoff) != "" {
				if buf.Len() > 0 {
					buf.WriteString("\n\n")
				}
				buf.WriteString("<!-- cloudsoul:handoff -->\n")
				buf.WriteString(ho.Handoff)
			}
		}
	}

	content := buf.String()
	if strings.TrimSpace(content) == "" {
		return
	}
	switch *format {
	case "claude":
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"hookSpecificOutput": map[string]string{
				"hookEventName":     "SessionStart",
				"additionalContext": content,
			},
		})
	default:
		fmt.Println(content)
	}
}
