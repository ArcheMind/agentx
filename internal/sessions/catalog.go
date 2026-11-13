package sessions

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
)

type sessionEntry struct {
	Summary
	detail *Session
}

func (s Service) catalog(agent string) ([]sessionEntry, error) {
	var entries []sessionEntry
	for _, spec := range agentSpecs {
		if agent != "" && spec.id != agent {
			continue
		}
		items, err := discoverSessions(spec, s.HomeDir)
		if err != nil {
			return nil, fmt.Errorf("load %s sessions: %w", spec.id, err)
		}
		entries = append(entries, items...)
	}
	return normalizeSessions(entries), nil
}

func discoverSessions(spec agentSpec, home string) ([]sessionEntry, error) {
	var patterns []string
	switch spec.id {
	case "claude":
		patterns = []string{
			filepath.Join(spec.root(home), "*", "*.jsonl"),
			filepath.Join(spec.root(home), "*", "*", "subagents", "*.jsonl"),
		}
	case "codex":
		patterns = []string{filepath.Join(spec.root(home), "*", "*", "*", "*.jsonl")}
	case "pi":
		patterns = []string{filepath.Join(spec.root(home), "*", "*.jsonl")}
	default:
		details, err := spec.load(home)
		if err != nil {
			return nil, err
		}
		entries := make([]sessionEntry, len(details))
		for i := range details {
			entries[i] = sessionEntry{Summary: SummaryOf(details[i]), detail: &details[i]}
		}
		return entries, nil
	}
	return discoverJSONLSessions(spec.id, patterns...)
}

func discoverJSONLSessions(agent string, patterns ...string) ([]sessionEntry, error) {
	var entries []sessionEntry
	for _, pattern := range patterns {
		items, err := readJSONLSummaries(pattern, sessionSummaryParser(agent, true))
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			entries = append(entries, sessionEntry{Summary: item})
		}
	}
	return entries, nil
}

func normalizeSessions(entries []sessionEntry) []sessionEntry {
	// Codex stores one session across rollout segments; other providers retain ambiguity.
	best := make(map[string]sessionEntry)
	out := make([]sessionEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Agent != "codex" {
			out = append(out, entry)
			continue
		}
		previous, ok := best[entry.ID]
		if !ok || entry.UpdatedAt > previous.UpdatedAt ||
			(entry.UpdatedAt == previous.UpdatedAt && entry.Source > previous.Source) {
			best[entry.ID] = entry
		}
	}
	for _, entry := range best {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		if out[i].Agent != out[j].Agent {
			return out[i].Agent < out[j].Agent
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (entry sessionEntry) visible(workspace string, includeSubagents bool) bool {
	return (includeSubagents || !entry.IsSubagent) && matchesWorkspaceSummary(entry.Summary, workspace)
}

func (entry sessionEntry) read() (Session, error) {
	var detail Session
	if entry.detail != nil {
		detail = *entry.detail
	} else {
		var err error
		detail, err = ParseNativeSessionFile(entry.Agent, entry.Source)
		if err != nil {
			return Session{}, err
		}
	}
	detail.ID = entry.ID
	detail.Provider = entry.Agent
	detail.Workspace = entry.Workspace
	detail.IsSubagent = entry.IsSubagent
	detail.StartedAt = entry.StartedAt
	detail.UpdatedAt = entry.UpdatedAt
	detail.Source = entry.Source
	return detail, nil
}

func (entry sessionEntry) summary() (Summary, error) {
	switch entry.Agent {
	case "claude", "codex", "pi":
		item, _, err := sessionSummaryParser(entry.Agent, false)(entry.Source)
		if err != nil {
			return Summary{}, err
		}
		entry.FirstUserMessage = item.FirstUserMessage
		entry.LatestUserMessage = item.LatestUserMessage
	}
	return entry.Summary, nil
}

func matchesWorkspaceSummary(item Summary, workspace string) bool {
	if workspace == "" {
		return true
	}
	if cleanPath(item.Workspace) == workspace {
		return true
	}
	if item.Agent != "gemini" || item.Workspace != "" {
		return false
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(workspace)))
	return filepath.Base(filepath.Dir(filepath.Dir(item.Source))) == hash
}
