package sessions

import (
	"bufio"
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
	for _, spec := range providerSpecs {
		items, err := recentProviderSummaries(spec, s.HomeDir, includeSubagents)
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
			if currentCounts[item.Provider] < limit {
				currentCounts[item.Provider]++
				result.Current = append(result.Current, item)
			}
			continue
		}
		if globalCounts[item.Provider] < limit {
			globalCounts[item.Provider]++
			result.Global = append(result.Global, item)
		}
	}
	return result, nil
}

func recentProviderSummaries(spec providerSpec, home string, includeSubagents bool) ([]Summary, error) {
	switch spec.id {
	case "claude":
		items, err := recentJSONLSummaries(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"), recentClaudeSummary)
		if err != nil || !includeSubagents {
			return items, err
		}
		children, err := recentJSONLSummaries(filepath.Join(home, ".claude", "projects", "*", "*", "subagents", "*.jsonl"), recentClaudeSummary)
		return append(items, children...), err
	case "codex":
		return recentJSONLSummaries(filepath.Join(home, ".codex", "sessions", "*", "*", "*", "*.jsonl"), recentCodexSummary)
	case "pi":
		return recentJSONLSummaries(filepath.Join(home, ".pi", "agent", "sessions", "*", "*.jsonl"), recentPiSummary)
	case "opencode":
		return recentOpenCodeSummaries(home)
	default:
		details, err := spec.load(home)
		if err != nil {
			return nil, err
		}
		items := make([]Summary, len(details))
		for index, detail := range details {
			items[index] = detail.Summary
		}
		return items, nil
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

func recentClaudeSummary(path string) (Summary, bool, error) {
	item := Summary{Provider: "claude", Source: path, UpdatedAt: recentFileTimestamp(path)}
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
		if kind == "user" && item.Title == "" {
			message, _ := record["message"].(map[string]any)
			content := flattenContent(message["content"])
			if isSessionTitle(content) {
				item.Title = oneLine(content, 96)
			}
		}
		return item.ID != "" && item.Workspace != "" && item.Title != ""
	})
	return item, item.ID != "", err
}

func recentCodexSummary(path string) (Summary, bool, error) {
	item := Summary{Provider: "codex", Source: path, UpdatedAt: recentFileTimestamp(path)}
	err := scanJSONL(path, func(record map[string]any) bool {
		kind := stringValue(record["type"])
		payload, _ := record["payload"].(map[string]any)
		if kind == "session_meta" && item.ID == "" {
			item.ID = firstNonEmpty(stringValue(payload["id"]), stringValue(payload["session_id"]))
			item.Workspace = stringValue(payload["cwd"])
			item.StartedAt = timeValue(payload["timestamp"])
			item.IsSubagent = codexSubagentSource(payload["source"])
		}
		if kind == "response_item" && stringValue(payload["type"]) == "message" && stringValue(payload["role"]) == "user" && item.Title == "" {
			content := flattenContent(payload["content"])
			if isSessionTitle(content) {
				item.Title = oneLine(content, 96)
			}
		}
		return item.ID != "" && item.Workspace != "" && item.Title != ""
	})
	return item, item.ID != "", err
}

func recentPiSummary(path string) (Summary, bool, error) {
	item := Summary{Provider: "pi", Source: path, UpdatedAt: recentFileTimestamp(path)}
	err := scanJSONL(path, func(record map[string]any) bool {
		kind := stringValue(record["type"])
		if kind == "session" {
			item.ID = stringValue(record["id"])
			item.Workspace = stringValue(record["cwd"])
			item.StartedAt = timeValue(record["timestamp"])
		}
		if kind == "message" && item.Title == "" {
			message, _ := record["message"].(map[string]any)
			if stringValue(message["role"]) == "user" {
				content := flattenContent(message["content"])
				if isSessionTitle(content) {
					item.Title = oneLine(content, 96)
				}
			}
		}
		return item.ID != "" && item.Workspace != "" && item.Title != ""
	})
	return item, item.ID != "", err
}

func scanJSONL(path string, visit func(map[string]any) bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var record map[string]any
		if json.Unmarshal(scanner.Bytes(), &record) == nil && visit(record) {
			return nil
		}
	}
	return scanner.Err()
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

func recentOpenCodeSummaries(home string) ([]Summary, error) {
	pattern := filepath.Join(home, ".local", "share", "opencode", "project", "*", "storage", "session", "info", "*.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var info map[string]any
		if json.Unmarshal(data, &info) != nil {
			continue
		}
		item := Summary{ID: stringValue(info["id"]), Provider: "opencode", IsSubagent: stringValue(info["parentID"]) != "", Title: stringValue(info["title"]), Source: path}
		if times, ok := info["time"].(map[string]any); ok {
			item.StartedAt = timeValue(times["created"])
			item.UpdatedAt = timeValue(times["updated"])
		}
		sessionRoot := filepath.Dir(filepath.Dir(path))
		messagePaths, _ := filepath.Glob(filepath.Join(sessionRoot, "message", item.ID, "*.json"))
		sort.Strings(messagePaths)
		for _, messagePath := range messagePaths {
			data, readErr := os.ReadFile(messagePath)
			if readErr != nil {
				return nil, readErr
			}
			var message map[string]any
			if json.Unmarshal(data, &message) == nil {
				if pathInfo, ok := message["path"].(map[string]any); ok {
					item.Workspace = firstNonEmpty(stringValue(pathInfo["root"]), stringValue(pathInfo["cwd"]))
				}
			}
			if item.Workspace != "" {
				break
			}
		}
		if item.ID != "" {
			items = append(items, item)
		}
	}
	return items, nil
}

func matchesWorkspaceSummary(item Summary, workspace string) bool {
	if cleanPath(item.Workspace) == workspace {
		return true
	}
	if item.Provider != "gemini" || item.Workspace != "" {
		return false
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(workspace)))
	return filepath.Base(filepath.Dir(filepath.Dir(item.Source))) == hash
}
