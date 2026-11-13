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
)

// Fake native binaries for the streaming adapters. PATH stubs re-exec the test binary with
// SM_FAKE_* set; behavior is keyed off prompt text: HOLD (never settles), WAIT (holds ~1.5s
// for a steer), FAIL (native failure), WRITE=<name> (writes a file into the cwd), else echo.

type fakeRecord struct {
	Kind string            `json:"kind"`
	Argv []string          `json:"argv,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
	Line json.RawMessage   `json:"line,omitempty"`
}

func fakeRecorder() func(fakeRecord) {
	path := os.Getenv("SM_FAKE_RECORD")
	return func(record fakeRecord) {
		if path == "" {
			return
		}
		data, _ := json.Marshal(record)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
		file.Write(append(data, '\n'))
		file.Close()
	}
}

func fakeSpawnRecord(rec func(fakeRecord)) {
	env := map[string]string{}
	for _, key := range []string{"CODEX_HOME", "PI_CODING_AGENT_DIR", "RUST_LOG", "SM_BINARY", "SM_DELEGATION_DEPTH", "SM_HOST_AGENT_DIR", "SM_MCP_CWD"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	rec(fakeRecord{Kind: "spawn", Argv: os.Args[1:], Env: env})
}

func stdinLines() <-chan string {
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 65536), 8*1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	return lines
}

func emitJSON(value any) {
	data, _ := json.Marshal(value)
	fmt.Println(string(data))
}

func argValueOf(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// ---- fake codex app-server -----------------------------------------------------------------

func fakeCodexMain() {
	rec := fakeRecorder()
	fakeSpawnRecord(rec)
	lines := stdinLines()
	const threadFallback = "01a15000-1111-7222-8333-944455556666"
	var cwd, prompt, turnID string
	var effortConfigured string
	var steers []string
	turnActive := false
	turnN := 0
	var deadline <-chan time.Time
	completeTurn := func(status string) {
		if name := promptDirective(prompt, "WRITE="); name != "" {
			os.WriteFile(filepath.Join(cwd, name), []byte("agent output\n"), 0600)
		}
		if os.Getenv("RUST_LOG") == codexTelemetryFilter {
			executed := envOr("SM_FAKE_EFFORT", effortConfigured)
			if executed == "" {
				executed = "medium"
			}
			fmt.Fprintf(os.Stderr, "2026-11-12T12:00:00Z  INFO codex_otel.trace_safe: event.name=\"codex.sse_event\" event.kind=response.completed input_token_count=1 model_reasoning_effort=%q model=test-model\n", executed)
		}
		if status == "failed" {
			emitJSON(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"id": turnID, "status": "failed", "error": map[string]any{"message": envOr("SM_FAKE_FAILMSG", "native failure")}}}})
		} else {
			text := "ECHO:" + prompt
			if len(steers) > 0 {
				text += "+STEER:" + strings.Join(steers, "|")
			} else if strings.Contains(prompt, "WAIT") {
				text += "+none"
			}
			emitJSON(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]any{"type": "agentMessage", "text": text}}})
			emitJSON(map[string]any{"method": "thread/tokenUsage/updated", "params": map[string]any{"tokenUsage": map[string]any{"last": map[string]any{"totalTokens": 5, "inputTokens": 4}}}})
			emitJSON(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"id": turnID, "status": "completed"}}})
		}
		turnActive, steers, deadline = false, nil, nil
	}
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				os.Exit(0)
			}
			var request struct {
				ID     *int `json:"id"`
				Method string
				Params json.RawMessage
			}
			if json.Unmarshal([]byte(line), &request) != nil || request.ID == nil {
				continue
			}
			rec(fakeRecord{Kind: "request", Line: json.RawMessage(line)})
			switch request.Method {
			case "initialize":
				emitJSON(map[string]any{"id": *request.ID, "result": map[string]any{}})
			case "thread/start", "thread/resume":
				var params struct {
					ThreadID string `json:"threadId"`
					Cwd      string `json:"cwd"`
					Config   struct {
						Effort string `json:"model_reasoning_effort"`
					} `json:"config"`
				}
				json.Unmarshal(request.Params, &params)
				cwd, effortConfigured = params.Cwd, params.Config.Effort
				id := params.ThreadID
				if id == "" {
					id = threadFallback
				}
				emitJSON(map[string]any{"id": *request.ID, "result": map[string]any{"thread": map[string]any{"id": id}}})
			case "turn/start":
				var params struct {
					Input []struct {
						Text string `json:"text"`
					} `json:"input"`
				}
				json.Unmarshal(request.Params, &params)
				turnN++
				turnID = fmt.Sprintf("turn-%d", turnN)
				prompt, steers, turnActive = params.Input[0].Text, nil, true
				emitJSON(map[string]any{"id": *request.ID, "result": map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}})
				emitJSON(map[string]any{"method": "turn/started", "params": map[string]any{"turn": map[string]any{"id": turnID}}})
				switch {
				case strings.Contains(prompt, "HOLD"):
				case strings.Contains(prompt, "WAIT"):
					deadline = time.After(1500 * time.Millisecond)
				case strings.Contains(prompt, "FAIL"):
					completeTurn("failed")
				default:
					completeTurn("completed")
				}
			case "turn/steer":
				if message := os.Getenv("SM_FAKE_STEER_ERROR"); message != "" {
					emitJSON(map[string]any{"id": *request.ID, "error": map[string]any{"code": -32602, "message": message}})
					continue
				}
				if !turnActive {
					emitJSON(map[string]any{"id": *request.ID, "error": map[string]any{"code": -32600, "message": "no active turn to steer"}})
					continue
				}
				var params struct {
					Input []struct {
						Text string `json:"text"`
					} `json:"input"`
				}
				json.Unmarshal(request.Params, &params)
				steers = append(steers, params.Input[0].Text)
				emitJSON(map[string]any{"id": *request.ID, "result": map[string]any{"turnId": turnID}})
				if strings.Contains(prompt, "WAIT") {
					completeTurn("completed")
				}
			default:
				emitJSON(map[string]any{"id": *request.ID, "result": map[string]any{}})
			}
		case <-deadline:
			completeTurn("completed")
		}
	}
}

// ---- fake pi rpc ----------------------------------------------------------------------------

func fakePiMain() {
	rec := fakeRecorder()
	fakeSpawnRecord(rec)
	args := os.Args[1:]
	session := argValueOf(args, "--session")
	model := argValueOf(args, "--model")
	provider, bare, _ := strings.Cut(model, "/")
	thinking := envOr("SM_FAKE_PI_THINKING", argValueOf(args, "--thinking"))
	observer := false
	for i, arg := range args {
		if arg == "--extension" && i+1 < len(args) && strings.Contains(args[i+1], "observer") {
			observer = true
		}
	}
	appendSession := func(line string) {
		if session == "" {
			return
		}
		file, err := os.OpenFile(session, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
		file.WriteString(line + "\n")
		file.Close()
	}
	appendSession(`{"type":"session","id":"pi-native"}`)
	emitJSON(map[string]any{"type": "session", "id": "pi-native"})
	lines := stdinLines()
	var prompt string
	var steers []string
	active := false
	var deadline <-chan time.Time
	settle := func() {
		if name := promptDirective(prompt, "WRITE="); name != "" {
			cwd, _ := os.Getwd()
			os.WriteFile(filepath.Join(cwd, name), []byte("agent output\n"), 0600)
		}
		if thinking != "" {
			appendSession(fmt.Sprintf(`{"type":"thinking_level_change","thinkingLevel":%q}`, thinking))
		}
		if observer && os.Getenv("SM_FAKE_PI_EFFORT") != "skip" {
			effort := os.Getenv("SM_FAKE_PI_EFFORT")
			data := `null`
			if effort != "" && effort != "null" {
				data = fmt.Sprintf("%q", effort)
			}
			appendSession(fmt.Sprintf(`{"type":"custom","customType":%q,"data":{"reasoning_effort":%s}}`, piObserverEntry, data))
		}
		emitProvider, emitModel := provider, bare
		if strings.Contains(prompt, "BADMODEL") {
			emitProvider = "other"
		}
		message := map[string]any{"role": "assistant", "provider": emitProvider, "model": emitModel, "usage": map[string]any{"input": 1}}
		text := "ECHO:" + prompt
		if len(steers) > 0 {
			text += "+STEER:" + strings.Join(steers, "|")
		} else if strings.Contains(prompt, "WAIT") {
			text += "+none"
		}
		if strings.Contains(prompt, "FAIL") {
			message["stopReason"] = "error"
			message["errorMessage"] = envOr("SM_FAKE_FAILMSG", "pi failure")
		} else {
			message["stopReason"] = "stop"
			message["content"] = []map[string]any{{"type": "text", "text": text}}
		}
		emitJSON(map[string]any{"type": "message_end", "message": message})
		emitJSON(map[string]any{"type": "agent_settled", "aborted": false})
		active, steers, deadline = false, nil, nil
	}
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				os.Exit(0)
			}
			rec(fakeRecord{Kind: "command", Line: json.RawMessage(line)})
			var command struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}
			if json.Unmarshal([]byte(line), &command) != nil {
				continue
			}
			switch command.Type {
			case "prompt":
				prompt, steers, active = command.Message, nil, true
				emitJSON(map[string]any{"type": "response", "command": "prompt", "success": true, "data": map[string]any{"disposition": "started"}})
				switch {
				case strings.Contains(prompt, "HOLD"):
				case strings.Contains(prompt, "WAIT"):
					deadline = time.After(1500 * time.Millisecond)
				default:
					settle()
				}
			case "steer":
				if message := os.Getenv("SM_FAKE_STEER_ERROR"); message != "" {
					emitJSON(map[string]any{"type": "response", "command": "steer", "success": false, "error": message})
					continue
				}
				emitJSON(map[string]any{"type": "response", "command": "steer", "success": true, "data": map[string]any{"disposition": "queued"}})
				if active {
					steers = append(steers, command.Message)
					if strings.Contains(prompt, "WAIT") {
						settle()
					}
				}
			default:
				emitJSON(map[string]any{"type": "response", "command": command.Type, "success": true, "data": map[string]any{}})
			}
		case <-deadline:
			settle()
		}
	}
}

// ---- helpers shared by fakes and tests --------------------------------------------------------

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// promptDirective extracts the word after a directive like WRITE= from the prompt.
func promptDirective(prompt, directive string) string {
	_, after, ok := strings.Cut(prompt, directive)
	if !ok {
		return ""
	}
	fields := strings.Fields(after)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// installFakeNative puts a PATH stub for the given agent binary that re-execs this test binary
// as the fake, with extra env baked in. Returns the record file path.
func installFakeNative(t *testing.T, agent string, extraEnv map[string]string) string {
	t.Helper()
	bin := t.TempDir()
	record := filepath.Join(t.TempDir(), agent+"-record.jsonl")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := fmt.Sprintf("SM_FAKE_NATIVE=%s SM_FAKE_RECORD=%q", agent, record)
	for key, value := range extraEnv {
		env += fmt.Sprintf(" %s=%q", key, value)
	}
	script := fmt.Sprintf("#!/bin/sh\n%s exec %q \"$@\"\n", env, self)
	if err := os.WriteFile(filepath.Join(bin, agent), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return record
}

func readFakeRecords(t *testing.T, path string) []fakeRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []fakeRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record fakeRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err, line)
		}
		records = append(records, record)
	}
	return records
}

// fakeRequest returns the params of the first recorded client request with the given method.
func fakeRequest(t *testing.T, records []fakeRecord, method string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record.Kind != "request" && record.Kind != "command" {
			continue
		}
		var request struct {
			Method string         `json:"method"`
			Type   string         `json:"type"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(record.Line, &request) != nil {
			continue
		}
		if request.Method == method || request.Type == method {
			if request.Params != nil {
				return request.Params
			}
			var raw map[string]any
			json.Unmarshal(record.Line, &raw)
			return raw
		}
	}
	t.Fatalf("no recorded %s request", method)
	return nil
}

func countFakeRequests(t *testing.T, records []fakeRecord, method string) int {
	t.Helper()
	count := 0
	for _, record := range records {
		if record.Kind != "request" && record.Kind != "command" {
			continue
		}
		var request struct {
			Method string `json:"method"`
			Type   string `json:"type"`
		}
		if json.Unmarshal(record.Line, &request) != nil {
			continue
		}
		if request.Method == method || request.Type == method {
			count++
		}
	}
	return count
}
