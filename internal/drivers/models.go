package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

type CommandModels struct {
	Plan runtime.CommandPlan
}

func (d CommandModels) ListModels(ctx context.Context, runner runtime.Runner) ([]runtime.Model, error) {
	result, err := runner.Execute(ctx, d.Plan, runtime.ExecuteOptions{})
	if err != nil {
		return nil, fmt.Errorf("read models: %w", err)
	}
	return modelsFromLines(result.Stdout, d.Plan.Executable), nil
}

func modelsFromLines(output, source string) []runtime.Model {
	seen := make(map[string]bool)
	var models []runtime.Model
	for _, line := range strings.Split(output, "\n") {
		id := strings.TrimSpace(line)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, runtime.Model{ID: id, Source: source})
	}
	return models
}

type CodexAppServerModels struct{}

func (CodexAppServerModels) ListModels(ctx context.Context, runner runtime.Runner) ([]runtime.Model, error) {
	rpc, ok := runner.(runtime.JSONRPCRunner)
	if !ok {
		return nil, fmt.Errorf("Codex model discovery requires JSON-RPC command support")
	}
	requests := []json.RawMessage{
		json.RawMessage(`{"id":1,"method":"initialize","params":{"clientInfo":{"name":"agentx","title":"AgentX","version":"0"},"capabilities":{"experimentalApi":false}}}`),
		json.RawMessage(`{"method":"initialized"}`),
		json.RawMessage(`{"id":2,"method":"model/list","params":{"includeHidden":false}}`),
	}
	raw, err := rpc.ExecuteJSONRPC(ctx, runtime.CommandPlan{Executable: "codex", Args: []string{"app-server", "--stdio"}}, requests, json.RawMessage(`2`))
	if err != nil {
		return nil, fmt.Errorf("read Codex account models: %w", err)
	}
	var response struct {
		Result *struct {
			Data []struct {
				ID          string `json:"id"`
				Model       string `json:"model"`
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
				Hidden      bool   `json:"hidden"`
			} `json:"data"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("parse Codex model/list response: %w", err)
	}
	if response.Error != nil {
		return nil, fmt.Errorf("Codex model/list failed (%d): %s", response.Error.Code, response.Error.Message)
	}
	if response.Result == nil {
		return nil, fmt.Errorf("Codex model/list returned no result")
	}
	seen := make(map[string]bool)
	models := make([]runtime.Model, 0, len(response.Result.Data))
	for _, item := range response.Result.Data {
		id := item.Model
		if id == "" {
			id = item.ID
		}
		if id == "" || item.Hidden || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, runtime.Model{
			ID: id, DisplayName: item.DisplayName, Description: item.Description, Source: "codex app-server model/list",
		})
	}
	return models, nil
}

type UnsupportedModels struct {
	Agent string
}

func (d UnsupportedModels) ListModels(context.Context, runtime.Runner) ([]runtime.Model, error) {
	return nil, fmt.Errorf("%s supports model selection but exposes no verified local model-list source", d.Agent)
}
