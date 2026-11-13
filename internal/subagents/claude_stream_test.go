package subagents

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
)

// TestMain doubles as a fake streaming claude: PATH stubs re-exec the test binary with
// SM_FAKE_CLAUDE set, so the daemon drives a real process over real pipes.
func TestMain(m *testing.M) {
	switch os.Getenv("SM_FAKE_NATIVE") {
	case "codex":
		fakeCodexMain()
		return
	case "pi":
		fakePiMain()
		return
	}
	if scenario := os.Getenv("SM_FAKE_CLAUDE"); scenario != "" {
		fakeClaudeMain(scenario)
		return
	}
	os.Exit(m.Run())
}

const fakeClaudeSession = "cccccccc-cccc-cccc-cccc-cccccccccccc"

func fakeClaudeMain(scenario string) {
	if file := os.Getenv("SM_FAKE_ARGS"); file != "" {
		_ = os.WriteFile(file, []byte(strings.Join(os.Args[1:], " ")), 0600)
		_ = os.WriteFile(file+".pid", []byte(fmt.Sprint(os.Getpid())), 0600)
	}
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 65536), 8*1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	emit := func(value map[string]any) {
		data, _ := json.Marshal(value)
		fmt.Println(string(data))
	}
	text := func(line string) string {
		var message struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &message) != nil || message.Type != "user" || len(message.Message.Content) == 0 {
			os.Exit(1)
		}
		return message.Message.Content[0].Text
	}
	first := true
	for line := range lines {
		prompt := text(line)
		emit(map[string]any{"type": "system", "subtype": "init", "session_id": fakeClaudeSession})
		switch {
		case scenario == "steer" && first:
			first = false
			extra := "none"
			select {
			case more, ok := <-lines:
				if ok {
					extra = text(more)
				}
			case <-time.After(1500 * time.Millisecond):
			}
			emit(map[string]any{"type": "result", "subtype": "success", "session_id": fakeClaudeSession, "is_error": false, "result": "turn:" + prompt + "+" + extra, "usage": map[string]any{"input_tokens": 7}})
		case scenario == "hang":
			select {}
		default:
			emit(map[string]any{"type": "result", "subtype": "success", "session_id": fakeClaudeSession, "is_error": false, "result": "echo:" + prompt, "usage": map[string]any{"input_tokens": 7}})
		}
	}
}

func installFakeClaude(t *testing.T, scenario string) string {
	t.Helper()
	bin := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "claude-args")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\nSM_FAKE_CLAUDE=%s SM_FAKE_ARGS=%q exec %q \"$@\"\n", scenario, argsFile, self)
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsFile
}

func claudeServer(t *testing.T, scenario string) (*Server, string) {
	t.Helper()
	s := testServer(t, runtime.ExecRunner{})
	argsFile := installFakeClaude(t, scenario)
	savePreset(t, s.Home, presets.Preset{Name: "claude-stream", Agent: "claude", Model: "opus", Description: "streaming test"})
	return s, argsFile
}

func readArgs(t *testing.T, argsFile string) string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestClaudeSteerIsDeliveredInsideTheSameTurn(t *testing.T) {
	s, argsFile := claudeServer(t, "steer")
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "long", "prompt": "task-A", "name": "c", "model": "claude-stream"})
	accepted := callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "c", "message": "fix-B"})
	if !strings.Contains(accepted, "disposition: steered") {
		t.Fatal(accepted)
	}
	report := waitReports(t, s, "c", 1)
	want := "turn:[sm:subagent]\ntask-A+" + steerPrefix + "fix-B"
	if report.Status != "completed" || report.Report != want {
		t.Fatalf("steer not delivered in the same turn: %#v", report)
	}
	s.mu.Lock()
	a, _ := s.target("c")
	s.mu.Unlock()
	if countReports(a.Dir) != 1 {
		t.Fatal("steer produced a second turn")
	}
	args := readArgs(t, argsFile)
	for _, want := range []string{"--input-format stream-json", "--output-format stream-json", "--dangerously-skip-permissions", "--model opus"} {
		if !strings.Contains(args, want) {
			t.Fatal("missing claude flag", want, args)
		}
	}
	meta, err := ReadMeta(a.Dir)
	if err != nil || meta.Session != fakeClaudeSession {
		t.Fatal("native session not persisted", meta, err)
	}
}

func TestClaudeQueuedMessageStartsNewRoundAfterSettleOnOneProcess(t *testing.T) {
	s, argsFile := claudeServer(t, "steer")
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "long", "prompt": "task-A", "name": "c", "model": "claude-stream"})
	accepted := callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "c", "message": "task-B", "delivery": "queued"})
	if !strings.Contains(accepted, "disposition: queued") {
		t.Fatal(accepted)
	}
	first := waitReports(t, s, "c", 1)
	if !strings.HasSuffix(first.Report, "+none") {
		t.Fatal("queued message leaked into the running turn", first.Report)
	}
	second := waitReports(t, s, "c", 2)
	if second.Report != "echo:task-B" {
		t.Fatal("queued message not delivered as a new round", second.Report)
	}
	if strings.Contains(readArgs(t, argsFile), "--resume") {
		t.Fatal("second round spawned a new process instead of reusing the live one")
	}
}

func TestClaudeStopKillsProcessGroupAndReportsCancelled(t *testing.T) {
	s, argsFile := claudeServer(t, "hang")
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "hang", "prompt": "never-settles", "name": "c", "model": "claude-stream"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(argsFile + ".pid"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake claude never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	text := callTest(t, s, "StopExternalSubagent", map[string]any{"task_id": "c"})
	if !strings.Contains(text, "Report status: cancelled.") {
		t.Fatal(text)
	}
	var pid int
	fmt.Sscan(strings.TrimSpace(readArgs(t, argsFile+".pid")), &pid)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pidAlive(pid) {
		t.Fatal("native claude process survived stop")
	}
	s.mu.Lock()
	a, _ := s.target("c")
	s.mu.Unlock()
	if state := s.runState(a); state != "cancelled" {
		t.Fatal(state)
	}
	if _, err := s.call("SendExternalSubagentMessage", []byte(`{"to":"c","message":"again"}`)); err == nil {
		t.Fatal("cancelled agent accepted a resume")
	}
}

func TestClaudeColdResumeAfterDaemonExitUsesResume(t *testing.T) {
	s, argsFile := claudeServer(t, "echo")
	s.SpawnDaemon = func(dir string) error {
		d := &Daemon{Home: s.Home, Dir: dir, Runner: s.Runner, Linger: 150 * time.Millisecond}
		go RunDaemon(d)
		return nil
	}
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "round-one", "name": "c", "model": "claude-stream", "run_in_background": false})
	s.mu.Lock()
	a, _ := s.target("c")
	s.mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := controlCall(a.Socket, controlRequest{Op: "status"}); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not linger out")
		}
		time.Sleep(20 * time.Millisecond)
	}
	accepted := callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "c", "message": "round-two"})
	if !strings.Contains(accepted, "disposition: started") {
		t.Fatal(accepted)
	}
	report := waitReports(t, s, "c", 2)
	if report.Report != "echo:round-two" {
		t.Fatal(report)
	}
	if args := readArgs(t, argsFile); !strings.Contains(args, "--resume "+fakeClaudeSession) {
		t.Fatal("cold resume did not resume the native session", args)
	}
}
