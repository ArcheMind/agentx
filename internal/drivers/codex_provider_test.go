package drivers

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/agentparams"
	"github.com/ArcheMind/agentx/internal/runtime"
)

func TestCodexProviderUsesNativeEffectiveConfig(t *testing.T) {
	for _, test := range []struct {
		name, response, provider, failure string
	}{
		{"default", `{"result":{"config":{"model_provider":null}}}`, "openai", ""},
		{"custom", `{"result":{"config":{"model_provider":"company"}}}`, "company", ""},
		{"empty", `{"result":{"config":{"model_provider":""}}}`, "", "empty model_provider"},
		{"missing config", `{"result":{}}`, "", "no config"},
		{"native error", `{"error":{"code":-1,"message":"invalid config"}}`, "", "invalid config"},
		{"invalid json", `{`, "", "parse Codex"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &codexModelsRunner{response: json.RawMessage(test.response)}
			plan := runtime.CommandPlan{Cwd: "/target", Env: map[string]string{"CODEX_HOME": "/auth"}}
			provider, err := CodexProvider(context.Background(), runner, plan, agentparams.Request{Flags: []string{"--dangerously-bypass-approvals-and-sandbox", "-c", `model_provider="company"`, "--config=model=example"}})
			if provider != test.provider || (test.failure == "" && err != nil) || (test.failure != "" && (err == nil || !strings.Contains(err.Error(), test.failure))) {
				t.Fatalf("provider = %q, err = %v", provider, err)
			}
			if runner.plan.Cwd != "" || !reflect.DeepEqual(runner.plan.Env, plan.Env) || !reflect.DeepEqual(runner.plan.Args, []string{"app-server", "--stdio", "-c", `model_provider="company"`, "--config=model=example"}) {
				t.Fatalf("plan = %+v", runner.plan)
			}
			var request struct {
				Method string `json:"method"`
				Params struct {
					Cwd string `json:"cwd"`
				} `json:"params"`
			}
			if err := json.Unmarshal(runner.requests[2], &request); err != nil || request.Method != "config/read" || request.Params.Cwd != "/target" {
				t.Fatalf("request = %s, err = %v", runner.requests[2], err)
			}
		})
	}
}

func TestCodexProviderRejectsUnsupportedConfigurationSelection(t *testing.T) {
	disabled := false
	for _, request := range []agentparams.Request{
		{UserConfig: &disabled},
		{Flags: []string{"--profile", "company"}},
		{Flags: []string{"--profile=company"}},
		{Flags: []string{"-p", "company"}},
		{Flags: []string{"--ignore-user-config"}},
	} {
		runner := &codexModelsRunner{}
		if _, err := CodexProvider(context.Background(), runner, runtime.CommandPlan{}, request); err == nil {
			t.Fatalf("silently ignored %#v", request)
		}
		if len(runner.plan.Args) != 0 {
			t.Fatal("started discovery with the wrong configuration")
		}
	}
}
