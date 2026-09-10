package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
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
	err := application.Run(context.Background(), []string{"--yaml", "install", "codex", "--dry-run"})
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
