package drivers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ArcheMind/agentx/internal/runtime"
)

func TestRegistryContainsSupportedAgents(t *testing.T) {
	registry := NewRegistry()
	want := []string{"claude", "codex", "dsh", "gemini", "opencode", "pi"}
	all := registry.All()
	got := make([]string, 0, len(all))
	for _, agent := range all {
		got = append(got, agent.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agent IDs = %v, want %v", got, want)
	}
}

func TestNativeAuthPlansUseAgentOAuthFlows(t *testing.T) {
	want := map[string]runtime.CommandPlan{
		"claude":   {Executable: "claude", Args: []string{"auth", "login", "--claudeai"}},
		"codex":    {Executable: "codex", Args: []string{"login"}},
		"gemini":   {Executable: "gemini"},
		"opencode": {Executable: "opencode", Args: []string{"auth", "login"}},
		"pi":       {Executable: "pi"},
	}
	registry := NewRegistry()
	for id, expected := range want {
		agent, err := registry.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if agent.Auth == nil {
			t.Fatalf("%s has no auth driver", id)
		}
		if got := agent.Auth.PlanLogin().Command; !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s auth plan = %#v, want %#v", id, got, expected)
		}
	}
}

func TestNativeAuthLogoutPlans(t *testing.T) {
	want := map[string]runtime.CommandPlan{
		"claude":   {Executable: "claude", Args: []string{"auth", "logout"}},
		"codex":    {Executable: "codex", Args: []string{"logout"}},
		"gemini":   {Executable: "gemini"},
		"opencode": {Executable: "opencode", Args: []string{"auth", "logout"}},
		"pi":       {Executable: "pi"},
	}
	registry := NewRegistry()
	for id, expected := range want {
		agent, err := registry.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := agent.Auth.PlanLogout().Command; !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s logout plan = %#v, want %#v", id, got, expected)
		}
	}
}

func TestNativeAuthStatusCommands(t *testing.T) {
	want := map[string]runtime.CommandPlan{
		"claude":   {Executable: "claude", Args: []string{"auth", "status", "--json"}},
		"codex":    {Executable: "codex", Args: []string{"login", "status"}},
		"opencode": {Executable: "opencode", Args: []string{"auth", "list"}},
	}
	registry := NewRegistry()
	for id, expected := range want {
		agent, err := registry.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		auth, ok := agent.Auth.(NativeAuth)
		if !ok {
			t.Fatalf("%s auth driver has type %T", id, agent.Auth)
		}
		if !reflect.DeepEqual(auth.StatusCommand, expected) {
			t.Fatalf("%s status command = %#v, want %#v", id, auth.StatusCommand, expected)
		}
	}
}

func TestNativeAuthStatusParsers(t *testing.T) {
	tests := []struct {
		name   string
		parser func(runtime.CommandResult, error) (runtime.AuthStatus, error)
		result runtime.CommandResult
		want   runtime.AuthStatus
	}{
		{
			name:   "claude",
			parser: parseClaudeAuthStatus,
			result: runtime.CommandResult{Stdout: `{"loggedIn":true,"authMethod":"claude.ai","email":"private@example.com","orgId":"private","subscriptionType":"max"}`},
			want:   runtime.AuthStatus{Supported: true, Providers: []runtime.AuthProvider{{ID: "claude", Method: "claude.ai", Subscription: "max"}}},
		},
		{
			name:   "codex logged in",
			parser: parseCodexAuthStatus,
			result: runtime.CommandResult{Stdout: "Logged in using ChatGPT\n"},
			want:   runtime.AuthStatus{Supported: true, Providers: []runtime.AuthProvider{{ID: "codex", Method: "ChatGPT"}}},
		},
		{
			name:   "codex logged out",
			parser: parseCodexAuthStatus,
			result: runtime.CommandResult{Stderr: "Not logged in\n", ExitCode: 1},
			want:   runtime.AuthStatus{Supported: true, Providers: []runtime.AuthProvider{}},
		},
		{
			name:   "opencode",
			parser: parseOpenCodeAuthStatus,
			result: runtime.CommandResult{Stdout: "\x1b[0mCredentials\n●  GitHub Copilot \x1b[90moauth\n1 credentials\nEnvironment\n4 environment variables\n"},
			want:   runtime.AuthStatus{Supported: true, Providers: []runtime.AuthProvider{{ID: "GitHub Copilot", Method: "oauth"}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var executeErr error
			if test.result.ExitCode != 0 {
				executeErr = errors.New("native command exited")
			}
			got, err := test.parser(test.result, executeErr)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("status = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestAuthStatusCapabilitiesMatchNativeSupport(t *testing.T) {
	registry := NewRegistry()
	for _, id := range []string{"claude", "codex", "opencode", "pi"} {
		agent, err := registry.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if !agent.Auth.SupportsStatus() || !containsCapability(agent.Capabilities, runtime.CapabilityAuthStatus) {
			t.Fatalf("%s should support auth status", id)
		}
	}
	for _, id := range []string{"gemini"} {
		agent, err := registry.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if agent.Auth.SupportsStatus() || containsCapability(agent.Capabilities, runtime.CapabilityAuthStatus) {
			t.Fatalf("%s should not support auth status", id)
		}
	}
}

func TestAuthLogoutCapabilitiesMatchNativeSupport(t *testing.T) {
	for _, agent := range NewRegistry().All() {
		if agent.Auth == nil {
			continue
		}
		if !agent.Auth.SupportsLogout() || !containsCapability(agent.Capabilities, runtime.CapabilityAuthLogout) {
			t.Fatalf("%s should support auth logout", agent.ID)
		}
	}
}

func TestInstallDriversBelongToAgents(t *testing.T) {
	for _, agent := range NewRegistry().All() {
		if agent.Install == nil {
			t.Fatalf("%s has no install driver", agent.ID)
		}
	}
}

func containsCapability(capabilities []runtime.Capability, target runtime.Capability) bool {
	for _, capability := range capabilities {
		if capability == target {
			return true
		}
	}
	return false
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

func TestDSHHasOnlyVerifiedDrivers(t *testing.T) {
	agent, err := NewRegistry().Get("dsh")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Models != nil || agent.Auth != nil {
		t.Fatalf("dsh should not expose unverified model or auth drivers: %#v", agent)
	}
	if !reflect.DeepEqual(agent.Capabilities, []runtime.Capability{runtime.CapabilityLaunch}) {
		t.Fatalf("dsh capabilities = %#v", agent.Capabilities)
	}
	plan, err := agent.Launch.PlanRun(runtime.RunRequest{Cwd: "/tmp/project", PassthroughArgs: []string{"web", "--no-open"}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable != "dsh" || !reflect.DeepEqual(plan.Args, []string{"web", "--no-open"}) || plan.Cwd != "/tmp/project" {
		t.Fatalf("dsh launch plan = %#v", plan)
	}
	if _, err := agent.Launch.PlanRun(runtime.RunRequest{Model: "unverified"}); err == nil {
		t.Fatal("dsh model selection should be rejected")
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
