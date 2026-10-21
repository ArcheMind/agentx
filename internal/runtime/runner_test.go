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

func TestExecRunnerUnsetsEnvAndRedactsCredentialsInDebugLog(t *testing.T) {
	t.Setenv("AGENTX_REDACT_TEST_TOKEN", "inherited-secret")
	var logs bytes.Buffer
	runner := ExecRunner{Debug: true, Log: &logs}
	result, err := runner.Execute(context.Background(), CommandPlan{
		Executable: "sh", Args: []string{"-c", `printf "${AGENTX_REDACT_TEST_TOKEN:-unset} sk-proj-abcdefghijklmnopqrstuvwxyz"; printf '{"access_token":"tok-value"}' >&2`},
		Env:      map[string]string{"AGENTX_OTHER_API_KEY": "configured-secret", "CODEX_HOME": "/visible/home"},
		UnsetEnv: []string{"AGENTX_REDACT_TEST_TOKEN"},
	}, ExecuteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Stdout, "unset ") {
		t.Fatalf("inherited variable reached child: %q", result.Stdout)
	}
	for _, leaked := range []string{"configured-secret", "abcdefghijklmnop", "tok-value"} {
		if strings.Contains(logs.String(), leaked) {
			t.Fatalf("debug log leaked %q: %s", leaked, logs.String())
		}
	}
	for _, kept := range []string{"/visible/home", "AGENTX_REDACT_TEST_TOKEN", `access_token\":\"\u003credacted\u003e`} {
		if !strings.Contains(logs.String(), kept) {
			t.Fatalf("debug log missing %q: %s", kept, logs.String())
		}
	}
}
