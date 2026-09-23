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
	for _, expected := range []string{`"event": "UserPromptSubmit"`, `"command": "./hooks/check.sh"`, `"portable": true`, `"PreToolUse"`, `not projected`} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("output %q does not contain %q", stdout.String(), expected)
		}
	}
	if strings.Contains(stdout.String(), `"targets"`) {
		t.Fatalf("output contains redundant targets field: %q", stdout.String())
	}
}

func TestHookListTextShowsPortableHooksAndWarning(t *testing.T) {
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
	for _, expected := range []string{"SCOPE", "EVENT", "COMMAND", "SessionStart", "load.sh", "warning: PreToolUse"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("output %q does not contain %q", stdout.String(), expected)
		}
	}
	if strings.Contains(stdout.String(), "TARGETS") {
		t.Fatalf("output contains redundant targets column: %q", stdout.String())
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

func TestAgentRunProjectsHooksIntoNativeLaunchArguments(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".agents", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"load-policy"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		provider string
		expected []string
	}{
		{"claude", []string{"--plugin-dir", ".agents"}},
		{"codex", []string{"-c", "hooks.SessionStart", "--handler"}},
		{"pi", []string{"--extension", "agentx-hooks.ts"}},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			var stdout bytes.Buffer
			application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
			application.Hooks = hooks.Service{Home: t.TempDir(), Project: project}
			if err := application.Run(context.Background(), []string{test.provider, "--cwd", project, "--dry-run"}); err != nil {
				t.Fatal(err)
			}
			for _, expected := range test.expected {
				if !strings.Contains(stdout.String(), expected) {
					t.Fatalf("launch plan %q does not contain %q", stdout.String(), expected)
				}
			}
		})
	}
}
