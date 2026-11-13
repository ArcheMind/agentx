package sessions

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func sessionSummaryParser(agent string, firstOnly bool) func(string) (Summary, bool, error) {
	return func(path string) (Summary, bool, error) {
		switch agent {
		case "claude":
			return claudeSummary(path, firstOnly)
		case "codex":
			return codexSummary(path, firstOnly)
		default:
			return piSummary(path, firstOnly)
		}
	}
}

func readJSONLSummaries(pattern string, parse func(string) (Summary, bool, error)) ([]Summary, error) {
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

func claudeSummary(path string, firstOnly bool) (Summary, bool, error) {
	item := Summary{Agent: "claude", Source: path, UpdatedAt: sessionFileTimestamp(path)}
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

func codexSummary(path string, firstOnly bool) (Summary, bool, error) {
	item := Summary{Agent: "codex", Source: path, UpdatedAt: sessionFileTimestamp(path)}
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

func piSummary(path string, firstOnly bool) (Summary, bool, error) {
	item := Summary{Agent: "pi", Source: path, UpdatedAt: sessionFileTimestamp(path)}
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

func sessionFileTimestamp(path string) string {
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
