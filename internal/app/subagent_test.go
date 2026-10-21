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
)

func TestSubagentLaunchDryRun(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	app.Hooks.Project = t.TempDir()
	t.Setenv("AX_SUBAGENT_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if err := app.Run(context.Background(), []string{"--json", "claude", "--cwd", app.Hooks.Project, "--subagents", "--dry-run"}); err == nil || !strings.Contains(err.Error(), "enable with ax sub-agent-management enable") {
		t.Fatal("unregistered AgentX treated as enabled", err)
	}
	registerAgentx(t, app.Hooks.Home)
	output.Reset()
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

func registerAgentx(t *testing.T, home string) {
	t.Helper()
	entry, err := agentxEntry()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"mcpServers": map[string]json.RawMessage{"agentx": entry}})
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentStatusEnabledIsMCPRegistrationAndHasNoConfigFile(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	t.Setenv("AX_SUBAGENT_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if err := presets.Create(app.Hooks.Home, presets.Preset{Name: "review", Agent: "codex", Model: "m", Description: "d"}); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(app.Hooks.Home, ".config", "agentx", "subagents.json")
	os.MkdirAll(filepath.Dir(legacy), 0o700)
	os.WriteFile(legacy, []byte(`{"enabled":true,"grants":{"review":{}}}`), 0o600)
	status := func() map[string]any {
		output.Reset()
		if err := app.Run(context.Background(), []string{"--json", "sub-agent-management", "status"}); err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(output.Bytes(), &value); err != nil {
			t.Fatal(err, output.String())
		}
		return value
	}
	before := status()
	if before["enabled"] != false || before["grant_problems"] != nil || before["configuration"] != nil {
		t.Fatalf("status still reads a separate switch or grants: %v", before)
	}
	registerAgentx(t, app.Hooks.Home)
	after := status()
	preset := after["presets"].([]any)[0].(map[string]any)
	if after["enabled"] != true || preset["execution"] != nil || preset["auth"] != nil || !strings.Contains(after["write_behavior"].(string), "read-only unless it sets write") {
		t.Fatalf("status = %v", after)
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
	if len(status.Roles) != 2 || status.Roles[0]["name"] != "general-purpose" || status.Roles[1]["name"] != "security" || len(status.InvalidRoles) != 1 || !strings.Contains(status.InvalidRoles[0].Err, "unknown field") || !strings.Contains(status.RoleBehavior, "never change agent, model, effort, write scope or permissions") {
		t.Fatalf("%+v", status)
	}
}
