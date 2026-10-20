package mcp

import (
	"github.com/ArcheMind/agentx/internal/inventory"
	"os"
	"path/filepath"
	"testing"
)

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestDiscoveryF01F03F04F07F09F10(t *testing.T) {
	home, project, other := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	put(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"alpha":{"command":"x"}},"projects":{`+quote(project)+`:{"mcpServers":{"local":{"command":"x"}}},`+quote(other)+`:{"mcpServers":{"foreign":{}}}}}`)
	put(t, filepath.Join(project, ".mcp.json"), `{"mcpServers":{"alpha":{"command":"x"}}}`)
	put(t, filepath.Join(project, ".gemini", "settings.json"), `{"mcpServers":{}}`)
	put(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.'alpha']\ncommand='x'\n")
	put(t, filepath.Join(project, "opencode.jsonc"), "{ // comment\n \"mcp\": {\"open\": {\"type\":\"local\",\"command\":[\"x\"],},},}")
	put(t, filepath.Join(home, ".gemini", "settings.json"), `{"mcpServers":`)
	result := (Service{Home: home, Project: project}).List()
	counts := map[string]int{}
	empty, failed, unsupported := false, false, false
	for _, source := range result.Sources {
		empty = empty || source.Status == "empty"
		failed = failed || source.Status == "failed"
		unsupported = unsupported || source.Status == "unsupported"
		for _, item := range source.Resources {
			counts[item.Name]++
			if item.Evidence.Connection != "unconfirmed" || item.Evidence.Session != "unconfirmed" {
				t.Fatalf("evidence=%+v", item)
			}
		}
	}
	if counts["alpha"] != 3 || counts["foreign"] != 0 || counts["local"] != 1 || counts["open"] != 1 || !empty || !failed || !unsupported {
		t.Fatalf("counts=%v empty=%v failed=%v unsupported=%v sources=%+v", counts, empty, failed, unsupported, result)
	}
	if _, err := result.Show("alpha", inventory.Selection{}); err == nil {
		t.Fatal("ambiguous alpha accepted")
	}
	detail, err := result.Show("alpha", inventory.Selection{Scope: "project", Source: inventory.Physical(filepath.Join(project, ".mcp.json"))})
	if err != nil || detail.Resource.Name != "alpha" {
		t.Fatalf("show=%+v %v", detail, err)
	}
}
func quote(value string) string { return `"` + filepath.ToSlash(value) + `"` }
func TestTOMLReadFailure(t *testing.T) {
	if _, err := tomlNames([]byte("[mcp_servers.bad\n")); err == nil {
		t.Fatal("malformed TOML accepted")
	}
}

func TestMCPLinkProjectionsF03(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	path := filepath.Join(project, ".mcp.json")
	put(t, path, `{"mcpServers":{"alpha":{"command":"x"}}}`)
	projection := filepath.Join(project, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(projection), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, projection); err != nil {
		t.Fatal(err)
	}
	result := (Service{Home: home, Project: project}).List()
	count := 0
	for _, source := range result.Sources {
		for _, item := range source.Resources {
			if item.Name == "alpha" {
				count++
				if len(item.Agents) != 2 || len(item.Projections) == 0 {
					t.Fatalf("projection=%+v", item)
				}
			}
		}
	}
	if count != 1 {
		t.Fatalf("count=%d", count)
	}
}
func TestInvalidServersDoNotBecomeEmpty(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	put(t, filepath.Join(home, ".claude.json"), `{"mcpServers":[],"projects":{}}`)
	found := false
	for _, source := range (Service{Home: home, Project: project}).List().Sources {
		if source.Path == inventory.Physical(filepath.Join(home, ".claude.json")) {
			found = true
			if source.Status != "failed" {
				t.Fatalf("source=%+v", source)
			}
		}
	}
	if !found {
		t.Fatal("source missing")
	}
}

func TestClaudeConfigOverride(t *testing.T) {
	home, project, override := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", override)
	put(t, filepath.Join(override, ".claude.json"), `{"mcpServers":{"override":{}}}`)
	put(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"wrong":{}}}`)
	found := false
	for _, source := range (Service{Home: home, Project: project}).List().Sources {
		for _, item := range source.Resources {
			if item.Name == "wrong" {
				t.Fatal("wrong default Claude home read")
			}
			if item.Name == "override" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("override missing")
	}
}
