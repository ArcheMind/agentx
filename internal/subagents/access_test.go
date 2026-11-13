package subagents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
)

func installPi(t *testing.T) {
	t.Helper()
	bin := filepath.Dir(must(t, "codex"))
	if err := os.WriteFile(filepath.Join(bin, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidModesRejectedBeforeAnyWork(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	installPi(t)
	savePreset(t, s.Home, presets.Preset{Name: "pi-review", Agent: "pi", Model: "provider/model", Description: "d"})
	for input, want := range map[string]string{
		`{"description":"x","prompt":"x","model":"review","mode":"dontAsk"}`:        `mode "dontAsk" unsupported`,
		`{"description":"x","prompt":"x","model":"review","mode":""}`:               "must not be empty",
		`{"description":"x","prompt":"x","model":"review","write":true}`:            "unknown field",
		`{"description":"x","prompt":"x","model":"review","writable_roots":["/x"]}`: "unknown field",
	} {
		_, err := s.call("ExternalSubagent", []byte(input))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", input, err, want)
		}
	}
	if len(r.plans) != 0 || len(s.agents) != 0 {
		t.Fatal("a rejected request started work")
	}
}

func TestNormalModePassesNoPermissionFlags(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "read", "prompt": "review", "model": "review", "mode": "normal", "run_in_background": false})
	r.mu.Lock()
	defer r.mu.Unlock()
	codex := strings.Join(r.plans[0].Args, " ")
	if strings.Contains(codex, "sandbox") || !strings.Contains(codex, `approval_policy="never"`) || !strings.Contains(text, "Mode: normal\n") || !strings.Contains(text, `Auth source for this turn: CODEX_HOME="`+os.Getenv("CODEX_HOME")+`"`+"\n") {
		t.Fatal(codex, text)
	}
	plan, err := drivers.ClaudePolicyExecPlan("/cwd", "opus", "", false, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	claude := strings.Join(plan.Args, " ")
	if strings.Contains(claude, "permission") || strings.Contains(claude, "dangerously") || !strings.Contains(claude, "--input-format stream-json") {
		t.Fatal(claude)
	}
	if r.plans[0].Env["CODEX_HOME"] != os.Getenv("CODEX_HOME") || len(r.plans[0].UnsetEnv) != 0 {
		t.Fatalf("auth environment not explicit: %#v %#v", r.plans[0].Env, r.plans[0].UnsetEnv)
	}
}

func TestYoloModeBypassesNativePermissions(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "mode": "yolo", "run_in_background": false})
	r.mu.Lock()
	defer r.mu.Unlock()
	codex := strings.Join(r.plans[0].Args, " ")
	if !strings.Contains(codex, `-c sandbox_mode="danger-full-access" -c approval_policy="never"`) || !strings.Contains(text, "Mode: yolo\n") {
		t.Fatal(codex, text)
	}
	plan, err := drivers.ClaudePolicyExecPlan("/cwd", "opus", "", true, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Args, " "), "--dangerously-skip-permissions") {
		t.Fatal(plan.Args)
	}
}

func TestModeIsFixedForContinuation(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "name": "y", "mode": "yolo", "run_in_background": false})
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "name": "n", "mode": "normal", "run_in_background": false})
	for _, name := range []string{"y", "n"} {
		callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": name, "message": "again"})
		waitReports(t, s, name, 2)
	}
	if _, err := s.call("SendExternalSubagentMessage", []byte(`{"to":"n","message":"x","mode":"yolo"}`)); err == nil {
		t.Fatal("continuation accepted a mode change")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	yolo := func(p runtime.CommandPlan) bool {
		return strings.Contains(strings.Join(p.Args, " "), "danger-full-access")
	}
	if len(r.plans) != 4 || !yolo(r.plans[0]) || !yolo(r.plans[2]) || yolo(r.plans[1]) || yolo(r.plans[3]) {
		t.Fatal("continuation changed mode:", len(r.plans))
	}
}

func TestAuthSourceIsNativeDefaultUnlessEnvironmentSetsIt(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	home, _ := os.UserHomeDir()
	if key, dir, err := authSource("codex"); err != nil || key != "CODEX_HOME" || dir != filepath.Join(home, ".codex") {
		t.Fatal(key, dir, err)
	}
	if key, dir, err := authSource("pi"); err != nil || key != "PI_CODING_AGENT_DIR" || dir != filepath.Join(home, ".pi", "agent") {
		t.Fatal(key, dir, err)
	}
	t.Setenv("CODEX_HOME", "relative")
	if _, _, err := authSource("codex"); err == nil || !strings.Contains(err.Error(), "no fallback identity") {
		t.Fatal(err)
	}
}

func TestAuthSourcePinnedWithoutPreflight(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	installPi(t)
	savePreset(t, s.Home, presets.Preset{Name: "review", Agent: "pi", Model: "provider/model", Description: "d"})
	_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","run_in_background":false}`))
	if err == nil || !strings.Contains(err.Error(), `PI_CODING_AGENT_DIR="`+os.Getenv("PI_CODING_AGENT_DIR")+`"`+"\n") {
		t.Fatal("pinned Pi auth source not reported:", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.plans) != 1 || r.plans[0].Args[0] == "auth" || r.plans[0].Env["PI_CODING_AGENT_DIR"] != os.Getenv("PI_CODING_AGENT_DIR") {
		t.Fatalf("native execution not pinned to the auth source: %#v", r.plans)
	}
}

func must(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

type leakingRunner struct{}

func (leakingRunner) Execute(_ context.Context, _ runtime.CommandPlan, _ runtime.ExecuteOptions) (runtime.CommandResult, error) {
	return runtime.CommandResult{Stderr: "auth header Bearer abcdefghijklmnopqrstuvwxyz0123 key sk-proj-abcdefghijklmnopqrstuvwxyz", ExitCode: 1}, os.ErrPermission
}

func TestNativeFailureReturnedToClaudeIsRedacted(t *testing.T) {
	s := testServer(t, leakingRunner{})
	_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","run_in_background":false}`))
	if err == nil || strings.Contains(err.Error(), "abcdefghijklmnop") || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatal(err)
	}
	a := &ExternalSubagentState{Option: presets.Preset{Agent: "codex"}}
	if _, _, err := parseOutput(a, `{"type":"turn.failed","error":{"message":"bad key sk-proj-abcdefghijklmnopqrstuvwxyz"}}`); err == nil || strings.Contains(err.Error(), "abcdefghijklmnop") {
		t.Fatal(err)
	}
}

func TestUserConfigFalseAddsIsolationFlags(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	f := false
	p := reviewPreset
	p.UserConfig = &f
	savePreset(t, s.Home, p)
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "run_in_background": false})
	r.mu.Lock()
	args := strings.Join(r.plans[0].Args, " ")
	r.mu.Unlock()
	for _, want := range []string{"--ignore-user-config", "--ignore-rules", "--disable", "hooks"} {
		if !strings.Contains(args, want) {
			t.Fatal("missing", want, args)
		}
	}
}

func TestPresetFlagsAppendedToNativeArgs(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	p := reviewPreset
	p.Flags = []string{"-c", "custom_setting=true"}
	savePreset(t, s.Home, p)
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "run_in_background": false})
	r.mu.Lock()
	args := strings.Join(r.plans[0].Args, " ")
	r.mu.Unlock()
	if !strings.Contains(args, "-c custom_setting=true") {
		t.Fatal("preset flags not appended", args)
	}
}
