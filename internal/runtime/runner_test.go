package runtime

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestExecRunnerDebugLogContainsRawIO(t *testing.T) {
	var logs bytes.Buffer
	runner := ExecRunner{Debug: true, Log: &logs}
	result, err := runner.Execute(context.Background(), CommandPlan{
		Executable: "sh", Args: []string{"-c", "printf stdout; printf stderr >&2"},
	}, ExecuteOptions{Stdin: strings.NewReader("input")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "stdout" || result.Stderr != "stderr" {
		t.Fatalf("result = %#v", result)
	}
	for _, expected := range []string{`"event":"external_call"`, `"stdin":"input"`, `"output":"stdout"`, `"error_output":"stderr"`} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("log %q does not contain %q", logs.String(), expected)
		}
	}
}

func TestExecRunnerStreamsAndCapturesOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result, err := (ExecRunner{}).Execute(context.Background(), CommandPlan{
		Executable: "sh", Args: []string{"-c", "cat; printf error >&2"},
	}, ExecuteOptions{Stdin: strings.NewReader("prompt"), Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "prompt" || result.Stdout != stdout.String() || stderr.String() != "error" || result.Stderr != stderr.String() {
		t.Fatalf("streamed output differs from captured output: %#v", result)
	}
}
