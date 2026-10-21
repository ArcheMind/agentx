package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/subagents"
)

func TestSubagentLaunchDryRun(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	app.Hooks.Project = t.TempDir()
	if err := subagents.SaveConfig(app.Hooks.Home, subagents.Config{Enabled: true}); err != nil {
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

func TestSubagentStatusShowsGrantOnlyForCallablePresets(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	t.Setenv("AX_SUBAGENT_HOME", "")
	for _, p := range []presets.Preset{{Name: "review", Agent: "codex", Model: "m", Description: "d"}, {Name: "daily", Agent: "claude", Model: "opus", Description: "d"}} {
		if err := presets.Create(app.Hooks.Home, p); err != nil {
			t.Fatal(err)
		}
	}
	grant := subagents.Grant{Execution: subagents.ExecutionPolicy{Policy: "read-only", Approval: "never"}, Auth: subagents.AuthSource{Source: "codex-home", Path: app.Hooks.Home, Method: "chatgpt"}}
	if err := subagents.SaveConfig(app.Hooks.Home, subagents.Config{Grants: map[string]subagents.Grant{"review": grant, "daily": grant}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background(), []string{"--json", "sub-agent-management", "status"}); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Presets       []map[string]any  `json:"presets"`
		GrantProblems []presets.Problem `json:"grant_problems"`
	}
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err, output.String())
	}
	for _, p := range status.Presets {
		_, hasGrant := p["execution"]
		if hasGrant != (p["name"] == "review") {
			t.Fatalf("grant shown on wrong preset: %v", p)
		}
	}
	if len(status.GrantProblems) != 1 || status.GrantProblems[0].Name != "daily" {
		t.Fatalf("stray grant not reported only as a problem: %+v", status.GrantProblems)
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
