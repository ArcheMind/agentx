package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
	"github.com/ArcheMind/agentx/internal/subagents"
)

type presetRunner struct {
	plans    []runtime.CommandPlan
	rpc      int
	response string
}

func (r *presetRunner) Execute(_ context.Context, plan runtime.CommandPlan, _ runtime.ExecuteOptions) (runtime.CommandResult, error) {
	r.plans = append(r.plans, plan)
	return runtime.CommandResult{}, nil
}

func (r *presetRunner) ExecuteJSONRPC(context.Context, runtime.CommandPlan, []json.RawMessage, json.RawMessage) (json.RawMessage, error) {
	r.rpc++
	return json.RawMessage(r.response), nil
}

func presetApp(t *testing.T, input string, binaries ...string) (App, *bytes.Buffer, *presetRunner) {
	t.Helper()
	bin := t.TempDir()
	for _, name := range binaries {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("AX_SUBAGENT_HOME", "")
	t.Chdir(t.TempDir())
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(input), &stdout, &bytes.Buffer{})
	runner := &presetRunner{}
	application.Runner = runner
	application.Hooks.Home = t.TempDir()
	application.Sessions = sessions.Service{HomeDir: t.TempDir()}
	return application, &stdout, runner
}

func mustCreate(t *testing.T, home string, p presets.Preset) {
	t.Helper()
	if err := presets.Create(home, p); err != nil {
		t.Fatal(err)
	}
}

var deepReview = presets.Preset{Name: "deep-review", Description: "Review complex changes and edge cases", Agent: "codex", Model: "gpt-5.6-luna", Effort: "high"}

func TestHomeListsPresetsBeforeHistoryWithCreateLast(t *testing.T) {
	application, stdout, _ := presetApp(t, "q")
	mustCreate(t, application.Hooks.Home, deepReview)
	mustCreate(t, application.Hooks.Home, presets.Preset{Name: "daily", Description: "Everyday development", Agent: "claude", Model: "opus"})
	groups := []sessionGroup{{Heading: "Current workspace", Items: []sessions.Summary{{ID: "s1", Provider: "codex", Title: "Fix login state"}}}, {Heading: "Global"}}
	if _, _, _, err := application.chooseHome(bufio.NewReader(application.Stdin), application.Hooks.Home, groups, 0); err == nil {
		t.Fatal("cancel started work")
	}
	output := stdout.String()
	order := []string{"New session · Presets", "daily", "Everyday development", "Claude · opus · native default effort", "deep-review", "Review complex changes", "Codex · gpt-5.6-luna · high", "+ Create preset…", "Recent sessions · Current workspace", "Fix login state", "Recent sessions · Global"}
	position := 0
	for _, want := range order {
		next := strings.Index(output[position:], want)
		if next < 0 {
			t.Fatalf("%q missing or out of order in %q", want, output)
		}
		position += next
	}
}

func TestSelectedPresetExpandsLongDescription(t *testing.T) {
	long := strings.Repeat("word ", 30) + "END"
	rows, _ := buildHomeRows([]presets.Preset{{Name: "long", Description: long, Agent: "codex", Model: "m"}}, nil, nil, nil, time.Now())
	text := func(rows []contentRow) string {
		var parts []string
		for _, row := range rows {
			parts = append(parts, row.text)
		}
		return strings.Join(parts, "\n")
	}
	collapsed := expandSelected(rows, 5)
	expanded := expandSelected(rows, 0)
	if len(expanded) <= len(collapsed) || strings.Contains(text(collapsed), "END") || !strings.Contains(text(expanded), "END") {
		t.Fatalf("collapsed = %#v expanded = %#v", collapsed, expanded)
	}
	for _, row := range expanded {
		if row.kind == contentRowPresetCombo && !strings.Contains(row.text, " · m · ") {
			t.Fatal("combination truncated", row.text)
		}
	}
}

func TestSelectingPresetStartsNewSessionWithExactCombination(t *testing.T) {
	application, stdout, runner := presetApp(t, "\r", "codex")
	mustCreate(t, application.Hooks.Home, deepReview)
	if err := application.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if len(runner.plans) != 1 {
		t.Fatalf("plans = %#v", runner.plans)
	}
	plan := runner.plans[0]
	args := strings.Join(plan.Args, " ")
	if plan.Executable != "codex" || plan.Cwd != cwd || !strings.Contains(args, "--model gpt-5.6-luna") || !strings.Contains(args, `-c model_reasoning_effort="high"`) || strings.Contains(args, "resume") {
		t.Fatalf("plan = %#v", plan)
	}
	if !strings.Contains(stdout.String(), "with preset deep-review: model gpt-5.6-luna · effort high") {
		t.Fatal(stdout.String())
	}
}

func TestNativeDefaultEffortPassesNoEffortFlag(t *testing.T) {
	application, _, runner := presetApp(t, "\r", "claude")
	mustCreate(t, application.Hooks.Home, presets.Preset{Name: "daily", Description: "Everyday", Agent: "claude", Model: "opus"})
	if err := application.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(runner.plans[0].Args, " ")
	if !strings.Contains(args, "--model opus") || strings.Contains(args, "--effort") {
		t.Fatal(args)
	}
}

func TestMissingAgentFailsWithoutSubstitution(t *testing.T) {
	application, stdout, runner := presetApp(t, "\r"+"j\r"+"q")
	mustCreate(t, application.Hooks.Home, deepReview)
	if err := application.Run(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "no session started") {
		t.Fatal(err)
	}
	if len(runner.plans) != 0 {
		t.Fatalf("started %#v", runner.plans)
	}
	for _, want := range []string{"Start · deep-review", "Error: Codex CLI is not installed. Install it with ax agent install codex, or edit the preset", "no other agent, model or effort was substituted", "Edit preset", "Back to presets"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("missing %q in %q", want, stdout.String())
		}
	}
}

func TestCreateWithManualModelSavesWithoutInference(t *testing.T) {
	input := "\r" + "\r" + "opus-4\n" + "j\r" + "deep review\n" + "deep-review\n" + "Review complex changes\n" + "j\r" + "q"
	application, stdout, runner := presetApp(t, input)
	if err := application.Run(context.Background(), nil); err == nil {
		t.Fatal("expected home cancel after save")
	}
	saved, err := presets.Read(application.Hooks.Home, "deep-review")
	want := presets.Preset{Name: "deep-review", Description: "Review complex changes", Agent: "claude", Model: "opus-4", Effort: "low"}
	if err != nil || saved != want {
		t.Fatalf("saved = %#v err = %v", saved, err)
	}
	if len(runner.plans) != 0 || runner.rpc != 0 {
		t.Fatalf("creation ran external commands: %#v", runner.plans)
	}
	output := stdout.String()
	for _, want := range []string{"Claude Code has no reliable local model list", "Enter the exact model ID", "unverified for this model", "Error: Invalid name", "Your other entries are kept", "Review preset", "Effort       low (unverified for this model)", "Availability Execution not verified; Claude Code is not installed", "MCP          Interactive launch only", "Saving does not run inference.", "Save and start", "Saved ", "New sessions use the saved presets now.", "MCP: ExternalSubagents are disabled", "deep-review  Review complex changes"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in %q", want, output)
		}
	}
}

func TestCreateOffersOnlyListedCodexEfforts(t *testing.T) {
	input := "\r" + "j\r" + "\r" + "jj\r" + "listed\n" + "Use listed\n" + "\r"
	application, stdout, runner := presetApp(t, input, "codex")
	runner.response = `{"id":2,"result":{"data":[{"model":"gpt-a","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"max"}],"defaultReasoningEffort":"low"}]}}`
	if err := application.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	saved, err := presets.Read(application.Hooks.Home, "listed")
	if err != nil || saved.Model != "gpt-a" || saved.Effort != "max" {
		t.Fatalf("saved = %#v err = %v", saved, err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Explicit levels listed by Codex for gpt-a") || !strings.Contains(output, "low (model default)") || strings.Contains(output, "xhigh") || !strings.Contains(output, "Callable as MCP ExternalSubagent") {
		t.Fatal(output)
	}
	if len(runner.plans) != 1 || !strings.Contains(strings.Join(runner.plans[0].Args, " "), `model_reasoning_effort="max"`) {
		t.Fatalf("save and start did not launch saved combination: %#v", runner.plans)
	}
}

func TestEffortNotApplicableOffersOnlyNativeDefault(t *testing.T) {
	_, choices := effortChoices(presetDraft{preset: presets.Preset{Agent: "gemini", Model: "m"}})
	note, piChoices := effortChoices(presetDraft{preset: presets.Preset{Agent: "pi", Model: "p/m"}})
	if len(choices) != 1 || len(piChoices) != 1 || choices[0].value != "" || !strings.Contains(note, "not applicable") {
		t.Fatal(choices, piChoices, note)
	}
}

func TestCreateCancelAndDuplicateNeverLeaveOrOverwrite(t *testing.T) {
	input := "j\r" + "\r" + "opus\n" + "\r" + "deep-review\n" + "\n" + "\r" + "\r" + "\n" + "q"
	application, stdout, _ := presetApp(t, input)
	mustCreate(t, application.Hooks.Home, deepReview)
	application.Run(context.Background(), nil)
	entries, _ := os.ReadDir(presets.Dir(application.Hooks.Home))
	if len(entries) != 1 {
		t.Fatalf("entries = %v", entries)
	}
	if original, _ := presets.Read(application.Hooks.Home, "deep-review"); original != deepReview {
		t.Fatalf("original overwritten: %#v", original)
	}
	if !strings.Contains(stdout.String(), "Error: Name already exists. Choose another name.") || strings.Count(stdout.String(), "Cancelled; nothing was saved.") != 2 {
		t.Fatal(stdout.String())
	}
}

func TestManageRenameDuplicateDeleteAndView(t *testing.T) {
	home := ""
	run := func(input string) string {
		application, stdout, _ := presetApp(t, input)
		if home == "" {
			home = application.Hooks.Home
			mustCreate(t, home, deepReview)
		}
		application.Hooks.Home = home
		application.Run(context.Background(), nil)
		return stdout.String()
	}
	output := run("m" + "\r" + "\r" + "q" + "q")
	for _, want := range []string{"Manage · deep-review", "Configuration", presets.Path(home, "deep-review"), "Review complex changes and edge cases", "Status", "Execution not verified"} {
		if !strings.Contains(output, want) {
			t.Fatalf("view lacks %q: %q", want, output)
		}
	}
	output = run("m" + "jjj\r" + "deep-review\n" + "review-low\n" + "j\r" + "q" + "q")
	copied, err := presets.Read(home, "review-low")
	if err != nil || copied.Model != deepReview.Model || copied.Effort != deepReview.Effort {
		t.Fatalf("copy = %#v err = %v output = %q", copied, err, output)
	}
	if !strings.Contains(output, "The copy is independent") {
		t.Fatal(output)
	}
	output = run("jm" + "jj\r" + "deep-review\n" + "quick\n" + "q" + "q")
	if _, err := presets.Read(home, "quick"); err != nil || !strings.Contains(output, "Error: Name already exists") {
		t.Fatalf("rename err = %v output = %q", err, output)
	}
	output = run("jm" + "jjjj\r" + "j\r" + "q")
	if _, err := presets.Read(home, "quick"); err == nil || !strings.Contains(output, "History, native agents, credentials and running work are kept") {
		t.Fatalf("delete output = %q", output)
	}
	if original, err := presets.Read(home, "deep-review"); err != nil || original != deepReview {
		t.Fatal("copy/rename/delete changed another preset", original, err)
	}
}

func TestBrokenPresetIsLocatedWithoutBlockingOthers(t *testing.T) {
	application, stdout, runner := presetApp(t, "j\r"+"jj\r"+"k\r", "codex")
	mustCreate(t, application.Hooks.Home, deepReview)
	broken := filepath.Join(presets.Dir(application.Hooks.Home), "edited")
	os.MkdirAll(broken, 0o700)
	os.WriteFile(filepath.Join(broken, presets.FileName), []byte("---\nagent: codex\n---\nHand edited\n"), 0o600)
	if err := application.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "edited       Error: model is required") || !strings.Contains(output, "Location: "+filepath.Join(broken, presets.FileName)) || !strings.Contains(output, "Other presets and recent sessions are unaffected") {
		t.Fatal(output)
	}
	if len(runner.plans) != 1 || runner.plans[0].Executable != "codex" {
		t.Fatalf("valid preset blocked: %#v", runner.plans)
	}
}

func TestChangeNoteTellsUserToReconnectLiveMCP(t *testing.T) {
	application, stdout, _ := presetApp(t, "")
	home := application.Hooks.Home
	subagents.SaveConfig(home, subagents.Config{Enabled: true})
	application.presetChangeNote(home)
	if !strings.Contains(stdout.String(), "no running AgentX MCP connection") {
		t.Fatal(stdout.String())
	}
	dir := filepath.Join(home, ".local", "state", "agentx", "connections")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "live.json"), []byte(fmt.Sprintf(`{"id":"connection-x","pid":%d}`, os.Getpid())), 0o600)
	stdout.Reset()
	application.presetChangeNote(home)
	if !strings.Contains(stdout.String(), "1 running AgentX MCP connection(s) still show the preset list from when they connected") || !strings.Contains(stdout.String(), "reconnect AgentX in Claude (/mcp)") {
		t.Fatal(stdout.String())
	}
}

func TestEditChangesOnlyConfirmedFieldsAndCancelKeepsOriginal(t *testing.T) {
	application, stdout, _ := presetApp(t, "m"+"j\r"+"q"+"q"+"q")
	home := application.Hooks.Home
	mustCreate(t, home, deepReview)
	application.Run(context.Background(), nil)
	if current, _ := presets.Read(home, "deep-review"); current != deepReview || !strings.Contains(stdout.String(), "Cancelled; nothing was saved.") {
		t.Fatalf("cancelled edit changed preset: %#v", current)
	}
	application.Stdin = strings.NewReader("m" + "j\r" + "j\r" + "jj\r" + "kkkkk\r" + "\r" + "q" + "q")
	stdout.Reset()
	application.Run(context.Background(), nil)
	want := deepReview
	want.Effort = ""
	if current, _ := presets.Read(home, "deep-review"); current != want {
		t.Fatalf("edited = %#v output = %q", current, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Effort       native default") {
		t.Fatal(stdout.String())
	}
}

func TestEditAndCopyEffortUseListedLevels(t *testing.T) {
	listed := `{"id":2,"result":{"data":[{"model":"gpt-5.6-luna","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"medium"}],"defaultReasoningEffort":"low"}]}}`
	application, stdout, runner := presetApp(t, "m"+"j\r"+"j\r"+"jj\r"+"jj\r"+"\r"+"q"+"q", "codex")
	runner.response = listed
	home := application.Hooks.Home
	mustCreate(t, home, deepReview)
	application.Run(context.Background(), nil)
	if current, _ := presets.Read(home, "deep-review"); current.Effort != "medium" {
		t.Fatalf("edited = %#v output = %q", current, stdout.String())
	}
	if output := stdout.String(); !strings.Contains(output, "Explicit levels listed by Codex for gpt-5.6-luna") || strings.Contains(output, "ultra") || strings.Contains(output, "entered manually") {
		t.Fatal(output)
	}
	application.Stdin = strings.NewReader("m" + "jjj\r" + "copy\n" + "jj\r" + "jj\r" + "k\r" + "j\r" + "q" + "q")
	stdout.Reset()
	application.Run(context.Background(), nil)
	if copied, err := presets.Read(home, "copy"); err != nil || copied.Effort != "low" || strings.Contains(stdout.String(), "ultra") {
		t.Fatalf("copy = %#v err = %v output = %q", copied, err, stdout.String())
	}
}

func TestUnlistedModelEffortNoteIsHonest(t *testing.T) {
	application, stdout, runner := presetApp(t, "m"+"j\r"+"j\r"+"jj\r"+"q"+"q"+"q", "codex")
	runner.response = `{"id":2,"result":{"data":[{"model":"other","supportedReasoningEfforts":[{"reasoningEffort":"low"}]}]}}`
	mustCreate(t, application.Hooks.Home, deepReview)
	application.Run(context.Background(), nil)
	if output := stdout.String(); !strings.Contains(output, "gpt-5.6-luna is not in the Codex CLI model list, so its levels were not confirmed") || !strings.Contains(output, "(unverified for this model)") {
		t.Fatal(output)
	}
}

func TestConfigurationErrorDoesNotOfferPresetEdit(t *testing.T) {
	application, stdout, runner := presetApp(t, "\r"+"\r"+"q", "claude")
	home := application.Hooks.Home
	mustCreate(t, home, presets.Preset{Name: "daily", Description: "Everyday", Agent: "claude", Model: "opus"})
	os.MkdirAll(filepath.Dir(subagents.ConfigPath(home)), 0o700)
	os.WriteFile(subagents.ConfigPath(home), []byte(`{"enabled":true,"default":"x","options":[]}`), 0o600)
	application.Run(context.Background(), nil)
	output := stdout.String()
	if len(runner.plans) != 0 || !strings.Contains(output, "docs/subagents.md") || !strings.Contains(output, "keep its grant under the preset name") || strings.Contains(output, "Edit preset") {
		t.Fatal(output)
	}
}
