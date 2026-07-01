// continuum-agent：机器端桥接，由工具的 hook 自动调用（非用户手敲）。
//
//	on-stop  --tool <t> [--session <jsonl>]     收工：上传本次会话（session 优先取 hook stdin 的 transcript_path）
//	on-start --tool <t> [--cwd dir] [--model m] [--format text|claude]
//	         开工：拉取合成习惯 + 最新 handoff（cwd 优先取 hook stdin），按 format 输出供注入
//
// 配置：优先读 ~/.continuum.json {"server_url","token"}，可被环境变量
// CONTINUUM_SERVER_URL / CONTINUUM_TOKEN 覆盖。
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
		if b, err := os.ReadFile(filepath.Join(home, ".continuum.json")); err == nil {
			_ = json.Unmarshal(b, &cfg)
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

// hookInput 是 Claude Code / Codex hook 通过 stdin 传入的 JSON（取需要的字段）。
type hookInput struct {
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	HookEventName  string `json:"hook_event_name"`
}

// readHookInput 仅当 stdin 是 pipe（hook 传入）时读取，tty 下返回 nil，避免手动运行时阻塞。
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

func main() {
	loadConfig()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: continuum-agent <on-stop|on-start> [flags]")
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

// onStop 上传本次会话归档。session 路径优先取 hook stdin 的 transcript_path。
func onStop(args []string) {
	fs := flag.NewFlagSet("on-stop", flag.ExitOnError)
	tool := fs.String("tool", "", "claude-code | codex")
	session := fs.String("session", "", "会话 JSONL 文件路径（无则取 hook stdin）")
	_ = fs.Parse(args)

	hi := readHookInput()
	path := *session
	if hi != nil && hi.TranscriptPath != "" {
		path = hi.TranscriptPath
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
}

// onStart 拉取合成习惯 + 最新 handoff，按 format 输出供注入。cwd 优先取 hook stdin。
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
			buf.WriteString(out.Habits)
		}
	} else {
		fmt.Fprintln(os.Stderr, "get habits:", err)
	}

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
				buf.WriteString("<!-- continuum:handoff -->\n")
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
