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

const piModelsFixture = `provider        model                  context  max-out  thinking  images
anthropic       claude-sonnet-4       200K     64K      yes       yes
github-copilot  gpt-5.4               400K     128K     yes       yes
openai-codex    gpt-5.3-codex         400K     128K     yes       yes
anthropic       claude-haiku-4.5      200K     64K      yes       yes
`

type staticProviderSource struct {
	providers []string
	err       error
}

func (s staticProviderSource) ListLoggedInProviders(context.Context) ([]string, error) {
	return s.providers, s.err
}

type modelRunner struct {
	result runtime.CommandResult
	err    error
	calls  int
}

func (r *modelRunner) Execute(context.Context, runtime.CommandPlan, runtime.ExecuteOptions) (runtime.CommandResult, error) {
	r.calls++
	return r.result, r.err
}

var piListPlan = runtime.CommandPlan{Executable: "pi", Args: []string{"--list-models"}}

func TestPiModelsListsEveryRowPiReportsAvailable(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: piModelsFixture}}
	models, err := PiModels{Plan: piListPlan}.ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := []runtime.Model{
		{ID: "anthropic/claude-sonnet-4", Source: "pi"},
		{ID: "github-copilot/gpt-5.4", Source: "pi"},
		{ID: "openai-codex/gpt-5.3-codex", Source: "pi"},
		{ID: "anthropic/claude-haiku-4.5", Source: "pi"},
	}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestPiModelsReturnsEmptyWhenPiListsNothing(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: "provider model context max-out thinking images\n"}}
	models, err := PiModels{Plan: piListPlan}.ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if models == nil || len(models) != 0 {
		t.Fatalf("models = %#v, want non-nil empty list", models)
	}
}

func TestPiModelsRejectsMalformedOutput(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: "provider model\nanthropic claude-sonnet-4"}}
	if _, err := (PiModels{Plan: piListPlan}).ListModels(context.Background(), runner); err == nil {
		t.Fatal("expected malformed Pi model output to fail")
	}
}

func TestDSHAuthProvidersReportsConfiguredKeyWithoutReadingItsValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".credentials.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nrefs:\n  DEEPSEEK_API_KEY: secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	providers, err := (DSHAuthProviders{Path: path}).ListLoggedInProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(providers, []string{dshProviderID}) {
		t.Fatalf("providers = %#v", providers)
	}
}

func TestDSHAuthProvidersHonorsEnvironmentWithoutReadingCredentialFile(t *testing.T) {
	providers, err := (DSHAuthProviders{Path: filepath.Join(t.TempDir(), "missing"), Env: func(string) string { return "secret" }}).ListLoggedInProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(providers, []string{dshProviderID}) {
		t.Fatalf("providers = %#v", providers)
	}
}

func TestDSHModelsExposeBundledCatalog(t *testing.T) {
	models, err := (DSHModels{}).ListModels(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 4 || models[0].ID != "deepseek-flash" || models[2].ID != "deepseek-v4-pro" {
		t.Fatalf("models = %#v", models)
	}
}

func TestPiModelsPropagatesListCommandError(t *testing.T) {
	want := errors.New("pi exited 1")
	_, err := PiModels{Plan: piListPlan}.ListModels(context.Background(), &modelRunner{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestPiModelsIncludesCustomAPIProvider(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: piModelsFixture + "deepseek       deepseek-v4-pro        128K     16K      yes       no\n"}}
	models, err := PiModels{Plan: piListPlan}.ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := runtime.Model{ID: "deepseek/deepseek-v4-pro", Source: "pi"}
	if last := models[len(models)-1]; !reflect.DeepEqual(last, want) {
		t.Fatalf("last model = %#v, want %#v", last, want)
	}
}

func TestPiSDKAuthStatusDerivesProvidersFromModelList(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: piModelsFixture}}
	status, err := PiSDKAuth{Models: PiModels{Plan: piListPlan}}.Status(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := runtime.AuthStatus{Supported: true, Providers: []runtime.AuthProvider{{ID: "anthropic"}, {ID: "github-copilot"}, {ID: "openai-codex"}}}
	if !reflect.DeepEqual(status, want) {
		t.Fatalf("status = %#v, want %#v", status, want)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
}

func TestPiSDKAuthStatusReportsNotLoggedInWhenPiListsNothing(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: "provider model context max-out thinking images\n"}}
	status, err := PiSDKAuth{Models: PiModels{Plan: piListPlan}}.Status(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Providers) != 0 {
		t.Fatalf("providers = %#v, want none", status.Providers)
	}
}

func TestProviderAuthUsesSameProviderSourceAsModels(t *testing.T) {
	driver := ProviderAuth{
		NativeAuth: NativeAuth{LogoutCommand: runtime.CommandPlan{Executable: "pi"}},
		Providers:  staticProviderSource{providers: []string{"anthropic", "openai-codex"}},
	}
	status, err := driver.Status(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := runtime.AuthStatus{Supported: true, Providers: []runtime.AuthProvider{{ID: "anthropic"}, {ID: "openai-codex"}}}
	if !reflect.DeepEqual(status, want) {
		t.Fatalf("status = %#v, want %#v", status, want)
	}
}
