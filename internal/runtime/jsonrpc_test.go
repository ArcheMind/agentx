package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
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
