package app

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/inventory"
)

func fixtureInventory() inventory.Result {
	return inventory.Normalize([]inventory.Source{
		{Scope: "user", Path: "/user/private", Agents: []string{"codex"}, Status: "empty"},
		{Scope: "project", Path: "/project/private", Agents: []string{"claude"}, Status: "ok", Resources: []inventory.Resource{{Name: "gamma", Agents: []string{"claude"}}}},
		{Scope: "project", Path: "/project/common", Preferred: true, Agents: []string{"claude", "codex"}, Status: "ok", Resources: []inventory.Resource{{Name: "alpha", Agents: []string{"claude", "codex"}}, {Name: "beta", Agents: []string{"claude"}}}},
		{Scope: "user", Path: "/user/common", Preferred: true, Agents: []string{"codex"}, Status: "ok", Resources: []inventory.Resource{{Name: "delta", Agents: []string{"codex"}}}},
		{Scope: "user", Path: "/user/failed", Status: "failed", Error: "fixture denied"},
		{Scope: "user", Path: "unavailable discovery", Status: "unsupported"},
	})
}
func TestInventoryTextF01F02F05F06F07(t *testing.T) {
	var output bytes.Buffer
	a := New(false, strings.NewReader(""), &output, &bytes.Buffer{})
	a.Output = OutputText
	if err := a.writeInventory(fixtureInventory()); err != nil {
		t.Fatal(err)
	}
	plain := output.String()
	for _, want := range []string{"Project\n\n/project/common  claude · codex\n  alpha\n  beta (claude)\n\n/project/private", "User\n\n/user/common", "Read failed: fixture denied", "Discovery unsupported", "No resources"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in %q", want, plain)
		}
	}
	if strings.ContainsAny(plain, "├└│") || strings.Contains(plain, "\x1b") || strings.Contains(plain, "Shared") || strings.Contains(plain, "Native") {
		t.Fatalf("invalid presentation=%q", plain)
	}
	output.Reset()
	a.Color = true
	if err := a.writeInventory(fixtureInventory()); err != nil {
		t.Fatal(err)
	}
	rich := output.String()
	if !strings.Contains(rich, ansiBold+"/project/common"+ansiReset) || strings.Contains(rich, ansiBold+"/project/private") || !strings.Contains(rich, agentColor("claude")+"claude"+ansiReset) || !strings.Contains(rich, agentColor("codex")+"codex"+ansiReset) {
		t.Fatalf("styles=%q", rich)
	}
	t.Setenv("NO_COLOR", "")
	if colorEnabled(os.Stdout) {
		t.Fatal("empty NO_COLOR did not disable ANSI")
	}
}
func TestInventoryShowEvidenceF08F09(t *testing.T) {
	var output bytes.Buffer
	a := App{Stdout: &output}
	a.writeEvidence(inventory.Evidence{Configuration: "discovered", Connection: "connected", ConnectionEvidence: "probe", Session: "loaded"})
	if !strings.Contains(output.String(), "Current session: unconfirmed") {
		t.Fatal(output.String())
	}
	output.Reset()
	a.writeEvidence(inventory.Evidence{Configuration: "discovered", Connection: "connected", ConnectionEvidence: "probe", Session: "loaded", SessionID: "session-a", SessionEvidence: "session-a registration"})
	if !strings.Contains(output.String(), "Session ID: session-a") || !strings.Contains(output.String(), "Current session: loaded") {
		t.Fatal(output.String())
	}
}
