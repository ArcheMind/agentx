package sessions

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type RecentGroups struct {
	Current []Summary
	Global  []Summary
}

func (s Service) RecentGroups(workspace string, limit int, includeSubagents bool) (RecentGroups, error) {
	if limit <= 0 {
		return RecentGroups{}, fmt.Errorf("recent session limit must be positive")
	}
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return RecentGroups{}, fmt.Errorf("read working directory: %w", err)
		}
	}
	workspace = cleanPath(workspace)

	var all []Summary
	for _, spec := range agentSpecs {
		items, err := recentAgentSummaries(spec, s.HomeDir, includeSubagents)
		if err != nil {
			return RecentGroups{}, fmt.Errorf("load %s session summaries: %w", spec.id, err)
		}
		all = append(all, items...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].UpdatedAt > all[j].UpdatedAt })

	result := RecentGroups{Current: []Summary{}, Global: []Summary{}}
	currentCounts := make(map[string]int)
	globalCounts := make(map[string]int)
	for _, item := range all {
		if item.IsSubagent && !includeSubagents {
			continue
		}
		if matchesWorkspaceSummary(item, workspace) {
			if currentCounts[item.Agent] < limit {
				currentCounts[item.Agent]++
				result.Current = append(result.Current, item)
			}
			continue
		}
		if globalCounts[item.Agent] < limit {
			globalCounts[item.Agent]++
			result.Global = append(result.Global, item)
		}
	}
	for _, items := range [][]Summary{result.Current, result.Global} {
		for i := range items {
			switch items[i].Agent {
			case "claude", "codex", "pi":
				item, _, err := recentSummaryParser(items[i].Agent, false)(items[i].Source)
				if err != nil {
					return RecentGroups{}, err
				}
				items[i].FirstUserMessage = item.FirstUserMessage
				items[i].LatestUserMessage = item.LatestUserMessage
			}
		}
	}
	return result, nil
}

func recentAgentSummaries(spec agentSpec, home string, includeSubagents bool) ([]Summary, error) {
	switch spec.id {
	case "claude":
		items, err := recentJSONLSummaries(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"), recentSummaryParser("claude", true))
		if err != nil || !includeSubagents {
			return items, err
		}
		children, err := recentJSONLSummaries(filepath.Join(home, ".claude", "projects", "*", "*", "subagents", "*.jsonl"), recentSummaryParser("claude", true))
		return append(items, children...), err
	case "codex":
		return recentJSONLSummaries(filepath.Join(home, ".codex", "sessions", "*", "*", "*", "*.jsonl"), recentSummaryParser("codex", true))
	case "pi":
		return recentJSONLSummaries(filepath.Join(home, ".pi", "agent", "sessions", "*", "*.jsonl"), recentSummaryParser("pi", true))
	default:
		details, err := spec.load(home)
		if err != nil {
			return nil, err
		}
		items := make([]Summary, len(details))
		for index, detail := range details {
			items[index] = SummaryOf(detail)
		}
		return items, nil
	}
}

func recentSummaryParser(agent string, firstOnly bool) func(string) (Summary, bool, error) {
	return func(path string) (Summary, bool, error) {
		switch agent {
		case "claude":
			return recentClaudeSummary(path, firstOnly)
		case "codex":
			return recentCodexSummary(path, firstOnly)
		default:
			return recentPiSummary(path, firstOnly)
		}
	}
}

func recentJSONLSummaries(pattern string, parse func(string) (Summary, bool, error)) ([]Summary, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(paths))
	for _, path := range paths {
		item, ok, err := parse(path)
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func recentClaudeSummary(path string, firstOnly bool) (Summary, bool, error) {
	item := Summary{Agent: "claude", Source: path, UpdatedAt: recentFileTimestamp(path)}
	err := scanJSONL(path, func(record map[string]any) bool {
		kind := stringValue(record["type"])
		if sidechain, _ := record["isSidechain"].(bool); sidechain {
			item.IsSubagent = true
			if agentID := stringValue(record["agentId"]); agentID != "" {
				item.ID = agentID
			}
		}
		if item.StartedAt == "" {
			item.StartedAt = timeValue(record["timestamp"])
		}
		if item.ID == "" {
			item.ID = stringValue(record["sessionId"])
		}
		if item.Workspace == "" {
			item.Workspace = stringValue(record["cwd"])
		}
		if kind == "user" {
			message, _ := record["message"].(map[string]any)
			content := extractText(message["content"])
			item.addUserMessage(content)
			if item.Title == "" && isSessionTitle(content) {
				item.Title = oneLine(content, 96)
			}
		}
		return firstOnly && item.ID != "" && item.Workspace != "" && item.Title != ""
	})
	return item, item.ID != "", err
}

func recentCodexSummary(path string, firstOnly bool) (Summary, bool, error) {
	item := Summary{Agent: "codex", Source: path, UpdatedAt: recentFileTimestamp(path)}
	err := scanJSONL(path, func(record map[string]any) bool {
		kind := stringValue(record["type"])
		payload, _ := record["payload"].(map[string]any)
		if kind == "session_meta" && item.ID == "" {
			item.ID = firstNonEmpty(stringValue(payload["id"]), stringValue(payload["session_id"]))
			item.Workspace = stringValue(payload["cwd"])
			item.StartedAt = timeValue(payload["timestamp"])
			item.IsSubagent = codexSubagentSource(payload["source"])
		}
		if kind == "response_item" && stringValue(payload["type"]) == "message" && stringValue(payload["role"]) == "user" {
			content := extractText(payload["content"])
			item.addUserMessage(content)
			if item.Title == "" && isSessionTitle(content) {
				item.Title = oneLine(content, 96)
			}
		}
		return firstOnly && item.ID != "" && item.Workspace != "" && item.Title != ""
	})
	return item, item.ID != "", err
}

func recentPiSummary(path string, firstOnly bool) (Summary, bool, error) {
	item := Summary{Agent: "pi", Source: path, UpdatedAt: recentFileTimestamp(path)}
	err := scanJSONL(path, func(record map[string]any) bool {
		kind := stringValue(record["type"])
		if kind == "session" {
			item.ID = stringValue(record["id"])
			item.Workspace = stringValue(record["cwd"])
			item.StartedAt = timeValue(record["timestamp"])
		}
		if kind == "message" {
			message, _ := record["message"].(map[string]any)
			if stringValue(message["role"]) == "user" {
				content := extractText(message["content"])
				item.addUserMessage(content)
				if item.Title == "" && isSessionTitle(content) {
					item.Title = oneLine(content, 96)
				}
			}
		}
		return firstOnly && item.ID != "" && item.Workspace != "" && item.Title != ""
	})
	return item, item.ID != "", err
}

func recentFileTimestamp(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	fallback := info.ModTime().UTC().Format(time.RFC3339Nano)
	file, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer file.Close()
	const tailSize = 256 * 1024
	offset := info.Size() - tailSize
	if offset < 0 {
		offset = 0
	}
	data := make([]byte, info.Size()-offset)
	count, _ := file.ReadAt(data, offset)
	data = data[:count]
	if offset > 0 {
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		}
	}
	lines := bytes.Split(data, []byte{'\n'})
	for index := len(lines) - 1; index >= 0; index-- {
		var record map[string]any
		if json.Unmarshal(lines[index], &record) != nil {
			continue
		}
		value := record["timestamp"]
		if message, ok := record["message"].(map[string]any); ok && value == nil {
			value = message["timestamp"]
		}
		if parsed := timeValue(value); parsed != "" {
			return parsed
		}
	}
	return fallback
}

func matchesWorkspaceSummary(item Summary, workspace string) bool {
	if cleanPath(item.Workspace) == workspace {
		return true
	}
	if item.Agent != "gemini" || item.Workspace != "" {
		return false
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(workspace)))
	return filepath.Base(filepath.Dir(filepath.Dir(item.Source))) == hash
}
