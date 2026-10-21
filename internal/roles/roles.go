package roles

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ArcheMind/agentx/internal/presets"
)

const (
	FileName = "ROLE.md"
	// GeneralPurpose is the implicit role: no role instructions are added.
	GeneralPurpose = "general-purpose"
	// MaxInstructions bounds the text passed on the native command line (Codex -c value).
	MaxInstructions = 32 * 1024
)

// Role is task framing only; it carries no agent, model, effort, permission or credential fields.
type Role struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

func Dir(home string) string {
	return filepath.Join(home, ".agentx", "roles")
}

func Path(home, name string) string {
	return filepath.Join(Dir(home), name, FileName)
}

func ValidName(name string) error {
	if name == GeneralPurpose {
		return fmt.Errorf("%s is the implicit role and cannot be declared", GeneralPurpose)
	}
	if err := presets.ValidName(name); err != nil {
		return fmt.Errorf("role %s", err)
	}
	return nil
}

func (r Role) Validate() error {
	if err := ValidName(r.Name); err != nil {
		return err
	}
	if strings.TrimSpace(r.Description) == "" {
		return fmt.Errorf("%s needs a description in its front matter: when to use this role", FileName)
	}
	if strings.ContainsAny(r.Description, "\r\n") {
		return fmt.Errorf("%s description must be one line", FileName)
	}
	if strings.TrimSpace(r.Instructions) == "" {
		return fmt.Errorf("%s has no instructions; write the role's task instructions below the front matter", FileName)
	}
	if len(r.Instructions) > MaxInstructions {
		return fmt.Errorf("%s instructions are %d bytes; at most %d are supported", FileName, len(r.Instructions), MaxInstructions)
	}
	if strings.ContainsRune(r.Instructions, 0) {
		return fmt.Errorf("%s instructions must not contain NUL bytes", FileName)
	}
	return nil
}

func Parse(name string, data []byte) (Role, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	r := Role{Name: name}
	if !strings.HasPrefix(text, "---\n") {
		return r, fmt.Errorf("%s must start with a --- front matter block holding description", FileName)
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return r, fmt.Errorf("%s front matter is not closed with --- followed by instructions", FileName)
	}
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &fields); err != nil {
		return r, fmt.Errorf("%s front matter is not valid YAML: %w", FileName, err)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key != "description" {
			return r, fmt.Errorf("%s has unknown field %q; allowed: description (a role cannot set agent, model, effort, permissions or credentials)", FileName, key)
		}
		value, ok := fields[key].(string)
		if !ok {
			return r, fmt.Errorf("%s field %q must be a string", FileName, key)
		}
		r.Description = strings.TrimSpace(value)
	}
	r.Instructions = strings.TrimSpace(rest[end+len("\n---\n"):])
	return r, r.Validate()
}

func Read(home, name string) (Role, error) {
	if err := ValidName(name); err != nil {
		return Role{Name: name}, err
	}
	entry := exactEntry(home, name)
	if entry == nil {
		return Role{Name: name}, fmt.Errorf("role %q is not declared in %s", name, Dir(home))
	}
	if !entry.IsDir() {
		return Role{Name: name}, fmt.Errorf("%s is not a role directory (symlinks are not followed); each role is a directory containing %s", filepath.Join(Dir(home), name), FileName)
	}
	data, err := os.ReadFile(Path(home, name))
	if os.IsNotExist(err) {
		return Role{Name: name}, fmt.Errorf("%s is missing", FileName)
	}
	if err != nil {
		return Role{Name: name}, err
	}
	return Parse(name, data)
}

// Load returns valid roles and per-entry problems so one broken role never hides the others.
func Load(home string) ([]Role, []presets.Problem, error) {
	entries, err := os.ReadDir(Dir(home))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read roles directory %s: %w", Dir(home), err)
	}
	var valid []Role
	var problems []presets.Problem
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		role, err := Read(home, name)
		if err != nil {
			problems = append(problems, presets.Problem{Name: name, Path: Path(home, name), Err: err.Error()})
			continue
		}
		valid = append(valid, role)
	}
	return valid, problems, nil
}

// exactEntry stops case-insensitive filesystems from resolving one role under another spelling;
// its type is the entry's own, so Load and Read judge a symlinked role directory identically.
func exactEntry(home, name string) os.DirEntry {
	entries, err := os.ReadDir(Dir(home))
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.Name() == name {
			return entry
		}
	}
	return nil
}
