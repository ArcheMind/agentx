package presets

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const FileName = "PRESET.md"

var NamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

var ErrExists = errors.New("a preset with this name already exists")

type Preset struct {
	Name        string `json:"name" yaml:"-"`
	Description string `json:"description" yaml:"-"`
	Agent       string `json:"agent" yaml:"agent"`
	Model       string `json:"model" yaml:"model"`
	Effort      string `json:"effort,omitempty" yaml:"effort,omitempty"`
}

type Problem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Err  string `json:"error"`
}

type AgentRule struct {
	Name string
	// Efforts are the explicit levels the native CLI accepts; per-model support may still differ.
	Efforts    []string
	EffortNote string
	Subagent   bool
}

var Agents = map[string]AgentRule{
	"claude":   {Name: "Claude", Efforts: []string{"low", "medium", "high", "xhigh", "max"}, EffortNote: "Claude Code --effort levels; per-model support is not verified locally"},
	"codex":    {Name: "Codex", Efforts: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}, EffortNote: "Codex reports supported levels per model", Subagent: true},
	"gemini":   {Name: "Gemini", EffortNote: "Gemini CLI exposes no verified effort control"},
	"opencode": {Name: "OpenCode", EffortNote: "OpenCode exposes no verified effort control"},
	"pi":       {Name: "Pi", EffortNote: "Pi may silently clamp thinking levels, so explicit effort is not supported", Subagent: true},
}

var AgentOrder = []string{"claude", "codex", "gemini", "opencode", "pi"}

var piModel = regexp.MustCompile(`^[A-Za-z0-9_-]+/[^*?:[:space:]]+$`)

func Dir(home string) string {
	return filepath.Join(home, ".agentx", "presets")
}

func Path(home, name string) string {
	return filepath.Join(Dir(home), name, FileName)
}

func ValidName(name string) error {
	if !NamePattern.MatchString(name) {
		return fmt.Errorf("name must start with a letter or digit and contain only letters, digits, _ or - (at most 64 characters)")
	}
	return nil
}

func (p Preset) Validate() error {
	if err := ValidName(p.Name); err != nil {
		return err
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("description is empty; write when to use this preset below the front matter")
	}
	rule, ok := Agents[p.Agent]
	if !ok {
		return fmt.Errorf("unsupported agent %q; supported: %s", p.Agent, strings.Join(AgentOrder, ", "))
	}
	if strings.TrimSpace(p.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if strings.ContainsFunc(p.Model, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return fmt.Errorf("model %q must not contain whitespace or control characters", p.Model)
	}
	if p.Agent == "pi" && !piModel.MatchString(p.Model) {
		return fmt.Errorf("Pi requires an exact provider/model ID without a thinking suffix or wildcard")
	}
	if p.Effort != "" {
		if len(rule.Efforts) == 0 {
			return fmt.Errorf("explicit effort is not supported for %s: %s; remove effort to use the native default", rule.Name, rule.EffortNote)
		}
		if !Contains(rule.Efforts, p.Effort) {
			return fmt.Errorf("effort %q is not a %s level; supported: %s", p.Effort, rule.Name, strings.Join(rule.Efforts, ", "))
		}
	}
	return nil
}

// LaunchArgs maps the preset effort to native interactive flags.
func (p Preset) LaunchArgs() []string {
	if p.Effort == "" {
		return nil
	}
	switch p.Agent {
	case "codex":
		return []string{"-c", fmt.Sprintf("model_reasoning_effort=%q", p.Effort)}
	case "claude":
		return []string{"--effort", p.Effort}
	}
	return nil
}

func AgentName(agent string) string {
	if rule, ok := Agents[agent]; ok {
		return rule.Name
	}
	return agent
}

func EffortLabel(effort string) string {
	if effort == "" {
		return "native default"
	}
	return effort
}

func Encode(p Preset) ([]byte, error) {
	front, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(front)
	out.WriteString("---\n\n")
	out.WriteString(strings.TrimSpace(p.Description))
	out.WriteString("\n")
	return out.Bytes(), nil
}

func Parse(name string, data []byte) (Preset, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Preset{Name: name}, fmt.Errorf("%s must start with a --- front matter block holding agent, model and optional effort", FileName)
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	body := ""
	if end < 0 {
		if !strings.HasSuffix(rest, "\n---") {
			return Preset{Name: name}, fmt.Errorf("%s front matter is not closed with ---", FileName)
		}
		end = len(rest) - len("\n---")
	} else {
		body = rest[end+len("\n---\n"):]
	}
	front := rest[:end]
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(front), &fields); err != nil {
		return Preset{Name: name}, fmt.Errorf("%s front matter is not valid YAML: %w", FileName, err)
	}
	p := Preset{Name: name, Description: strings.TrimSpace(body)}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, ok := fields[key].(string)
		switch key {
		case "agent", "model", "effort":
			if !ok {
				return p, fmt.Errorf("%s field %q must be a string", FileName, key)
			}
		case "name":
			return p, fmt.Errorf("%s must not set name; the directory name is the preset name", FileName)
		case "description":
			return p, fmt.Errorf("%s must not set description in front matter; write it below the front matter", FileName)
		default:
			return p, fmt.Errorf("%s has unknown field %q; allowed: agent, model, effort", FileName, key)
		}
		switch key {
		case "agent":
			p.Agent = value
		case "model":
			p.Model = value
		case "effort":
			p.Effort = value
		}
	}
	return p, p.Validate()
}

func Read(home, name string) (Preset, error) {
	if err := entryName(name); err != nil {
		return Preset{Name: name}, err
	}
	data, err := os.ReadFile(Path(home, name))
	if os.IsNotExist(err) {
		if info, statErr := os.Stat(filepath.Join(Dir(home), name)); statErr == nil && info.IsDir() {
			return Preset{Name: name}, fmt.Errorf("%s is missing", FileName)
		}
		return Preset{Name: name}, fmt.Errorf("preset %q does not exist in %s", name, Dir(home))
	}
	if err != nil {
		return Preset{Name: name}, err
	}
	return Parse(name, data)
}

// Load returns valid presets and per-entry problems so one broken entry never hides the others.
func Load(home string) ([]Preset, []Problem, error) {
	entries, err := os.ReadDir(Dir(home))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read presets directory %s: %w", Dir(home), err)
	}
	var valid []Preset
	var problems []Problem
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(Dir(home), name)
		if !entry.IsDir() {
			problems = append(problems, Problem{Name: name, Path: path, Err: "not a preset directory; each preset is a directory containing " + FileName})
			continue
		}
		if err := ValidName(name); err != nil {
			problems = append(problems, Problem{Name: name, Path: path, Err: "directory " + err.Error()})
			continue
		}
		preset, err := Read(home, name)
		if err != nil {
			problems = append(problems, Problem{Name: name, Path: Path(home, name), Err: err.Error()})
			continue
		}
		valid = append(valid, preset)
	}
	return valid, problems, nil
}

func Create(home string, p Preset) error {
	if err := p.Validate(); err != nil {
		return err
	}
	data, err := Encode(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(home), 0o700); err != nil {
		return err
	}
	target := filepath.Join(Dir(home), p.Name)
	if _, err := os.Lstat(target); err == nil {
		return ErrExists
	}
	staging, err := os.MkdirTemp(Dir(home), ".staging-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := os.WriteFile(filepath.Join(staging, FileName), data, 0o600); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return ErrExists
	}
	if err := os.Rename(staging, target); err != nil {
		if _, statErr := os.Lstat(target); statErr == nil {
			return ErrExists
		}
		return err
	}
	return nil
}

// Update replaces the preset file in place; previous valid content survives any failure.
func Update(home string, p Preset) error {
	if err := p.Validate(); err != nil {
		return err
	}
	data, err := Encode(p)
	if err != nil {
		return err
	}
	dir := filepath.Join(Dir(home), p.Name)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("preset %q does not exist in %s", p.Name, Dir(home))
	}
	file, err := os.CreateTemp(dir, ".PRESET-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), Path(home, p.Name))
}

func Rename(home, from, to string) error {
	if err := entryName(from); err != nil {
		return err
	}
	if err := ValidName(to); err != nil {
		return err
	}
	if from == to {
		return nil
	}
	source := filepath.Join(Dir(home), from)
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("preset %q does not exist in %s", from, Dir(home))
	}
	target := filepath.Join(Dir(home), to)
	if _, err := os.Lstat(target); err == nil && !strings.EqualFold(from, to) {
		return ErrExists
	}
	// Two-step rename lets case-only renames work on case-insensitive filesystems.
	temp := filepath.Join(Dir(home), ".rename-"+randomSuffix())
	if err := os.Rename(source, temp); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		os.Rename(temp, source)
		return ErrExists
	}
	if err := os.Rename(temp, target); err != nil {
		os.Rename(temp, source)
		return err
	}
	return nil
}

func Delete(home, name string) error {
	if err := entryName(name); err != nil {
		return err
	}
	path := filepath.Join(Dir(home), name)
	if _, err := os.Lstat(path); err != nil {
		return fmt.Errorf("preset %q does not exist in %s", name, Dir(home))
	}
	return os.RemoveAll(path)
}

func entryName(name string) error {
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid preset entry %q", name)
	}
	return nil
}

func Contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func randomSuffix() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}
