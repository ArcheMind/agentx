package subagents

import (
	"github.com/ArcheMind/agentx/internal/presets"
	"strings"
	"testing"
)

func TestCodexSilentEffortClampIsReported(t *testing.T) {
	record := installFakeNative(t, "codex", map[string]string{"SM_FAKE_EFFORT": "max"})
	s := testServer(t, nil)
	ultra := reviewPreset
	ultra.Effort = "ultra"
	savePreset(t, s.Home, ultra)
	clamped := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "run_in_background": false})
	if !strings.Contains(clamped, "WARNING") || !strings.Contains(clamped, "ultra") || !strings.Contains(clamped, "max") {
		t.Fatal(clamped)
	}
	if env := readFakeRecords(t, record)[0].Env["RUST_LOG"]; env != codexTelemetryFilter {
		t.Fatal(env)
	}
	native := reviewPreset
	native.Effort = ""
	savePreset(t, s.Home, native)
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "run_in_background": false})
	if !strings.Contains(text, "effort=native default") || !strings.Contains(text, "Native evidence: executed effort max") {
		t.Fatal(text)
	}
}

func TestCodexEffortWithoutTelemetryIsUnverified(t *testing.T) {
	if summary, err := verifyEffort("high", nil, "Codex response.completed telemetry"); err != nil || !strings.Contains(summary, "could not be verified") {
		t.Fatal(summary, err)
	}
	if summary, err := verifyEffort("", nil, "x"); err != nil || summary != "" {
		t.Fatal(summary, err)
	}
	stderr := "x INFO codex_otel.trace_safe: event.name=\"codex.sse_event\" event.kind=response.created model_reasoning_effort=\"low\"\n" +
		"x INFO codex_otel.trace_safe: event.name=\"codex.sse_event\" event.kind=response.completed input_token_count=1 model_reasoning_effort=\"high\" slug=m\n"
	if got := codexExecutedEfforts(stderr); strings.Join(got, ",") != "high" {
		t.Fatal(got)
	}
	if got := withoutCodexTelemetry(stderr + "real error\n"); strings.Contains(got, "codex_otel") || !strings.Contains(got, "real error") {
		t.Fatal(got)
	}
}

func piServer(t *testing.T, effort string) *Server {
	s := testServer(t, nil)
	savePreset(t, s.Home, presets.Preset{Name: "pi-review", Agent: "pi", Model: "provider/model", Description: "Pi", Effort: effort})
	return s
}

func TestPiEffortVerifiedFromObserverRecords(t *testing.T) {
	record := installFakeNative(t, "pi", map[string]string{"SM_FAKE_PI_EFFORT": "high"})
	s := piServer(t, "high")
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "name": "pi", "model": "pi-review", "run_in_background": false})
	if !strings.Contains(text, "effort=high") || !strings.Contains(text, "executed effort high (1 model request(s)") {
		t.Fatal(text)
	}
	args := strings.Join(readFakeRecords(t, record)[0].Argv, " ")
	for _, want := range []string{"--thinking high", "--extension ", "sessionman-observer.js"} {
		if !strings.Contains(args, want) {
			t.Fatal(want, args)
		}
	}
	if !strings.Contains(args, "--append-system-prompt") {
		t.Fatal("append-system-prompt should be present for presets with a description", args)
	}
	callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "pi", "message": "again"})
	last := waitReports(t, s, "pi", 2)
	if last.Status != "completed" || !strings.Contains(last.Evidence, "1 model request(s)") {
		t.Fatal("continuation evidence must count only the new turn", last.Status, last.Failure, last.Evidence)
	}
}

func TestPiClampAndMissingObserverReported(t *testing.T) {
	cases := []struct{ name, thinking, effort, want string }{
		{"session clamp", "medium", "medium", `thinking level "medium"`},
		{"provider remap", "high", "low", "WARNING: preset requested high but native executed low"},
		{"no effort field", "high", "null", "WARNING: preset requested high but native executed none sent"},
		{"observer absent", "high", "skip", "could not be verified"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			installFakeNative(t, "pi", map[string]string{"SM_FAKE_PI_THINKING": c.thinking, "SM_FAKE_PI_EFFORT": c.effort})
			s := piServer(t, "high")
			text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "pi-review", "run_in_background": false})
			if !strings.Contains(text, c.want) {
				t.Fatal(text)
			}
		})
	}
}

func TestPiWithoutEffortKeepsPlainPlan(t *testing.T) {
	record := installFakeNative(t, "pi", nil)
	s := piServer(t, "")
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "pi-review", "run_in_background": false})
	args := strings.Join(readFakeRecords(t, record)[0].Argv, " ")
	if strings.Contains(args, "--extension") || strings.Contains(args, "--thinking") || strings.Contains(text, "Native evidence") {
		t.Fatal(args, text)
	}
}

func TestNativeEffortRejectionReasonIsReturned(t *testing.T) {
	installFakeNative(t, "codex", map[string]string{"SM_FAKE_FAILMSG": "Unsupported value: 'minimal' is not supported with the 'test-model' model."})
	s := testServer(t, nil)
	if _, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"FAIL","model":"review","run_in_background":false}`)); err == nil || !strings.Contains(err.Error(), "Unsupported value: 'minimal'") {
		t.Fatal(err)
	}
}
