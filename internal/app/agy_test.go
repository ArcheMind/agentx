package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/sessions"
)

func TestAgyConvertSQLite(t *testing.T) {
	var binary bytes.Buffer
	application := App{Output: OutputJSON, Stdin: strings.NewReader(`{"id":"source","provider":"claude","workspace":"/tmp","messages":[{"id":"u","role":"user","content":[{"type":"text","text":"remember this"}]}]}`), Stdout: &binary}
	if err := application.convertTo("agy"); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(binary.Bytes(), []byte("SQLite format 3\x00")) {
		t.Fatal("convert output must be native SQLite")
	}
	var unified bytes.Buffer
	application.Stdin, application.Stdout = bytes.NewReader(binary.Bytes()), &unified
	if err := application.convertFrom("agy"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unified.String(), "remember this") {
		t.Fatal(unified.String())
	}
}

func TestAgyContinueStream(t *testing.T) {
	for _, test := range []struct {
		output    string
		failed    bool
		completed bool
	}{
		{`{"event":"init","conversation_id":"native"}` + "\n" + `{"event":"result","result":{"conversation_id":"native","status":"SUCCESS"}}`, false, true},
		{`{"event":"init","conversation_id":"wrong"}`, true, false},
		{`{"event":"result","result":{"conversation_id":"wrong","status":"SUCCESS"}}`, true, false},
		{`{"event":"result","result":{"conversation_id":"native","status":"ERROR","response":"rejected"}}`, false, false},
	} {
		run, err := parseContinueStream("agy", "future-model", "native", test.output)
		if (err != nil) != test.failed || run.completed != test.completed {
			t.Fatalf("run=%+v err=%v", run, err)
		}
	}
	if _, err := parseContinueStream("unknown", "", "", ""); err == nil {
		t.Fatal("unknown agent accepted")
	}
}

func TestAgyContinueNativePath(t *testing.T) {
	application := App{Sessions: sessions.Service{HomeDir: "/home/native"}}
	path, err := application.continueSessionFile("agy", "/workspace", "id", "")
	if err != nil || path != "/home/native/.gemini/antigravity-cli/conversations/id.db" {
		t.Fatalf("%s %v", path, err)
	}
}

func TestAgyMCPRequiresExplicitWorkspace(t *testing.T) {
	t.Setenv("SM_MCP_CWD", "")
	if err := (App{}).agyMCPServe(context.Background()); err == nil || !strings.Contains(err.Error(), "start agy through SessionMan") {
		t.Fatalf("%v", err)
	}
	plan := runtime.CommandPlan{Cwd: t.TempDir(), Env: map[string]string{"existing": "kept"}}
	prepareAgyNativePlan("agy", &plan)
	if plan.Env["SM_MCP_CWD"] != plan.Cwd || plan.Env["existing"] != "kept" {
		t.Fatalf("%+v", plan)
	}
}

func TestAgyCatalogPartialWarningKeepsUsableList(t *testing.T) {
	home := t.TempDir()
	valid := sessions.Session{Workspace: t.TempDir(), Messages: []sessions.Message{{ID: "u", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: "usable transcript"}}}}}
	id, err := sessions.WriteAgySessionFile(valid, home)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(home, ".gemini", "antigravity-cli", "conversations", "broken.db")
	if err := os.WriteFile(bad, []byte("broken SQLite"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &stderr)
	application.Sessions = sessions.Service{HomeDir: home}
	if err := application.Run(context.Background(), []string{"--json", "session", "list", "--all"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), id) || !strings.Contains(stderr.String(), "partial session catalog") || !strings.Contains(stderr.String(), "broken.db") {
		t.Fatalf("stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

type agyContinueRunner struct {
	home   string
	prompt string
	plan   runtime.CommandPlan
}

func (r *agyContinueRunner) Execute(_ context.Context, plan runtime.CommandPlan, opts runtime.ExecuteOptions) (runtime.CommandResult, error) {
	r.plan = plan
	data, err := io.ReadAll(opts.Stdin)
	if err != nil {
		return runtime.CommandResult{}, err
	}
	var event struct {
		Event   string `json:"event"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return runtime.CommandResult{}, err
	}
	if event.Event != "user" {
		return runtime.CommandResult{}, fmt.Errorf("unexpected input event %s", event.Event)
	}
	r.prompt = event.Message.Content
	token := flagValue(plan.Args, "--conversation")
	path := filepath.Join(r.home, ".gemini", "antigravity-cli", "conversations", token+".db")
	detail, err := sessions.ParseAgySessionFile(path)
	if err != nil {
		return runtime.CommandResult{}, err
	}
	detail.Messages = append(detail.Messages, sessions.Message{ID: "echo", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: r.prompt}}}, sessions.Message{ID: "answer", Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: "new answer"}}})
	if err := os.Remove(path); err != nil {
		return runtime.CommandResult{}, err
	}
	if err := sessions.WriteAgyDatabaseFile(detail, path); err != nil {
		return runtime.CommandResult{}, err
	}
	return runtime.CommandResult{Stdout: fmt.Sprintf("{\"event\":\"init\",\"conversation_id\":%q}\n{\"event\":\"result\",\"result\":{\"conversation_id\":%q,\"status\":\"SUCCESS\"}}\n", token, token)}, nil
}

func TestAgyContinueReparsesProjectedHistoryPrefix(t *testing.T) {
	home, workspace := t.TempDir(), testWorkspace(t)
	source := sessions.Session{ID: "source", Provider: "claude", Workspace: workspace, Messages: []sessions.Message{
		{ID: "history", Role: "assistant", Content: []sessions.ContentBlock{{Type: "thinking", Text: "foreign reasoning", Signature: "foreign-signature"}, {Type: "text", Text: "historical answer"}}},
		{ID: "prompt", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: "new prompt\nwith special \"quotes\""}}},
	}}
	input, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := New(false, bytes.NewReader(input), &stdout, &stderr)
	app.Sessions = sessions.Service{HomeDir: home}
	app.Hooks.Home = home
	runner := &agyContinueRunner{home: home}
	app.Runner = runner
	if err := app.Run(context.Background(), []string{"session", "continue", "agy", "--model", "native-future-model"}); err != nil {
		t.Fatal(err)
	}
	var delta sessions.Session
	if err := json.Unmarshal(stdout.Bytes(), &delta); err != nil {
		t.Fatal(err)
	}
	if len(delta.Messages) != 1 || sessions.MessageText(delta.Messages[0]) != "new answer" || runner.prompt != "new prompt\nwith special \"quotes\"" || runner.plan.Env["SM_MCP_CWD"] != workspace {
		t.Fatalf("delta=%+v prompt=%q plan=%+v", delta, runner.prompt, runner.plan)
	}
}

func TestAgyWizardEffortIsAlwaysUnverifiedPerModel(t *testing.T) {
	draft := presetDraft{preset: presets.Preset{Agent: "agy", Model: "native-future-model"}, listed: &runtime.Model{ID: "native-future-model", Source: "agy models"}}
	note, choices := effortChoices(draft)
	if !strings.Contains(note, "unverified") || len(choices) != 6 {
		t.Fatalf("%s %+v", note, choices)
	}
	for _, choice := range choices[1:] {
		if choice.status != "unverified for this model" {
			t.Fatalf("%+v", choice)
		}
	}
}
