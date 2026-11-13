package drivers

import (
	"context"
	"errors"
	"os"
	"reflect"
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
	if agent.Auth.SupportsStatus() || !agent.Auth.SupportsLogout() {
		t.Fatal("authentication support does not match native capabilities")
	}
}

func TestAGYInstallUsesOfficialInstallerAndRejectsVersionPins(t *testing.T) {
	plan, err := (AGYInstaller{}).PlanInstall("")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable != "sh" || len(plan.Args) != 2 || plan.Args[0] != "-c" || !strings.Contains(plan.Args[1], "https://antigravity.google/cli/install.sh") || !strings.Contains(plan.Args[1], `sh "$installer"`) {
		t.Fatalf("plan = %#v", plan)
	}
	if _, err := (AGYInstaller{}).PlanInstall("1.5.8"); err == nil {
		t.Fatal("unsupported version pin accepted")
	}
}
