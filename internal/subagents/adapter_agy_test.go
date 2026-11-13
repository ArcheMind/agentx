package subagents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
)

const fakeAgySession = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func fakeAgyMain() {
	rec := fakeRecorder()
	fakeSpawnRecord(rec)
	var active []int
	var permission int
	var child *exec.Cmd
	complete := func(id int, text string) {
		emitJSON(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": fakeAgySession, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": text}}}})
		emitJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"stopReason": "end_turn", "usage": map[string]any{"inputTokens": 3, "nativeExtra": "preserved"}}})
	}
	for line := range stdinLines() {
		rec(fakeRecord{Kind: "request", Line: json.RawMessage(line)})
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Prompt []struct {
					Text string `json:"text"`
				} `json:"prompt"`
				Session string `json:"sessionId"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(line), &req) != nil {
			continue
		}
		var id int
		json.Unmarshal(req.ID, &id)
		switch req.Method {
		case "initialize", "session/set_model", "session/set_config_option":
			emitJSON(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}})
		case "session/new":
			emitJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"sessionId": fakeAgySession}})
		case "session/load":
			if os.Getenv("SM_FAKE_LOAD_FAIL") != "" {
				emitJSON(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": "original missing"}})
				continue
			}
			// Native load replays old messages before returning.
			emitJSON(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": fakeAgySession, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "OLD_HISTORY"}}}})
			emitJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
		case "session/prompt":
			text := req.Params.Prompt[0].Text
			switch {
			case strings.Contains(text, "PERMISSION"):
				permission = id
				emitJSON(map[string]any{"jsonrpc": "2.0", "id": "permission-1", "method": "session/request_permission", "params": map[string]any{}})
			case strings.Contains(text, "HOLD"):
				active = append(active, id)
				if path := os.Getenv("SM_FAKE_CHILD_PID"); path != "" {
					child = exec.Command("sh", "-c", "sleep 120")
					runtime.Detach(child)
					if child.Start() == nil {
						os.WriteFile(path, []byte(fmt.Sprint(child.Process.Pid)), 0600)
					}
				}
			case strings.Contains(text, "STEER"):
				if len(active) > 0 {
					emitJSON(map[string]any{"jsonrpc": "2.0", "id": active[0], "result": map[string]any{"stopReason": "end_turn", "usage": map[string]any{"inputTokens": 17, "nativeFirst": "kept"}}})
					active = nil
					time.Sleep(120 * time.Millisecond)
				}
				complete(id, "STEER_SEEN")
			case strings.Contains(text, "STOP_REASON="):
				reason := promptDirective(text, "STOP_REASON=")
				emitJSON(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": fakeAgySession, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "PARTIAL"}}}})
				result := map[string]any{}
				if reason != "missing" {
					result["stopReason"] = reason
				}
				emitJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
			case strings.Contains(text, "FAIL"):
				emitJSON(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32602, "message": "native failure"}})
			default:
				complete(id, "ECHO:"+text)
			}
		case "":
			if string(req.ID) == `"permission-1"` {
				emitJSON(map[string]any{"jsonrpc": "2.0", "id": permission, "result": map[string]any{"stopReason": "cancelled"}})
			}
		}
	}
	if child != nil {
		child.Process.Kill()
		child.Wait()
	}
}

func agyServer(t *testing.T, extra map[string]string) (*Server, string) {
	t.Helper()
	if _, ok := presets.Agents["agy"]; !ok {
		presets.Agents["agy"] = presets.AgentRule{Name: "agy", Efforts: []string{"low", "medium", "high"}, Bypassable: true}
		t.Cleanup(func() { delete(presets.Agents, "agy") })
	}
	s := testServer(t, runtime.ExecRunner{})
	record := installFakeNative(t, "agy", extra)
	savePreset(t, s.Home, presets.Preset{Name: "agy-test", Agent: "agy", Model: "gemini-3.8-flash-low", Effort: "low", Description: "review"})
	return s, record
}

func startAgyTest(t *testing.T, s *Server, prompt string, background bool) *ExternalSubagentState {
	t.Helper()
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "acp", "prompt": prompt, "name": "agy", "model": "agy-test", "run_in_background": background})
	s.mu.Lock()
	a, _ := s.target("agy")
	s.mu.Unlock()
	return a
}

func TestAgySteerWaitsForAllInflightAndThenDeliversQueue(t *testing.T) {
	s, record := agyServer(t, nil)
	a := startAgyTest(t, s, "HOLD", true)
	callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "agy", "message": "queued-next", "delivery": "queued"})
	accepted := callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "agy", "message": "STEER"})
	if !strings.Contains(accepted, "steered") {
		t.Fatal(accepted)
	}
	time.Sleep(40 * time.Millisecond)
	if countReports(a.Dir) != 0 {
		t.Fatal("old prompt response prematurely settled the turn")
	}
	first := waitReports(t, s, "agy", 1)
	if first.Status != "completed" || first.Report != "STEER_SEEN" {
		t.Fatalf("%#v", first)
	}
	var merged struct {
		Prompts []agyPromptResult `json:"prompts"`
	}
	if err := json.Unmarshal(first.Usage, &merged); err != nil || len(merged.Prompts) != 2 {
		t.Fatal(string(first.Usage), err)
	}
	if merged.Prompts[0].ID == merged.Prompts[1].ID || !strings.Contains(string(merged.Prompts[0].Usage), `"inputTokens":17`) || !strings.Contains(string(merged.Prompts[1].Usage), `"inputTokens":3`) {
		t.Fatal(string(first.Usage))
	}
	second := waitReports(t, s, "agy", 2)
	if second.Status != "completed" || !strings.Contains(second.Report, "queued-next") {
		t.Fatalf("%#v", second)
	}
	if !strings.Contains(string(second.Usage), `"nativeExtra":"preserved"`) {
		t.Fatal(string(second.Usage))
	}
	if !strings.Contains(second.Evidence, "unverified") {
		t.Fatal(second.Evidence)
	}
	records := readFakeRecords(t, record)
	if countFakeRequests(t, records, "session/new") != 1 || countFakeRequests(t, records, "session/prompt") != 3 {
		t.Fatal(records)
	}
	if _, ok := fakeRequest(t, records, "initialize")["clientInfo"].(map[string]any)["version"]; ok {
		t.Fatal("fabricated client version")
	}
	firstPrompt := fakeRequest(t, records, "session/prompt")["prompt"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.HasPrefix(firstPrompt, "[sm:subagent]\nRole: agy-test\nreview\n\nHOLD") {
		t.Fatal("first native prompt lost its first-line subagent marker", firstPrompt)
	}
	if fakeRequest(t, records, "session/set_config_option")["value"] != "low" {
		t.Fatal(records)
	}
	auth, err := DiscoverAuth("agy")
	if err != nil || auth.Key != "" {
		t.Fatal(auth, err)
	}
}

func TestAgyColdLoadKeepsSessionAndDiscardsReplay(t *testing.T) {
	s, record := agyServer(t, nil)
	s.SpawnDaemon = func(dir string) error {
		go RunDaemon(&Daemon{Home: s.Home, Dir: dir, Runner: s.Runner, Linger: 80 * time.Millisecond})
		return nil
	}
	a := startAgyTest(t, s, "first", false)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := controlCall(a.Socket, controlRequest{Op: "status"}); err != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "agy", "message": "second"})
	report := waitReports(t, s, "agy", 2)
	if report.Status != "completed" || strings.Contains(report.Report, "OLD_HISTORY") || !strings.Contains(report.Report, "second") {
		t.Fatalf("%#v", report)
	}
	records := readFakeRecords(t, record)
	if fakeRequest(t, records, "session/load")["sessionId"] != fakeAgySession || countFakeRequests(t, records, "session/new") != 1 {
		t.Fatal(records)
	}
}

func TestAgyPermissionWithoutHumanCancelsAndNativeErrorFails(t *testing.T) {
	for _, prompt := range []string{"PERMISSION", "FAIL"} {
		t.Run(prompt, func(t *testing.T) {
			s, _ := agyServer(t, nil)
			startAgyTest(t, s, prompt, true)
			report := waitReports(t, s, "agy", 1)
			want := "failed"
			if prompt == "PERMISSION" {
				want = "cancelled"
			}
			if report.Status != want {
				t.Fatalf("%#v", report)
			}
		})
	}
}

func TestAgyStopTerminatesDetachedDescendant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "child.pid")
	s, _ := agyServer(t, map[string]string{"SM_FAKE_CHILD_PID": path})
	a := startAgyTest(t, s, "HOLD", true)
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		if _, err := fmt.Sscan(string(data), &pid); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("descendant never started")
	}
	response, err := controlCall(a.Socket, controlRequest{Op: "stop"})
	if err != nil || !response.Cancelled {
		t.Fatal(response, err)
	}
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("detached descendant survived stop", pid)
}

func TestAgyNestedMCPAndUnknownExecutor(t *testing.T) {
	d := &Daemon{Dir: "/agent", agent: &ExternalSubagentState{Cwd: "/child-work"}, meta: AgentMeta{AxBinary: "/sm", DelegationDepth: 1}}
	mount := d.agyMCPServers()
	if len(mount) != 1 || mount[0].Command != "/sm" || len(mount[0].Env) != 3 {
		t.Fatal(mount)
	}
	d.agent = &ExternalSubagentState{Option: presets.Preset{Agent: "unknown"}}
	if response := d.sendAdapterLocked("x", "steer"); !strings.Contains(response.Error, "no native subagent") {
		t.Fatal(response)
	}
	if _, err := DiscoverAuth("unknown"); err == nil {
		t.Fatal("unknown executor inherited Codex auth")
	}
}

func TestAgyColdLoadFailureDoesNotCreateReplacement(t *testing.T) {
	s, record := agyServer(t, map[string]string{"SM_FAKE_LOAD_FAIL": "1"})
	s.SpawnDaemon = func(dir string) error {
		go RunDaemon(&Daemon{Home: s.Home, Dir: dir, Runner: s.Runner, Linger: 80 * time.Millisecond})
		return nil
	}
	a := startAgyTest(t, s, "first", false)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := controlCall(a.Socket, controlRequest{Op: "status"}); err != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, err := s.call("SendExternalSubagentMessage", []byte(`{"to":"agy","message":"second"}`))
	if err == nil || !strings.Contains(err.Error(), "original missing") {
		t.Fatal(err)
	}
	records := readFakeRecords(t, record)
	if countFakeRequests(t, records, "session/new") != 1 || countFakeRequests(t, records, "session/load") != 1 {
		t.Fatal(records)
	}
}

func TestAgyNormalLeavesNativePermissionConfiguration(t *testing.T) {
	s, record := agyServer(t, nil)
	preset, err := presets.Read(s.Home, "agy-test")
	if err != nil {
		t.Fatal(err)
	}
	preset.Mode = ModeNormal
	savePreset(t, s.Home, preset)
	a := startAgyTest(t, s, "normal", false)
	records := readFakeRecords(t, record)
	for _, record := range records {
		if record.Kind == "spawn" && (record.Env["SM_MCP_CWD"] != a.Cwd || record.Env["SM_HOST_AGENT_DIR"] != a.Dir || record.Env["SM_DELEGATION_DEPTH"] != "1") {
			t.Fatal(record)
		}
		if record.Kind == "spawn" && strings.Contains(strings.Join(record.Argv, " "), "--dangerously-skip-permissions") {
			t.Fatal(record)
		}
	}
	if fakeRequest(t, records, "session/set_model")["modelId"] != "gemini-3.8-flash-low" {
		t.Fatal(records)
	}
}

func TestAgyRequiresNativeTerminalReason(t *testing.T) {
	for _, reason := range []string{"missing", "unknown", "max_tokens", "max_turn_requests", "refusal"} {
		t.Run(reason, func(t *testing.T) {
			s, record := agyServer(t, nil)
			startAgyTest(t, s, "STOP_REASON="+reason, true)
			report := waitReports(t, s, "agy", 1)
			if report.Status != "failed" {
				t.Fatalf("%#v", report)
			}
			if !strings.Contains(report.Failure, "stopReason") {
				t.Fatal(report.Failure)
			}
			if reason != "missing" && !strings.Contains(report.Failure, reason) {
				t.Fatal(report.Failure)
			}
			if reason != "missing" && reason != "unknown" && report.Report != "PARTIAL" {
				t.Fatal(report)
			}
			if countFakeRequests(t, readFakeRecords(t, record), "session/prompt") != 1 {
				t.Fatal("implicit retry")
			}
		})
	}
}

func TestAgyPluginEnvironmentOverridesParentIdentity(t *testing.T) {
	t.Setenv("SM_MCP_CWD", "/parent-work")
	t.Setenv("SM_HOST_AGENT_DIR", "/parent-agent")
	t.Setenv("SM_DELEGATION_DEPTH", "0")
	t.Setenv("SM_SUBAGENT_HOME", "/preset-home")
	d := &Daemon{Dir: "/child-agent", agent: &ExternalSubagentState{Cwd: "/child-work"}, meta: AgentMeta{DelegationDepth: 1}}
	env := d.agyEnvironment()
	for key, want := range map[string]string{"SM_MCP_CWD": "/child-work", "SM_HOST_AGENT_DIR": "/child-agent", "SM_DELEGATION_DEPTH": "2", "SM_SUBAGENT_HOME": "/preset-home"} {
		if env[key] != want {
			t.Fatal(env)
		}
	}
}
