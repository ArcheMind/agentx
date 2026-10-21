package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestJSONRPCHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_JSONRPC_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}
		if len(request.ID) == 0 {
			continue
		}
		fmt.Printf(`{"id":%s,"result":{"seen":true}}`, request.ID)
		fmt.Println()
		if string(request.ID) == "2" {
			break
		}
	}
}

func TestClaudeControlHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CLAUDE_CONTROL_HELPER") != "1" {
		return
	}
	defer os.Exit(0)
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	if os.Getenv("GO_WANT_CLAUDE_CONTROL_WAIT") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	if os.Getenv("GO_WANT_CLAUDE_CONTROL_INVALID") == "1" {
		fmt.Println(`{"type":`)
		return
	}
	if os.Getenv("GO_WANT_CLAUDE_CONTROL_EOF") == "1" {
		fmt.Println(`{"type":"control_response","response":{"request_id":"other"}}`)
		return
	}
	fmt.Println(`{"type":"control_response","response":{"request_id":"other","subtype":"success"}}`)
	fmt.Println(`{"type":"control_response","response":{"request_id":"agentx-models","subtype":"success"}}`)
}

func TestExecRunnerClaudeControlWaitsForMatchingRequestID(t *testing.T) {
	runner := ExecRunner{}
	response, err := runner.ExecuteClaudeControl(context.Background(), CommandPlan{
		Executable: os.Args[0],
		Args:       []string{"-test.run=^TestClaudeControlHelperProcess$"},
		Env:        map[string]string{"GO_WANT_CLAUDE_CONTROL_HELPER": "1"},
	}, json.RawMessage(`{"type":"control_request"}`), "agentx-models")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response), `"request_id":"agentx-models"`) {
		t.Fatalf("response = %s", response)
	}
}

func TestExecRunnerClaudeControlReturnsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := (ExecRunner{}).ExecuteClaudeControl(ctx, CommandPlan{
		Executable: os.Args[0],
		Args:       []string{"-test.run=^TestClaudeControlHelperProcess$"},
		Env:        map[string]string{"GO_WANT_CLAUDE_CONTROL_HELPER": "1", "GO_WANT_CLAUDE_CONTROL_WAIT": "1"},
	}, json.RawMessage(`{"type":"control_request"}`), "missing")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestExecRunnerClaudeControlRejectsMalformedJSONAndEarlyEOF(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
		want string
	}{
		{name: "malformed JSON", mode: "GO_WANT_CLAUDE_CONTROL_INVALID", want: "parse external command"},
		{name: "early EOF", mode: "GO_WANT_CLAUDE_CONTROL_EOF", want: "closed before Claude control response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (ExecRunner{}).ExecuteClaudeControl(context.Background(), CommandPlan{
				Executable: os.Args[0],
				Args:       []string{"-test.run=^TestClaudeControlHelperProcess$"},
				Env:        map[string]string{"GO_WANT_CLAUDE_CONTROL_HELPER": "1", tc.mode: "1"},
			}, json.RawMessage(`{"type":"control_request"}`), "missing")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestExecRunnerJSONRPCWaitsForTargetResponse(t *testing.T) {
	var logs bytes.Buffer
	runner := ExecRunner{Debug: true, Log: &logs}
	requests := []json.RawMessage{
		json.RawMessage(`{"id":1,"method":"initialize"}`),
		json.RawMessage(`{"method":"initialized"}`),
		json.RawMessage(`{"id":2,"method":"model/list"}`),
	}
	response, err := runner.ExecuteJSONRPC(context.Background(), CommandPlan{
		Executable: os.Args[0],
		Args:       []string{"-test.run=^TestJSONRPCHelperProcess$"},
		Env:        map[string]string{"GO_WANT_JSONRPC_HELPER": "1"},
	}, requests, json.RawMessage(`2`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response), `"id":2`) || !strings.Contains(string(response), `"seen":true`) {
		t.Fatalf("response = %s", response)
	}
	for _, expected := range []string{`\"method\":\"model/list\"`, `\"id\":2`, `"event":"external_call"`} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("debug log %q does not contain %q", logs.String(), expected)
		}
	}
}
