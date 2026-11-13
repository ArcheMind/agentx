package drivers

import (
	"context"
	"errors"
	"os"
	"reflect"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/runtime"
)

type agyModelsRunner struct {
	result runtime.CommandResult
	err    error
	plan   runtime.CommandPlan
}

func (r *agyModelsRunner) Execute(_ context.Context, plan runtime.CommandPlan, _ runtime.ExecuteOptions) (runtime.CommandResult, error) {
	r.plan = plan
	return r.result, r.err
}

func TestAGYModelsUseNativeTSVCatalog(t *testing.T) {
	data, err := os.ReadFile("testdata/agy-models.txt")
	if err != nil {
		t.Fatal(err)
	}
	runner := &agyModelsRunner{result: runtime.CommandResult{Stdout: string(data)}}
	models, err := (AGYModels{}).ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if runner.plan.Executable != "agy" || !reflect.DeepEqual(runner.plan.Args, []string{"models"}) {
		t.Fatalf("plan = %#v", runner.plan)
	}
	if len(models) != 6 || models[0].ID != "gemini-3.8-flash-high" || models[0].DisplayName != "Gemini 3.8 Flash (High)" {
		t.Fatalf("models = %#v", models)
	}
	for _, model := range models {
		if model.Source != "agy models" || len(model.Efforts) != 0 || model.DefaultEffort != "" {
			t.Fatalf("invented metadata: %#v", model)
		}
	}
	runner.result.Stdout = "future-model\tFuture model\nfuture-model\tFuture model\n"
	models, err = (AGYModels{}).ListModels(context.Background(), runner)
	if err != nil || len(models) != 1 || models[0].ID != "future-model" {
		t.Fatalf("dynamic models = %#v, error = %v", models, err)
	}
}

func TestAGYModelsFailOnNativeErrorsAndMalformedOutput(t *testing.T) {
	nativeErr := errors.New("native unavailable")
	runner := &agyModelsRunner{err: nativeErr}
	if _, err := (AGYModels{}).ListModels(context.Background(), runner); !errors.Is(err, nativeErr) {
		t.Fatalf("error = %v", err)
	}
	for _, output := range []string{"not a catalog", "model\t", "model with spaces\tName"} {
		runner = &agyModelsRunner{result: runtime.CommandResult{Stdout: output}}
		if _, err := (AGYModels{}).ListModels(context.Background(), runner); err == nil {
			t.Fatalf("accepted output %q", output)
		}
	}
}

func TestAGYLaunchPreservesExactModelAndNativeArguments(t *testing.T) {
	agent, err := NewRegistry().Get("agy")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--effort", "high", "--conversation", "native-id", "--settings", `{"modelProvider":"gemini"}`}
	plan, err := agent.Launch.PlanRun(runtime.RunRequest{Cwd: "/project", Model: "future-native-model", PassthroughArgs: args})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable != "agy" || plan.Cwd != "/project" || !reflect.DeepEqual(plan.Args, append([]string{"--model", "future-native-model"}, args...)) {
		t.Fatalf("plan = %#v", plan)
	}
	if !agent.Auth.SupportsStatus() || !agent.Auth.SupportsLogout() {
		t.Fatal("authentication support does not match native capabilities")
	}
}

func TestAGYInstallUsesPlatformOfficialInstaller(t *testing.T) {
	for _, platform := range []string{"darwin", "linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			plan, err := agyInstallPlan("", platform)
			if err != nil {
				t.Fatal(err)
			}
			if platform == "windows" {
				if plan.Executable != "powershell" || len(plan.Args) != 5 || !reflect.DeepEqual(plan.Args[:4], []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command"}) || !strings.Contains(plan.Args[4], "https://antigravity.google/cli/install.ps1") || !strings.Contains(plan.Args[4], "$ErrorActionPreference = 'Stop'") {
					t.Fatalf("Windows plan = %#v", plan)
				}
			} else if plan.Executable != "bash" || len(plan.Args) != 2 || plan.Args[0] != "-c" || !strings.Contains(plan.Args[1], "https://antigravity.google/cli/install.sh") || !strings.Contains(plan.Args[1], `bash "$installer"`) || !strings.Contains(plan.Args[1], "set -euo pipefail") {
				t.Fatalf("Unix plan = %#v", plan)
			}
			if _, err := agyInstallPlan("1.5.8", platform); err == nil {
				t.Fatal("unsupported version pin accepted")
			}
		})
	}
	got, err := (AGYInstaller{}).PlanInstall("")
	if err != nil {
		t.Fatal(err)
	}
	want, err := agyInstallPlan("", goruntime.GOOS)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("host plan = %#v, want %#v, error = %v", got, want, err)
	}
}

func TestAGYAuthStatusUsesNativeModels(t *testing.T) {
	for _, test := range []struct {
		name          string
		output        string
		err           error
		wantProviders int
	}{
		{name: "available", output: "native-model\tNative Model\n", wantProviders: 1},
		{name: "empty"},
		{name: "native error", err: errors.New("native unavailable")},
		{name: "malformed", output: "not a catalog"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &agyModelsRunner{result: runtime.CommandResult{Stdout: test.output}, err: test.err}
			status, err := (AGYAuth{}).Status(context.Background(), runner)
			if test.err != nil || test.name == "malformed" {
				if err == nil {
					t.Fatal("expected status error")
				}
				if test.err != nil && !errors.Is(err, test.err) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil || !status.Supported || len(status.Providers) != test.wantProviders {
				t.Fatalf("status = %#v, error = %v", status, err)
			}
			if test.wantProviders > 0 && status.Providers[0] != (runtime.AuthProvider{ID: "agy"}) {
				t.Fatalf("account = %#v", status.Providers[0])
			}
		})
	}
}
