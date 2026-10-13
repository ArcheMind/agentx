package subagents

import (
	"github.com/ArcheMind/agentx/internal/drivers"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexPlanKeepsNativeSessionAndReadOnlyPolicy(t *testing.T) {
	id := "0199a213-81c0-7800-8aa1-bbab2a035a53"
	plan, err := drivers.CodexExecPlan("/project", "test-model", id)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable != "codex" || plan.Cwd != "/project" || strings.Join(plan.Args, " ") != `exec resume --json -c sandbox_mode="read-only" -c approval_policy="never" --model test-model `+id+` -` {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if _, err := drivers.CodexExecPlan("/project", "", "--last"); err == nil {
		t.Fatal("accepted a flag instead of a session UUID")
	}
}

func TestProjectionIsImmutableAndDryRunDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	root, err := Prepare(home, "/bin/ax", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote files: %v", err)
	}
	actual, err := Prepare(home, "/bin/ax", true)
	if err != nil || actual != root {
		t.Fatalf("prepare: %q %v", actual, err)
	}
	if _, err := Prepare(home, "/bin/ax", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bridge.json"), []byte("user content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(home, "/bin/ax", true); err == nil {
		t.Fatal("overwrote an altered projection")
	}
}
