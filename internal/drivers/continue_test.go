package drivers

import (
	"reflect"
	"strings"
	"testing"
)

const continueUUID = "11111111-2222-4333-8444-555555555555"

func TestContinueClaudePlanCarriesResumeAccessMCPAndModel(t *testing.T) {
	mcp := MCPConfig{Servers: map[string]MCPServer{
		"harness": {Command: "/usr/bin/node", Args: []string{"bridge.mjs"}, Env: map[string]string{"KERNEL_URL": "http://localhost:1"}},
	}}
	plan, err := ContinueExecPlan("claude", "/ws", "opus", continueUUID, "high", "stay focused", true, true, mcp)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable != "claude" || plan.Cwd != "/ws" {
		t.Fatalf("plan = %+v", plan)
	}
	joined := strings.Join(plan.Args, " ")
	for _, expect := range []string{
		"-p --output-format stream-json --verbose",
		"--resume " + continueUUID,
		"--dangerously-skip-permissions",
		"--effort high",
		"--append-system-prompt stay focused",
		"--strict-mcp-config",
		"--model opus",
	} {
		if !strings.Contains(joined, expect) {
			t.Fatalf("args %q missing %q", joined, expect)
		}
	}
	config := plan.Args[indexOf(t, plan.Args, "--mcp-config")+1]
	for _, expect := range []string{`"command":"/usr/bin/node"`, `"args":["bridge.mjs"]`, `"env":{"KERNEL_URL":"http://localhost:1"}`, `"type":"stdio"`} {
		if !strings.Contains(config, expect) {
			t.Fatalf("mcp config %q missing %q", config, expect)
		}
	}
}

func TestContinueClaudeWithoutResumeOrAccessStaysPlain(t *testing.T) {
	plan, err := ContinueExecPlan("claude", "/ws", "", "", "", "", false, true, MCPConfig{})
	if err != nil {
		t.Fatal(err)
	}
	expect := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if !reflect.DeepEqual(plan.Args, expect) {
		t.Fatalf("args = %q", plan.Args)
	}
}

func TestContinueClaudeRejectsNonUUIDResume(t *testing.T) {
	if _, err := ContinueExecPlan("claude", "/ws", "", "not-a-uuid", "", "", false, true, MCPConfig{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestContinueCodexPlanTranslatesMCPAndSandbox(t *testing.T) {
	mcp := MCPConfig{Servers: map[string]MCPServer{
		"harness": {Command: "/usr/bin/node", Args: []string{"bridge.mjs"}, Env: map[string]string{"A": "1"}, EnvPassthrough: []string{"KERNEL_URL", "PATH"}},
	}}
	plan, err := ContinueExecPlan("codex", "/ws", "gpt-6", continueUUID, "low", "be terse", true, true, mcp)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Args, " ")
	for _, expect := range []string{
		"exec resume --json --skip-git-repo-check",
		`sandbox_mode="danger-full-access"`,
		`approval_policy="never"`,
		`model_reasoning_effort="low"`,
		`developer_instructions="be terse"`,
		`mcp_servers.harness.command="/usr/bin/node"`,
		`mcp_servers.harness.args=["bridge.mjs"]`,
		`mcp_servers.harness.env.A="1"`,
		`mcp_servers.harness.env_vars=["KERNEL_URL","PATH"]`,
		"--model gpt-6",
	} {
		if !strings.Contains(joined, expect) {
			t.Fatalf("args %q missing %q", joined, expect)
		}
	}
	if plan.Args[len(plan.Args)-1] != "-" || plan.Args[len(plan.Args)-2] != continueUUID {
		t.Fatalf("args should end with resume id then stdin marker: %q", plan.Args)
	}
}

func TestContinueCodexNormalModePassesNoSandboxFlags(t *testing.T) {
	plan, err := ContinueExecPlan("codex", "/ws", "", "", "", "", false, true, MCPConfig{})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Args, " ")
	if strings.Contains(joined, "sandbox_mode") {
		t.Fatalf("normal mode must leave sandboxing to the user's configuration: %q", joined)
	}
	if !strings.Contains(joined, `approval_policy="never"`) {
		t.Fatalf("headless codex always needs approval_policy never: %q", joined)
	}
}

func TestContinuePiPlanUsesSessionFileAndThinking(t *testing.T) {
	plan, err := ContinueExecPlan("pi", "/ws", "anthropic/claude-x", "/state/abc.jsonl", "medium", "persona", false, true, MCPConfig{})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Args, " ")
	for _, expect := range []string{
		"--print --mode json --no-approve",
		"--session /state/abc.jsonl",
		"--thinking medium",
		"--append-system-prompt persona",
		"--model anthropic/claude-x",
	} {
		if !strings.Contains(joined, expect) {
			t.Fatalf("args %q missing %q", joined, expect)
		}
	}
	if strings.Contains(joined, "--tools") {
		t.Fatalf("continue must not restrict pi tools: %q", joined)
	}
}

func TestContinuePiRejectsYolo(t *testing.T) {
	if _, err := ContinueExecPlan("pi", "/ws", "anthropic/claude-x", "", "", "", true, true, MCPConfig{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestContinuePiRejectsMCP(t *testing.T) {
	mcp := MCPConfig{Servers: map[string]MCPServer{"harness": {Command: "node"}}}
	if _, err := ContinueExecPlan("pi", "/ws", "anthropic/claude-x", "", "", "", false, true, mcp); err == nil {
		t.Fatal("expected error")
	}
}

func TestContinueRejectsAgentsWithoutAdapter(t *testing.T) {
	for _, agent := range []string{"gemini", "opencode", "dsh"} {
		if _, err := ContinueExecPlan(agent, "/ws", "", "", "", "", false, true, MCPConfig{}); err == nil {
			t.Fatalf("%s: expected error", agent)
		}
	}
}

func indexOf(t *testing.T, args []string, value string) int {
	t.Helper()
	for i, arg := range args {
		if arg == value {
			return i
		}
	}
	t.Fatalf("args %q missing %q", args, value)
	return -1
}

func TestContinueUserConfigFalseAddsIsolationArgs(t *testing.T) {
	for agent, want := range map[string]string{
		"claude": "--setting-sources project,local",
		"codex":  "--ignore-user-config --ignore-rules --disable hooks",
		"pi":     "--no-skills --no-prompt-templates --no-extensions",
	} {
		on, err := ContinueExecPlan(agent, "/ws", "m", "", "", "", false, true, MCPConfig{})
		if err != nil {
			t.Fatal(err)
		}
		off, err := ContinueExecPlan(agent, "/ws", "m", "", "", "", false, false, MCPConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(on.Args, " "), want) || !strings.Contains(strings.Join(off.Args, " "), want) {
			t.Fatalf("%s: on=%q off=%q", agent, on.Args, off.Args)
		}
	}
}
