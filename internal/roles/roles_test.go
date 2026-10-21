package roles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRole(t *testing.T, home, name, content string) {
	t.Helper()
	dir := filepath.Join(Dir(home), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRoleFileParsesDescriptionAndInstructionsOnly(t *testing.T) {
	home := t.TempDir()
	writeRole(t, home, "security-reviewer", "---\ndescription: Security review of changed code\n---\n\nFocus on injection and auth.\nReport findings by severity.\n")
	role, err := Read(home, "security-reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if role.Description != "Security review of changed code" || role.Instructions != "Focus on injection and auth.\nReport findings by severity." {
		t.Fatalf("%+v", role)
	}
}

func TestRoleRejectsExecutionFieldsReservedNamesAndEmptyParts(t *testing.T) {
	home := t.TempDir()
	cases := map[string]string{
		"model":   "---\ndescription: x\nmodel: gpt\n---\nDo it.\n",
		"effort":  "---\ndescription: x\neffort: high\n---\nDo it.\n",
		"policy":  "---\ndescription: x\npolicy: bounded-write\n---\nDo it.\n",
		"nodesc":  "---\n---\nDo it.\n",
		"noinstr": "---\ndescription: x\n---\n\n",
		"nofront": "Do it.\n",
		"multi":   "---\ndescription: |\n  a\n  b\n---\nDo it.\n",
		"big":     "---\ndescription: x\n---\n" + strings.Repeat("a", MaxInstructions+1) + "\n",
	}
	for name, content := range cases {
		writeRole(t, home, name, content)
		if _, err := Read(home, name); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if err := ValidName(GeneralPurpose); err == nil {
		t.Fatal("implicit role declarable")
	}
	writeRole(t, home, "alpha", "---\ndescription: a\n---\nA.\n")
	if _, err := Read(home, "Alpha"); err == nil {
		t.Fatal("case-variant name resolved another role")
	}
	if _, err := Read(home, "../x"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err := Read(home, "absent"); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatal(err)
	}
}

func TestLoadKeepsValidRolesBesideBrokenOnes(t *testing.T) {
	home := t.TempDir()
	if list, problems, err := Load(home); err != nil || list != nil || problems != nil {
		t.Fatal("missing directory must mean only the implicit role")
	}
	writeRole(t, home, "alpha", "---\ndescription: a\n---\nA.\n")
	writeRole(t, home, "broken", "---\ndescription: b\nagent: codex\n---\nB.\n")
	writeRole(t, home, GeneralPurpose, "---\ndescription: g\n---\nG.\n")
	os.WriteFile(filepath.Join(Dir(home), "stray.md"), []byte("x"), 0o600)
	list, problems, err := Load(home)
	if err != nil || len(list) != 1 || list[0].Name != "alpha" || len(problems) != 3 {
		t.Fatalf("%v %v %v", list, problems, err)
	}
}

func TestSymlinkedRoleDirectoryIsRejectedByLoadAndRead(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, FileName), []byte("---\ndescription: Linked\n---\nLinked instructions.\n"), 0o600)
	os.MkdirAll(Dir(home), 0o700)
	if err := os.Symlink(target, filepath.Join(Dir(home), "linked")); err != nil {
		t.Fatal(err)
	}
	list, problems, err := Load(home)
	if err != nil || len(list) != 0 || len(problems) != 1 || problems[0].Name != "linked" || !strings.Contains(problems[0].Err, "symlinks are not followed") {
		t.Fatal("Load:", list, problems, err)
	}
	if _, err := Read(home, "linked"); err == nil || !strings.Contains(err.Error(), "symlinks are not followed") {
		t.Fatal("Read accepted a symlinked role directory:", err)
	}
}
