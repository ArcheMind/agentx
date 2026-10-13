package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/runtime"
)

func TestSubagentLaunchDryRun(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	app.Hooks.Project = t.TempDir()
	if err := app.Run(context.Background(), []string{"--json", "claude", "--cwd", app.Hooks.Project, "--subagents", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	var plan runtime.CommandPlan
	if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Args) != 2 || plan.Args[0] != "--plugin-dir" {
		t.Fatalf("plan = %#v", plan)
	}
	if _, err := os.Stat(plan.Args[1]); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote projection: %v", err)
	}
	if err := app.Run(context.Background(), []string{"codex", "--subagents", "--dry-run"}); err == nil {
		t.Fatal("accepted Claude flag for Codex")
	}
}

func TestExecRejectsStructuredNativeOutputAndInvalidSession(t *testing.T) {
	app := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	for _, args := range [][]string{
		{"--json", "agent", "exec", "codex"},
		{"agent", "exec", "claude"},
		{"agent", "exec", "codex", "--resume", "--last"},
	} {
		if err := app.Run(context.Background(), args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
