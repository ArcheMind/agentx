package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArcheMind/agentx/internal/agentparams"
	"github.com/ArcheMind/agentx/internal/runtime"
)

// CodexProvider delegates project and user config layering to Codex itself.
func CodexProvider(ctx context.Context, runner runtime.Runner, plan runtime.CommandPlan, input agentparams.Request) (string, error) {
	rpc, ok := runner.(runtime.JSONRPCRunner)
	if !ok {
		return "", fmt.Errorf("Codex provider discovery requires JSON-RPC command support")
	}
	plan.Executable = "codex"
	plan.Args = []string{"app-server", "--stdio"}
	configArgs, err := agentparams.CodexConfigArgs(input)
	if err != nil {
		return "", err
	}
	plan.Args = append(plan.Args, configArgs...)
	params := map[string]any{"includeLayers": false}
	if plan.Cwd != "" {
		params["cwd"] = plan.Cwd
	}
	request, err := json.Marshal(map[string]any{"id": 2, "method": "config/read", "params": params})
	if err != nil {
		return "", err
	}
	// Imported history may name a workspace that no longer exists.
	plan.Cwd = ""
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := rpc.ExecuteJSONRPC(ctx, plan, []json.RawMessage{
		json.RawMessage(`{"id":1,"method":"initialize","params":{"clientInfo":{"name":"sessionman","version":"0"},"capabilities":{"experimentalApi":false}}}`),
		json.RawMessage(`{"method":"initialized"}`),
		request,
	}, json.RawMessage(`2`))
	if err != nil {
		return "", fmt.Errorf("read Codex provider: %w", err)
	}
	var response struct {
		Result *struct {
			Config *struct {
				Provider *string `json:"model_provider"`
			} `json:"config"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", fmt.Errorf("parse Codex config/read response: %w", err)
	}
	if response.Error != nil {
		return "", fmt.Errorf("Codex config/read failed (%d): %s", response.Error.Code, response.Error.Message)
	}
	if response.Result == nil || response.Result.Config == nil {
		return "", fmt.Errorf("Codex config/read returned no config")
	}
	if response.Result.Config.Provider == nil {
		return "openai", nil
	}
	if strings.TrimSpace(*response.Result.Config.Provider) == "" {
		return "", fmt.Errorf("Codex config/read returned an empty model_provider")
	}
	return *response.Result.Config.Provider, nil
}
