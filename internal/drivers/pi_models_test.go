package drivers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"agentx/internal/runtime"
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

func TestPiModelsFiltersMultipleLoggedInProviders(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: piModelsFixture}}
	driver := PiModels{
		Plan:      runtime.CommandPlan{Executable: "pi", Args: []string{"--list-models"}},
		Providers: staticProviderSource{providers: []string{"anthropic", "openai-codex"}},
	}
	models, err := driver.ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := []runtime.Model{
		{ID: "anthropic/claude-sonnet-4", Source: "pi"},
		{ID: "openai-codex/gpt-5.3-codex", Source: "pi"},
		{ID: "anthropic/claude-haiku-4.5", Source: "pi"},
	}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestPiModelsExcludesProvidersWithoutNativeLogin(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: piModelsFixture}}
	driver := PiModels{
		Plan:      runtime.CommandPlan{Executable: "pi", Args: []string{"--list-models"}},
		Providers: staticProviderSource{providers: []string{"github-copilot"}},
	}
	models, err := driver.ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := []runtime.Model{{ID: "github-copilot/gpt-5.4", Source: "pi"}}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestPiModelsReturnsEmptyWithoutLoggedInProviders(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: piModelsFixture}}
	driver := PiModels{
		Plan:      runtime.CommandPlan{Executable: "pi", Args: []string{"--list-models"}},
		Providers: staticProviderSource{},
	}
	models, err := driver.ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if models == nil || len(models) != 0 {
		t.Fatalf("models = %#v, want non-nil empty list", models)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestPiModelsRejectsMalformedOutput(t *testing.T) {
	runner := &modelRunner{result: runtime.CommandResult{Stdout: "provider model\nanthropic claude-sonnet-4"}}
	driver := PiModels{
		Plan:      runtime.CommandPlan{Executable: "pi", Args: []string{"--list-models"}},
		Providers: staticProviderSource{providers: []string{"anthropic"}},
	}
	if _, err := driver.ListModels(context.Background(), runner); err == nil {
		t.Fatal("expected malformed Pi model output to fail")
	}
}

func TestPiAuthFileProvidersReadsProviderIDsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	raw := []byte(`{
  "anthropic": {"type":"api_key","key":"secret"},
  "github-copilot": {"type":"oauth","access":"secret","refresh":"secret","expires":1}
}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	providers, err := (PiAuthFileProviders{Path: path}).ListLoggedInProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"anthropic": true, "github-copilot": true}
	if len(providers) != len(want) {
		t.Fatalf("providers = %v, want %v", providers, want)
	}
	for _, provider := range providers {
		if !want[provider] {
			t.Fatalf("unexpected provider %q", provider)
		}
	}
}

func TestPiAuthFileProvidersTreatsMissingFileAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	providers, err := (PiAuthFileProviders{Path: path}).ListLoggedInProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if providers == nil || len(providers) != 0 {
		t.Fatalf("providers = %#v, want non-nil empty list", providers)
	}
}

func TestPiAuthFileProvidersRejectsMalformedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(`{"anthropic":`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (PiAuthFileProviders{Path: path}).ListLoggedInProviders(context.Background())
	if err == nil {
		t.Fatal("expected malformed Pi auth state to fail")
	}
}

func TestPiModelsPropagatesProviderSourceError(t *testing.T) {
	want := errors.New("provider state unavailable")
	driver := PiModels{Providers: staticProviderSource{err: want}}
	_, err := driver.ListModels(context.Background(), &modelRunner{})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
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
