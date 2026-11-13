package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/sessions"
)

func userMsg(id, text string) sessions.Message {
	return sessions.Message{ID: id, Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: text}}}
}

func assistantMsg(id, text string) sessions.Message {
	return sessions.Message{ID: id, Role: "assistant", Content: []sessions.ContentBlock{{Type: "text", Text: text}}}
}

func toolMsg(id, toolUseID string, result any) sessions.Message {
	return sessions.Message{ID: id, Role: "tool", ToolUseID: toolUseID, Result: result}
}

func TestFreshHeadEndLocatesFirstNUserTurns(t *testing.T) {
	messages := []sessions.Message{
		userMsg("u1", "first"),
		assistantMsg("a1", "answer1"),
		toolMsg("t1", "call1", "output"),
		userMsg("u2", "second"),
		assistantMsg("a2", "answer2"),
		userMsg("u3", "third"),
	}
	if end := freshHeadEnd(messages, 2); end != 5 {
		t.Fatalf("headEnd(2) = %d, want 5 (up to and including a2)", end)
	}
	if end := freshHeadEnd(messages, 1); end != 3 {
		t.Fatalf("headEnd(1) = %d, want 3 (up to and including t1)", end)
	}
	if end := freshHeadEnd(messages, 10); end != 6 {
		t.Fatalf("headEnd(10) = %d, want 6 (whole slice)", end)
	}
	if end := freshHeadEnd(nil, 2); end != 0 {
		t.Fatalf("headEnd(nil) = %d, want 0", end)
	}
}

func TestFreshTailStartLocatesThirdUserFromEnd(t *testing.T) {
	messages := []sessions.Message{
		userMsg("u1", "first"),
		assistantMsg("a1", "answer"),
		toolMsg("t1", "call1", "output"),
		userMsg("u2", "second"),
		assistantMsg("a2", "answer"),
		userMsg("u3", "third"),
		toolMsg("t2", "call2", "output"),
		userMsg("u4", "fourth"),
		assistantMsg("a3", "answer"),
		userMsg("u5", "fifth"),
	}
	if start := freshTailStart(messages); start != 5 {
		t.Fatalf("start = %d, want 5 (u3)", start)
	}
	// Exactly three user prompts: the window is the whole transcript.
	if start := freshTailStart(messages[5:]); start != 0 {
		t.Fatalf("start = %d, want 0", start)
	}
	// Fewer than three user prompts: no trim.
	short := []sessions.Message{userMsg("u1", "a"), assistantMsg("a1", "b"), userMsg("u2", "c")}
	if start := freshTailStart(short); start != 0 {
		t.Fatalf("start = %d, want 0", start)
	}
	if start := freshTailStart(nil); start != 0 {
		t.Fatalf("start = %d, want 0", start)
	}
}

func TestOffloadToolResultsStubsOnlyToolEntries(t *testing.T) {
	messages := []sessions.Message{
		userMsg("u1", "question"),
		assistantMsg("a1", "answer"),
		toolMsg("t1", "call1", "big output"),
		toolMsg("t2", "call2", map[string]any{"stdout": "lines"}),
	}
	out := offloadToolResults(messages, "/tmp/history.json")
	stub := "[offloaded: /tmp/history.json]"
	if out[2].Result != stub || out[3].Result != stub {
		t.Fatalf("tool results = %#v / %#v", out[2].Result, out[3].Result)
	}
	if out[2].ToolUseID != "call1" || out[3].ToolUseID != "call2" {
		t.Fatalf("tool pairing lost: %#v", out)
	}
	if out[0].Content[0].Text != "question" || out[1].Content[0].Text != "answer" {
		t.Fatalf("non-tool messages changed: %#v", out)
	}
	if messages[2].Result != "big output" {
		t.Fatalf("input mutated: %#v", messages[2].Result)
	}
}

func TestEstimateContextTokensAndThreshold(t *testing.T) {
	messages := []sessions.Message{
		userMsg("u1", strings.Repeat("a", 400)),
		toolMsg("t1", "call1", strings.Repeat("b", 200)),
	}
	if tokens := estimateContextTokens(messages); tokens != 150 {
		t.Fatalf("tokens = %d, want 150", tokens)
	}
	structured := []sessions.Message{toolMsg("t1", "call1", map[string]any{"k": "v"})}
	encoded, _ := json.Marshal(map[string]any{"k": "v"})
	if tokens := estimateContextTokens(structured); tokens != len(encoded)/4 {
		t.Fatalf("tokens = %d, want %d", tokens, len(encoded)/4)
	}

	atLimit := []sessions.Message{userMsg("u1", strings.Repeat("a", freshTokenThreshold*4))}
	if tokens, large := exceedsFreshThreshold(atLimit); large || tokens != freshTokenThreshold {
		t.Fatalf("at limit: tokens = %d large = %t", tokens, large)
	}
	over := []sessions.Message{userMsg("u1", strings.Repeat("a", freshTokenThreshold*4+4))}
	if tokens, large := exceedsFreshThreshold(over); !large || tokens != freshTokenThreshold+1 {
		t.Fatalf("over limit: tokens = %d large = %t", tokens, large)
	}
}

func TestFreshSessionDetailOffloadsAndRechains(t *testing.T) {
	home := t.TempDir()
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: home}
	detail := sessions.Session{
		ID: "orig-session", Provider: "codex", Workspace: "/work",
		Messages: []sessions.Message{
			userMsg("u1", "first"),
			toolMsg("t1", "call1", "huge early output"),
			userMsg("u2", "second"),
			assistantMsg("a1", "answer"),
			userMsg("u3", "third"),
			toolMsg("t2", "call2", "huge late output"),
			userMsg("u4", "fourth"),
			assistantMsg("a2", "answer"),
			userMsg("u5", "fifth"),
		},
	}
	detail.MessageCount = len(detail.Messages)

	out, err := application.freshSessionDetail(detail)
	if err != nil {
		t.Fatal(err)
	}

	historyPath := filepath.Join(home, ".local", "state", "sessionman", "history", "orig-session.json")
	info, err := os.Stat(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("history mode = %v", info.Mode().Perm())
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	var history sessions.Session
	if err := json.Unmarshal(data, &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Messages) != 9 || history.Messages[1].Result != "huge early output" {
		t.Fatalf("history file lost content: %#v", history.Messages)
	}

	// Head (u1,t1,u2,a1) + history pointer + tail (u3,t2,u4,a2,u5) = 10.
	if len(out.Messages) != 10 || out.MessageCount != 10 {
		t.Fatalf("messages = %d count = %d, want 10", len(out.Messages), out.MessageCount)
	}
	if out.Messages[0].Content[0].Text != "first" {
		t.Fatalf("head starts at %#v, want u1", out.Messages[0])
	}
	header := out.Messages[4]
	if header.Role != "user" {
		t.Fatalf("header = %#v", header)
	}
	want := fmt.Sprintf("Full conversation history: %s — read it when earlier context is needed.", historyPath)
	if len(header.Content) != 1 || header.Content[0].Text != want {
		t.Fatalf("header text = %#v", header.Content)
	}
	if out.Messages[5].Content[0].Text != "third" {
		t.Fatalf("tail starts at %#v, want u3", out.Messages[5])
	}
	stub := "[offloaded: " + historyPath + "]"
	stubbed := 0
	for _, message := range out.Messages {
		if message.Role == "tool" {
			stubbed++
			if message.Result != stub {
				t.Fatalf("tool result = %#v", message.Result)
			}
		}
	}
	if stubbed != 2 {
		t.Fatalf("stubbed = %d, want 2 (t1 in head, t2 in tail)", stubbed)
	}
	for i, message := range out.Messages {
		if message.Timestamp == "" {
			t.Fatalf("messages[%d] has no timestamp", i)
		}
		if i == 0 {
			continue
		}
		if message.ParentID != out.Messages[i-1].ID {
			t.Fatalf("chain broken at %d: parent %q, previous id %q", i, message.ParentID, out.Messages[i-1].ID)
		}
	}
}

func TestFreshSessionDetailKeepsShortTranscriptsWhole(t *testing.T) {
	home := t.TempDir()
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: home}
	detail := sessions.Session{
		ID: "short", Provider: "claude",
		Messages: []sessions.Message{userMsg("u1", "only"), toolMsg("t1", "call1", "output"), assistantMsg("a1", "done")},
	}
	out, err := application.freshSessionDetail(detail)
	if err != nil {
		t.Fatal(err)
	}
	historyPath := filepath.Join(home, ".local", "state", "sessionman", "history", "short.json")
	if _, err := os.Stat(historyPath); err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 4 {
		t.Fatalf("messages = %d, want 3 originals (head) + header", len(out.Messages))
	}
	if out.Messages[0].Content[0].Text != "only" {
		t.Fatalf("head[0] = %#v, want u1", out.Messages[0])
	}
	if out.Messages[1].Result != "[offloaded: "+historyPath+"]" {
		t.Fatalf("tool result = %#v", out.Messages[1].Result)
	}
	wantHeader := fmt.Sprintf("Full conversation history: %s — read it when earlier context is needed.", historyPath)
	if out.Messages[3].Content[0].Text != wantHeader {
		t.Fatalf("header not at end: %#v", out.Messages[3])
	}
}

func TestFreshKeyReadsAsSelectionFresh(t *testing.T) {
	for _, input := range []string{"f", "F"} {
		key, err := readSelectionKey(bufio.NewReader(strings.NewReader(input)))
		if err != nil || key != selectionFresh {
			t.Fatalf("input %q: key = %v err = %v", input, key, err)
		}
	}
}

func TestConfirmLargeContextKeys(t *testing.T) {
	for input, want := range map[string]selectionKey{"xf": selectionFresh, "\r": selectionConfirm, "q": selectionCancel} {
		var stdout bytes.Buffer
		application := New(false, strings.NewReader(input), &stdout, &bytes.Buffer{})
		key, err := application.confirmLargeContext(bufio.NewReader(application.Stdin), 312_000)
		if err != nil || key != want {
			t.Fatalf("input %q: key = %v err = %v", input, key, err)
		}
		if !strings.Contains(stdout.String(), "Context ~312K tokens — [f] fresh  [Enter] full  [q] cancel") {
			t.Fatalf("prompt = %q", stdout.String())
		}
	}
}

func TestResumeInteractiveLargeContextCancel(t *testing.T) {
	var stdout bytes.Buffer
	application := New(false, strings.NewReader("q"), &stdout, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: t.TempDir()}
	detail := sessions.Session{
		ID: "big", Provider: "codex",
		Messages: []sessions.Message{userMsg("u1", strings.Repeat("a", (freshTokenThreshold+1)*4))},
	}
	err := application.resumeSessionInteractive(context.Background(), bufio.NewReader(application.Stdin), detail, false)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "Context ~") {
		t.Fatalf("no large-context prompt in %q", stdout.String())
	}
}

func TestFreshKeyResumesAsNewNativeSessionSameAgent(t *testing.T) {
	application, _, runner := presetApp(t, "jjf\r", "codex")
	mustCreate(t, application.Hooks.Home, deepReview)
	home := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	application.Sessions = sessions.Service{HomeDir: home}
	workspace := t.TempDir()
	path := filepath.Join(home, ".codex", "sessions", "2026", "11", "01", "orig.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := fmt.Sprintf("{\"type\":\"session_meta\",\"timestamp\":\"2026-11-01T10:00:00Z\",\"payload\":{\"id\":\"orig-id\",\"cwd\":%q}}\n{\"type\":\"response_item\",\"timestamp\":\"2026-11-01T10:01:00Z\",\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":\"original question\"}}", workspace)
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := application.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(runner.plans) != 1 {
		t.Fatalf("plans = %#v", runner.plans)
	}
	plan := runner.plans[0]
	token := ""
	for i, arg := range plan.Args {
		if arg == "resume" && i+1 < len(plan.Args) {
			token = plan.Args[i+1]
		}
	}
	if plan.Executable != "codex" || token == "" || token == "orig-id" {
		t.Fatalf("fresh resume must launch a new native session, plan = %#v", plan)
	}
	historyPath := filepath.Join(home, ".local", "state", "sessionman", "history", "orig-id.json")
	history, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(history), "original question") {
		t.Fatalf("history = %q", history)
	}
	matches, err := filepath.Glob(filepath.Join(codexHome, "sessions", "*", "*", "*", "*-"+token+".jsonl"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("rollouts = %v err = %v", matches, err)
	}
	rollout, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rollout), "Full conversation history: "+historyPath) || !strings.Contains(string(rollout), "original question") {
		t.Fatalf("rollout = %q", rollout)
	}
}
