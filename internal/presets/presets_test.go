package presets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripKeepsDescriptionInMarkdownBody(t *testing.T) {
	home := t.TempDir()
	preset := Preset{Name: "deep-review", Description: "Review complex changes.\n\nNot for quick fixes.", Agent: "codex", Model: "gpt-5.6-luna", Effort: "high"}
	if err := Create(home, preset); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path(home, "deep-review"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\nagent: codex\nmodel: gpt-5.6-luna\neffort: high\n---\n\nReview complex changes.") || strings.Contains(text, "name:") {
		t.Fatalf("PRESET.md = %q", text)
	}
	loaded, problems, err := Load(home)
	if err != nil || len(problems) != 0 || len(loaded) != 1 || !loaded[0].Equal(preset) {
		t.Fatalf("loaded = %#v problems = %#v err = %v", loaded, problems, err)
	}
}

func TestNativeDefaultEffortIsOmitted(t *testing.T) {
	home := t.TempDir()
	if err := Create(home, Preset{Name: "pi-review", Description: "Independent review", Agent: "pi", Model: "openai-codex/gpt-5.6-luna"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(Path(home, "pi-review"))
	if strings.Contains(string(data), "effort") {
		t.Fatalf("native default effort was written: %q", data)
	}
}

func TestCreateRejectsDuplicateWithoutOverwriting(t *testing.T) {
	home := t.TempDir()
	original := Preset{Name: "daily", Description: "Original", Agent: "claude", Model: "opus"}
	if err := Create(home, original); err != nil {
		t.Fatal(err)
	}
	err := Create(home, Preset{Name: "daily", Description: "Replacement", Agent: "codex", Model: "x"})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v", err)
	}
	loaded, err := Read(home, "daily")
	if err != nil || !loaded.Equal(original) {
		t.Fatalf("loaded = %#v err = %v", loaded, err)
	}
	entries, _ := os.ReadDir(Dir(home))
	if len(entries) != 1 {
		t.Fatalf("staging left behind: %v", entries)
	}
}

func TestValidationRejectsKnownUnsupportedCombinations(t *testing.T) {
	for _, preset := range []Preset{
		{Name: "bad name", Description: "x", Agent: "codex", Model: "m"},
		{Name: "blank", Description: "  \n ", Agent: "codex", Model: "m"},
		{Name: "dsh", Description: "x", Agent: "dsh", Model: "m"},
		{Name: "pi-off", Description: "x", Agent: "pi", Model: "p/m", Effort: "off"},
		{Name: "pi-minimal", Description: "x", Agent: "pi", Model: "p/m", Effort: "minimal"},
		{Name: "pi-ultra", Description: "x", Agent: "pi", Model: "p/m", Effort: "ultra"},
		{Name: "pi-fuzzy", Description: "x", Agent: "pi", Model: "model"},
		{Name: "gemini-effort", Description: "x", Agent: "gemini", Model: "m", Effort: "high"},
		{Name: "claude-ultra", Description: "x", Agent: "claude", Model: "m", Effort: "ultra"},
		{Name: "no-model", Description: "x", Agent: "codex"},
	} {
		if err := preset.Validate(); err == nil {
			t.Fatalf("accepted %#v", preset)
		}
		if err := Create(t.TempDir(), preset); err == nil {
			t.Fatalf("saved %#v", preset)
		}
	}
}

func TestPiAcceptsOnlyCheckableThinkingLevels(t *testing.T) {
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		if err := (Preset{Name: "pi", Description: "x", Agent: "pi", Model: "p/m", Effort: effort}).Validate(); err != nil {
			t.Fatal(effort, err)
		}
	}
}

func TestLoadReportsBrokenEntriesWithoutHidingValidOnes(t *testing.T) {
	home := t.TempDir()
	if err := Create(home, Preset{Name: "good", Description: "Works", Agent: "codex", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		os.MkdirAll(filepath.Join(Dir(home), name), 0o700)
		if content != "" {
			os.WriteFile(Path(home, name), []byte(content), 0o600)
		}
	}
	write("missing", "")
	write("no-front", "just text")
	write("unknown", "---\nagent: codex\nmodel: m\ncolor: blue\n---\nx\n")
	write("named", "---\nname: other\nagent: codex\nmodel: m\n---\nx\n")
	write("bad name", "---\nagent: codex\nmodel: m\n---\nx\n")
	write(".staging-abc", "")
	os.WriteFile(filepath.Join(Dir(home), "stray.txt"), []byte("x"), 0o600)
	valid, problems, err := Load(home)
	if err != nil || len(valid) != 1 || valid[0].Name != "good" {
		t.Fatalf("valid = %#v err = %v", valid, err)
	}
	want := map[string]string{"missing": "PRESET.md is missing", "no-front": "front matter", "unknown": `unknown field "color"`, "named": "directory name is the preset name", "bad name": "directory name must", "stray.txt": "not a preset directory"}
	if len(problems) != len(want) {
		t.Fatalf("problems = %#v", problems)
	}
	for _, problem := range problems {
		if !strings.Contains(problem.Err, want[problem.Name]) || problem.Path == "" {
			t.Fatalf("problem %#v does not mention %q", problem, want[problem.Name])
		}
	}
}

func TestRenameCopyDeleteAreIndependent(t *testing.T) {
	home := t.TempDir()
	base := Preset{Name: "review", Description: "Review", Agent: "codex", Model: "m", Effort: "low"}
	if err := Create(home, base); err != nil {
		t.Fatal(err)
	}
	copied := base
	copied.Name, copied.Effort = "review-high", "high"
	if err := Create(home, copied); err != nil {
		t.Fatal(err)
	}
	if err := Rename(home, "review", "review-high"); !errors.Is(err, ErrExists) {
		t.Fatalf("rename over existing = %v", err)
	}
	if err := Rename(home, "review", "quick"); err != nil {
		t.Fatal(err)
	}
	if err := Rename(home, "quick", "Quick"); err != nil {
		t.Fatalf("case-only rename = %v", err)
	}
	if err := Delete(home, "review-high"); err != nil {
		t.Fatal(err)
	}
	valid, problems, _ := Load(home)
	if len(valid) != 1 || valid[0].Name != "Quick" || valid[0].Effort != "low" || len(problems) != 0 {
		t.Fatalf("valid = %#v problems = %#v", valid, problems)
	}
	for _, bad := range []string{"..", "../x", ".staging", ""} {
		if err := Delete(home, bad); err == nil {
			t.Fatalf("deleted %q", bad)
		}
	}
}

func TestUpdateKeepsOriginalOnInvalidInput(t *testing.T) {
	home := t.TempDir()
	base := Preset{Name: "review", Description: "Review", Agent: "codex", Model: "m"}
	Create(home, base)
	if err := Update(home, Preset{Name: "review", Description: " ", Agent: "codex", Model: "m"}); err == nil {
		t.Fatal("accepted blank description")
	}
	if loaded, _ := Read(home, "review"); !loaded.Equal(base) {
		t.Fatalf("loaded = %#v", loaded)
	}
	base.Effort = "high"
	if err := Update(home, base); err != nil {
		t.Fatal(err)
	}
	if loaded, _ := Read(home, "review"); !loaded.Equal(base) {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestLaunchArgsNeverInventEffort(t *testing.T) {
	cases := map[string][]string{
		"codex":  {"-c", `model_reasoning_effort="high"`},
		"claude": {"--effort", "high"},
		"pi":     {"--thinking", "high"},
	}
	for agent, want := range cases {
		got := Preset{Agent: agent, Effort: "high"}.LaunchArgs()
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("%s args = %v", agent, got)
		}
		if args := (Preset{Agent: agent}).LaunchArgs(); len(args) != 0 {
			t.Fatalf("%s native default args = %v", agent, args)
		}
	}
}

func TestParsePresetWithNewFields(t *testing.T) {
	data := []byte("---\nagent: codex\nmodel: test\nsandbox: false\nuser_config: false\nflags:\n  - --custom\n  - value\n---\nA test preset\n")
	p, err := Parse("test", data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Sandbox == nil || *p.Sandbox != false {
		t.Fatal("sandbox not parsed")
	}
	if p.UserConfig == nil || *p.UserConfig != false {
		t.Fatal("user_config not parsed")
	}
	if len(p.Flags) != 2 || p.Flags[0] != "--custom" || p.Flags[1] != "value" {
		t.Fatal("flags not parsed", p.Flags)
	}
}
