package app

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/presets"
)

func TestRemovedGeminiCommandsRejectLegacyAgent(t *testing.T) {
	for _, args := range [][]string{
		{"gemini", "--dry-run"},
		{"agent", "show", "gemini"},
		{"agent", "run", "gemini", "--dry-run"},
		{"agent", "exec", "gemini", "--dry-run"},
		{"auth", "show", "gemini"},
		{"session", "list", "--source", "gemini", "--all"},
		{"session", "show", "missing", "--source", "gemini"},
		{"session", "resume", "gemini", "missing"},
		{"session", "import", "gemini"},
		{"session", "continue", "gemini"},
		{"convert", "to", "gemini"},
		{"convert", "from", "gemini"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			application, _, runner := presetApp(t, "")
			err := application.Run(context.Background(), args)
			if err == nil || !strings.Contains(err.Error(), "gemini") ||
				(!strings.Contains(err.Error(), "unknown") && err.Error() != "gemini has no native subagent execution adapter") {
				t.Fatalf("err = %v", err)
			}
			if len(runner.plans) != 0 {
				t.Fatalf("removed agent invoked: %#v", runner.plans)
			}
		})
	}
}

func TestRemovedGeminiDoesNotRemoveAgyProduct(t *testing.T) {
	application, stdout, _ := presetApp(t, "", "agy")
	if err := application.Run(context.Background(), []string{"agent", "run", "agy", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "agy") {
		t.Fatal(stdout.String())
	}
	stdout.Reset()
	if err := application.Run(context.Background(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "gemini") || !strings.Contains(stdout.String(), "agy") {
		t.Fatal(stdout.String())
	}
	if err := application.Run(context.Background(), []string{"session", "list", "--source", "agy", "--all"}); err != nil {
		t.Fatal(err)
	}
}

func TestRemovedGeminiPresetUsesGenericRepair(t *testing.T) {
	application, stdout, runner := presetApp(t, "")
	home := application.Hooks.Home
	path := presets.Path(home, "legacy")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := "---\nagent: gemini\nmodel: old-model\n---\nReview changes\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := presets.Read(home, "legacy"); err == nil || !strings.Contains(err.Error(), `unsupported agent "gemini"`) {
		t.Fatalf("err = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatalf("legacy preset changed without repair: %q, %v", data, err)
	}
	problem := presets.Problem{Name: "legacy", Path: path, Err: `unsupported agent "gemini"`}
	input := "\r\r" + "opus\n" + "\r\r\n\r"
	if err := application.manageProblem(context.Background(), bufio.NewReader(strings.NewReader(input)), home, problem); err != nil {
		t.Fatal(err)
	}
	saved, err := presets.Read(home, "legacy")
	if err != nil || saved.Agent != "claude" || saved.Model != "opus" {
		t.Fatalf("saved = %#v, err = %v, output = %s", saved, err, stdout.String())
	}
	if len(runner.plans) != 0 || !strings.Contains(stdout.String(), "Repair preset · legacy") {
		t.Fatalf("plans=%#v output=%s", runner.plans, stdout.String())
	}
}
