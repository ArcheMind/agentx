package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/skills"
)

func TestSkillResourceGrammarAndStructuredOutput(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	path := filepath.Join(project, ".agents", "skills", "review")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: review\ndescription: Review changes\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Skills = skills.Service{Home: home, Project: project, Runner: application.Runner}
	if err := application.Run(context.Background(), []string{"--json", "skill", "list"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"skills":`, `"name": "review"`, `"agent": "dsh"`, `"supported": false`} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("output %q does not contain %q", stdout.String(), expected)
		}
	}
	stdout.Reset()
	if err := application.Run(context.Background(), []string{"--yaml", "skill", "show", "review"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "name: review") || !strings.Contains(stdout.String(), "description: Review changes") {
		t.Fatalf("show output = %q", stdout.String())
	}
}

func TestSkillInstallDryRunUsesAgentXOutput(t *testing.T) {
	project := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: review\ndescription: Review changes\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &bytes.Buffer{})
	application.Skills = skills.Service{Home: t.TempDir(), Project: project, Runner: application.Runner}
	if err := application.Run(context.Background(), []string{"skill", "install", source, "--scope", "project", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"source_type": "local"`, `"name": "review"`, `"claude_projection":`} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("output %q does not contain %q", stdout.String(), expected)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote project state: %v", err)
	}
}

func TestSkillInstallRejectsInvalidOptions(t *testing.T) {
	application := New(false, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	for _, args := range [][]string{
		{"skill", "install"},
		{"skill", "install", "source", "--path"},
		{"skill", "install", "source", "--scope"},
		{"skill", "install", "source", "--unknown"},
	} {
		if err := application.Run(context.Background(), args); err == nil {
			t.Fatalf("args %v unexpectedly succeeded", args)
		}
	}
}
