package drivers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		"dsh":      {Executable: "dsh", Args: []string{"web"}},
		"gemini":   {Executable: "gemini"},
		"opencode": {Executable: "opencode", Args: []string{"auth", "login"}},
		"pi":       {Executable: "node", Args: []string{"<Pi SDK login adapter>"}},
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
	for _, id := range []string{"claude", "codex", "dsh", "opencode", "pi"} {
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
		if agent.Auth.SupportsLogout() != containsCapability(agent.Capabilities, runtime.CapabilityAuthLogout) {
			t.Fatalf("%s auth logout support and capability disagree", agent.ID)
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
	if agent.Models == nil || agent.Auth == nil {
		t.Fatalf("dsh should expose verified model and auth drivers: %#v", agent)
	}
	want := []runtime.Capability{runtime.CapabilityLaunch, runtime.CapabilityAuthLogin, runtime.CapabilityAuthStatus, runtime.CapabilityModelList}
	if !reflect.DeepEqual(agent.Capabilities, want) {
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
		t.Fatal("dsh model selection must remain owned by the native profile")
	}
}

type codexModelsRunner struct {
	response json.RawMessage
	plan     runtime.CommandPlan
	requests []json.RawMessage
}

func (r *codexModelsRunner) Execute(context.Context, runtime.CommandPlan, runtime.ExecuteOptions) (runtime.CommandResult, error) {
	return runtime.CommandResult{}, errors.New("unexpected command execution")
}

func (r *codexModelsRunner) ExecuteJSONRPC(_ context.Context, plan runtime.CommandPlan, requests []json.RawMessage, _ json.RawMessage) (json.RawMessage, error) {
	r.plan = plan
	r.requests = requests
	return r.response, nil
}

func TestCodexModelsUseNativeAccountCatalogInsteadOfCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cachePath := filepath.Join(home, ".codex", "models_cache.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte(`{"models":[{"slug":"gpt-6-sol"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &codexModelsRunner{response: json.RawMessage(`{"id":2,"result":{"data":[{"id":"available","model":"gpt-5.6-sol","displayName":"GPT-5.6-Sol","description":"available","hidden":false}],"nextCursor":null}}`)}
	models, err := (CodexAppServerModels{}).ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gpt-5.6-sol" || models[0].Source != "codex app-server model/list" {
		t.Fatalf("models = %#v", models)
	}
	if runner.plan.Executable != "codex" || !reflect.DeepEqual(runner.plan.Args, []string{"app-server", "--stdio"}) {
		t.Fatalf("plan = %#v", runner.plan)
	}
	if len(runner.requests) != 3 || !strings.Contains(string(runner.requests[2]), `"includeHidden":false`) {
		t.Fatalf("requests = %s", runner.requests)
	}
}

func TestCodexModelsFailClosedOnRPCError(t *testing.T) {
	runner := &codexModelsRunner{response: json.RawMessage(`{"id":2,"error":{"code":-32600,"message":"model catalog unavailable"}}`)}
	if _, err := (CodexAppServerModels{}).ListModels(context.Background(), runner); err == nil || !strings.Contains(err.Error(), "model catalog unavailable") {
		t.Fatalf("error = %v", err)
	}
}

func TestCodexModelCapabilitiesUseNativeAccountCatalog(t *testing.T) {
	agent, err := NewRegistry().Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := agent.Models.(CodexAppServerModels); !ok {
		t.Fatalf("Codex model driver = %T", agent.Models)
	}
	if !containsCapability(agent.Capabilities, runtime.CapabilityModelList) || !containsCapability(agent.Capabilities, runtime.CapabilityModelSelect) {
		t.Fatalf("Codex capabilities = %#v", agent.Capabilities)
	}
}
