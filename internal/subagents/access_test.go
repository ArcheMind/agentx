package subagents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/profiles"
	"github.com/ArcheMind/agentx/internal/runtime"
)

func installPi(t *testing.T) {
	t.Helper()
	bin := filepath.Dir(must(t, "codex"))
	if err := os.WriteFile(filepath.Join(bin, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestWriteRequestsRejectedBeforeAnyWork(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	installPi(t)
	saveProfile(t, s.Home, profiles.Profile{Name: "pi-review", Agent: "pi", Model: "provider/model", Description: "d"})
	dir := t.TempDir()
	for input, want := range map[string]string{
		`{"description":"x","prompt":"x","model":"review","writable_roots":["` + dir + `"]}`:                            "writable_roots requires write: true",
		`{"description":"x","prompt":"x","model":"review","write":true}`:                                                "write without isolation requires writable_roots",
		`{"description":"x","prompt":"x","model":"review","write":true,"writable_roots":[]}`:                            "write without isolation requires writable_roots",
		`{"description":"x","prompt":"x","model":"pi-review","write":true,"writable_roots":["` + dir + `"]}`:            "Pi has no runtime write boundary",
		`{"description":"x","prompt":"x","model":"pi-review","write":true,"isolation":"worktree"}`:                      "Pi has no runtime write boundary",
		`{"description":"x","prompt":"x","model":"review","write":true,"writable_roots":null}`:                          "must not be null",
		`{"description":"x","prompt":"x","model":"review","write":"yes","writable_roots":["` + dir + `"]}`:              "cannot unmarshal",
		`{"description":"x","prompt":"x","model":"review","write":true,"isolation":"worktree","writable_roots":["/x"]}`: "the worktree is the only writable root",
	} {
		_, err := s.call("ExternalSubagent", []byte(input))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", input, err, want)
		}
	}
	if len(r.plans) != 0 || len(s.agents) != 0 {
		t.Fatal("a rejected write request started work")
	}
}

func TestCallWithoutWriteIsReadOnly(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "read", "prompt": "review", "model": "review", "run_in_background": false})
	r.mu.Lock()
	args := strings.Join(r.plans[0].Args, " ")
	r.mu.Unlock()
	if !strings.Contains(args, `sandbox_mode="read-only"`) || strings.Contains(args, "workspace-write") || strings.Contains(args, "writable_roots") || !strings.Contains(text, "Write scope: read-only (no write requested)") {
		t.Fatal(args, text)
	}
}

func TestWritableRootsReachCodexSandboxAsGivenAndAuthIsDiscovered(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	target, extra := t.TempDir(), t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "write", "prompt": "edit", "model": "review", "write": true, "writable_roots": []string{link, extra}, "run_in_background": false})
	r.mu.Lock()
	plan := r.plans[0]
	r.mu.Unlock()
	args := strings.Join(plan.Args, " ")
	for _, want := range []string{`sandbox_mode="workspace-write"`, `sandbox_workspace_write.writable_roots=["` + link + `","` + extra + `"]`, `sandbox_workspace_write.network_access=false`, `exclude_slash_tmp=true`, `exclude_tmpdir_env_var=true`, `approval_policy="never"`, `model_reasoning_effort="low"`} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %s in %s", want, args)
		}
	}
	if !strings.Contains(text, "Write scope: working directory") || !strings.Contains(text, "In write mode the MCP working directory is always writable as well (native Codex behavior); writable_roots grants additional directories.") || !strings.Contains(text, `Auth (discovered by preflight for this turn): CODEX_HOME="`+os.Getenv("CODEX_HOME")+`" (chatgpt)`) {
		t.Fatal(text)
	}
	if plan.Env["CODEX_HOME"] != os.Getenv("CODEX_HOME") || len(plan.UnsetEnv) != 0 {
		t.Fatalf("auth environment not explicit: %#v %#v", plan.Env, plan.UnsetEnv)
	}
}

func TestWriteScopeIsFixedForContinuation(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	root := t.TempDir()
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "name": "w", "write": true, "writable_roots": []string{root}, "run_in_background": false})
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "name": "ro", "run_in_background": false})
	for _, name := range []string{"w", "ro"} {
		callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": name, "message": "again"})
		waitAgent(t, s, name)
	}
	if _, err := s.call("SendExternalSubagentMessage", []byte(`{"to":"ro","message":"x","write":true}`)); err == nil {
		t.Fatal("continuation accepted a write request")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	scope := func(p runtime.CommandPlan) string {
		args := strings.Join(p.Args, " ")
		return args[strings.Index(args, "sandbox_mode"):strings.Index(args, "approval_policy")]
	}
	if len(r.plans) != 4 || scope(r.plans[0]) != scope(r.plans[2]) || scope(r.plans[1]) != scope(r.plans[3]) || !strings.Contains(scope(r.plans[3]), "read-only") {
		t.Fatal("continuation changed write scope:", len(r.plans))
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

func TestFailedAuthPreflightStartsNoNativeExecution(t *testing.T) {
	for _, login := range []string{"Not logged in", "Something else - sk-proj-abcdefghijklmnopqrstuvwxyz"} {
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
	for _, status := range []string{`{"status":"not_ready","provider":"provider","reason":"credentials_not_configured"}`, `{"status":"ready","provider":"other","authType":"oauth"}`, `not json`} {
		r := &authRunner{pi: status}
		s := testServer(t, r)
		installPi(t)
		saveProfile(t, s.Home, profiles.Profile{Name: "review", Agent: "pi", Model: "provider/model", Description: "d"})
		_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","run_in_background":false}`))
		if err == nil || !strings.Contains(err.Error(), "no fallback identity") {
			t.Fatal(status, err)
		}
		if len(r.plans) != 0 {
			t.Fatal("native execution started without verified auth")
		}
	}
	r := &authRunner{pi: `{"status":"ready","provider":"provider","authType":"api_key"}`}
	s := testServer(t, r)
	installPi(t)
	saveProfile(t, s.Home, profiles.Profile{Name: "review", Agent: "pi", Model: "provider/model", Description: "d"})
	_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","run_in_background":false}`))
	if err == nil || !strings.Contains(err.Error(), `PI_CODING_AGENT_DIR="`+os.Getenv("PI_CODING_AGENT_DIR")+`" (api_key)`) || len(r.plans) != 1 {
		t.Fatal("discovered Pi auth not used or reported:", err)
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
	a := &ExternalSubagentState{Option: profiles.Profile{Agent: "codex"}}
	if _, _, err := parseOutput(a, `{"type":"turn.failed","error":{"message":"bad key sk-proj-abcdefghijklmnopqrstuvwxyz"}}`); err == nil || strings.Contains(err.Error(), "abcdefghijklmnop") {
		t.Fatal(err)
	}
}

func TestSandboxFalseOmitsRestrictions(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	f := false
	p := reviewProfile
	p.Sandbox = &f
	saveProfile(t, s.Home, p)
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "write": true, "writable_roots": []string{t.TempDir()}, "run_in_background": false})
	r.mu.Lock()
	args := strings.Join(r.plans[0].Args, " ")
	r.mu.Unlock()
	if strings.Contains(args, "network_access") || strings.Contains(args, "exclude_tmpdir") || strings.Contains(args, "exclude_slash_tmp") {
		t.Fatal("sandbox:false still has restrictions", args)
	}
	if !strings.Contains(args, "workspace-write") {
		t.Fatal("sandbox mode missing", args)
	}
}

func TestUserConfigFalseAddsIsolationFlags(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	f := false
	p := reviewProfile
	p.UserConfig = &f
	saveProfile(t, s.Home, p)
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

func TestProfileFlagsAppendedToNativeArgs(t *testing.T) {
	r := &testRunner{}
	s := testServer(t, r)
	p := reviewProfile
	p.Flags = []string{"-c", "custom_setting=true"}
	saveProfile(t, s.Home, p)
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "review", "run_in_background": false})
	r.mu.Lock()
	args := strings.Join(r.plans[0].Args, " ")
	r.mu.Unlock()
	if !strings.Contains(args, "-c custom_setting=true") {
		t.Fatal("profile flags not appended", args)
	}
}
