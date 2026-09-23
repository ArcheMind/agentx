package hooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/hooks"
)

func TestListPortableHooksFromUserAndProjectScopes(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfig(t, filepath.Join(home, ".agents", "hooks.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"user-start"}]}]}}`)
	writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"project-before","timeout":5}]}]}}`)

	result, err := (hooks.Service{Home: home, Project: project}).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hooks) != 2 || len(result.Warnings) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.Hooks[0].Scope != hooks.ScopeUser || result.Hooks[0].Event != "SessionStart" || result.Hooks[0].Command != "user-start" {
		t.Fatalf("user hook = %#v", result.Hooks[0])
	}
	if result.Hooks[1].Scope != hooks.ScopeProject || result.Hooks[1].Event != "UserPromptSubmit" || result.Hooks[1].Command != "project-before" {
		t.Fatalf("project hook = %#v", result.Hooks[1])
	}
	for _, item := range result.Hooks {
		if !item.Portable {
			t.Fatalf("hook is not portable: %#v", item)
		}
	}
}

func TestListWarnsAndSkipsOnlyNonPortableItems(t *testing.T) {
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), `{
  "hooks": {
    "SessionStart": [{"hooks": [
      {"type":"command","command":"portable"},
      {"type":"command","command":"background","async":true},
      {"type":"prompt","prompt":"check it"}
    ]}],
    "SessionEnd": [{"matcher":"(?=exit)","hooks":[{"type":"command","command":"complex"}]}],
    "PreToolUse": [{"hooks":[{"type":"command","command":"native-only"}]}]
  }
}`)

	result, err := (hooks.Service{Home: t.TempDir(), Project: project}).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hooks) != 1 || result.Hooks[0].Command != "portable" {
		t.Fatalf("hooks = %#v", result.Hooks)
	}
	joined := warningText(result.Warnings)
	for _, expected := range []string{"PreToolUse", "async", "handler type prompt", "matcher"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("warnings %q do not contain %q", joined, expected)
		}
	}
}

func TestListRejectsInvalidPortableConfiguration(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
	}{
		{"json", `{`},
		{"missing-hooks", `{}`},
		{"empty-command", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":""}]}]}}`},
		{"timeout", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"report","timeout":0}]}]}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := t.TempDir()
			writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), test.content)
			if _, err := (hooks.Service{Home: t.TempDir(), Project: project}).List(); err == nil {
				t.Fatal("expected invalid configuration to fail")
			}
		})
	}
}

func TestSessionEndTimeoutAboveThreeSecondsIsNotPortable(t *testing.T) {
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), `{"hooks":{"SessionEnd":[{"hooks":[{"type":"command","command":"slow-cleanup","timeout":4},{"type":"command","command":"fast-cleanup","timeout":3}]}]}}`)
	result, err := (hooks.Service{Home: t.TempDir(), Project: project}).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hooks) != 1 || result.Hooks[0].Command != "fast-cleanup" {
		t.Fatalf("hooks = %#v", result.Hooks)
	}
	if !strings.Contains(warningText(result.Warnings), "above 3 seconds") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func warningText(warnings []hooks.Warning) string {
	parts := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		parts = append(parts, warning.Event+" "+warning.Message)
	}
	return strings.Join(parts, "\n")
}
