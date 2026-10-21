package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/subagents"
)

func TestSubagentLaunchDryRun(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	app.Hooks.Project = t.TempDir()
	if err := subagents.SaveConfig(app.Hooks.Home, subagents.Config{Enabled: true, Options: []subagents.Option{{Name: "test", Agent: "codex", Model: "test", Description: "test", Enabled: true}}, Grants: map[string]subagents.Grant{"test": {Execution: subagents.ExecutionPolicy{Policy: "read-only", Approval: "never"}, Auth: subagents.AuthSource{Source: "codex-home", Path: app.Hooks.Home, Method: "chatgpt"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background(), []string{"--json", "claude", "--cwd", app.Hooks.Project, "--subagents", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	var plan runtime.CommandPlan
	if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Args) != 4 || plan.Args[0] != "--mcp-config" || plan.Args[2] != "--dangerously-load-development-channels" || plan.Args[3] != "server:agentx" {
		t.Fatalf("plan = %#v", plan)
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
