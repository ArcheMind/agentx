package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Evidence struct {
	Configuration      string `json:"configuration" yaml:"configuration"`
	Connection         string `json:"connection" yaml:"connection"`
	Session            string `json:"session" yaml:"session"`
	ConnectionEvidence string `json:"connection_evidence,omitempty" yaml:"connection_evidence,omitempty"`
	SessionID          string `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	SessionEvidence    string `json:"session_evidence,omitempty" yaml:"session_evidence,omitempty"`
}
type Projection struct {
	Path   string   `json:"path" yaml:"path"`
	Agents []string `json:"agents" yaml:"agents"`
}
type Resource struct {
	Name         string       `json:"name" yaml:"name"`
	Path         string       `json:"path" yaml:"path"`
	Description  string       `json:"description,omitempty" yaml:"description,omitempty"`
	Agents       []string     `json:"agents" yaml:"agents"`
	Projections  []Projection `json:"projections,omitempty" yaml:"projections,omitempty"`
	Evidence     Evidence     `json:"evidence" yaml:"evidence"`
	Observations []Evidence   `json:"observations,omitempty" yaml:"observations,omitempty"`
}
type Source struct {
	Scope     string     `json:"scope" yaml:"scope"`
	Path      string     `json:"path" yaml:"path"`
	Agents    []string   `json:"agents" yaml:"agents"`
	Preferred bool       `json:"preferred" yaml:"preferred"`
	Status    string     `json:"status" yaml:"status"`
	Error     string     `json:"error,omitempty" yaml:"error,omitempty"`
	Resources []Resource `json:"resources" yaml:"resources"`
}
type Result struct {
	Sources []Source `json:"sources" yaml:"sources"`
}

func (e Evidence) Verified() Evidence {
	if e.ConnectionEvidence == "" {
		e.Connection = "unconfirmed"
	}
	if e.SessionID == "" || e.SessionEvidence == "" {
		e.Session = "unconfirmed"
	}
	return e
}
func Configured() Evidence {
	return Evidence{Configuration: "discovered", Connection: "unconfirmed", Session: "unconfirmed"}
}
func Physical(path string) string {
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		p = path
	}
	p, _ = filepath.Abs(p)
	return filepath.Clean(p)
}
func Union(values []string, extra ...string) []string {
	for _, value := range extra {
		found := false
		for _, old := range values {
			if old == value {
				found = true
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

// Normalize uses source identity, never resource names or contents, to merge projections.
func Normalize(input []Source) Result {
	result := Result{Sources: []Source{}}
	input = append([]Source{}, input...)
	sort.SliceStable(input, func(i, j int) bool {
		if input[i].Preferred != input[j].Preferred {
			return input[i].Preferred
		}
		return len(input[i].Path) < len(input[j].Path)
	})
	for _, source := range input {
		source.Resources = append([]Resource{}, source.Resources...)
		for i := range source.Resources {
			source.Resources[i].Evidence = source.Resources[i].Evidence.Verified()
			source.Resources[i].Observations = mergeObservations(source.Resources[i].Observations, []Evidence{source.Resources[i].Evidence})
		}
		originalCount := len(source.Resources)
		remaining := []Resource{}
		for _, item := range source.Resources {
			found := false
			for si := range result.Sources {
				prior := &result.Sources[si]
				if prior.Scope != source.Scope {
					continue
				}
				for ri := range prior.Resources {
					existing := &prior.Resources[ri]
					if item.Path != "" && existing.Name == item.Name && existing.Path == item.Path {
						mergeResource(existing, item)
						prior.Agents = Union(prior.Agents, item.Agents...)
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				remaining = append(remaining, item)
			}
		}
		source.Resources = remaining
		if originalCount > 0 && len(remaining) == 0 && source.Status == "ok" {
			continue
		}
		index := -1
		for i, prior := range result.Sources {
			if prior.Scope == source.Scope && prior.Path == source.Path {
				index = i
				break
			}
		}
		if index < 0 {
			result.Sources = append(result.Sources, source)
			continue
		}
		prior := &result.Sources[index]
		prior.Agents = Union(prior.Agents, source.Agents...)
		prior.Preferred = prior.Preferred || source.Preferred
		if source.Status == "failed" {
			prior.Status = "failed"
			prior.Error = strings.TrimSpace(prior.Error + " " + source.Error)
		}
		for _, item := range source.Resources {
			found := false
			for i := range prior.Resources {
				existing := &prior.Resources[i]
				if existing.Name == item.Name && existing.Path == item.Path {
					mergeResource(existing, item)
					found = true
					break
				}
			}
			if !found {
				prior.Resources = append(prior.Resources, item)
			}
		}
	}
	sort.SliceStable(result.Sources, func(i, j int) bool {
		a, b := result.Sources[i], result.Sources[j]
		if a.Scope != b.Scope {
			return a.Scope == "project"
		}
		if a.Preferred != b.Preferred {
			return a.Preferred
		}
		return a.Path < b.Path
	})
	for i := range result.Sources {
		source := &result.Sources[i]
		sort.Slice(source.Resources, func(i, j int) bool { return source.Resources[i].Name < source.Resources[j].Name })
		if source.Status == "empty" && len(source.Resources) > 0 {
			source.Status = "ok"
		}
		if source.Status == "ok" && len(source.Resources) == 0 {
			source.Status = "empty"
		}
	}
	return result
}

type Selection struct{ Scope, Source, Path string }
type Detail struct {
	Source   Source   `json:"source" yaml:"source"`
	Resource Resource `json:"resource" yaml:"resource"`
}

func (r Result) Show(name string, selection Selection) (Detail, error) {
	matches := []Detail{}
	for _, source := range r.Sources {
		if selection.Scope != "" && source.Scope != selection.Scope {
			continue
		}
		if selection.Source != "" && filepath.Clean(source.Path) != filepath.Clean(selection.Source) {
			continue
		}
		for _, item := range source.Resources {
			if item.Name == name && (selection.Path == "" || filepath.Clean(item.Path) == filepath.Clean(selection.Path)) {
				matches = append(matches, Detail{source, item})
			}
		}
	}
	if len(matches) == 0 {
		return Detail{}, fmt.Errorf("resource %q not found in discovered sources", name)
	}
	if len(matches) > 1 {
		choices := []string{}
		for _, item := range matches {
			choices = append(choices, fmt.Sprintf("--scope %s --source %q --path %q", item.Source.Scope, item.Source.Path, item.Resource.Path))
		}
		return Detail{}, fmt.Errorf("resource %q is ambiguous; select a source: %s", name, strings.Join(choices, "; "))
	}
	matches[0].Source.Resources = nil
	return matches[0], nil
}

// ProjectDirectories follows only the current ancestor chain, stopping at the nearest repository boundary.
func ProjectDirectories(project string) []string {
	path, err := filepath.Abs(project)
	if err != nil {
		return []string{project}
	}
	result := []string{}
	for {
		result = append(result, path)
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return result
}

func UserDirectory(home, agent string) string {
	variable, fallback := "", ""
	switch agent {
	case "claude":
		variable, fallback = "CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude")
	case "codex":
		variable, fallback = "CODEX_HOME", filepath.Join(home, ".codex")
	case "dsh":
		variable, fallback = "DSH_HOME", filepath.Join(home, ".dsh")
	case "pi":
		variable, fallback = "PI_CODING_AGENT_DIR", filepath.Join(home, ".pi", "agent")
	case "gemini":
		if value := os.Getenv("GEMINI_CLI_HOME"); value != "" {
			home = value
		}
		return filepath.Join(home, ".gemini")
	case "opencode":
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "opencode")
	}
	if value := os.Getenv(variable); value != "" {
		return value
	}
	return fallback
}

func mergeProjections(prior, extra []Projection) []Projection {
	for _, candidate := range extra {
		found := false
		for i := range prior {
			if prior[i].Path == candidate.Path {
				prior[i].Agents = Union(prior[i].Agents, candidate.Agents...)
				found = true
				break
			}
		}
		if !found {
			prior = append(prior, candidate)
		}
	}
	return prior
}

func mergeObservations(prior, extra []Evidence) []Evidence {
	for _, e := range extra {
		found := false
		for _, old := range prior {
			if old == e {
				found = true
				break
			}
		}
		if !found {
			prior = append(prior, e)
		}
	}
	return prior
}
func summarizeEvidence(observations []Evidence) Evidence {
	result := Configured()
	sessions := map[string]Evidence{}
	for _, item := range observations {
		item = item.Verified()
		if item.ConnectionEvidence != "" {
			result.Connection = item.Connection
			result.ConnectionEvidence = item.ConnectionEvidence
		}
		if item.SessionID != "" && item.SessionEvidence != "" {
			sessions[item.SessionID] = item
		}
	}
	if len(sessions) == 1 {
		for _, item := range sessions {
			result.Session = item.Session
			result.SessionID = item.SessionID
			result.SessionEvidence = item.SessionEvidence
		}
	}
	return result
}

func mergeResource(existing *Resource, item Resource) {
	existing.Agents = Union(existing.Agents, item.Agents...)
	existing.Projections = mergeProjections(existing.Projections, item.Projections)
	existing.Observations = mergeObservations(existing.Observations, item.Observations)
	existing.Evidence = summarizeEvidence(existing.Observations)
}
