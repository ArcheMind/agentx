package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/roles"
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

func TestSubagentStatusListsRolesAndInvalidRoles(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	t.Setenv("AX_SUBAGENT_HOME", "")
	for name, content := range map[string]string{"security": "---\ndescription: Security review\n---\nCheck auth.\n", "broken": "---\ndescription: x\neffort: high\n---\nx\n"} {
		dir := filepath.Join(roles.Dir(app.Hooks.Home), name)
		os.MkdirAll(dir, 0o700)
		os.WriteFile(filepath.Join(dir, roles.FileName), []byte(content), 0o600)
	}
	if err := app.Run(context.Background(), []string{"--json", "sub-agent-management", "status"}); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Roles        []map[string]any  `json:"roles"`
		InvalidRoles []presets.Problem `json:"invalid_roles"`
		RoleBehavior string            `json:"role_behavior"`
	}
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err, output.String())
	}
	if len(status.Roles) != 2 || status.Roles[0]["name"] != "general-purpose" || status.Roles[1]["name"] != "security" || len(status.InvalidRoles) != 1 || !strings.Contains(status.InvalidRoles[0].Err, "unknown field") || !strings.Contains(status.RoleBehavior, "never change agent, model, effort, grant or permissions") {
		t.Fatalf("%+v", status)
	}
}
