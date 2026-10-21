package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
				Efforts     []struct {
					Effort string `json:"reasoningEffort"`
				} `json:"supportedReasoningEfforts"`
				DefaultEffort string `json:"defaultReasoningEffort"`
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
		model := runtime.Model{
			ID: id, DisplayName: item.DisplayName, Description: item.Description, Source: "codex app-server model/list", DefaultEffort: item.DefaultEffort,
		}
		for _, effort := range item.Efforts {
			if effort.Effort != "" {
				model.Efforts = append(model.Efforts, effort.Effort)
			}
		}
		models = append(models, model)
	}
	return models, nil
}

type ClaudeModels struct{}

func (ClaudeModels) ListModels(ctx context.Context, runner runtime.Runner) ([]runtime.Model, error) {
	control, ok := runner.(runtime.ClaudeControlRunner)
	if !ok {
		return nil, fmt.Errorf("Claude model discovery requires control-protocol command support")
	}
	const requestID = "agentx-models"
	request := json.RawMessage(`{"type":"control_request","request_id":"agentx-models","request":{"subtype":"initialize"}}`)
	commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--settings", `{"disableAllHooks":true}`}
	raw, err := control.ExecuteClaudeControl(commandCtx, runtime.CommandPlan{Executable: "claude", Args: args, UnsetEnv: []string{"CLAUDECODE"}}, request, requestID)
	if err != nil {
		return nil, fmt.Errorf("read Claude account models: %w", err)
	}
	var envelope struct {
		Response *struct {
			Subtype  string `json:"subtype"`
			Error    string `json:"error"`
			Response *struct {
				Models *[]struct {
					Value                 string   `json:"value"`
					DisplayName           string   `json:"displayName"`
					Description           string   `json:"description"`
					SupportedEffortLevels []string `json:"supportedEffortLevels"`
				} `json:"models"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse Claude initialize response: %w", err)
	}
	if envelope.Response == nil {
		return nil, fmt.Errorf("Claude initialize response has no response object")
	}
	if envelope.Response.Subtype != "success" {
		return nil, fmt.Errorf("Claude initialize failed (%s): %s", envelope.Response.Subtype, envelope.Response.Error)
	}
	if envelope.Response.Response == nil || envelope.Response.Response.Models == nil {
		return nil, fmt.Errorf("Claude initialize response has no models")
	}
	models := make([]runtime.Model, 0, len(*envelope.Response.Response.Models))
	seen := make(map[string]bool)
	for _, item := range *envelope.Response.Response.Models {
		if item.Value == "" || seen[item.Value] {
			continue
		}
		seen[item.Value] = true
		models = append(models, runtime.Model{ID: item.Value, DisplayName: item.DisplayName, Description: item.Description, Source: "claude initialize", Efforts: item.SupportedEffortLevels})
	}
	return models, nil
}

type UnsupportedModels struct {
	Agent string
}

func (d UnsupportedModels) ListModels(context.Context, runtime.Runner) ([]runtime.Model, error) {
	return nil, fmt.Errorf("%s supports model selection but exposes no verified local model-list source", d.Agent)
}
