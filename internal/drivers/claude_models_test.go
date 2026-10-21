package drivers

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/runtime"
)

type claudeModelsRunner struct {
	response  json.RawMessage
	plan      runtime.CommandPlan
	request   json.RawMessage
	requestID string
	deadline  bool
}

func (r *claudeModelsRunner) Execute(context.Context, runtime.CommandPlan, runtime.ExecuteOptions) (runtime.CommandResult, error) {
	return runtime.CommandResult{}, nil
}

func (r *claudeModelsRunner) ExecuteClaudeControl(ctx context.Context, plan runtime.CommandPlan, request json.RawMessage, requestID string) (json.RawMessage, error) {
	r.plan, r.request, r.requestID = plan, request, requestID
	_, r.deadline = ctx.Deadline()
	return r.response, nil
}

func TestClaudeModelsUseInitializeControlResponse(t *testing.T) {
	runner := &claudeModelsRunner{response: json.RawMessage(`{"type":"control_response","response":{"request_id":"agentx-models","subtype":"success","response":{"models":[{"value":"claude-opus","displayName":"Claude Opus","description":"Current model","supportedEffortLevels":["low","high"]},{"value":"claude-opus","displayName":"duplicate"},{"value":""}]}}}`)}
	models, err := (ClaudeModels{}).ListModels(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "claude-opus" || models[0].DisplayName != "Claude Opus" || models[0].Description != "Current model" || models[0].Source != "claude initialize" || !reflect.DeepEqual(models[0].Efforts, []string{"low", "high"}) {
		t.Fatalf("models = %#v", models)
	}
	if !runner.deadline || runner.requestID != "agentx-models" || !strings.Contains(string(runner.request), `"subtype":"initialize"`) {
		t.Fatalf("initialize request/call = %#v", runner)
	}
	if runner.plan.Executable != "claude" || !reflect.DeepEqual(runner.plan.UnsetEnv, []string{"CLAUDECODE"}) || !containsArgSequence(runner.plan.Args, "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`) || !containsArgSequence(runner.plan.Args, "--settings", `{"disableAllHooks":true}`) {
		t.Fatalf("plan = %#v", runner.plan)
	}
}

func TestClaudeModelsAcceptEmptyCatalogAndRejectBadResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty catalog", body: `{"response":{"request_id":"agentx-models","subtype":"success","response":{"models":[]}}}`},
		{name: "initialize error", body: `{"response":{"request_id":"agentx-models","subtype":"error","error":"unavailable"}}`, want: "unavailable"},
		{name: "missing models", body: `{"response":{"request_id":"agentx-models","subtype":"success","response":{}}}`, want: "no models"},
		{name: "malformed response", body: `{"response":`, want: "parse Claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &claudeModelsRunner{response: json.RawMessage(tc.body)}
			models, err := (ClaudeModels{}).ListModels(context.Background(), runner)
			if tc.want == "" {
				if err != nil || len(models) != 0 {
					t.Fatalf("models=%#v err=%v", models, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func containsArgSequence(args []string, sequence ...string) bool {
	for i := 0; i+len(sequence) <= len(args); i++ {
		if reflect.DeepEqual(args[i:i+len(sequence)], sequence) {
			return true
		}
	}
	return false
}
