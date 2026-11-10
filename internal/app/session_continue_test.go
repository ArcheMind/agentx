package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
)

// continueRunner simulates one native agent turn: it appends the turn's
// messages to the native session file and answers with the native stream.
type continueRunner struct {
	home    string
	agent   string
	model   string
	reply   []sessions.Message
	plans   []runtime.CommandPlan
	prompts []string
	fresh   string
}

func (r *continueRunner) Execute(_ context.Context, plan runtime.CommandPlan, options runtime.ExecuteOptions) (runtime.CommandResult, error) {
	r.plans = append(r.plans, plan)
	prompt := ""
	if options.Stdin != nil {
		data, _ := io.ReadAll(options.Stdin)
		prompt = string(data)
	}
	r.prompts = append(r.prompts, prompt)
	switch r.agent {
	case "claude":
		return r.runClaude(plan, prompt)
	case "codex":
		return r.runCodex(plan, prompt)
	default:
		return r.runPi(plan, prompt)
	}
}

func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func (r *continueRunner) appendTurn(path, agent, prompt string, write func(sessions.Session) []map[string]any) error {
	detail := sessions.Session{ID: filepath.Base(path), Provider: agent}
	if _, err := os.Stat(path); err == nil {
		parsed, err := sessions.ParseNativeSessionFile(agent, path)
		if err != nil {
			return err
		}
		detail = parsed
	}
	user := sessions.Message{ID: "echo-user", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: prompt}}}
	detail.Messages = append(detail.Messages, user)
	detail.Messages = append(detail.Messages, r.reply...)
	records := write(detail)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return os.WriteFile(path, buffer.Bytes(), 0o600)
}

func (r *continueRunner) runClaude(plan runtime.CommandPlan, prompt string) (runtime.CommandResult, error) {
	id := flagValue(plan.Args, "--resume")
	if id == "" {
		id = r.fresh
	}
	path := sessions.ClaudeProjectFile(r.home, plan.Cwd, id)
	detailID := id
	if err := r.appendTurn(path, "claude", prompt, func(detail sessions.Session) []map[string]any {
		detail.ID = detailID
		return sessions.SerializeClaude(detail)
	}); err != nil {
		return runtime.CommandResult{}, err
	}
	stdout := fmt.Sprintf(`{"type":"system","subtype":"init","session_id":%q}`+"\n"+`{"type":"result","result":"done","is_error":false,"session_id":%q}`+"\n", id, id)
	return runtime.CommandResult{Stdout: stdout}, nil
}

func (r *continueRunner) runCodex(plan runtime.CommandPlan, prompt string) (runtime.CommandResult, error) {
	id := plan.Args[len(plan.Args)-2]
	path, err := sessions.FindCodexRollout(plan.Env["CODEX_HOME"], id)
	if err != nil {
		return runtime.CommandResult{}, err
	}
	if err := r.appendTurn(path, "codex", prompt, sessions.SerializeCodex); err != nil {
		return runtime.CommandResult{}, err
	}
	stdout := fmt.Sprintf(`{"type":"thread.started","thread_id":%q}`+"\n"+`{"type":"turn.completed","usage":{}}`+"\n", id)
	return runtime.CommandResult{Stdout: stdout}, nil
}

func (r *continueRunner) runPi(plan runtime.CommandPlan, prompt string) (runtime.CommandResult, error) {
	path := flagValue(plan.Args, "--session")
	if err := r.appendTurn(path, "pi", prompt, sessions.SerializePi); err != nil {
		return runtime.CommandResult{}, err
	}
	provider, model, _ := strings.Cut(r.model, "/")
	stdout := fmt.Sprintf(`{"type":"session","id":"pi-native"}`+"\n"+
		`{"type":"tool_execution_end","toolCallId":"call_1","toolName":"read","result":{"content":[{"type":"text","text":"file body"}],"details":{"truncated":false}},"isError":false}`+"\n"+
		`{"type":"message_end","message":{"role":"assistant","provider":%q,"model":%q,"stopReason":"stop","content":[{"type":"text","text":"done"}]}}`+"\n"+
		`{"type":"agent_end"}`+"\n"+
		`{"type":"compaction_end","reason":"threshold","result":{"summary":"earlier work","firstKeptEntryId":"e1","tokensBefore":1200,"details":{}},"aborted":false,"willRetry":false}`+"\n"+
		`{"type":"agent_settled","aborted":false}`+"\n", provider, model)
	return runtime.CommandResult{Stdout: stdout}, nil
}

func continueApp(t *testing.T, agent, model, input string) (App, *bytes.Buffer, *continueRunner) {
	t.Helper()
	home := t.TempDir()
	runner := &continueRunner{home: home, agent: agent, model: model, fresh: "99999999-8888-4777-a666-555555555555"}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(input), &stdout, &bytes.Buffer{})
	application.Runner = runner
	application.Sessions = sessions.Service{HomeDir: home}
	return application, &stdout, runner
}

func continueTranscript(t *testing.T, workspace string) string {
	t.Helper()
	detail := sessions.Session{
		ID:        "turn-1",
		Provider:  "harness",
		Workspace: workspace,
		Messages: []sessions.Message{
			{ID: "m1", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: "hello"}}},
			{ID: "m2", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "hi there"}}},
			{ID: "m3", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: "what now?"}}},
		},
	}
	detail.MessageCount = len(detail.Messages)
	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func testWorkspace(t *testing.T) string {
	t.Helper()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func decodeDelta(t *testing.T, stdout *bytes.Buffer) sessions.Session {
	t.Helper()
	var delta sessions.Session
	if err := json.Unmarshal(stdout.Bytes(), &delta); err != nil {
		t.Fatalf("stdout %q: %v", stdout.String(), err)
	}
	return delta
}

func TestSessionContinueClaudeEmitsNamespacedDelta(t *testing.T) {
	workspace := testWorkspace(t)
	application, stdout, runner := continueApp(t, "claude", "", continueTranscript(t, workspace))
	runner.reply = []sessions.Message{{ID: "a1", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "the answer"}}}}
	if err := application.Run(context.Background(), []string{"session", "continue", "claude", "--mode", "yolo"}); err != nil {
		t.Fatal(err)
	}
	if len(runner.prompts) != 1 || runner.prompts[0] != "what now?" {
		t.Fatalf("prompts = %q", runner.prompts)
	}
	delta := decodeDelta(t, stdout)
	if delta.Provider != "claude" || delta.Source != "continue" || delta.Workspace != workspace {
		t.Fatalf("delta = %+v", delta)
	}
	if len(delta.Messages) != 1 {
		t.Fatalf("delta messages = %+v", delta.Messages)
	}
	message := delta.Messages[0]
	if message.Role != "assistant" || sessions.MessageText(message) != "the answer" {
		t.Fatalf("message = %+v", message)
	}
	run := runner.plans[len(runner.plans)-1]
	token := flagValue(run.Args, "--resume")
	if token == "" {
		t.Fatalf("claude plan missing --resume: %q", run.Args)
	}
	if !strings.HasPrefix(message.ID, token+":") {
		t.Fatalf("id %q not namespaced by %q", message.ID, token)
	}
	if delta.ID != token {
		t.Fatalf("delta session id %q != resumed session %q", delta.ID, token)
	}
}

func TestSessionContinueClaudeFreshSessionWithoutHistory(t *testing.T) {
	workspace := testWorkspace(t)
	detail := sessions.Session{ID: "turn-1", Provider: "harness", Workspace: workspace, Messages: []sessions.Message{
		{ID: "m1", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: "first contact"}}},
	}, MessageCount: 1}
	input, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	application, stdout, runner := continueApp(t, "claude", "", string(input))
	runner.reply = []sessions.Message{{ID: "a1", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "welcome"}}}}
	if err := application.Run(context.Background(), []string{"session", "continue", "claude"}); err != nil {
		t.Fatal(err)
	}
	run := runner.plans[len(runner.plans)-1]
	if flagValue(run.Args, "--resume") != "" {
		t.Fatalf("fresh turn must not resume: %q", run.Args)
	}
	delta := decodeDelta(t, stdout)
	if delta.ID != runner.fresh {
		t.Fatalf("delta.ID = %q", delta.ID)
	}
	if len(delta.Messages) != 1 || sessions.MessageText(delta.Messages[0]) != "welcome" {
		t.Fatalf("delta = %+v", delta.Messages)
	}
}

func TestSessionContinueCodexEmitsDeltaFromConfiguredHome(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	workspace := testWorkspace(t)
	application, stdout, runner := continueApp(t, "codex", "", continueTranscript(t, workspace))
	runner.reply = []sessions.Message{{ID: "a1", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "codex answer"}}}}
	if err := application.Run(context.Background(), []string{"session", "continue", "codex", "--model", "gpt-6", "--mode", "yolo"}); err != nil {
		t.Fatal(err)
	}
	delta := decodeDelta(t, stdout)
	if delta.Provider != "codex" || len(delta.Messages) != 1 {
		t.Fatalf("delta = %+v", delta)
	}
	if sessions.MessageText(delta.Messages[0]) != "codex answer" {
		t.Fatalf("message = %+v", delta.Messages[0])
	}
	if len(runner.plans) != 1 || runner.plans[0].Env["CODEX_HOME"] != codexHome {
		t.Fatalf("codex must run in the configured home: %+v", runner.plans)
	}
	if _, err := os.Stat(filepath.Join(runner.home, ".codex", "sessions")); !os.IsNotExist(err) {
		t.Fatalf("expected no writes in the default Codex sessions directory, stat error = %v", err)
	}
}

func TestSessionContinuePiEmitsDelta(t *testing.T) {
	workspace := testWorkspace(t)
	application, stdout, runner := continueApp(t, "pi", "anthropic/claude-x", continueTranscript(t, workspace))
	runner.reply = []sessions.Message{{ID: "a1", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "pi answer"}}}}
	if err := application.Run(context.Background(), []string{"session", "continue", "pi", "--model", "anthropic/claude-x"}); err != nil {
		t.Fatal(err)
	}
	delta := decodeDelta(t, stdout)
	if delta.Provider != "pi" || delta.ID != "pi-native" {
		t.Fatalf("delta = %+v", delta)
	}
	if len(delta.Messages) != 1 || sessions.MessageText(delta.Messages[0]) != "pi answer" {
		t.Fatalf("messages = %+v", delta.Messages)
	}
}

func TestSessionContinueRejectsTranscriptNotEndingWithUser(t *testing.T) {
	workspace := testWorkspace(t)
	detail := sessions.Session{ID: "turn-1", Provider: "harness", Workspace: workspace, Messages: []sessions.Message{
		{ID: "m1", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "hello"}}},
	}, MessageCount: 1}
	input, _ := json.Marshal(detail)
	application, _, _ := continueApp(t, "claude", "", string(input))
	err := application.Run(context.Background(), []string{"session", "continue", "claude"})
	if err == nil || !strings.Contains(err.Error(), "must end with a user message") {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionContinueRequiresModelForPi(t *testing.T) {
	application, _, _ := continueApp(t, "pi", "", continueTranscript(t, testWorkspace(t)))
	err := application.Run(context.Background(), []string{"session", "continue", "pi"})
	if err == nil || !strings.Contains(err.Error(), "--model is required for pi") {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionContinueRejectsAgentsWithoutAdapter(t *testing.T) {
	application, _, _ := continueApp(t, "claude", "", "")
	err := application.Run(context.Background(), []string{"session", "continue", "gemini"})
	if err == nil || !strings.Contains(err.Error(), "no headless continue adapter") {
		t.Fatalf("err = %v", err)
	}
}

// The caller's real contract: each delta is appended to the transcript AS-IS
// (namespaced ids, dangling parentIds, native fields) and continued. The
// second import must keep the whole history on one Claude ancestor chain.
func TestSessionContinueRoundTripAppendsDeltaAsIs(t *testing.T) {
	workspace := testWorkspace(t)
	secret := "Remember the secret passphrase PINEAPPLE-77."
	first := sessions.Session{ID: "turn-1", Provider: "harness", Workspace: workspace, Messages: []sessions.Message{
		{ID: "u1", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: secret}}},
	}, MessageCount: 1}
	firstInput, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	application, stdout, runner := continueApp(t, "claude", "", string(firstInput))
	runner.reply = []sessions.Message{{ID: "a1", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "OK"}}}}
	if err := application.Run(context.Background(), []string{"session", "continue", "claude"}); err != nil {
		t.Fatal(err)
	}
	delta1 := decodeDelta(t, stdout)

	second := first
	second.Messages = append(append([]sessions.Message{}, first.Messages...), delta1.Messages...)
	second.Messages = append(second.Messages, sessions.Message{
		ID: "u2", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: "What was the passphrase?"}},
	})
	second.MessageCount = len(second.Messages)
	secondInput, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	application2, stdout2, runner2 := continueApp(t, "claude", "", string(secondInput))
	application2.Sessions = application.Sessions
	runner2.home = runner.home
	runner2.reply = []sessions.Message{{ID: "a2", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "PINEAPPLE-77"}}}}
	if err := application2.Run(context.Background(), []string{"session", "continue", "claude"}); err != nil {
		t.Fatal(err)
	}
	delta2 := decodeDelta(t, stdout2)
	if len(delta2.Messages) != 1 || sessions.MessageText(delta2.Messages[0]) != "PINEAPPLE-77" {
		t.Fatalf("second delta = %+v", delta2.Messages)
	}

	run2 := runner2.plans[len(runner2.plans)-1]
	token := flagValue(run2.Args, "--resume")
	if token == "" {
		t.Fatalf("second turn must resume the imported history: %q", run2.Args)
	}
	records := readClaudeRecords(t, sessions.ClaudeProjectFile(runner2.home, workspace, token))
	joined := ""
	// Only the imported history (first 2 records) is under test; the fake
	// runner rewrites the file when simulating the turn.
	if len(records) < 2 {
		t.Fatalf("imported history has %d records", len(records))
	}
	if records[0]["parentUuid"] != "" {
		t.Fatalf("history root carries parentUuid %v", records[0]["parentUuid"])
	}
	for i := 1; i < 2; i++ {
		if records[i]["parentUuid"] != records[i-1]["uuid"] {
			t.Fatalf("history record %d is off the ancestor chain: parentUuid=%v previous uuid=%v", i, records[i]["parentUuid"], records[i-1]["uuid"])
		}
	}
	for _, record := range records {
		data, _ := json.Marshal(record)
		joined += string(data)
	}
	if !strings.Contains(joined, "PINEAPPLE-77") {
		t.Fatalf("imported history lost the passphrase: %s", joined)
	}
}

func readClaudeRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}
