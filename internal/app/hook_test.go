package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/hooks"
)

func TestHookListUsesPublicConfigurationAndStructuredOutput(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".agents", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"./hooks/check.sh"}]}],"PreToolUse":[{"hooks":[{"type":"command","command":"native.sh"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Hooks = hooks.Service{Home: t.TempDir(), Project: project}
	if err := application.Run(context.Background(), []string{"--json", "hook", "list"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"event": "UserPromptSubmit"`, `"command": "./hooks/check.sh"`, `"portable": true`, `"claude"`, `"PreToolUse"`, `not projected`} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("output %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestHookListTextShowsPortableTargetsAndWarning(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".agents", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"load.sh"}]}],"PreToolUse":[]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Hooks = hooks.Service{Home: t.TempDir(), Project: project}
	if err := application.Run(context.Background(), []string{"hook", "list"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"SCOPE", "SessionStart", "load.sh", "claude,codex,gemini,pi", "warning: PreToolUse"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("output %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestHookListRejectsOtherGrammar(t *testing.T) {
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	for _, args := range [][]string{{"hook"}, {"hook", "show"}, {"hook", "list", "extra"}} {
		if err := application.Run(context.Background(), args); err == nil {
			t.Fatalf("args %v unexpectedly succeeded", args)
		}
	}
}
