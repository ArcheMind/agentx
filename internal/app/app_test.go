package app

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
)

func TestOutputFormatIsGlobal(t *testing.T) {
	format, args, err := parseOutputFormat([]string{"--yaml", "models", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if format != OutputYAML || strings.Join(args, " ") != "models codex" {
		t.Fatalf("format = %q, args = %v", format, args)
	}
	if _, _, err := parseOutputFormat([]string{"--json", "--yaml", "list"}); err == nil {
		t.Fatal("expected conflicting output formats to fail")
	}
}

func TestYAMLCoversInstallDryRun(t *testing.T) {
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	err := application.Run(context.Background(), []string{"--yaml", "agent", "install", "codex", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"executable: npm", "- install", "- --global", "- '@openai/codex'"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("YAML %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestYAMLCoversAuthDryRun(t *testing.T) {
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	err := application.Run(context.Background(), []string{"--yaml", "auth", "login", "claude", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"command:", "executable: claude", "- auth", "- login", "- --claudeai"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("YAML %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestAuthStatusStructuredOutput(t *testing.T) {
	status := runtime.AuthStatus{Agent: "claude", Supported: true, Providers: []runtime.AuthProvider{{ID: "claude", Method: "claude.ai", Subscription: "max"}}}
	for _, test := range []struct {
		name     string
		format   OutputFormat
		expected []string
	}{
		{name: "json", format: OutputJSON, expected: []string{`"agent": "claude"`, `"providers":`, `"subscription": "max"`}},
		{name: "yaml", format: OutputYAML, expected: []string{"agent: claude", "providers:", "subscription: max"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			application := App{Stdout: &stdout, Output: test.format}
			if err := application.writeAuthStatus(status); err != nil {
				t.Fatal(err)
			}
			for _, expected := range test.expected {
				if !strings.Contains(stdout.String(), expected) {
					t.Fatalf("output %q does not contain %q", stdout.String(), expected)
				}
			}
		})
	}
}

func TestAuthStatusTextOutput(t *testing.T) {
	var stdout bytes.Buffer
	application := App{Stdout: &stdout, Output: OutputText}
	if err := application.writeAuthStatus(runtime.AuthStatus{Agent: "pi", Supported: false, Providers: []runtime.AuthProvider{}}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "pi\tunsupported\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestStructuredOutputRejectsNativePassthrough(t *testing.T) {
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	err := application.Run(context.Background(), []string{"--json", "agent", "install", "codex"})
	if err == nil || !strings.Contains(err.Error(), "only supported for agent install with --dry-run") {
		t.Fatalf("error = %v", err)
	}
}

func TestStructuredErrorEnvelope(t *testing.T) {
	var stderr bytes.Buffer
	err := WriteError(&stderr, []string{"--json", "agent", "wat"}, fmt.Errorf("unknown agent operation"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"code": "usage"`, `"message": "unknown agent operation"`, `"exit_code": 1`} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("JSON %q does not contain %q", stderr.String(), expected)
		}
	}
}

func TestAgentResourceGrammar(t *testing.T) {
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err := application.Run(context.Background(), []string{"agent", "install", "codex", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if err := application.Run(context.Background(), []string{"install", "codex", "--dry-run"}); err == nil {
		t.Fatal("expected removed action-leading command to fail")
	}
}

func TestAgentShortcutUsesRunGrammar(t *testing.T) {
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	if err := application.Run(context.Background(), []string{"codex", "--model", "gpt-5.4", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"executable": "codex"`, `"args": [`, `"--model"`, `"gpt-5.4"`} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("shortcut plan %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestChooseRejectsInvalidSelection(t *testing.T) {
	var stdout bytes.Buffer
	application := New(false, strings.NewReader("3\n"), &stdout, &bytes.Buffer{})
	if _, err := application.choose(bufio.NewReader(application.Stdin), "Choose:\n", []string{"one", "two"}); err == nil || !strings.Contains(err.Error(), "1 to 2") {
		t.Fatalf("error = %v", err)
	}
}

func TestBareAXStartsInteractiveSessionResume(t *testing.T) {
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: t.TempDir()}
	err := application.Run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "no recent sessions found") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Current workspace") || !strings.Contains(stdout.String(), "Global") {
		t.Fatalf("interactive output = %q", stdout.String())
	}
}

func TestSessionSelectorMovesAcrossGroupBoundary(t *testing.T) {
	current := sessions.Summary{ID: "current", Provider: "codex", Title: "Current work"}
	global := sessions.Summary{ID: "global", Provider: "claude", Workspace: "/work/other", Title: "Other work"}
	groups := []sessionGroup{
		{Heading: "Current workspace", Items: []sessions.Summary{current}},
		{Heading: "Global", Items: []sessions.Summary{global}},
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader("\x1b[B\r"), &stdout, &bytes.Buffer{})
	selected, err := application.chooseSession(bufio.NewReader(application.Stdin), groups)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != global.ID {
		t.Fatalf("selected = %#v, want global session", selected)
	}
	if !strings.Contains(stdout.String(), "Current workspace") || !strings.Contains(stdout.String(), "Global") {
		t.Fatalf("selector output = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "/work/other") {
		t.Fatalf("selector output does not show global workspace: %q", stdout.String())
	}
}

func TestSessionSelectorHandlesEmptySections(t *testing.T) {
	t.Run("current workspace empty", func(t *testing.T) {
		global := sessions.Summary{ID: "global", Provider: "codex"}
		groups := []sessionGroup{
			{Heading: "Current workspace"},
			{Heading: "Global", Items: []sessions.Summary{global}},
		}
		var stdout bytes.Buffer
		application := New(false, strings.NewReader("\r"), &stdout, &bytes.Buffer{})
		selected, err := application.chooseSession(bufio.NewReader(application.Stdin), groups)
		if err != nil {
			t.Fatal(err)
		}
		if selected.ID != global.ID || !strings.Contains(stdout.String(), "No recent sessions") {
			t.Fatalf("selected = %#v, output = %q", selected, stdout.String())
		}
	})

	t.Run("global empty", func(t *testing.T) {
		current := sessions.Summary{ID: "current", Provider: "codex"}
		groups := []sessionGroup{
			{Heading: "Current workspace", Items: []sessions.Summary{current}},
			{Heading: "Global"},
		}
		application := New(false, strings.NewReader("\r"), &bytes.Buffer{}, &bytes.Buffer{})
		selected, err := application.chooseSession(bufio.NewReader(application.Stdin), groups)
		if err != nil {
			t.Fatal(err)
		}
		if selected.ID != current.ID {
			t.Fatalf("selected = %#v", selected)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		groups := []sessionGroup{{Heading: "Current workspace"}, {Heading: "Global"}}
		application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
		_, err := application.chooseSession(bufio.NewReader(application.Stdin), groups)
		if err == nil || !strings.Contains(err.Error(), "current workspace or globally") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSessionSelectorCanBeCancelled(t *testing.T) {
	groups := []sessionGroup{{Heading: "Current workspace", Items: []sessions.Summary{{ID: "current", Provider: "codex"}}}}
	application := New(false, strings.NewReader("q"), &bytes.Buffer{}, &bytes.Buffer{})
	_, err := application.chooseSession(bufio.NewReader(application.Stdin), groups)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error = %v", err)
	}
}

func TestSessionSelectorViewport(t *testing.T) {
	var currentItems []sessions.Summary
	for i := 0; i < 3; i++ {
		currentItems = append(currentItems, sessions.Summary{
			ID: fmt.Sprintf("c%d", i), Provider: "codex", Title: fmt.Sprintf("Task %d", i),
			UpdatedAt: "2026-09-10T12:00:00Z",
		})
	}
	var globalItems []sessions.Summary
	for i := 0; i < 3; i++ {
		globalItems = append(globalItems, sessions.Summary{
			ID: fmt.Sprintf("g%d", i), Provider: "claude", Title: fmt.Sprintf("Global %d", i),
			Workspace: "/work/other", UpdatedAt: "2026-09-10T11:00:00Z",
		})
	}
	groups := []sessionGroup{
		{Heading: "Current workspace", Items: currentItems},
		{Heading: "Global", Items: globalItems},
	}

	t.Run("shows bottom indicator when content overflows", func(t *testing.T) {
		var stdout bytes.Buffer
		app := New(false, strings.NewReader("\r"), &stdout, &bytes.Buffer{})
		app.termHeight = 10
		selected, err := app.chooseSession(bufio.NewReader(app.Stdin), groups)
		if err != nil {
			t.Fatal(err)
		}
		if selected.ID != "c0" {
			t.Fatalf("selected = %v, want c0", selected.ID)
		}
		output := stdout.String()
		if !strings.Contains(output, "↓") || !strings.Contains(output, "more\r\n") {
			t.Fatalf("expected bottom scroll indicator, output = %q", output)
		}
	})

	t.Run("scrolls to keep selection visible", func(t *testing.T) {
		keys := strings.Repeat("\x1b[B", 5) + "\r"
		var stdout bytes.Buffer
		app := New(false, strings.NewReader(keys), &stdout, &bytes.Buffer{})
		app.termHeight = 10
		selected, err := app.chooseSession(bufio.NewReader(app.Stdin), groups)
		if err != nil {
			t.Fatal(err)
		}
		if selected.ID != "g2" {
			t.Fatalf("selected = %v, want g2", selected.ID)
		}
		output := stdout.String()
		if !strings.Contains(output, "↑") {
			t.Fatalf("expected top scroll indicator after scrolling down, output = %q", output)
		}
	})

	t.Run("no viewport when content fits", func(t *testing.T) {
		small := []sessionGroup{
			{Heading: "Current workspace", Items: currentItems[:1]},
			{Heading: "Global"},
		}
		var stdout bytes.Buffer
		app := New(false, strings.NewReader("\r"), &stdout, &bytes.Buffer{})
		app.termHeight = 20
		_, err := app.chooseSession(bufio.NewReader(app.Stdin), small)
		if err != nil {
			t.Fatal(err)
		}
		output := stdout.String()
		if strings.Contains(output, "more\r\n") {
			t.Fatalf("unexpected scroll indicator when content fits, output = %q", output)
		}
	})
}

func TestRecentSessionGroupsExcludeCurrentSessionsFromGlobal(t *testing.T) {
	home := t.TempDir()
	currentWorkspace := filepath.Join(t.TempDir(), "current")
	globalWorkspace := filepath.Join(t.TempDir(), "global")
	if err := os.MkdirAll(currentWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(currentWorkspace)

	writeCodexSession := func(name, id, workspace, updated string) {
		t.Helper()
		path := filepath.Join(home, ".codex", "sessions", "2026", "09", "10", name+".jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf("{\"type\":\"session_meta\",\"timestamp\":%q,\"payload\":{\"id\":%q,\"cwd\":%q}}\n{\"type\":\"response_item\",\"timestamp\":%q,\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":\"work\"}}", updated, id, workspace, updated)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeCodexSession("current", "current-id", currentWorkspace, "2026-09-10T12:00:00Z")
	writeCodexSession("global", "global-id", globalWorkspace, "2026-09-10T11:00:00Z")

	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: home}
	groups, err := application.recentSessionGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || len(groups[0].Items) != 1 || groups[0].Items[0].ID != "current-id" {
		t.Fatalf("current group = %#v", groups)
	}
	if len(groups[1].Items) != 1 || groups[1].Items[0].ID != "global-id" {
		t.Fatalf("global group = %#v", groups)
	}
}

func TestSessionPresentationUsesLocalTimeWorkspaceAndUnicode(t *testing.T) {
	location := time.FixedZone("PDT", -7*60*60)
	now := time.Date(2026, time.September, 10, 16, 0, 0, 0, location)
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: "2026-09-10T22:23:23.05Z", want: "Today 15:23"},
		{value: "2026-09-09T20:03:54Z", want: "Yesterday 13:03"},
		{value: "2026-08-21T19:30:00Z", want: "Aug 21 12:30"},
		{value: "2025-09-10T19:30:00Z", want: "Sep 10 2025"},
	} {
		if got := displaySessionTime(test.value, now); got != test.want {
			t.Fatalf("displaySessionTime(%q) = %q, want %q", test.value, got, test.want)
		}
	}
	for _, test := range []struct {
		started string
		updated string
		want    string
	}{
		{started: "2026-09-10T15:00:00Z", updated: "2026-09-10T15:00:08Z", want: "(8s)"},
		{started: "2026-09-10T15:00:00Z", updated: "2026-09-10T15:42:00Z", want: "(42m)"},
		{started: "2026-09-10T15:00:00Z", updated: "2026-09-10T16:07:00Z", want: "(1h 07m)"},
		{started: "2026-09-07T13:00:00Z", updated: "2026-09-10T15:00:00Z", want: "(3d 02h)"},
		{started: "", updated: "2026-09-10T15:00:00Z", want: ""},
		{started: "2026-09-10T16:00:00Z", updated: "2026-09-10T15:00:00Z", want: ""},
	} {
		if got := displaySessionDuration(test.started, test.updated); got != test.want {
			t.Fatalf("displaySessionDuration(%q, %q) = %q, want %q", test.started, test.updated, got, test.want)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(home, "work", "company", "project")
	if got := displayWorkspace(workspace, 40); got != filepath.Join("~", "work", "company", "project") {
		t.Fatalf("displayWorkspace = %q", got)
	}
	if got := displayWorkspace("", 40); got != "(workspace unknown)" {
		t.Fatalf("empty workspace = %q", got)
	}
	if got := singleLine("你好世界", 3); got != "你好…" {
		t.Fatalf("singleLine = %q", got)
	}
}

func TestSessionRowsUseWorkspaceColumnOnlyForGlobal(t *testing.T) {
	now := time.Date(2026, time.September, 10, 16, 0, 0, 0, time.UTC)
	groups := []sessionGroup{
		{
			Heading: "Current workspace",
			Items: []sessions.Summary{{
				Provider:  "claude",
				StartedAt: "2026-09-10T14:00:00Z",
				UpdatedAt: "2026-09-10T15:00:00Z",
				Workspace: "/work/current",
				Title:     strings.Repeat("current", 10),
			}},
		},
		{
			Heading: "Global",
			Items: []sessions.Summary{{
				Provider:  "codex",
				UpdatedAt: "invalid timestamp that is too long",
				Workspace: filepath.Join(string(filepath.Separator), "work", "company", "very-long-project-name"),
				Title:     strings.Repeat("title", 12),
			}},
		},
	}

	rows := buildContentRows(groups, now)
	if len(rows) != 6 {
		t.Fatalf("rows = %#v", rows)
	}
	currentRow := rows[2].text
	currentTitle := singleLine(groups[0].Items[0].Title, 57)
	if strings.Contains(currentRow, groups[0].Items[0].Workspace) || !strings.HasSuffix(currentRow, currentTitle) {
		t.Fatalf("current row should omit workspace column: %q", currentRow)
	}
	if !strings.Contains(currentRow, "Today 15:00 (1h 00m)") {
		t.Fatalf("current row should include session duration in the time column: %q", currentRow)
	}
	if len([]rune(currentRow)) != 94 {
		t.Fatalf("current row width = %d, want 94 before selection marker: %q", len([]rune(currentRow)), currentRow)
	}

	globalRow := rows[5].text
	workspace := displayWorkspace(groups[1].Items[0].Workspace, 20)
	globalTitle := singleLine(groups[1].Items[0].Title, 35)
	if !strings.Contains(globalRow, workspace+"  "+globalTitle) {
		t.Fatalf("global row does not put workspace before title: %q", globalRow)
	}
	if !strings.HasPrefix(workspace, "…") || !strings.HasSuffix(workspace, "long-project-name") {
		t.Fatalf("workspace = %q, want suffix-preserving truncation", workspace)
	}
	if len([]rune(globalRow)) != 94 {
		t.Fatalf("global row width = %d, want 94 before selection marker: %q", len([]rune(globalRow)), globalRow)
	}
}

func TestSessionListUsesSourceTerminology(t *testing.T) {
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: t.TempDir()}
	if err := application.Run(context.Background(), []string{"session", "list", "--source", "pi", "--all"}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionInfoUsesGlobalYAMLOutput(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	path := filepath.Join(home, ".claude", "projects", "fixture", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"type":"user","sessionId":"session-id","cwd":"` + workspace + `","timestamp":"2026-09-01T10:00:00Z","message":{"content":"continue work"}}`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: home}
	if err := application.Run(context.Background(), []string{"--yaml", "session", "info", "session-id", "--source", "claude"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"id: session-id", "provider: claude", "content: continue work"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("YAML %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestSessionResumeDryRunIsNativeAgentPlan(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	path := filepath.Join(home, ".pi", "agent", "sessions", "fixture", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"type":"session","id":"pi-session","cwd":"` + workspace + `"}` + "\n" +
		`{"type":"message","message":{"role":"user","content":"continue work"}}`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Sessions = sessions.Service{HomeDir: home}
	if err := application.Run(context.Background(), []string{"--json", "session", "resume", "codex", "pi-session", "--source", "pi", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"executable": "codex"`, `"cwd": "` + workspace + `"`, "continue work"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("JSON %q does not contain %q", stdout.String(), expected)
		}
	}
}
