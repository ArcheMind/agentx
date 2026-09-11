package sessions

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltInProviders(t *testing.T) {
	service := Service{
		HomeDir: t.TempDir(),
		LookPath: func(binary string) (string, error) {
			if binary == "codex" {
				return "/bin/codex", nil
			}
			return "", os.ErrNotExist
		},
	}
	providers := service.Providers()
	if len(providers) != 5 {
		t.Fatalf("providers = %d, want 5", len(providers))
	}
	for _, provider := range providers {
		if provider.Installed != (provider.ID == "codex") {
			t.Fatalf("%s installed = %v", provider.ID, provider.Installed)
		}
	}
}

func TestNativeSessionReaders(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFixture(t, filepath.Join(home, ".claude", "projects", "project", "claude-id.jsonl"), strings.Join([]string{
		fmt.Sprintf(`{"type":"user","sessionId":"claude-id","cwd":%q,"timestamp":"2026-09-01T10:00:00Z","message":{"role":"user","content":"fix claude"}}`, workspace),
		`{"type":"assistant","sessionId":"claude-id","timestamp":"2026-09-01T10:01:00Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"done"}]}}`,
	}, "\n"))

	writeFixture(t, filepath.Join(home, ".codex", "sessions", "2026", "09", "01", "rollout.jsonl"), strings.Join([]string{
		fmt.Sprintf(`{"type":"session_meta","timestamp":"2026-09-01T11:00:00Z","payload":{"id":"codex-id","cwd":%q,"timestamp":"2026-09-01T11:00:00Z"}}`, workspace),
		`{"type":"response_item","timestamp":"2026-09-01T11:01:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"fix codex"}]}}`,
	}, "\n"))

	projectHash := fmt.Sprintf("%x", sha256.Sum256([]byte(workspace)))
	writeFixture(t, filepath.Join(home, ".gemini", "tmp", projectHash, "chats", "session-gemini-id.json"),
		`{"sessionId":"gemini-id","startTime":"2026-09-01T12:00:00Z","lastUpdated":"2026-09-01T12:01:00Z","messages":[{"type":"user","content":"fix gemini","timestamp":"2026-09-01T12:00:00Z"},{"type":"gemini","content":"done","timestamp":"2026-09-01T12:01:00Z"}]}`)

	openCodeRoot := filepath.Join(home, ".local", "share", "opencode", "project", "project", "storage", "session")
	writeFixture(t, filepath.Join(openCodeRoot, "info", "opencode-id.json"),
		`{"id":"opencode-id","title":"fix opencode","time":{"created":1788271200000,"updated":1788271260000}}`)
	writeFixture(t, filepath.Join(openCodeRoot, "message", "opencode-id", "message-id.json"),
		fmt.Sprintf(`{"id":"message-id","sessionID":"opencode-id","role":"user","path":{"root":%q},"time":{"created":1788271200000}}`, workspace))
	writeFixture(t, filepath.Join(openCodeRoot, "part", "opencode-id", "message-id", "part-id.json"),
		`{"id":"part-id","messageID":"message-id","sessionID":"opencode-id","type":"text","text":"fix opencode"}`)

	writeFixture(t, filepath.Join(home, ".pi", "agent", "sessions", "project", "pi.jsonl"), strings.Join([]string{
		fmt.Sprintf(`{"type":"session","id":"pi-id","cwd":%q,"timestamp":"2026-09-01T14:00:00Z"}`, workspace),
		`{"type":"message","message":{"role":"user","timestamp":1788271200000,"content":[{"type":"text","text":"fix pi"}]}}`,
	}, "\n"))

	service := Service{HomeDir: home}
	for _, test := range []struct {
		provider string
		id       string
		content  string
	}{
		{provider: "claude", id: "claude-id", content: "fix claude"},
		{provider: "codex", id: "codex-id", content: "fix codex"},
		{provider: "gemini", id: "gemini-id", content: "fix gemini"},
		{provider: "opencode", id: "opencode-id", content: "fix opencode"},
		{provider: "pi", id: "pi-id", content: "fix pi"},
	} {
		t.Run(test.provider, func(t *testing.T) {
			detail, err := service.Info(test.id, test.provider)
			if err != nil {
				t.Fatal(err)
			}
			if detail.Provider != test.provider || detail.MessageCount == 0 || !strings.Contains(detail.Messages[0].Content, test.content) {
				t.Fatalf("detail = %#v", detail)
			}
		})
	}

	items, err := service.List(ListOptions{Workspace: workspace, Limit: 10, Sort: "date"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Fatalf("workspace sessions = %d, want 5: %#v", len(items), items)
	}
}

func TestSessionIDMustBeUnambiguous(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".claude", "projects", "x", "one.jsonl"),
		`{"type":"user","sessionId":"same-prefix-one","message":{"content":"one"}}`)
	writeFixture(t, filepath.Join(home, ".pi", "agent", "sessions", "x", "two.jsonl"), strings.Join([]string{
		`{"type":"session","id":"same-prefix-two"}`,
		`{"type":"message","message":{"role":"user","content":"two"}}`,
	}, "\n"))
	_, err := (Service{HomeDir: home}).Info("same-prefix", "")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %v", err)
	}
}

func TestResumePromptDropsReasoningAndCapsSize(t *testing.T) {
	detail := Detail{Summary: Summary{ID: "id", Provider: "codex"}, Messages: []Message{
		{Role: "user", Content: strings.Repeat("a", resumeContextLimit)},
		{Role: "assistant", Content: "latest"},
	}}
	prompt := ResumePrompt(detail)
	if len(prompt) > resumeContextLimit || !strings.Contains(prompt, "latest") || !strings.Contains(prompt, "truncated") {
		t.Fatalf("unexpected prompt length/content: %d", len(prompt))
	}
}

func TestCodexSummarySkipsInjectedContextAndPreservesUTF8(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	path := filepath.Join(home, ".codex", "sessions", "2026", "09", "10", "session.jsonl")
	writeFixture(t, path, strings.Join([]string{
		fmt.Sprintf(`{"type":"session_meta","timestamp":"2026-09-10T20:00:00Z","payload":{"id":"codex-title","cwd":%q,"timestamp":"2026-09-10T20:00:00Z"}}`, workspace),
		`{"type":"response_item","timestamp":"2026-09-10T20:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions\n\n<INSTRUCTIONS>internal context</INSTRUCTIONS>"}]}}`,
		`{"type":"response_item","timestamp":"2026-09-10T20:00:02Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"修复中文标题并提高会话加载速度"}]}}`,
		`{"type":"response_item","timestamp":"2026-09-10T20:00:03Z","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}}`,
	}, "\n"))

	groups, err := (Service{HomeDir: home}).RecentGroups(workspace, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups.Current) != 1 || groups.Current[0].Title != "修复中文标题并提高会话加载速度" {
		t.Fatalf("recent groups = %#v", groups)
	}
	if groups.Current[0].StartedAt != "2026-09-10T20:00:00Z" {
		t.Fatalf("recent session started at = %q", groups.Current[0].StartedAt)
	}
	items, err := (Service{HomeDir: home}).List(ListOptions{Workspace: workspace, Limit: 10, Sort: "date"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "修复中文标题并提高会话加载速度" {
		t.Fatalf("list = %#v", items)
	}
	if got := oneLine("你好世界", 3); got != "你好…" {
		t.Fatalf("oneLine = %q", got)
	}
}

func TestClaudeRecentSummaryCapturesStartedAt(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	path := filepath.Join(home, ".claude", "projects", "project", "session.jsonl")
	writeFixture(t, path, strings.Join([]string{
		fmt.Sprintf(`{"type":"user","sessionId":"claude-id","cwd":%q,"timestamp":"2026-09-10T20:00:00Z","message":{"role":"user","content":"work"}}`, workspace),
		`{"type":"assistant","sessionId":"claude-id","timestamp":"2026-09-10T20:05:00Z","message":{"role":"assistant","content":"done"}}`,
	}, "\n"))

	groups, err := (Service{HomeDir: home}).RecentGroups(workspace, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups.Current) != 1 || groups.Current[0].StartedAt != "2026-09-10T20:00:00Z" {
		t.Fatalf("recent groups = %#v", groups)
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
