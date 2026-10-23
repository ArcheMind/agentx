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
	"github.com/ArcheMind/agentx/internal/runtime"
)

func TestSubagentLaunchDryRun(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	app.Hooks.Project = t.TempDir()
	t.Setenv("AX_SUBAGENT_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if err := app.Run(context.Background(), []string{"--json", "claude", "--cwd", app.Hooks.Project, "--subagents", "--dry-run"}); err == nil || !strings.Contains(err.Error(), "claude mcp add --scope user agentx -- ") {
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
	if len(plan.Args) != 7 || plan.Args[0] != "--mcp-config" || plan.Args[2] != "--allowedTools" || plan.Args[3] != "mcp__agentx__ExternalSubagent" {
		t.Fatalf("plan = %#v", plan)
	}
	if err := app.Run(context.Background(), []string{"codex", "--subagents", "--dry-run"}); err == nil {
		t.Fatal("accepted Claude flag for Codex")
	}
}

// registerAgentx writes the entry exactly as claude mcp add --scope user agentx -- <ax> preset serve does.
func registerAgentx(t *testing.T, home string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"agentx": map[string]any{"type": "stdio", "command": executable, "args": []string{"preset", "serve"}, "env": map[string]string{}}}})
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPresetListIgnoresLegacyConfigAndShowsAllPresets(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	t.Setenv("AX_SUBAGENT_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	for _, p := range []presets.Preset{{Name: "review", Agent: "codex", Model: "m", Description: "d"}, {Name: "daily", Agent: "claude", Model: "opus", Description: "d"}} {
		if err := presets.Create(app.Hooks.Home, p); err != nil {
			t.Fatal(err)
		}
	}
	legacy := filepath.Join(app.Hooks.Home, ".config", "agentx", "subagents.json")
	os.MkdirAll(filepath.Dir(legacy), 0o700)
	os.WriteFile(legacy, []byte(`{"enabled":true,"grants":{"review":{}}}`), 0o600)
	if err := app.Run(context.Background(), []string{"--json", "preset", "list"}); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(output.Bytes(), &value); err != nil {
		t.Fatal(err, output.String())
	}
	if value["enabled"] != nil || value["grant_problems"] != nil || value["configuration"] != nil {
		t.Fatalf("list still reads a separate switch or grants: %v", value)
	}
	listed := value["presets"].([]any)
	if len(listed) != 2 {
		t.Fatalf("presets = %v", listed)
	}
	byName := map[string]map[string]any{}
	for _, raw := range listed {
		entry := raw.(map[string]any)
		byName[entry["name"].(string)] = entry
	}
	review, daily := byName["review"], byName["daily"]
	if review == nil || daily == nil || review["execution"] != nil || review["auth"] != nil || review["limits"] == nil {
		t.Fatalf("list = %v", listed)
	}
	if daily["limits"] == nil {
		t.Fatalf("claude preset missing limits: %v", daily)
	}
	for _, removed := range []string{"enable", "disable", "status"} {
		if err := app.Run(context.Background(), []string{"preset", removed}); err == nil {
			t.Fatalf("%s still exists", removed)
		}
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

func TestPresetShowReportsSinglePresetAndInvalidOnes(t *testing.T) {
	var output bytes.Buffer
	app := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	app.Hooks.Home = t.TempDir()
	t.Setenv("AX_SUBAGENT_HOME", "")
	if err := presets.Create(app.Hooks.Home, presets.Preset{Name: "review", Agent: "codex", Model: "m", Description: "d"}); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(presets.Dir(app.Hooks.Home), "broken")
	os.MkdirAll(broken, 0o700)
	os.WriteFile(filepath.Join(broken, "PRESET.md"), []byte("---\nagent: codex\n---\nx\n"), 0o600)
	if err := app.Run(context.Background(), []string{"--json", "preset", "show", "review"}); err != nil {
		t.Fatal(err)
	}
	var shown map[string]any
	if err := json.Unmarshal(output.Bytes(), &shown); err != nil {
		t.Fatal(err, output.String())
	}
	if shown["name"] != "review" || shown["limits"] == nil || shown["execution"] != nil || shown["auth"] != nil {
		t.Fatalf("show = %v", shown)
	}
	if err := app.Run(context.Background(), []string{"preset", "show", "broken"}); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("broken preset not reported: %v", err)
	}
	if err := app.Run(context.Background(), []string{"preset", "show", "missing"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing preset not reported: %v", err)
	}
}
