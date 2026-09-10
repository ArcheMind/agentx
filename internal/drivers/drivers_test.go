package drivers

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"agentx/internal/runtime"
)

func TestRegistryContainsSupportedAgents(t *testing.T) {
	registry := NewRegistry()
	want := []string{"casr", "claude", "codex", "gemini", "opencode", "pi"}
	all := registry.All()
	got := make([]string, 0, len(all))
	for _, agent := range all {
		got = append(got, agent.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agent IDs = %v, want %v", got, want)
	}
}

func TestNPMPackagePlanUsesArgumentBoundaries(t *testing.T) {
	plan, err := (NPMPackage{Package: "@openai/codex"}).PlanInstall("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "--global", "@openai/codex@1.2.3"}
	if plan.Executable != "npm" || !reflect.DeepEqual(plan.Args, want) {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestNativeLaunchPreservesPassthroughArgs(t *testing.T) {
	plan, err := (NativeLaunch{Binary: "codex", ModelFlag: "--model"}).PlanRun(runtime.RunRequest{
		Cwd: "/tmp/project", Model: "gpt-test", PassthroughArgs: []string{"--full-auto", "fix it"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", "gpt-test", "--full-auto", "fix it"}
	if !reflect.DeepEqual(plan.Args, want) || plan.Cwd != "/tmp/project" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestCodexCacheModels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	data := []byte(`{"models":[{"slug":"gpt-test","display_name":"GPT Test","description":"fixture"}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	models, err := (CodexCacheModels{Path: path}).ListModels(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gpt-test" || models[0].Source != path {
		t.Fatalf("models = %#v", models)
	}
}

func TestCASRRejectsUnknownSubcommand(t *testing.T) {
	_, err := (CASRSessions{}).Plan([]string{"delete", "session"})
	if err == nil {
		t.Fatal("expected unsupported command error")
	}
}
