package subagents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
)

func TestGrantValidationRejectsUnsupportedAndMissing(t *testing.T) {
	home := t.TempDir()
	codex := []presets.Preset{{Name: "w", Agent: "codex", Model: "m", Description: "d"}}
	pi := []presets.Preset{{Name: "p", Agent: "pi", Model: "provider/model", Description: "d"}}
	interactive := []presets.Preset{{Name: "i", Agent: "claude", Model: "opus", Description: "d"}}
	valid := testGrant(t, "codex")
	cases := map[string]struct {
		list   []presets.Preset
		grants map[string]Grant
	}{
		"missing grant":     {codex, nil},
		"orphan grant":      {codex, map[string]Grant{"w": valid, "other": valid}},
		"interactive grant": {interactive, map[string]Grant{"i": valid}},
		"unknown policy":    {codex, map[string]Grant{"w": {Execution: ExecutionPolicy{Policy: "full-access", Approval: "never"}, Auth: valid.Auth}}},
		"empty policy":      {codex, map[string]Grant{"w": {Execution: ExecutionPolicy{Approval: "never"}, Auth: valid.Auth}}},
		"approval":          {codex, map[string]Grant{"w": {Execution: ExecutionPolicy{Policy: "read-only", Approval: "on-request"}, Auth: valid.Auth}}},
		"read-only roots":   {codex, map[string]Grant{"w": {Execution: ExecutionPolicy{Policy: "read-only", WritableRoots: []string{"/x"}, Approval: "never"}, Auth: valid.Auth}}},
		"no roots":          {codex, map[string]Grant{"w": {Execution: ExecutionPolicy{Policy: "bounded-write", Approval: "never"}, Auth: valid.Auth}}},
		"relative root":     {codex, map[string]Grant{"w": {Execution: ExecutionPolicy{Policy: "bounded-write", WritableRoots: []string{"rel"}, Approval: "never"}, Auth: valid.Auth}}},
		"filesystem root":   {codex, map[string]Grant{"w": {Execution: ExecutionPolicy{Policy: "bounded-write", WritableRoots: []string{"/"}, Approval: "never"}, Auth: valid.Auth}}},
		"pi write":          {pi, map[string]Grant{"p": {Execution: ExecutionPolicy{Policy: "bounded-write", WritableRoots: []string{"/x"}, Approval: "never"}, Auth: testGrant(t, "pi").Auth}}},
		"wrong source":      {pi, map[string]Grant{"p": valid}},
		"wrong method":      {codex, map[string]Grant{"w": {Execution: valid.Execution, Auth: AuthSource{Source: "codex-home", Path: valid.Auth.Path, Method: "oauth"}}}},
		"relative auth":     {codex, map[string]Grant{"w": {Execution: valid.Execution, Auth: AuthSource{Source: "codex-home", Path: "home", Method: "chatgpt"}}}},
	}
	for name, c := range cases {
		if err := (Config{Grants: c.grants}).Validate(home, c.list, nil); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	err := Config{Grants: map[string]Grant{"p": {Execution: ExecutionPolicy{Policy: "bounded-write", WritableRoots: []string{"/x"}, Approval: "never"}, Auth: testGrant(t, "pi").Auth}}}.Validate(home, pi, nil)
	if err == nil || !strings.Contains(err.Error(), "no runtime sandbox") {
		t.Fatal(err)
	}
	if err := (Config{}).Validate(home, interactive, nil); err != nil {
		t.Fatal("interactive-launch-only preset must not require a grant:", err)
	}
	if err := (Config{Grants: map[string]Grant{"broken": valid}}).Validate(home, nil, []presets.Problem{{Name: "broken"}}); err != nil {
		t.Fatal("grant for a broken preset entry must not hide the rest of the configuration:", err)
	}
	missing := Config{Grants: map[string]Grant{"w": {Execution: valid.Execution, Auth: AuthSource{Source: "codex-home", Path: filepath.Join(t.TempDir(), "absent"), Method: "chatgpt"}}}}
	if err := missing.Validate(home, codex, nil); err != nil {
		t.Fatal(err)
	}
	if err := missing.Grants["w"].Ready(); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatal(err)
	}
}

func boundedServer(t *testing.T, r runtime.Runner) (*Server, string) {
	t.Helper()
	s := testServer(t, r)
	root := t.TempDir()
	extra := t.TempDir()
	config, _ := LoadConfig(s.Home)
	grant := config.Grants["review"]
	grant.Execution = ExecutionPolicy{Policy: "bounded-write", WritableRoots: []string{root, extra}, Approval: "never"}
	config.Grants["review"] = grant
	if err := SaveConfig(s.Home, config); err != nil {
		t.Fatal(err)
	}
	s.Cwd = filepath.Join(root, "work")
	if err := os.Mkdir(s.Cwd, 0700); err != nil {
		t.Fatal(err)
	}
	return s, extra
}

func TestBoundedWriteMapsToCodexSandboxAndConfiguredAuth(t *testing.T) {
	t.Setenv("AGENTX_TEST_API_KEY", "sk-test-should-not-pass-0123456789")
	r := &testRunner{}
	s, extra := boundedServer(t, r)
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "write", "prompt": "edit", "model": "review", "run_in_background": false})
	if !strings.Contains(text, "Grant: policy=bounded-write roots=") || !strings.Contains(text, "approval=never") || !strings.Contains(text, "auth=codex-home") {
		t.Fatal(text)
	}
	r.mu.Lock()
	plan := r.plans[0]
	r.mu.Unlock()
	args := strings.Join(plan.Args, " ")
	realExtra, _ := filepath.EvalSymlinks(extra)
	for _, want := range []string{`--ignore-user-config`, `--ignore-rules`, `--disable hooks`, `sandbox_mode="workspace-write"`, `sandbox_workspace_write.writable_roots=[`, realExtra, `sandbox_workspace_write.network_access=false`, `exclude_slash_tmp=true`, `exclude_tmpdir_env_var=true`, `approval_policy="never"`, `model_reasoning_effort="low"`} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %s in %s", want, args)
		}
	}
	config, _ := LoadConfig(s.Home)
	if plan.Env["CODEX_HOME"] != config.Grants["review"].Auth.Path || !presets.Contains(plan.UnsetEnv, "AGENTX_TEST_API_KEY") {
		t.Fatalf("auth environment not explicit: %#v %#v", plan.Env, plan.UnsetEnv)
	}
	for _, value := range plan.Env {
		if strings.Contains(value, "sk-test") {
			t.Fatal("credential copied into runtime environment")
		}
	}
}

func TestReadOnlyCodexPlanUsesNativeReadOnlySandbox(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "read", "prompt": "review", "model": "review", "run_in_background": false})
	r.mu.Lock()
	args := strings.Join(r.plans[0].Args, " ")
	r.mu.Unlock()
	if !strings.Contains(args, `sandbox_mode="read-only"`) || strings.Contains(args, "workspace-write") || strings.Contains(args, "writable_roots") {
		t.Fatal(args)
	}
}

func TestBoundedWriteRejectsWorkingDirectoryOutsideRoots(t *testing.T) {
	r := &testRunner{}
	s, _ := boundedServer(t, r)
	s.Cwd = t.TempDir()
	_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","run_in_background":false}`))
	if err == nil || !strings.Contains(err.Error(), "outside writable roots") {
		t.Fatal(err)
	}
	if len(r.plans) != 0 || len(s.agents) != 0 {
		t.Fatal("work started despite exceeding grant")
	}
}

type authRunner struct {
	testRunner
	login string
	pi    string
}

func (r *authRunner) Execute(ctx context.Context, p runtime.CommandPlan, o runtime.ExecuteOptions) (runtime.CommandResult, error) {
	if len(p.Args) > 1 && p.Args[0] == "login" {
		if strings.HasPrefix(r.login, "Not") {
			return runtime.CommandResult{Stderr: r.login, ExitCode: 1}, os.ErrPermission
		}
		return runtime.CommandResult{Stderr: r.login}, nil
	}
	if len(p.Args) > 1 && p.Args[0] == "auth" {
		return runtime.CommandResult{Stdout: r.pi}, nil
	}
	return r.testRunner.Execute(ctx, p, o)
}

func TestMissingOrMismatchedAuthFailsWithoutFallback(t *testing.T) {
	for _, login := range []string{"Not logged in", "Logged in using an API key - sk-proj-abcdefghijklmnopqrstuvwxyz"} {
		r := &authRunner{login: login}
		s := testServer(t, r)
		_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","run_in_background":false}`))
		if err == nil || !strings.Contains(err.Error(), "no fallback identity") || strings.Contains(err.Error(), "abcdefghijklmnop") {
			t.Fatal(err)
		}
		if len(r.plans) != 0 {
			t.Fatal("native execution started without verified auth")
		}
	}
	for _, status := range []string{`{"status":"not_ready","provider":"provider","reason":"credentials_not_configured"}`, `{"status":"ready","provider":"provider","authType":"api_key"}`, `{"status":"ready","provider":"other","authType":"oauth"}`} {
		r := &authRunner{pi: status}
		s := testServer(t, r)
		bin := filepath.Dir(must(t, "codex"))
		if err := os.WriteFile(filepath.Join(bin, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
		savePreset(t, s.Home, presets.Preset{Name: "review", Agent: "pi", Model: "provider/model", Description: "d"})
		if err := SaveConfig(s.Home, Config{Enabled: true, Grants: map[string]Grant{"review": testGrant(t, "pi")}}); err != nil {
			t.Fatal(err)
		}
		_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","run_in_background":false}`))
		if err == nil || !strings.Contains(err.Error(), "no fallback identity") {
			t.Fatal(status, err)
		}
		if len(r.plans) != 0 {
			t.Fatal("native execution started without verified auth")
		}
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

func TestGrantChangeCannotRebindExistingSession(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "name": "pinned", "run_in_background": false})
	config, _ := LoadConfig(s.Home)
	root := t.TempDir()
	grant := config.Grants["review"]
	grant.Execution = ExecutionPolicy{Policy: "bounded-write", WritableRoots: []string{root}, Approval: "never"}
	config.Grants["review"] = grant
	if err := SaveConfig(s.Home, config); err != nil {
		t.Fatal(err)
	}
	_, err := s.call("SendExternalSubagentMessage", []byte(`{"to":"pinned","message":"now write"}`))
	if err == nil || !strings.Contains(err.Error(), "grant changed") {
		t.Fatal(err)
	}
}

type leakingRunner struct{}

func (leakingRunner) Execute(_ context.Context, p runtime.CommandPlan, _ runtime.ExecuteOptions) (runtime.CommandResult, error) {
	if result, ok := authCheck(p); ok {
		return result, nil
	}
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
