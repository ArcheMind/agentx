package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
)

func TestOutputFormatIsGlobal(t *testing.T) {
	format, args, err := parseOutputFormat([]string{"--yaml", "models", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if format != OutputYAML || strings.Join(args, " ") != "models codex" {
		t.Fatalf("format = %q, args = %v", format, args)
	}
	if _, _, err := parseOutputFormat([]string{"--json", "--yaml", "list"}); err == nil {
		t.Fatal("expected conflicting output formats to fail")
	}
}

func TestYAMLCoversInstallDryRun(t *testing.T) {
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	err := application.Run(context.Background(), []string{"--yaml", "agent", "install", "codex", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"executable: npm", "- install", "- --global", "- '@openai/codex'"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("YAML %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestYAMLCoversAuthDryRun(t *testing.T) {
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	err := application.Run(context.Background(), []string{"--yaml", "auth", "login", "claude", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"command:", "executable: claude", "- auth", "- login", "- --claudeai"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("YAML %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestAuthStatusStructuredOutput(t *testing.T) {
	status := runtime.AuthStatus{Agent: "claude", Supported: true, Providers: []runtime.AuthProvider{{ID: "claude", Method: "claude.ai", Subscription: "max"}}}
	for _, test := range []struct {
		name     string
		format   OutputFormat
		expected []string
	}{
		{name: "json", format: OutputJSON, expected: []string{`"agent": "claude"`, `"providers":`, `"subscription": "max"`}},
		{name: "yaml", format: OutputYAML, expected: []string{"agent: claude", "providers:", "subscription: max"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			application := App{Stdout: &stdout, Output: test.format}
			if err := application.writeAuthStatus(status); err != nil {
				t.Fatal(err)
			}
			for _, expected := range test.expected {
				if !strings.Contains(stdout.String(), expected) {
					t.Fatalf("output %q does not contain %q", stdout.String(), expected)
				}
			}
		})
	}
}

func TestAuthStatusTextOutput(t *testing.T) {
	var stdout bytes.Buffer
	application := App{Stdout: &stdout, Output: OutputText}
	if err := application.writeAuthStatus(runtime.AuthStatus{Agent: "pi", Supported: false, Providers: []runtime.AuthProvider{}}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "pi\tunsupported\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestStructuredOutputRejectsNativePassthrough(t *testing.T) {
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	err := application.Run(context.Background(), []string{"--json", "agent", "install", "codex"})
	if err == nil || !strings.Contains(err.Error(), "only supported for agent install with --dry-run") {
		t.Fatalf("error = %v", err)
	}
}

func TestStructuredErrorEnvelope(t *testing.T) {
	var stderr bytes.Buffer
	err := WriteError(&stderr, []string{"--json", "agent", "wat"}, fmt.Errorf("unknown agent operation"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"code": "usage"`, `"message": "unknown agent operation"`, `"exit_code": 1`} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("JSON %q does not contain %q", stderr.String(), expected)
		}
	}
}

func TestAgentResourceGrammar(t *testing.T) {
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err := application.Run(context.Background(), []string{"agent", "install", "codex", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if err := application.Run(context.Background(), []string{"install", "codex", "--dry-run"}); err == nil {
		t.Fatal("expected removed action-leading command to fail")
	}
}

func TestSessionListUsesSourceTerminology(t *testing.T) {
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: t.TempDir()}
	if err := application.Run(context.Background(), []string{"session", "list", "--source", "pi", "--all"}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionInfoUsesGlobalYAMLOutput(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	path := filepath.Join(home, ".claude", "projects", "fixture", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"type":"user","sessionId":"session-id","cwd":"` + workspace + `","timestamp":"2026-09-01T10:00:00Z","message":{"content":"continue work"}}`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: home}
	if err := application.Run(context.Background(), []string{"--yaml", "session", "info", "session-id", "--source", "claude"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"id: session-id", "provider: claude", "content: continue work"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("YAML %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestSessionResumeDryRunIsNativeAgentPlan(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	path := filepath.Join(home, ".pi", "agent", "sessions", "fixture", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"type":"session","id":"pi-session","cwd":"` + workspace + `"}` + "\n" +
		`{"type":"message","message":{"role":"user","content":"continue work"}}`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: home}
	if err := application.Run(context.Background(), []string{"--json", "session", "resume", "codex", "pi-session", "--source", "pi", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"executable": "codex"`, `"cwd": "` + workspace + `"`, "continue work"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("JSON %q does not contain %q", stdout.String(), expected)
		}
	}
}
