package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/inventory"
)

func TestInventoryProjectionIndependentSourcesAndScopesF01F03F04F07F09F10(t *testing.T) {
	home, project, other := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	t.Setenv("DSH_AGENTS_HOME", filepath.Join(home, ".agents"))
	if err := os.Mkdir(filepath.Join(project, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(project, ".agents", "skills", "alpha")
	writeSkill(t, common, "alpha", "same")
	writeSkill(t, filepath.Join(project, ".codex", "skills", "alpha"), "alpha", "same")
	writeSkill(t, filepath.Join(home, ".agents", "skills", "delta"), "delta", "user")
	writeSkill(t, filepath.Join(other, ".agents", "skills", "foreign"), "foreign", "other")
	projection := filepath.Join(project, ".claude", "skills", "alpha")
	if err := os.MkdirAll(filepath.Dir(projection), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(common, projection); err != nil {
		t.Fatal(err)
	}
	cross := filepath.Join(home, ".claude", "skills", "alpha")
	if err := os.MkdirAll(filepath.Dir(cross), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(common, cross); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".gemini", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(project, ".dsh", "skills", "broken")
	if err := os.MkdirAll(filepath.Dir(broken), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent-fixture", broken); err != nil {
		t.Fatal(err)
	}
	result := (Service{Home: home, Project: project}).Inventory()
	alpha := 0
	empty, failed, unsupported := false, false, false
	for _, source := range result.Sources {
		empty = empty || source.Status == "empty"
		failed = failed || source.Status == "failed"
		unsupported = unsupported || source.Status == "unsupported"
		for _, item := range source.Resources {
			if item.Name == "alpha" {
				alpha++
			}
			if item.Name == "foreign" {
				t.Fatal("other project leaked")
			}
		}
	}
	if alpha != 3 || !empty || !failed || !unsupported {
		t.Fatalf("alpha=%d empty=%v failed=%v unsupported=%v result=%+v", alpha, empty, failed, unsupported, result)
	}
	detail, err := result.Show("alpha", inventory.Selection{Scope: "project", Source: inventory.Physical(filepath.Dir(common))})
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Resource.Projections) < 1 || !strings.Contains(strings.Join(detail.Resource.Agents, ","), "claude") {
		t.Fatalf("projection=%+v", detail)
	}
	if _, err := result.Show("alpha", inventory.Selection{}); err == nil {
		t.Fatal("independent names silently selected")
	}
	if !result.Sources[0].Preferred {
		t.Fatalf("common source not preferred: %+v", result.Sources[0])
	}
}
func TestInventoryNativeFilesystemRoots(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	t.Setenv("DSH_AGENTS_HOME", filepath.Join(home, ".agents"))
	for _, root := range []string{".claude/skills", ".codex/skills", ".gemini/skills", ".dsh/skills", ".opencode/skills", ".pi/skills"} {
		writeSkill(t, filepath.Join(project, root, "native"), "native", "native")
	}
	counts := map[string]int{}
	for _, source := range (Service{Home: home, Project: project}).Inventory().Sources {
		for _, item := range source.Resources {
			if item.Name == "native" {
				for _, agent := range item.Agents {
					counts[agent]++
				}
			}
		}
	}
	for _, agent := range []string{"claude", "codex", "gemini", "dsh", "opencode", "pi"} {
		if counts[agent] == 0 {
			t.Fatalf("native %s missing: %v", agent, counts)
		}
	}
}

func TestMultipleAliasesAndSameRootNamesF03F09(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	source := filepath.Join(project, ".agents", "skills", "original")
	writeSkill(t, source, "alpha", "original")
	writeSkill(t, filepath.Join(project, ".agents", "skills", "independent"), "alpha", "same name")
	for _, alias := range []string{"one", "two"} {
		path := filepath.Join(project, ".claude", "skills", alias)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(source, path); err != nil {
			t.Fatal(err)
		}
	}
	result := (Service{Home: home, Project: project}).Inventory()
	detail, err := result.Show("alpha", inventory.Selection{Scope: "project", Source: inventory.Physical(filepath.Dir(source)), Path: inventory.Physical(source)})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, projection := range detail.Resource.Projections {
		paths[filepath.Base(projection.Path)] = true
	}
	if !paths["one"] || !paths["two"] {
		t.Fatalf("projections=%+v", detail.Resource.Projections)
	}
}
func TestFlatAndNestedAgentApplicability(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	path := filepath.Join(project, ".pi", "skills", "flat.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: flat\ndescription: flat\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(project, ".agents", "skills", "group", "nested"), "nested", "nested")
	found := false
	for _, source := range (Service{Home: home, Project: project}).Inventory().Sources {
		for _, item := range source.Resources {
			if item.Name == "flat" {
				found = true
			}
			if item.Name == "nested" {
				for _, agent := range item.Agents {
					if agent == "dsh" {
						t.Fatalf("DSH incorrectly assigned: %+v", item)
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("flat pi skill missing")
	}
}

func TestUserOverrides(t *testing.T) {
	home, project, custom := t.TempDir(), t.TempDir(), t.TempDir()
	for _, agent := range []struct{ name, env, dir string }{{"claude", "CLAUDE_CONFIG_DIR", "claude"}, {"codex", "CODEX_HOME", "codex"}, {"pi", "PI_CODING_AGENT_DIR", "pi"}, {"dsh", "DSH_HOME", "dsh"}, {"opencode", "XDG_CONFIG_HOME", "config"}, {"gemini", "GEMINI_CLI_HOME", "gemini-home"}} {
		base := filepath.Join(custom, agent.dir)
		t.Setenv(agent.env, base)
		root := filepath.Join(base, "skills")
		if agent.name == "opencode" {
			root = filepath.Join(base, "opencode", "skills")
		}
		if agent.name == "gemini" {
			root = filepath.Join(base, ".gemini", "skills")
		}
		writeSkill(t, filepath.Join(root, agent.name), agent.name, "override")
	}
	result := (Service{Home: home, Project: project}).Inventory()
	found := map[string]bool{}
	for _, source := range result.Sources {
		for _, item := range source.Resources {
			for _, agent := range item.Agents {
				if item.Name == agent {
					found[agent] = true
				}
			}
		}
	}
	for _, agent := range []string{"claude", "codex", "pi", "dsh", "opencode", "gemini"} {
		if !found[agent] {
			t.Fatalf("override %s missing", agent)
		}
	}
}

func TestNestedProjectionIdentity(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	source := filepath.Join(project, ".agents", "skills", "group", "alpha")
	writeSkill(t, source, "nested", "nested")
	projection := filepath.Join(project, ".claude", "skills", "linked")
	if err := os.MkdirAll(filepath.Dir(projection), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, projection); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, block := range (Service{Home: home, Project: project}).Inventory().Sources {
		for _, item := range block.Resources {
			if item.Name == "nested" {
				count++
				if !block.Preferred || len(item.Projections) == 0 {
					t.Fatalf("source=%+v", block)
				}
			}
		}
	}
	if count != 1 {
		t.Fatalf("count=%d", count)
	}
}

func TestPiOverrideFlatAndInvalidMetadata(t *testing.T) {
	home, project, override := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", override)
	root := filepath.Join(override, "skills")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"valid.md": "---\nname: valid\ndescription: valid\n---\n", "plain.md": "# ordinary notes", "invalid.md": "---\nname: [\n---\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill(t, filepath.Join(root, "empty-description"), "empty-description", "")
	found, failures := false, 0
	for _, source := range (Service{Home: home, Project: project}).Inventory().Sources {
		if source.Status == "failed" {
			failures++
		}
		for _, item := range source.Resources {
			if item.Name == "valid" {
				found = true
			}
			if item.Name == "empty-description" {
				t.Fatal("invalid bundle accepted")
			}
		}
	}
	if !found || failures != 2 {
		t.Fatalf("found=%v failures=%d", found, failures)
	}
}
