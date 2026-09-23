package hooks_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ArcheMind/agentx/internal/hooks"
	"github.com/ArcheMind/agentx/internal/runtime"
)

func TestPortableHooksProjectThroughEachNativeLoadingSurface(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfig(t, filepath.Join(home, ".agents", "hooks.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"user-start"}]}]}}`)
	writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), `{"hooks":{"SessionEnd":[{"hooks":[{"type":"command","command":"project-end","timeout":3}]}],"UserPromptSubmit":[{"hooks":[{"type":"command","command":"project-before"}]}],"Stop":[{"hooks":[{"type":"command","command":"project-after"}]}]}}`)
	service := hooks.Service{Home: home, Project: project}

	tests := []struct {
		provider string
		check    func(t *testing.T, projection hooks.Projection)
	}{
		{"claude", func(t *testing.T, projection hooks.Projection) {
			if len(projection.Args) != 2 || projection.Args[0] != "--plugin-dir" {
				t.Fatalf("args = %#v", projection.Args)
			}
			assertFileContains(t, filepath.Join(projection.Args[1], ".claude-plugin", "plugin.json"), `"name": "agentx-hooks"`)
			content := readFile(t, filepath.Join(projection.Args[1], "hooks", "hooks.json"))
			assertContains(t, content, `"UserPromptSubmit"`)
			assertProjectedHandler(t, content, "claude", "UserPromptSubmit", hooks.Handler{Command: "project-before", Timeout: 600, Project: project})
			assertProjectedHandler(t, content, "claude", "SessionEnd", hooks.Handler{Command: "project-end", Timeout: 3, Project: project})
		}},
		{"codex", func(t *testing.T, projection hooks.Projection) {
			joined := strings.Join(projection.Args, " ")
			for _, event := range []string{"SessionStart", "SessionEnd", "UserPromptSubmit", "Stop"} {
				if !strings.Contains(joined, "hooks."+event) || !strings.Contains(joined, "--provider codex --event "+event) {
					t.Fatalf("args %q do not project %s", joined, event)
				}
			}
			assertProjectedHandler(t, joined, "codex", "UserPromptSubmit", hooks.Handler{Command: "project-before", Timeout: 600, Project: project})
			if len(projection.Artifacts) != 0 {
				t.Fatalf("codex artifacts = %#v", projection.Artifacts)
			}
		}},
		{"gemini", func(t *testing.T, projection hooks.Projection) {
			if len(projection.Args) != 0 || len(projection.Artifacts) != 1 {
				t.Fatalf("projection = %#v", projection)
			}
			root := projection.Artifacts[0]
			assertFileContains(t, filepath.Join(root, "gemini-extension.json"), `"name": "agentx-hooks"`)
			content := readFile(t, filepath.Join(root, "hooks", "hooks.json"))
			assertContains(t, content, `"BeforeAgent"`, `"AfterAgent"`)
			assertProjectedHandler(t, content, "gemini", "UserPromptSubmit", hooks.Handler{Command: "project-before", Timeout: 600, Project: project})
		}},
		{"pi", func(t *testing.T, projection hooks.Projection) {
			if len(projection.Args) != 2 || projection.Args[0] != "--extension" {
				t.Fatalf("args = %#v", projection.Args)
			}
			content := readFile(t, projection.Args[1])
			assertContains(t, content, `pi.on("session_start"`, `pi.on("input"`, `pi.on("before_agent_start"`, `pi.on("agent_settled"`, `"hook", "__dispatch"`, `"command":"project-before"`, `"project":"`+project+`"`, `"timeout":600`)
		}},
	}

	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			projection, err := service.Prepare(test.provider, "/opt/agentx/bin/ax", true)
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, projection)
		})
	}
}

func TestProjectedCommandTransportDoesNotExposeShellSyntax(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	command := `echo %PATH% & "quoted" | more`
	writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo %PATH% & \"quoted\" | more"}]}]}}`)
	projection, err := (hooks.Service{Home: home, Project: project}).Prepare("codex", `C:\Program Files\AgentX\ax.exe`, false)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Join(projection.Args, "\n")
	if strings.Contains(content, command) || strings.Contains(content, "%PATH%") {
		t.Fatalf("projection exposes command to provider shell: %s", content)
	}
	assertProjectedHandler(t, content, "codex", "Stop", hooks.Handler{Command: command, Timeout: 600, Project: project})
}

func TestHookProjectionDryRunDoesNotWriteAndOwnedArtifactIsRequired(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"start"}]}]}}`)
	service := hooks.Service{Home: home, Project: project}
	projection, err := service.Prepare("claude", "/opt/agentx/bin/ax", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projection.Artifacts[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run artifact stat error = %v", err)
	}

	geminiRoot := filepath.Join(home, ".gemini", "extensions", "agentx-hooks")
	if err := os.MkdirAll(geminiRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prepare("gemini", "/opt/agentx/bin/ax", false); err == nil || !strings.Contains(err.Error(), "non-AgentX") {
		t.Fatalf("collision error = %v", err)
	}
}

func TestEmptyConfigurationRemovesOnlyOwnedProjection(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	path := filepath.Join(project, ".agents", "hooks.json")
	writeConfig(t, path, `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"start"}]}]}}`)
	service := hooks.Service{Home: home, Project: project}
	projection, err := service.Prepare("gemini", "/opt/agentx/bin/ax", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prepare("gemini", "/opt/agentx/bin/ax", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projection.Artifacts[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale artifact stat error = %v", err)
	}

	userRoot := filepath.Join(home, ".gemini", "extensions", "agentx-hooks")
	if err := os.MkdirAll(userRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userRoot, "keep"), []byte("user-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prepare("gemini", "/opt/agentx/bin/ax", true); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, filepath.Join(userRoot, "keep"), "user-owned")
}

func TestProjectionRefreshRemovesStaleOwnedFiles(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"start"}]}]}}`)
	service := hooks.Service{Home: home, Project: project}
	projection, err := service.Prepare("claude", "/opt/agentx/bin/ax", true)
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(projection.Artifacts[0], "hooks", "stale.json")
	if err := os.WriteFile(stale, []byte(`{"hooks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prepare("claude", "/opt/agentx/bin/ax", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale file stat error = %v", err)
	}
}

func TestDispatchPresentsCanonicalCommandIOAcrossAgents(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ".agents", "hooks.json"), `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"policy-check"}]}]}}`)
	tests := []struct {
		provider    string
		payload     string
		outputEvent string
	}{
		{"claude", `{"hook_event_name":"UserPromptSubmit","session_id":"s1","cwd":"` + project + `","prompt":"review this"}`, "UserPromptSubmit"},
		{"codex", `{"hook_event_name":"UserPromptSubmit","session_id":"s1","cwd":"` + project + `","prompt":"review this"}`, "UserPromptSubmit"},
		{"gemini", `{"hook_event_name":"BeforeAgent","session_id":"s1","cwd":"` + project + `","prompt":"review this"}`, "BeforeAgent"},
		{"pi", `{"hook_event_name":"input","session_id":"s1","cwd":"` + project + `","text":"review this"}`, "UserPromptSubmit"},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			projection, err := (hooks.Service{Home: home, Project: project}).Prepare(test.provider, "/opt/agentx/bin/ax", true)
			if err != nil {
				t.Fatal(err)
			}
			handler := projectedHandlerFromProjection(t, projection)
			runner := &recordingHookRunner{stdout: `{"systemMessage":"checked","hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"policy context"}}`}
			result, err := hooks.DispatchOne(context.Background(), runner, test.provider, "UserPromptSubmit", []byte(test.payload), handler)
			if err != nil {
				t.Fatal(err)
			}
			input := runner.singleInput(t)
			if input["hook_event_name"] != "UserPromptSubmit" || input["prompt"] != "review this" || input["session_id"] != "s1" {
				t.Fatalf("canonical input = %#v", input)
			}
			var output map[string]any
			if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
				t.Fatal(err)
			}
			specific := output["hookSpecificOutput"].(map[string]any)
			if output["systemMessage"] != "checked" || specific["hookEventName"] != test.outputEvent || specific["additionalContext"] != "policy context" {
				t.Fatalf("native output = %#v", output)
			}
		})
	}
}

func TestProjectedProjectHookDoesNotRunInAnotherProject(t *testing.T) {
	runner := &recordingHookRunner{stdout: `{"systemMessage":"unexpected"}`}
	result, err := hooks.DispatchOne(context.Background(), runner, "claude", "SessionStart", []byte(`{"hook_event_name":"SessionStart","session_id":"s1","cwd":"/other","source":"startup"}`), hooks.Handler{Command: "project-start", Project: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "" || len(runner.inputs) != 0 {
		t.Fatalf("cross-project dispatch = %#v, inputs = %#v", result, runner.inputs)
	}
}

func TestUserPromptBlockUsesEachProviderDecision(t *testing.T) {
	tests := []struct {
		provider string
		payload  string
		decision string
	}{
		{"claude", `{"hook_event_name":"UserPromptSubmit","session_id":"s1","cwd":"/work","prompt":"stop"}`, "block"},
		{"codex", `{"hook_event_name":"UserPromptSubmit","session_id":"s1","cwd":"/work","prompt":"stop"}`, "block"},
		{"gemini", `{"hook_event_name":"BeforeAgent","session_id":"s1","cwd":"/work","prompt":"stop"}`, "deny"},
		{"pi", `{"hook_event_name":"input","session_id":"s1","cwd":"/work","text":"stop"}`, "block"},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			runner := &recordingHookRunner{stdout: `{"decision":"block","reason":"denied"}`}
			result, err := hooks.DispatchOne(context.Background(), runner, test.provider, "UserPromptSubmit", []byte(test.payload), hooks.Handler{Command: "policy", Timeout: 600})
			if err != nil {
				t.Fatal(err)
			}
			var output map[string]any
			if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
				t.Fatal(err)
			}
			if output["decision"] != test.decision || output["reason"] != "denied" {
				t.Fatalf("output = %#v", output)
			}
		})
	}
}

type recordingHookRunner struct {
	mu     sync.Mutex
	inputs []map[string]any
	stdout string
}

func (r *recordingHookRunner) Execute(_ context.Context, _ runtime.CommandPlan, options runtime.ExecuteOptions) (runtime.CommandResult, error) {
	data, err := io.ReadAll(options.Stdin)
	if err != nil {
		return runtime.CommandResult{}, err
	}
	var input map[string]any
	if err := json.Unmarshal(data, &input); err != nil {
		return runtime.CommandResult{}, err
	}
	r.mu.Lock()
	r.inputs = append(r.inputs, input)
	r.mu.Unlock()
	return runtime.CommandResult{Stdout: r.stdout}, nil
}

func (r *recordingHookRunner) singleInput(t *testing.T) map[string]any {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.inputs) != 1 {
		t.Fatalf("inputs = %#v", r.inputs)
	}
	return r.inputs[0]
}

func assertFileContains(t *testing.T, path string, expected ...string) {
	t.Helper()
	content := readFile(t, path)
	assertContains(t, content, expected...)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertContains(t *testing.T, content string, expected ...string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(content, value) {
			t.Fatalf("content does not contain %q:\n%s", value, content)
		}
	}
}

func assertProjectedHandler(t *testing.T, content, provider, event string, expected hooks.Handler) {
	t.Helper()
	marker := "--provider " + provider + " --event " + event + " --handler "
	index := strings.Index(content, marker)
	if index < 0 {
		t.Fatalf("projection does not contain %q:\n%s", marker, content)
	}
	actual := decodeHandler(t, content[index+len(marker):])
	if actual != expected {
		t.Fatalf("handler = %#v, want %#v", actual, expected)
	}
}

func projectedHandlerFromProjection(t *testing.T, projection hooks.Projection) hooks.Handler {
	t.Helper()
	content := strings.Join(projection.Args, "\n")
	for _, root := range projection.Artifacts {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				content += "\n" + readFile(t, path)
			}
			return nil
		})
	}
	if index := strings.Index(content, "--handler "); index >= 0 {
		return decodeHandler(t, content[index+len("--handler "):])
	}
	if index := strings.Index(content, `"payload":"`); index >= 0 {
		return decodeHandler(t, content[index+len(`"payload":"`):])
	}
	t.Fatalf("projection has no encoded handler:\n%s", content)
	return hooks.Handler{}
}

func decodeHandler(t *testing.T, suffix string) hooks.Handler {
	t.Helper()
	end := 0
	for end < len(suffix) {
		character := suffix[end]
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			end++
			continue
		}
		break
	}
	data, err := base64.RawURLEncoding.DecodeString(suffix[:end])
	if err != nil {
		t.Fatal(err)
	}
	var handler hooks.Handler
	if err := json.Unmarshal(data, &handler); err != nil {
		t.Fatal(err)
	}
	return handler
}
