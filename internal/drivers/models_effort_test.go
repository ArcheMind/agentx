package drivers

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestCodexModelsReportListedReasoningEfforts(t *testing.T) {
	runner := &codexModelsRunner{response: json.RawMessage(`{"id":2,"result":{"data":[{"model":"listed","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"max"}],"defaultReasoningEffort":"low"},{"model":"bare"}]}}`)}
	models, err := (CodexAppServerModels{}).ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || !reflect.DeepEqual(models[0].Efforts, []string{"low", "max"}) || models[0].DefaultEffort != "low" || models[1].Efforts != nil {
		t.Fatalf("models = %#v", models)
	}
}
