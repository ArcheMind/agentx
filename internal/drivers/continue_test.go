package drivers

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/agentparams"
)

func TestPlanConvertsNativeTransport(t *testing.T) {
	cases := []struct {
		agent     string
		transport agentparams.Transport
		request   agentparams.Request
		prefix    []string
	}{
		{"claude", agentparams.OneShot, agentparams.Request{Cwd: "/ws", Model: "opus", Mode: agentparams.ModeYolo}, []string{"-p", "--output-format", "stream-json", "--verbose"}},
		{"codex", agentparams.OneShot, agentparams.Request{Cwd: "/ws", Model: "gpt-6", Mode: agentparams.ModeNormal}, []string{"exec", "--json", "--skip-git-repo-check"}},
		{"agy", agentparams.OneShot, agentparams.Request{Cwd: "/ws", Model: "gemini"}, []string{"--input-format", "stream-json", "--output-format", "stream-json"}},
		{"pi", agentparams.OneShot, agentparams.Request{Cwd: "/ws", Model: "native"}, []string{"--print", "--mode", "json", "--no-approve"}},
	}
	for _, tc := range cases {
		t.Run(tc.agent+"/"+string(tc.transport), func(t *testing.T) {
			got, err := Plan(tc.agent, tc.transport, tc.request)
			if err != nil {
				t.Fatal(err)
			}
			if got.Executable != tc.agent || got.Cwd != tc.request.Cwd || len(got.Args) < len(tc.prefix) || !reflect.DeepEqual(got.Args[:len(tc.prefix)], tc.prefix) {
				t.Fatalf("plan = %#v", got)
			}
		})
	}
}

func TestPlanRejectsUnsupportedTransportAndMCP(t *testing.T) {
	if _, err := Plan("opencode", agentparams.OneShot, agentparams.Request{}); err == nil {
		t.Fatal("expected unsupported transport")
	}
	if _, err := Plan("pi", agentparams.OneShot, agentparams.Request{MCP: agentparams.MCPConfig{Servers: map[string]agentparams.MCPServer{"tool": {Command: "node"}}}}); err == nil {
		t.Fatal("expected unsupported MCP")
	}
}

func TestPlanCarriesFlagsWithoutOverridingExplicitModel(t *testing.T) {
	got, err := Plan("claude", agentparams.Interactive, agentparams.Request{Model: "opus", Flags: []string{"--verbose"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Args, []string{"--model", "opus", "--verbose"}) {
		t.Fatal(got.Args)
	}
	_, err = Plan("claude", agentparams.Interactive, agentparams.Request{Model: "opus", Flags: []string{"--model", "sonnet"}})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestCommandPlanDoesNotDiscardProtocolParameters(t *testing.T) {
	for _, item := range []struct {
		agent     string
		transport agentparams.Transport
	}{
		{"codex", agentparams.AppServer}, {"agy", agentparams.ACP},
	} {
		if _, err := Plan(item.agent, item.transport, agentparams.Request{Cwd: "/ws", Model: "native"}); err == nil {
			t.Fatal("discarded session parameters")
		}
	}
}
