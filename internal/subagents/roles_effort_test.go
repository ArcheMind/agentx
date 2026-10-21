package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/roles"
	"github.com/ArcheMind/agentx/internal/runtime"
)

// codexTelemetry imitates Codex's trace-safe response.completed event; executed overrides the
// level the fake backend reports, otherwise the configured level (or the model default) is echoed.
func codexTelemetry(p runtime.CommandPlan, executed string) string {
	if p.Env["RUST_LOG"] != codexTelemetryFilter {
		return ""
	}
	if executed == "" {
		executed = "medium"
		for _, arg := range p.Args {
			if value, ok := strings.CutPrefix(arg, "model_reasoning_effort="); ok {
				executed = strings.Trim(value, `"`)
			}
		}
	}
	return fmt.Sprintf("2026-10-21T12:09:12Z  INFO codex_otel.trace_safe: event.name=\"codex.sse_event\" event.kind=response.completed input_token_count=1 model_reasoning_effort=%q model=test-model\n", executed)
}

func writeRole(t *testing.T, home, name, description, instructions string) {
	t.Helper()
	dir := filepath.Join(roles.Dir(home), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, roles.FileName), []byte("---\ndescription: "+description+"\n---\n\n"+instructions+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func argValue(args []string, prefix string) string {
	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, prefix); ok {
			return value
		}
	}
	return ""
}

// codexRoleRunner writes the rollout Codex would write and reports a configurable executed effort.
type codexRoleRunner struct {
	testRunner
	executed       string
	dropDeveloper  bool
	codexHomeByRun []string
}

func (r *codexRoleRunner) Execute(ctx context.Context, p runtime.CommandPlan, o runtime.ExecuteOptions) (runtime.CommandResult, error) {
	if result, ok := authCheck(p); ok {
		return result, nil
	}
	data, _ := io.ReadAll(o.Stdin)
	r.mu.Lock()
	r.plans = append(r.plans, p)
	r.mu.Unlock()
	r.mu.Lock()
	thread := fmt.Sprintf("12345678-1234-1234-1234-%012d", len(r.plans))
	r.mu.Unlock()
	if presets.Contains(p.Args, "resume") {
		thread = p.Args[len(p.Args)-2]
	}
	dir := filepath.Join(p.Env["CODEX_HOME"], "sessions", "2026", "10", "21")
	os.MkdirAll(dir, 0o700)
	rollout := filepath.Join(dir, "rollout-2026-10-21T00-00-00-"+thread+".jsonl")
	if _, err := os.Stat(rollout); err != nil {
		var developer string
		if value := argValue(p.Args, "developer_instructions="); value != "" && !r.dropDeveloper {
			json.Unmarshal([]byte(value), &developer)
		}
		entry, _ := json.Marshal(map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_text", "text": developer}}}})
		os.WriteFile(rollout, append(entry, '\n'), 0o600)
	}
	return runtime.CommandResult{Stderr: codexTelemetry(p, r.executed), Stdout: fmt.Sprintf("{\"type\":\"thread.started\",\"thread_id\":%q}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":%q}}\n{\"type\":\"turn.completed\"}\n", thread, string(data))}, nil
}

func roleServer(t *testing.T, r runtime.Runner) *Server {
	s := testServer(t, r)
	writeRole(t, s.Home, "alpha", "Alpha reviewer", "End every reply with ROLE-ALPHA.")
	writeRole(t, s.Home, "beta", "Beta reviewer", "End every reply with ROLE-BETA.")
	return s
}

func TestDeclaredRolesShapeCodexWithoutChangingExecution(t *testing.T) {
	r := &codexRoleRunner{}
	s := roleServer(t, r)
	alpha := callTest(t, s, "ExternalSubagent", map[string]any{"description": "a", "prompt": "x", "model": "review", "subagent_type": "alpha", "run_in_background": false})
	beta := callTest(t, s, "ExternalSubagent", map[string]any{"description": "b", "prompt": "x", "model": "review", "subagent_type": "beta", "run_in_background": false})
	plain := callTest(t, s, "ExternalSubagent", map[string]any{"description": "c", "prompt": "x", "model": "review", "subagent_type": "general-purpose", "run_in_background": false})
	if !strings.Contains(alpha, "role=alpha") || !strings.Contains(alpha, "role instructions recorded as a Codex developer message") || !strings.Contains(beta, "role=beta") || !strings.Contains(plain, "role=general-purpose") || !strings.Contains(alpha, "executed effort low") {
		t.Fatal(alpha, beta, plain)
	}
	if got := argValue(r.plans[0].Args, "developer_instructions="); got != `"End every reply with ROLE-ALPHA."` {
		t.Fatal(got)
	}
	if got := argValue(r.plans[1].Args, "developer_instructions="); got != `"End every reply with ROLE-BETA."` {
		t.Fatal(got)
	}
	strip := func(args []string) string {
		var kept []string
		for i, arg := range args {
			if strings.HasPrefix(arg, "developer_instructions=") || (i+1 < len(args) && strings.HasPrefix(args[i+1], "developer_instructions=")) {
				continue
			}
			kept = append(kept, arg)
		}
		return strings.Join(kept, " ")
	}
	if strip(r.plans[0].Args) != strings.Join(r.plans[2].Args, " ") || strip(r.plans[1].Args) != strip(r.plans[0].Args) {
		t.Fatal("role changed the execution combination", r.plans)
	}
	if strings.Contains(strings.Join(r.plans[2].Args, " "), "developer_instructions") {
		t.Fatal("general-purpose added role instructions")
	}
}

func TestUnknownInvalidAndChangedRolesAreRejected(t *testing.T) {
	r := &codexRoleRunner{}
	s := roleServer(t, r)
	writeRole(t, s.Home, "broken", "Broken", "x")
	os.WriteFile(roles.Path(s.Home, "broken"), []byte("---\ndescription: b\nmodel: other\n---\nx\n"), 0o600)
	for _, role := range []string{"missing", "broken", "../alpha", "Alpha"} {
		if _, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","subagent_type":"`+role+`"}`)); err == nil || !strings.Contains(err.Error(), "no other role was substituted") {
			t.Fatal(role, err)
		}
	}
	if len(s.agents) != 0 {
		t.Fatal("rejected role created work")
	}
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "a", "prompt": "x", "name": "pinned", "model": "review", "subagent_type": "alpha", "run_in_background": false})
	writeRole(t, s.Home, "alpha", "Alpha reviewer", "Different instructions.")
	if _, err := s.call("SendExternalSubagentMessage", []byte(`{"to":"pinned","message":"again"}`)); err == nil || !strings.Contains(err.Error(), "original role alpha changed") {
		t.Fatal("role edit rebound the session", err)
	}
	os.RemoveAll(filepath.Join(roles.Dir(s.Home), "alpha"))
	if _, err := s.call("SendExternalSubagentMessage", []byte(`{"to":"pinned","message":"again"}`)); err == nil {
		t.Fatal("removed role continued")
	}
	if len(r.plans) != 1 {
		t.Fatal("changed role reached execution", len(r.plans))
	}
}

func TestCodexRoleMissingFromRolloutFails(t *testing.T) {
	s := roleServer(t, &codexRoleRunner{dropDeveloper: true})
	if _, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","subagent_type":"alpha","run_in_background":false}`)); err == nil || !strings.Contains(err.Error(), "role instructions not found as a developer message") {
		t.Fatal(err)
	}
}

func TestCodexSilentEffortClampFailsLoudly(t *testing.T) {
	r := &codexRoleRunner{executed: "max"}
	s := roleServer(t, r)
	ultra := reviewPreset
	ultra.Effort = "ultra"
	savePreset(t, s.Home, ultra)
	_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","run_in_background":false}`))
	if err == nil || !strings.Contains(err.Error(), "native effort mismatch: preset effort ultra") || !strings.Contains(err.Error(), "max executed") {
		t.Fatal(err)
	}
	if env := r.plans[0].Env["RUST_LOG"]; env != codexTelemetryFilter {
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
	if _, err := verifyEffort("high", nil, "Codex response.completed telemetry"); err == nil || !strings.Contains(err.Error(), "could not be verified") {
		t.Fatal(err)
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

// piRunner appends what Pi plus the AgentX observer would write to the session file.
type piRunner struct {
	testRunner
	thinking string
	effort   *string
	role     *bool
	observer bool
}

func (r *piRunner) Execute(ctx context.Context, p runtime.CommandPlan, o runtime.ExecuteOptions) (runtime.CommandResult, error) {
	if result, ok := authCheck(p); ok {
		return result, nil
	}
	io.ReadAll(o.Stdin)
	r.mu.Lock()
	r.plans = append(r.plans, p)
	r.mu.Unlock()
	session := ""
	for i, arg := range p.Args {
		if arg == "--session" {
			session = p.Args[i+1]
		}
	}
	file, _ := os.OpenFile(session, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	defer file.Close()
	if r.thinking != "" {
		fmt.Fprintf(file, "{\"type\":\"thinking_level_change\",\"thinkingLevel\":%q}\n", r.thinking)
	}
	if r.observer {
		data := map[string]any{"reasoning_effort": r.effort}
		if r.role != nil {
			data["role_included"] = *r.role
		}
		entry, _ := json.Marshal(map[string]any{"type": "custom", "customType": piObserverEntry, "data": data})
		file.Write(append(entry, '\n'))
	}
	return runtime.CommandResult{Stdout: "{\"type\":\"session\",\"id\":\"pi-native\"}\n{\"type\":\"message_end\",\"message\":{\"role\":\"assistant\",\"provider\":\"provider\",\"model\":\"model\",\"content\":[{\"type\":\"text\",\"text\":\"report\"}]}}\n{\"type\":\"agent_end\"}\n"}, nil
}

func piServer(t *testing.T, r runtime.Runner, effort string) *Server {
	s := roleServer(t, r)
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0o700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	savePreset(t, s.Home, presets.Preset{Name: "pi-review", Agent: "pi", Model: "provider/model", Description: "Pi", Effort: effort})
	config, _ := LoadConfig(s.Home)
	config.Grants["pi-review"] = testGrant(t, "pi")
	if err := SaveConfig(s.Home, config); err != nil {
		t.Fatal(err)
	}
	return s
}

func ptr[T any](value T) *T { return &value }

func TestPiEffortAndRoleVerifiedFromObserverRecords(t *testing.T) {
	r := &piRunner{thinking: "high", effort: ptr("high"), role: ptr(true), observer: true}
	s := piServer(t, r, "high")
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "name": "pi", "model": "pi-review", "subagent_type": "alpha", "run_in_background": false})
	if !strings.Contains(text, "effort=high; role=alpha") || !strings.Contains(text, "executed effort high (1 model request(s)") || !strings.Contains(text, "role instructions present in all 1 provider request(s)") {
		t.Fatal(text)
	}
	args := strings.Join(r.plans[0].Args, " ")
	for _, want := range []string{"--thinking high", "--extension ", "agentx-observer.js", "--append-system-prompt ", "role.md", "--tools read,grep,find,ls", "--no-extensions"} {
		if !strings.Contains(args, want) {
			t.Fatal(want, args)
		}
	}
	roleFile := r.plans[0].Env["AGENTX_PI_ROLE_FILE"]
	if data, err := os.ReadFile(roleFile); err != nil || string(data) != "End every reply with ROLE-ALPHA." {
		t.Fatal(string(data), err)
	}
	r.thinking = ""
	callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "pi", "message": "again"})
	waitAgent(t, s, "pi")
	s.mu.Lock()
	a, _ := s.target("pi")
	last := a.Jobs[len(a.Jobs)-1]
	s.mu.Unlock()
	if last.Status != "completed" || !strings.Contains(last.Evidence, "1 model request(s)") {
		t.Fatal("continuation evidence must count only the new turn", last.Status, last.Failure, last.Evidence)
	}
}

func TestPiClampRemapMissingRoleAndMissingObserverFail(t *testing.T) {
	cases := []struct {
		name   string
		runner *piRunner
		role   string
		want   string
	}{
		{"session clamp", &piRunner{thinking: "medium", effort: ptr("medium"), observer: true}, "", "Pi session records thinking level \"medium\""},
		{"provider remap", &piRunner{thinking: "high", effort: ptr("low"), observer: true}, "", "low executed"},
		{"no effort field", &piRunner{thinking: "high", observer: true}, "", "none sent executed"},
		{"role dropped", &piRunner{thinking: "high", effort: ptr("high"), role: ptr(false), observer: true}, "alpha", "instructions were missing"},
		{"observer absent", &piRunner{thinking: "high"}, "", "recorded no Pi provider request"},
	}
	for _, c := range cases {
		s := piServer(t, c.runner, "high")
		input := `{"description":"x","prompt":"x","model":"pi-review","run_in_background":false`
		if c.role != "" {
			input += `,"subagent_type":"` + c.role + `"`
		}
		if _, err := s.call("ExternalSubagent", []byte(input+"}")); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatal(c.name, err)
		}
	}
}

func TestPiWithoutRoleOrEffortKeepsPlainPlan(t *testing.T) {
	r := &piRunner{}
	s := piServer(t, r, "")
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "model": "pi-review", "run_in_background": false})
	args := strings.Join(r.plans[0].Args, " ")
	if strings.Contains(args, "--extension") || strings.Contains(args, "--thinking") || strings.Contains(args, "--append-system-prompt") || strings.Contains(text, "Native evidence") {
		t.Fatal(args, text)
	}
}

func TestRoleCatalogAndSchemaShowDeclaredRoles(t *testing.T) {
	s := roleServer(t, &codexRoleRunner{})
	writeRole(t, s.Home, "broken", "x", "y")
	os.WriteFile(roles.Path(s.Home, "broken"), []byte("no front matter"), 0o600)
	tools := s.tools().(map[string]any)["tools"].([]any)
	create := tools[0].(map[string]any)
	enum := create["inputSchema"].(map[string]any)["properties"].(map[string]any)["subagent_type"].(map[string]any)["enum"].([]string)
	if strings.Join(enum, ",") != "general-purpose,alpha,beta" {
		t.Fatal(enum)
	}
	description := create["description"].(string)
	for _, want := range []string{"--- begin role data ---", "- general-purpose: implicit", `- alpha: user description="Alpha reviewer"; instructions="End every reply with ROLE-ALPHA."`, "- broken: invalid role at", "never changes the preset's agent, model, effort, grant", "role instructions are sent as Codex developer_instructions"} {
		if !strings.Contains(description, want) {
			t.Fatal(want, description)
		}
	}
	result, _ := s.handle("initialize", []byte(`{}`))
	if !strings.Contains(result.(map[string]any)["instructions"].(string), "- beta: user description=\"Beta reviewer\"") {
		t.Fatal("initialization lacks role data")
	}
	writeRole(t, s.Home, "gamma", "Gamma", "G.")
	if note := s.staleNote(); !strings.Contains(note, "preset or role collection changed") {
		t.Fatal(note)
	}
}
