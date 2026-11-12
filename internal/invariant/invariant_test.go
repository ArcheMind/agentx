package invariant

import (
	"strings"
	"testing"
)

func TestEqualPassesWhenEqual(t *testing.T) {
	if err := Equal("same", 1, 1, nil); err != nil {
		t.Fatal(err)
	}
}

func TestEqualReportsPromptWhenUnequal(t *testing.T) {
	err := Equal("two-is-not-one", 2, 1, map[string]string{"path": "/tmp/a.jsonl"})
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	for _, want := range []string{
		"Invariant: two-is-not-one",
		"invariant_test.go:",
		"Actual:\n2\n",
		"Expected:\n1\n",
		"\"path\": \"/tmp/a.jsonl\"",
		"https://github.com/ArcheMind/agentx/issues",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}
