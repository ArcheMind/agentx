package agentparams

import (
	"reflect"
	"strings"
	"testing"
)

const testUUID = "11111111-2222-4333-8444-555555555555"

func TestResolveUsesExplicitOverridesWithoutInventingDefaults(t *testing.T) {
	falseValue := false
	base := Request{Model: "base", Effort: "low", Mode: ModeNormal, Flags: []string{"--verbose"}, UserConfig: &falseValue, MCP: MCPConfig{Servers: map[string]MCPServer{"one": {Command: "node"}}}}
	override := Request{Model: "override", Flags: []string{}, MCP: MCPConfig{Servers: map[string]MCPServer{}}}
	got, err := Resolve("claude", base, override)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "override" || got.Effort != "low" || got.Mode != ModeNormal || got.UserConfig == nil || *got.UserConfig || got.Flags == nil || len(got.Flags) != 0 || got.MCP.Servers == nil || len(got.MCP.Servers) != 0 {
		t.Fatalf("resolved %#v", got)
	}
	empty, err := Resolve("codex", Request{}, Request{})
	if err != nil || empty.Mode != "" {
		t.Fatalf("defaults injected: %#v %v", empty, err)
	}
}

func TestTransportBehavior(t *testing.T) {
	cases := []struct {
		agent string
		t     Transport
		r     Request
		want  []string
	}{
		{"claude", Interactive, Request{Model: "opus", Mode: ModeYolo, Session: testUUID}, []string{"--resume", testUUID, "--dangerously-skip-permissions", "--model", "opus"}},
		{"claude", Stream, Request{Model: "opus", Mode: ModeNormal}, []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--model", "opus"}},
		{"codex", Interactive, Request{Model: "gpt-6", Mode: ModeYolo, Session: testUUID}, []string{"resume", testUUID, "--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-6"}},
		{"codex", OneShot, Request{Mode: ModeNormal}, []string{"exec", "--json", "--skip-git-repo-check", "-c", `approval_policy="never"`, "-"}},
		{"pi", OneShot, Request{Model: "native", Effort: "low", Mode: ModeYolo}, []string{"--print", "--mode", "json", "--no-approve", "--thinking", "low", "--model", "native"}},
		{"pi", Exec, Request{Model: "native"}, []string{"--print", "--mode", "json", "--model", "native"}},
		{"agy", Exec, Request{Model: "gemini", Session: testUUID}, []string{"--input-format", "stream-json", "--output-format", "stream-json", "--conversation", testUUID, "--model", "gemini"}},
		{"opencode", Interactive, Request{Session: "token"}, []string{"--session", "token"}},
	}
	for _, tc := range cases {
		t.Run(tc.agent+"/"+string(tc.t), func(t *testing.T) {
			got, err := Build(tc.agent, tc.t, tc.r)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Args, tc.want) {
				t.Fatalf("args %q, want %q", got.Args, tc.want)
			}
		})
	}
}

func TestStructuredTransportsAndMCP(t *testing.T) {
	mcp := MCPConfig{Servers: map[string]MCPServer{"bridge": {Command: "node", Args: []string{"bridge.mjs"}, Env: map[string]string{"A": "1"}}}}
	agy, err := Build("agy", ACP, Request{Cwd: "/ws", Model: "gemini", Effort: "high", Mode: ModeYolo, Session: "native", SystemPrompt: "role", MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(agy.Args, []string{"acp", "--stdio", "--dangerously-skip-permissions"}) || agy.Params["sessionId"] != "native" || len(agy.Settings) != 2 || agy.Settings[0].Params["modelId"] != "gemini" || agy.Settings[1].Params["value"] != "high" || agy.PromptPrefix != "role" {
		t.Fatalf("ACP %#v", agy)
	}
	codex, err := Build("codex", AppServer, Request{Cwd: "/ws", Model: "gpt-6", Effort: "high", Mode: ModeYolo, Session: testUUID, SystemPrompt: "role", MCP: mcp, Flags: []string{"--verbose"}})
	if err != nil {
		t.Fatal(err)
	}
	config := codex.Params["config"].(map[string]any)
	if !reflect.DeepEqual(codex.Args, []string{"app-server", "--verbose"}) || codex.Params["threadId"] != testUUID || codex.Params["excludeTurns"] != true || codex.Params["approvalPolicy"] != "never" || codex.Params["sandbox"] != "danger-full-access" || codex.Params["developerInstructions"] != "role" || config["model_reasoning_effort"] != "high" || config["mcp_servers"] == nil {
		t.Fatalf("AppServer %#v", codex)
	}
	claudeInteractive, err := Build("claude", Interactive, Request{MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	claudeOneShot, err := Build("claude", OneShot, Request{MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(claudeInteractive.Args, " "), "--strict-mcp-config") || !strings.Contains(strings.Join(claudeOneShot.Args, " "), "--strict-mcp-config") {
		t.Fatal("MCP strictness differs by transport")
	}
}

func TestRejectsUnsupportedAndConflictingParameters(t *testing.T) {
	cases := []struct {
		agent string
		t     Transport
		r     Request
	}{
		{"codex", Interactive, Request{UserConfig: boolPtr(false)}},
		{"pi", OneShot, Request{MCP: MCPConfig{Servers: map[string]MCPServer{"x": {Command: "node"}}}}},
		{"agy", OneShot, Request{SystemPrompt: "role"}},
		{"opencode", Interactive, Request{Mode: ModeYolo}},
		{"dsh", Interactive, Request{Model: "model"}},
		{"claude", Interactive, Request{Model: "opus", Flags: []string{"--model=sonnet"}}},
		{"codex", Interactive, Request{Effort: "high", Flags: []string{"--config=model_reasoning_effort=low"}}},
		{"claude", Interactive, Request{Session: "bad"}},
		{"claude", Interactive, Request{Effort: "ultra"}},
	}
	for _, tc := range cases {
		t.Run(tc.agent+"/"+string(tc.t)+"/"+tc.r.Model+tc.r.Effort, func(t *testing.T) {
			if _, err := Build(tc.agent, tc.t, tc.r); err == nil {
				t.Fatalf("accepted %#v", tc.r)
			}
		})
	}
}

func TestFlagsTreatValuesAsValuesAndHonorTerminator(t *testing.T) {
	cases := [][]string{
		{"--prompt", "--model", "opus"},
		{"--", "--model", "opus"},
		{"--config", "mcp_servers.other.command='node'"},
	}
	for _, flags := range cases {
		if _, err := Build("codex", Interactive, Request{Model: "gpt-6", MCP: MCPConfig{Servers: map[string]MCPServer{"mine": {Command: "node"}}}, Flags: flags}); err != nil {
			t.Fatalf("flags %q: %v", flags, err)
		}
	}
	conflicts := []struct {
		r     Request
		flags []string
	}{
		{Request{Model: "gpt-6"}, []string{"-c", ` "model" = "other"`}},
		{Request{Mode: ModeNormal}, []string{"--config=approval_policy='never'"}},
		{Request{Mode: ModeYolo}, []string{"--sandbox", "read-only"}},
		{Request{MCP: MCPConfig{Servers: map[string]MCPServer{"mine": {Command: "node"}}}}, []string{"-c", `mcp_servers.mine.command='other'`}},
	}
	for _, tc := range conflicts {
		tc.r.Flags = tc.flags
		if _, err := Build("codex", Interactive, tc.r); err == nil {
			t.Fatalf("flags %q should conflict", tc.flags)
		}
	}
}

func TestPiProjectFileTrustFlagDoesNotConflictWithMode(t *testing.T) {
	got, err := Build("pi", Interactive, Request{Mode: ModeYolo, Flags: []string{"--no-approve"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Args, []string{"--no-approve"}) {
		t.Fatal(got.Args)
	}
}
func boolPtr(v bool) *bool { return &v }

func TestOneShotClaudePreservesNativeResumeAndMCP(t *testing.T) {
	mcp := MCPConfig{Servers: map[string]MCPServer{"harness": {Command: "/usr/bin/node", Args: []string{"bridge.mjs"}, Env: map[string]string{"KERNEL_URL": "http://localhost:1"}}}}
	got, err := Build("claude", OneShot, Request{Cwd: "/ws", Model: "opus", Session: testUUID, Effort: "high", SystemPrompt: "stay focused", Mode: ModeYolo, MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(got.Args, " ")
	for _, want := range []string{"-p --output-format stream-json --verbose", "--resume " + testUUID, "--dangerously-skip-permissions", "--effort high", "--append-system-prompt stay focused", "--strict-mcp-config", "--model opus"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q missing %q", args, want)
		}
	}
	config := got.Args[findArg(t, got.Args, "--mcp-config")+1]
	for _, want := range []string{`"command":"/usr/bin/node"`, `"args":["bridge.mjs"]`, `"env":{"KERNEL_URL":"http://localhost:1"}`, `"type":"stdio"`} {
		if !strings.Contains(config, want) {
			t.Fatalf("MCP config %q missing %q", config, want)
		}
	}
	plain, err := Build("claude", OneShot, Request{Mode: ModeNormal})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plain.Args, []string{"-p", "--output-format", "stream-json", "--verbose"}) {
		t.Fatal(plain.Args)
	}
	if _, err := Build("claude", OneShot, Request{Session: "bad"}); err == nil {
		t.Fatal("invalid UUID accepted")
	}
}

func TestOneShotCodexTranslatesNativeMCPAndInstructions(t *testing.T) {
	mcp := MCPConfig{Servers: map[string]MCPServer{"harness": {Command: "/usr/bin/node", Args: []string{"bridge.mjs"}, Env: map[string]string{"A": "1"}, EnvPassthrough: []string{"KERNEL_URL", "PATH"}}}}
	got, err := Build("codex", OneShot, Request{Cwd: "/ws", Model: "gpt-6", Session: testUUID, Effort: "low", SystemPrompt: "be terse", Mode: ModeYolo, MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(got.Args, " ")
	for _, want := range []string{"exec resume --json --skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", `model_reasoning_effort="low"`, `developer_instructions="be terse"`, `mcp_servers.harness.command="/usr/bin/node"`, `mcp_servers.harness.args=["bridge.mjs"]`, `mcp_servers.harness.env.A="1"`, `mcp_servers.harness.env_vars=["KERNEL_URL","PATH"]`, "--model gpt-6"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q missing %q", args, want)
		}
	}
	if got.Args[len(got.Args)-1] != "-" || got.Args[len(got.Args)-2] != testUUID {
		t.Fatalf("resume marker %q", got.Args)
	}
	normal, err := Build("codex", OneShot, Request{Mode: ModeNormal})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(normal.Args, " "), "sandbox_mode") || !strings.Contains(strings.Join(normal.Args, " "), `approval_policy="never"`) {
		t.Fatal(normal.Args)
	}
	if _, err := Build("codex", OneShot, Request{Session: "bad"}); err == nil {
		t.Fatal("invalid UUID accepted")
	}
}

func TestOneShotPiAndAgyKeepNativeSessionRules(t *testing.T) {
	pi, err := Build("pi", OneShot, Request{Model: "anthropic/claude-x", Session: "/state/abc.jsonl", Effort: "medium", SystemPrompt: "persona", Mode: ModeYolo})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(pi.Args, " ")
	for _, want := range []string{"--print --mode json --no-approve", "--session /state/abc.jsonl", "--thinking medium", "--append-system-prompt persona", "--model anthropic/claude-x"} {
		if !strings.Contains(args, want) {
			t.Fatalf("Pi %q missing %q", args, want)
		}
	}
	if strings.Contains(args, "--tools") {
		t.Fatal("Pi tools were restricted", args)
	}
	agy, err := Build("agy", OneShot, Request{Model: "native-model", Session: testUUID, Effort: "high", Mode: ModeYolo})
	if err != nil {
		t.Fatal(err)
	}
	args = strings.Join(agy.Args, " ")
	for _, want := range []string{"--input-format stream-json --output-format stream-json", "--conversation " + testUUID, "--effort high", "--model native-model", "--dangerously-skip-permissions"} {
		if !strings.Contains(args, want) {
			t.Fatalf("agy %q missing %q", args, want)
		}
	}
	if _, err := Build("agy", OneShot, Request{MCP: MCPConfig{Servers: map[string]MCPServer{"x": {Command: "tool"}}}}); err == nil {
		t.Fatal("temporary MCP accepted")
	}
	if _, err := Build("agy", OneShot, Request{Session: "bad"}); err == nil {
		t.Fatal("invalid UUID accepted")
	}
	for _, agent := range []string{"opencode", "dsh"} {
		if _, err := Build(agent, OneShot, Request{}); err == nil {
			t.Fatalf("%s OneShot accepted", agent)
		}
	}
}

func TestAgyACPMCPInheritsExplicitEnvironment(t *testing.T) {
	t.Setenv("AGY_TEST_PASSTHROUGH", "inherited")
	servers := AgyMCPServers(MCPConfig{Servers: map[string]MCPServer{"tool": {Command: "native", EnvPassthrough: []string{"AGY_TEST_PASSTHROUGH"}, Env: map[string]string{"EXPLICIT": "value"}}}})
	if len(servers) != 1 || len(servers[0].Env) != 2 || servers[0].Env[0].Name != "AGY_TEST_PASSTHROUGH" || servers[0].Env[0].Value != "inherited" || servers[0].Env[1].Name != "EXPLICIT" || servers[0].Env[1].Value != "value" {
		t.Fatal(servers)
	}
}

func findArg(t *testing.T, args []string, want string) int {
	t.Helper()
	for i, arg := range args {
		if arg == want {
			return i
		}
	}
	t.Fatalf("args %q missing %q", args, want)
	return -1
}

func TestCodexOmittedParametersKeepNativeDefaults(t *testing.T) {
	plan, err := Build("codex", AppServer, Request{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"model", "cwd", "approvalPolicy", "sandbox", "config", "developerInstructions", "threadId"} {
		if _, present := plan.Params[key]; present {
			t.Fatalf("unspecified %s overrides a native default: %#v", key, plan.Params)
		}
	}
}

func TestOpenCodeResumeRequiresNativeSessionIdentity(t *testing.T) {
	plan, err := Build("opencode", Interactive, Request{Session: "ses_native"})
	if err != nil || !reflect.DeepEqual(plan.Args, []string{"--session", "ses_native"}) {
		t.Fatalf("resume = %#v, %v", plan, err)
	}
	if _, err := Build("opencode", Interactive, Request{Session: "/tmp/imported-session"}); err == nil {
		t.Fatal("accepted imported directory as a native session ID")
	}
	if _, err := Build("opencode", Interactive, Request{Session: "ses_native", Flags: []string{"-c"}}); err == nil {
		t.Fatal("continue flag overrides the selected session")
	}
}
