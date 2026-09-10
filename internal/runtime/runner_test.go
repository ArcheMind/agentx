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
	}, ExecuteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "stdout" || result.Stderr != "stderr" {
		t.Fatalf("result = %#v", result)
	}
	for _, expected := range []string{`"event":"external_call"`, `"output":"stdout"`, `"error_output":"stderr"`} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("log %q does not contain %q", logs.String(), expected)
		}
	}
}
