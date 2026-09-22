package skills

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	sharedruntime "github.com/ArcheMind/agentx/internal/runtime"
)

func TestListDeduplicatesClaudeProjectionAndReportsSupport(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	canonical := filepath.Join(project, ".agents", "skills", "review")
	writeSkill(t, canonical, "review", "Review changes")
	projection := filepath.Join(project, ".claude", "skills", "review")
	if err := os.MkdirAll(filepath.Dir(projection), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", ".agents", "skills", "review"), projection); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symbolic links are unavailable: %v", err)
		}
		t.Fatal(err)
	}
	result, err := (Service{Home: home, Project: project}).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("skills = %#v", result.Skills)
	}
	item := result.Skills[0]
	if strings.Join(item.Agents, ",") != "claude,codex,gemini,pi" || item.Path != canonical {
		t.Fatalf("skill = %#v", item)
	}
	for _, support := range result.Agents {
		if support.Agent == "opencode" {
			t.Fatalf("OpenCode must remain outside new skill support: %#v", result.Agents)
		}
		if support.Agent == "dsh" && support.Supported {
			t.Fatalf("DSH support = %#v", support)
		}
	}
}

func TestInstallLocalSkillAndClaudeProjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires symbolic-link support")
	}
	home := t.TempDir()
	project := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeSkill(t, source, "review", "Review changes")
	if err := os.MkdirAll(filepath.Join(source, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "references", "rules.md"), []byte("rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{Home: home, Project: project}
	plan, err := service.Install(context.Background(), InstallOptions{Source: source, Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Name != "review" || plan.SourceType != "local" {
		t.Fatalf("plan = %#v", plan)
	}
	target, err := filepath.EvalSymlinks(plan.Projection)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := filepath.EvalSymlinks(plan.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if target != destination {
		t.Fatalf("projection target = %q, destination = %q", target, destination)
	}
	result, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 || strings.Join(result.Skills[0].Agents, ",") != "claude,codex,gemini,pi" {
		t.Fatalf("skills = %#v", result.Skills)
	}
	if result.Skills[0].Source != "local" || result.Skills[0].Origin != source {
		t.Fatalf("origin = %#v", result.Skills[0])
	}
}

func TestInstallDryRunDoesNotWrite(t *testing.T) {
	project := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeSkill(t, source, "review", "Review changes")
	service := Service{Home: t.TempDir(), Project: project}
	plan, err := service.Install(context.Background(), InstallOptions{Source: source, Scope: ScopeProject, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Destination == "" || plan.Projection == "" {
		t.Fatalf("plan = %#v", plan)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote canonical directory: %v", err)
	}
}

func TestInstallUserScopeTargetsHome(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeSkill(t, source, "review", "Review changes")
	plan, err := (Service{Home: home, Project: t.TempDir()}).Install(context.Background(), InstallOptions{Source: source, Scope: ScopeUser, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Destination != filepath.Join(home, ".agents", "skills", "review") || plan.Projection != filepath.Join(home, ".claude", "skills", "review") {
		t.Fatalf("user-scope plan = %#v", plan)
	}
}

func TestInstallRejectsInvalidAndConflictingSkills(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "frontmatter", content: "# Missing frontmatter", want: "frontmatter"},
		{name: "name", content: "---\nname: Bad Name\ndescription: bad\n---\n", want: "invalid skill name"},
		{name: "description", content: "---\nname: valid\n---\n", want: "description is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source")
			if err := os.MkdirAll(source, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(test.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := (Service{Home: t.TempDir(), Project: t.TempDir()}).Install(context.Background(), InstallOptions{Source: source, DryRun: true})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	project := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeSkill(t, source, "review", "Review changes")
	writeSkill(t, filepath.Join(project, ".agents", "skills", "review"), "review", "Existing")
	_, err := (Service{Home: t.TempDir(), Project: project}).Install(context.Background(), InstallOptions{Source: source, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v", err)
	}
}

func TestInstallRejectsSymlinkResources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires symbolic-link support")
	}
	source := filepath.Join(t.TempDir(), "source")
	writeSkill(t, source, "review", "Review changes")
	if err := os.Symlink(filepath.Join(source, "SKILL.md"), filepath.Join(source, "linked.md")); err != nil {
		t.Fatal(err)
	}
	_, err := (Service{Home: t.TempDir(), Project: t.TempDir()}).Install(context.Background(), InstallOptions{Source: source, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("error = %v", err)
	}
}

type cloneRunner struct {
	repository string
	plan       sharedruntime.CommandPlan
}

func (r *cloneRunner) Execute(_ context.Context, plan sharedruntime.CommandPlan, _ sharedruntime.ExecuteOptions) (sharedruntime.CommandResult, error) {
	r.plan = plan
	destination := plan.Args[len(plan.Args)-1]
	return sharedruntime.CommandResult{}, copyDirectory(r.repository, destination)
}

func TestInstallFromGitSubpath(t *testing.T) {
	repository := t.TempDir()
	writeSkill(t, filepath.Join(repository, "skills", "review"), "review", "Review changes")
	runner := &cloneRunner{repository: repository}
	service := Service{Home: t.TempDir(), Project: t.TempDir(), TempParent: t.TempDir(), Runner: runner}
	plan, err := service.Install(context.Background(), InstallOptions{Source: "https://example.invalid/skills.git", Path: "skills/review", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.SourceType != "git" || plan.Name != "review" || runner.plan.Executable != "git" {
		t.Fatalf("plan = %#v, clone = %#v", plan, runner.plan)
	}
}

func writeSkill(t *testing.T, path, name, description string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
