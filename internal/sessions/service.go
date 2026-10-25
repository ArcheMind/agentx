package sessions

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	transcript "github.com/ArcheMind/agentx-protocol/go"
)

type Agent struct {
	ID        string `json:"id" yaml:"id"`
	Name      string `json:"name" yaml:"name"`
	Installed bool   `json:"installed" yaml:"installed"`
	Root      string `json:"session_root" yaml:"session_root"`
}

// Transcript shapes are owned by the shared protocol module; agentx defines
// no message shapes of its own.
type (
	ContentBlock = transcript.ContentBlock
	Message      = transcript.Message
	Detail       = transcript.Session
)

type Summary struct {
	ID           string `json:"id" yaml:"id"`
	Agent        string `json:"agent" yaml:"agent"`
	IsSubagent   bool   `json:"isSubagent,omitempty" yaml:"isSubagent,omitempty"`
	Workspace    string `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	Title        string `json:"title,omitempty" yaml:"title,omitempty"`
	StartedAt    string `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
	MessageCount int    `json:"messageCount" yaml:"messageCount"`
	Source       string `json:"source" yaml:"source"`
}

func summaryOf(detail Detail) Summary {
	return Summary{
		ID: detail.ID, Agent: detail.Provider, IsSubagent: detail.IsSubagent,
		Workspace: detail.Workspace, Title: detail.Title,
		StartedAt: detail.StartedAt, UpdatedAt: detail.UpdatedAt,
		MessageCount: detail.MessageCount, Source: detail.Source,
	}
}

type ListOptions struct {
	Agent            string
	Workspace        string
	All              bool
	IncludeSubagents bool
	Limit            int
	Sort             string
}

type Service struct {
	HomeDir  string
	LookPath func(string) (string, error)
}

func New() Service {
	home, _ := os.UserHomeDir()
	return Service{HomeDir: home, LookPath: exec.LookPath}
}

func (s Service) Agents() []Agent {
	agents := make([]Agent, 0, len(agentSpecs))
	for _, spec := range agentSpecs {
		_, err := s.lookPath(spec.binary)
		agents = append(agents, Agent{
			ID: spec.id, Name: spec.name, Installed: err == nil, Root: spec.root(s.HomeDir),
		})
	}
	return agents
}

func (s Service) List(options ListOptions) ([]Summary, error) {
	if options.Agent != "" && agentSpecByID(options.Agent) == nil {
		return nil, fmt.Errorf("unknown session agent %q", options.Agent)
	}
	if options.Sort == "" {
		options.Sort = "date"
	}
	if options.Sort != "date" && options.Sort != "messages" && options.Sort != "agent" {
		return nil, fmt.Errorf("unsupported session sort %q", options.Sort)
	}
	if options.Limit < 0 {
		return nil, fmt.Errorf("session limit must not be negative")
	}

	workspace := options.Workspace
	if workspace == "" && !options.All {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("read working directory: %w", err)
		}
	}
	if workspace != "" {
		workspace = cleanPath(workspace)
	}

	var all []Summary
	for _, spec := range agentSpecs {
		if options.Agent != "" && spec.id != options.Agent {
			continue
		}
		details, err := spec.load(s.HomeDir)
		if err != nil {
			return nil, fmt.Errorf("load %s sessions: %w", spec.id, err)
		}
		for _, detail := range details {
			if detail.IsSubagent && !options.IncludeSubagents {
				continue
			}
			if workspace != "" && !matchesWorkspace(detail, workspace) {
				continue
			}
			all = append(all, summaryOf(detail))
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		switch options.Sort {
		case "messages":
			return all[i].MessageCount > all[j].MessageCount
		case "agent":
			if all[i].Agent == all[j].Agent {
				return all[i].UpdatedAt > all[j].UpdatedAt
			}
			return all[i].Agent < all[j].Agent
		default:
			return all[i].UpdatedAt > all[j].UpdatedAt
		}
	})
	if options.Limit == 0 {
		return all, nil
	}
	counts := make(map[string]int)
	limited := make([]Summary, 0, len(all))
	for _, item := range all {
		if counts[item.Agent] >= options.Limit {
			continue
		}
		counts[item.Agent]++
		limited = append(limited, item)
	}
	return limited, nil
}

func (s Service) Info(id, source string) (Detail, error) {
	if id == "" {
		return Detail{}, errors.New("session ID is required")
	}
	if source != "" && agentSpecByID(source) == nil {
		return Detail{}, fmt.Errorf("unknown session agent %q", source)
	}
	var matches []Detail
	for _, spec := range agentSpecs {
		if source != "" && spec.id != source {
			continue
		}
		details, err := spec.load(s.HomeDir)
		if err != nil {
			return Detail{}, fmt.Errorf("load %s sessions: %w", spec.id, err)
		}
		for _, detail := range details {
			if detail.ID == id || strings.HasPrefix(detail.ID, id) {
				matches = append(matches, detail)
			}
		}
	}
	if len(matches) == 0 {
		return Detail{}, fmt.Errorf("session %q was not found", id)
	}
	if len(matches) > 1 {
		return Detail{}, fmt.Errorf("session ID %q is ambiguous; pass --source <agent>", id)
	}
	return matches[0], nil
}

func (s Service) lookPath(binary string) (string, error) {
	if s.LookPath == nil {
		return exec.LookPath(binary)
	}
	return s.LookPath(binary)
}

type agentSpec struct {
	id     string
	name   string
	binary string
	root   func(string) string
	load   func(string) ([]Detail, error)
}

var agentSpecs = []agentSpec{
	{id: "claude", name: "Claude Code", binary: "claude", root: func(home string) string { return filepath.Join(home, ".claude", "projects") }, load: loadClaude},
	{id: "codex", name: "Codex CLI", binary: "codex", root: func(home string) string { return filepath.Join(home, ".codex", "sessions") }, load: loadCodex},
	{id: "gemini", name: "Gemini CLI", binary: "gemini", root: func(home string) string { return filepath.Join(home, ".gemini", "tmp") }, load: loadGemini},
	{id: "opencode", name: "OpenCode", binary: "opencode", root: func(home string) string { return filepath.Join(home, ".local", "share", "opencode", "project") }, load: loadOpenCode},
	{id: "pi", name: "Pi Coding Agent", binary: "pi", root: func(home string) string { return filepath.Join(home, ".pi", "agent", "sessions") }, load: loadPi},
}

func agentSpecByID(id string) *agentSpec {
	for index := range agentSpecs {
		if agentSpecs[index].id == id {
			return &agentSpecs[index]
		}
	}
	return nil
}

func loadClaude(home string) ([]Detail, error) {
	root, err := loadJSONLGlob(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"), parseClaude)
	if err != nil {
		return nil, err
	}
	children, err := loadJSONLGlob(filepath.Join(home, ".claude", "projects", "*", "*", "subagents", "*.jsonl"), parseClaude)
	if err != nil {
		return nil, err
	}
	details := append(root, children...)
	sortDetails(details)
	return details, nil
}

func parseClaude(path string, records []map[string]any) (Detail, bool) {
	detail := newDetail("claude", path)
	var lastID string
	for _, record := range records {
		kind := stringValue(record["type"])
		if sidechain, _ := record["isSidechain"].(bool); sidechain {
			detail.IsSubagent = true
			if agentID := stringValue(record["agentId"]); agentID != "" {
				detail.ID = agentID
			}
		} else if detail.ID == "" {
			detail.ID = stringValue(record["sessionId"])
		}
		if detail.Workspace == "" {
			detail.Workspace = stringValue(record["cwd"])
		}
		if kind != "user" && kind != "assistant" && kind != "system" {
			continue
		}
		message, _ := record["message"].(map[string]any)
		ts := timeValue(record["timestamp"])
		parentID := stringValue(record["parentUuid"])
		if parentID == "" {
			parentID = lastID
		}

		if kind == "system" {
			// Most system records are bookkeeping without text; only text-bearing ones are conversation-visible.
			text := stringValue(record["content"])
			if strings.TrimSpace(text) == "" {
				continue
			}
			msgID := stringValue(record["uuid"])
			if msgID == "" {
				msgID = fmt.Sprintf("claude-%d", len(detail.Messages))
			}
			detail.Messages = append(detail.Messages, Message{
				ID: msgID, ParentID: parentID, Role: "system",
				Content: []ContentBlock{{Type: "text", Text: text}}, Timestamp: ts,
			})
			updateTimeBounds(&detail, ts)
			lastID = msgID
			continue
		}

		if kind == "assistant" {
			blocks := mapContentBlocks(message["content"])
			if len(blocks) == 0 {
				continue
			}
			msgID := stringValue(record["uuid"])
			if msgID == "" {
				msgID = fmt.Sprintf("claude-%d", len(detail.Messages))
			}
			m := Message{
				ID: msgID, ParentID: parentID, Role: "assistant",
				Content: blocks, Timestamp: ts,
				Model:      stringValue(message["model"]),
				StopReason: stringValue(message["stop_reason"]),
			}
			detail.Messages = append(detail.Messages, m)
			updateTimeBounds(&detail, ts)
			lastID = msgID
			continue
		}

		// kind == "user": may contain tool_result blocks mixed with text
		nativeContent, _ := message["content"].([]any)
		var userBlocks []ContentBlock
		for _, item := range nativeContent {
			obj, ok := item.(map[string]any)
			if !ok {
				if s, ok := item.(string); ok {
					userBlocks = append(userBlocks, ContentBlock{Type: "text", Text: s})
				}
				continue
			}
			blockType := stringValue(obj["type"])
			if blockType == "tool_result" {
				toolUseID := stringValue(obj["tool_use_id"])
				toolName := claudeToolNameByID(&detail, toolUseID)
				toolMsgID := fmt.Sprintf("claude-%d", len(detail.Messages))
				isError, _ := obj["is_error"].(bool)
				toolMsg := Message{
					ID: toolMsgID, ParentID: lastID, Role: "tool",
					Result:    obj["content"],
					Timestamp: ts, ToolUseID: toolUseID, ToolName: toolName, IsError: isError,
				}
				detail.Messages = append(detail.Messages, toolMsg)
				updateTimeBounds(&detail, ts)
				lastID = toolMsgID
				continue
			}
			if blockType == "image" {
				if source, ok := obj["source"].(map[string]any); ok {
					userBlocks = append(userBlocks, ContentBlock{Type: "image", Source: source})
				}
				continue
			}
			userBlocks = append(userBlocks, ContentBlock{Type: "text", Text: firstNonEmpty(stringValue(obj["text"]), extractText(obj["content"]))})
		}
		// If nativeContent was nil, content is a plain string
		if nativeContent == nil {
			if s := extractText(message["content"]); s != "" {
				userBlocks = append(userBlocks, ContentBlock{Type: "text", Text: s})
			}
		}
		userBlocks = nonEmptyBlocks(userBlocks)
		if len(userBlocks) == 0 {
			continue
		}
		msgID := stringValue(record["uuid"])
		if msgID == "" {
			msgID = fmt.Sprintf("claude-%d", len(detail.Messages))
		}
		m := Message{ID: msgID, ParentID: parentID, Role: "user", Content: userBlocks, Timestamp: ts}
		detail.Messages = append(detail.Messages, m)
		updateTimeBounds(&detail, ts)
		lastID = msgID
	}
	return finishDetail(detail)
}

func loadCodex(home string) ([]Detail, error) {
	pattern := filepath.Join(home, ".codex", "sessions", "*", "*", "*", "*.jsonl")
	return loadJSONLGlob(pattern, parseCodex)
}

func parseCodex(path string, records []map[string]any) (Detail, bool) {
	detail := newDetail("codex", path)
	// Track function_call names by call_id for tool result messages
	callNames := map[string]string{}
	var lastID string
	for _, record := range records {
		kind := stringValue(record["type"])
		payload, _ := record["payload"].(map[string]any)
		if kind == "session_meta" {
			if detail.ID == "" {
				detail.ID = firstNonEmpty(stringValue(payload["id"]), stringValue(payload["session_id"]))
				detail.Workspace = stringValue(payload["cwd"])
				detail.StartedAt = timeValue(payload["timestamp"])
				detail.IsSubagent = codexSubagentSource(payload["source"])
			}
			continue
		}
		if kind != "response_item" {
			continue
		}
		ts := timeValue(record["timestamp"])
		payloadType := stringValue(payload["type"])

		if payloadType == "function_call" || payloadType == "custom_tool_call" {
			callID := stringValue(payload["call_id"])
			name := stringValue(payload["name"])
			callNames[callID] = name
			rawInput := stringValue(payload["arguments"])
			if payloadType == "custom_tool_call" {
				rawInput = stringValue(payload["input"])
			}
			var input any
			if rawInput != "" {
				var parsed map[string]any
				if json.Unmarshal([]byte(rawInput), &parsed) == nil && parsed != nil {
					input = parsed
				} else {
					input = map[string]any{"raw": rawInput}
				}
			}
			block := ContentBlock{Type: "tool_use", ID: callID, Name: name, Input: input}
			// Append to last assistant message if possible
			if n := len(detail.Messages); n > 0 && detail.Messages[n-1].Role == "assistant" {
				detail.Messages[n-1].Content = append(detail.Messages[n-1].Content, block)
			} else {
				msgID := fmt.Sprintf("codex-%d", len(detail.Messages))
				detail.Messages = append(detail.Messages, Message{
					ID: msgID, ParentID: lastID, Role: "assistant",
					Content: []ContentBlock{block}, Timestamp: ts,
				})
				updateTimeBounds(&detail, ts)
				lastID = msgID
			}
			continue
		}

		if payloadType == "reasoning" {
			var summary []string
			if items, ok := payload["summary"].([]any); ok {
				for _, item := range items {
					if obj, ok := item.(map[string]any); ok {
						if text := stringValue(obj["text"]); text != "" {
							summary = append(summary, text)
						}
					}
				}
			}
			block := ContentBlock{Type: "thinking", Text: strings.Join(summary, "\n"), Signature: stringValue(payload["encrypted_content"])}
			if n := len(detail.Messages); n > 0 && detail.Messages[n-1].Role == "assistant" {
				detail.Messages[n-1].Content = append(detail.Messages[n-1].Content, block)
			} else {
				msgID := fmt.Sprintf("codex-%d", len(detail.Messages))
				detail.Messages = append(detail.Messages, Message{
					ID: msgID, ParentID: lastID, Role: "assistant",
					Content: []ContentBlock{block}, Timestamp: ts,
				})
				updateTimeBounds(&detail, ts)
				lastID = msgID
			}
			continue
		}

		if payloadType == "function_call_output" || payloadType == "custom_tool_call_output" {
			callID := stringValue(payload["call_id"])
			msgID := fmt.Sprintf("codex-%d", len(detail.Messages))
			m := Message{
				ID: msgID, ParentID: lastID, Role: "tool",
				Result:    payload["output"],
				Timestamp: ts, ToolUseID: callID, ToolName: callNames[callID],
			}
			detail.Messages = append(detail.Messages, m)
			updateTimeBounds(&detail, ts)
			lastID = msgID
			continue
		}

		if payloadType != "message" {
			continue
		}
		role := stringValue(payload["role"])
		if role != "user" && role != "assistant" && role != "developer" {
			continue
		}
		if role == "developer" {
			role = "system"
		}
		blocks := codexContentBlocks(payload["content"], role)
		if len(blocks) == 0 {
			continue
		}
		msgID := stringValue(payload["id"])
		if msgID == "" {
			msgID = fmt.Sprintf("codex-%d", len(detail.Messages))
		}
		m := Message{ID: msgID, ParentID: lastID, Role: role, Content: blocks, Timestamp: ts}
		detail.Messages = append(detail.Messages, m)
		updateTimeBounds(&detail, ts)
		lastID = msgID
	}
	return finishDetail(detail)
}

func loadGemini(home string) ([]Detail, error) {
	pattern := filepath.Join(home, ".gemini", "tmp", "*", "chats", "session-*.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var details []Detail
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var record map[string]any
		if json.Unmarshal(data, &record) != nil {
			continue
		}
		detail, ok := parseGeminiRecord(record)
		if !ok {
			continue
		}
		detail.Source = path
		rootFile := filepath.Join(filepath.Dir(filepath.Dir(path)), ".project_root")
		if data, err := os.ReadFile(rootFile); err == nil {
			detail.Workspace = strings.TrimSpace(string(data))
		}
		details = append(details, detail)
	}
	sortDetails(details)
	return details, nil
}

func loadPi(home string) ([]Detail, error) {
	pattern := filepath.Join(home, ".pi", "agent", "sessions", "*", "*.jsonl")
	return loadJSONLGlob(pattern, parsePi)
}

func parsePi(path string, records []map[string]any) (Detail, bool) {
	detail := newDetail("pi", path)
	var lastID string
	for _, record := range records {
		kind := stringValue(record["type"])
		if kind == "session" {
			detail.ID = stringValue(record["id"])
			detail.Workspace = stringValue(record["cwd"])
			detail.StartedAt = timeValue(record["timestamp"])
			continue
		}
		if kind != "message" {
			continue
		}
		message, _ := record["message"].(map[string]any)
		role := stringValue(message["role"])
		// Pi stores the tree id/parentId on the record. The record timestamp is write time and
		// differs from the message's own epoch-millis timestamp, which is the event time we keep.
		ts := firstNonEmpty(timeValue(message["timestamp"]), timeValue(record["timestamp"]))
		parentID := stringValue(record["parentId"])
		if parentID == "" {
			parentID = lastID
		}
		recordID := stringValue(record["id"])

		if role == "toolResult" {
			toolCallID := stringValue(message["toolCallId"])
			toolName := stringValue(message["toolName"])
			msgID := firstNonEmpty(recordID, fmt.Sprintf("pi-%d", len(detail.Messages)))
			isError, _ := message["isError"].(bool)
			m := Message{
				ID: msgID, ParentID: parentID, Role: "tool",
				Result:    message["content"],
				Timestamp: ts, ToolUseID: toolCallID, ToolName: toolName, IsError: isError,
			}
			detail.Messages = append(detail.Messages, m)
			updateTimeBounds(&detail, ts)
			lastID = msgID
			continue
		}

		if role == "assistant" {
			blocks := piAssistantBlocks(message["content"])
			if len(blocks) == 0 {
				continue
			}
			msgID := firstNonEmpty(recordID, fmt.Sprintf("pi-%d", len(detail.Messages)))
			m := Message{
				ID: msgID, ParentID: parentID, Role: "assistant",
				Content: blocks, Timestamp: ts,
				Model:      stringValue(message["model"]),
				StopReason: stringValue(message["stopReason"]),
			}
			detail.Messages = append(detail.Messages, m)
			updateTimeBounds(&detail, ts)
			lastID = msgID
			continue
		}

		if role != "user" {
			continue
		}
		var blocks []ContentBlock
		if text := extractText(message["content"]); text != "" {
			blocks = append(blocks, ContentBlock{Type: "text", Text: text})
		}
		if len(blocks) == 0 {
			continue
		}
		msgID := firstNonEmpty(recordID, fmt.Sprintf("pi-%d", len(detail.Messages)))
		m := Message{ID: msgID, ParentID: parentID, Role: "user", Content: blocks, Timestamp: ts}
		detail.Messages = append(detail.Messages, m)
		updateTimeBounds(&detail, ts)
		lastID = msgID
	}
	return finishDetail(detail)
}

func loadOpenCode(home string) ([]Detail, error) {
	pattern := filepath.Join(home, ".local", "share", "opencode", "project", "*", "storage", "session", "info", "*.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var details []Detail
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var info map[string]any
		if json.Unmarshal(data, &info) != nil {
			continue
		}
		detail := newDetail("opencode", path)
		detail.ID = stringValue(info["id"])
		detail.IsSubagent = stringValue(info["parentID"]) != ""
		detail.Title = stringValue(info["title"])
		if times, ok := info["time"].(map[string]any); ok {
			detail.StartedAt = timeValue(times["created"])
			detail.UpdatedAt = timeValue(times["updated"])
		}
		sessionRoot := filepath.Dir(filepath.Dir(path))
		messagePattern := filepath.Join(sessionRoot, "message", detail.ID, "*.json")
		messagePaths, _ := filepath.Glob(messagePattern)
		type openCodeMessage struct {
			id        string
			role      string
			timestamp string
		}
		var messages []openCodeMessage
		for _, messagePath := range messagePaths {
			data, readErr := os.ReadFile(messagePath)
			if readErr != nil {
				return nil, readErr
			}
			var message map[string]any
			if json.Unmarshal(data, &message) != nil {
				continue
			}
			if detail.Workspace == "" {
				if pathInfo, ok := message["path"].(map[string]any); ok {
					detail.Workspace = firstNonEmpty(stringValue(pathInfo["root"]), stringValue(pathInfo["cwd"]))
				}
			}
			timestamp := ""
			if times, ok := message["time"].(map[string]any); ok {
				timestamp = timeValue(times["created"])
			}
			messages = append(messages, openCodeMessage{id: stringValue(message["id"]), role: stringValue(message["role"]), timestamp: timestamp})
		}
		sort.Slice(messages, func(i, j int) bool { return messages[i].timestamp < messages[j].timestamp })
		var lastID string
		for _, message := range messages {
			partPattern := filepath.Join(sessionRoot, "part", detail.ID, message.id, "*.json")
			partPaths, _ := filepath.Glob(partPattern)
			sort.Strings(partPaths)
			var blocks []ContentBlock
			var toolMessages []Message
			for _, partPath := range partPaths {
				data, readErr := os.ReadFile(partPath)
				if readErr != nil {
					return nil, readErr
				}
				var part map[string]any
				if json.Unmarshal(data, &part) != nil {
					continue
				}
				partType := stringValue(part["type"])
				switch partType {
				case "text":
					blocks = append(blocks, ContentBlock{Type: "text", Text: stringValue(part["text"])})
				case "step-start":
					blocks = append(blocks, ContentBlock{Type: "tool_use", ID: stringValue(part["id"]), Name: "step"})
				case "step-finish":
					tokenInfo := ""
					if tokens, ok := part["tokens"].(map[string]any); ok {
						data, _ := json.Marshal(tokens)
						tokenInfo = string(data)
					}
					toolMsgID := fmt.Sprintf("oc-%d-tool", len(detail.Messages)+len(toolMessages))
					toolMessages = append(toolMessages, Message{
						ID: toolMsgID, Role: "tool",
						Result:    tokenInfo,
						Timestamp: message.timestamp,
						ToolUseID: stringValue(part["id"]), ToolName: "step",
					})
				case "patch":
					var input map[string]any
					if h := stringValue(part["hash"]); h != "" {
						input = map[string]any{"hash": h}
					}
					if files, ok := part["files"].([]any); ok {
						if input == nil {
							input = map[string]any{}
						}
						input["files"] = files
					}
					blocks = append(blocks, ContentBlock{Type: "tool_use", ID: stringValue(part["id"]), Name: "patch", Input: input})
				}
			}
			blocks = nonEmptyBlocks(blocks)
			if len(blocks) == 0 && len(toolMessages) == 0 {
				continue
			}
			if len(blocks) > 0 {
				msgID := fmt.Sprintf("oc-%d", len(detail.Messages))
				m := Message{ID: msgID, ParentID: lastID, Role: message.role, Content: blocks, Timestamp: message.timestamp}
				detail.Messages = append(detail.Messages, m)
				updateTimeBounds(&detail, message.timestamp)
				lastID = msgID
			}
			for i := range toolMessages {
				toolMessages[i].ParentID = lastID
				detail.Messages = append(detail.Messages, toolMessages[i])
				updateTimeBounds(&detail, message.timestamp)
				lastID = toolMessages[i].ID
			}
		}
		if finished, ok := finishDetail(detail); ok {
			details = append(details, finished)
		}
	}
	sortDetails(details)
	return details, nil
}

func loadJSONLGlob(pattern string, parse func(string, []map[string]any) (Detail, bool)) ([]Detail, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var details []Detail
	for _, path := range paths {
		records, err := readJSONL(path)
		if err != nil {
			return nil, err
		}
		if detail, ok := parse(path, records); ok {
			details = append(details, detail)
		}
	}
	sortDetails(details)
	return details, nil
}

func readJSONL(path string) ([]map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	var records []map[string]any
	for scanner.Scan() {
		var record map[string]any
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func newDetail(agent, source string) Detail {
	return Detail{Provider: agent, Source: source}
}

func codexSubagentSource(value any) bool {
	source, _ := value.(map[string]any)
	_, ok := source["subagent"]
	return ok
}

func updateTimeBounds(detail *Detail, timestamp string) {
	if detail.StartedAt == "" || timestamp != "" && timestamp < detail.StartedAt {
		detail.StartedAt = timestamp
	}
	if timestamp > detail.UpdatedAt {
		detail.UpdatedAt = timestamp
	}
}

func finishDetail(detail Detail) (Detail, bool) {
	if detail.ID == "" {
		return Detail{}, false
	}
	detail.MessageCount = len(detail.Messages)
	if detail.Title == "" {
		for _, message := range detail.Messages {
			text := textContent(message.Content)
			if message.Role == "user" && isSessionTitle(text) {
				detail.Title = oneLine(text, 96)
				break
			}
		}
	}
	return detail, true
}

func extractText(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case []any:
		var parts []string
		for _, child := range item {
			if object, ok := child.(map[string]any); ok {
				kind := stringValue(object["type"])
				if kind == "thinking" || kind == "tool_use" || kind == "toolCall" {
					continue
				}
				parts = append(parts, firstNonEmpty(stringValue(object["text"]), stringValue(object["output"]), extractText(object["content"])))
				continue
			}
			parts = append(parts, extractText(child))
		}
		return strings.Join(nonEmpty(parts), "\n")
	case map[string]any:
		return firstNonEmpty(stringValue(item["text"]), stringValue(item["output"]), extractText(item["content"]))
	default:
		return ""
	}
}

func TextContent(blocks []ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// MessageText returns the plain text of any message: block text for
// conversational roles, the derived result text for tool messages.
func MessageText(m Message) string {
	if m.Role == "tool" {
		return resultText(m.Result)
	}
	return TextContent(m.Content)
}

func textContent(blocks []ContentBlock) string {
	return TextContent(blocks)
}

func mapContentBlocks(value any) []ContentBlock {
	items, ok := value.([]any)
	if !ok {
		if s, ok := value.(string); ok && s != "" {
			return []ContentBlock{{Type: "text", Text: s}}
		}
		return nil
	}
	var blocks []ContentBlock
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			if s, ok := item.(string); ok {
				blocks = append(blocks, ContentBlock{Type: "text", Text: s})
			}
			continue
		}
		kind := stringValue(obj["type"])
		switch kind {
		case "thinking":
			blocks = append(blocks, ContentBlock{
				Type:      "thinking",
				Text:      stringValue(obj["thinking"]),
				Signature: stringValue(obj["signature"]),
			})
		case "tool_use":
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				ID:    stringValue(obj["id"]),
				Name:  stringValue(obj["name"]),
				Input: obj["input"],
			})
		case "text":
			blocks = append(blocks, ContentBlock{Type: "text", Text: stringValue(obj["text"])})
		default:
			if text := firstNonEmpty(stringValue(obj["text"]), stringValue(obj["output"])); text != "" {
				blocks = append(blocks, ContentBlock{Type: "text", Text: text})
			}
		}
	}
	return blocks
}

func claudeToolNameByID(detail *Detail, toolUseID string) string {
	for i := len(detail.Messages) - 1; i >= 0; i-- {
		for _, b := range detail.Messages[i].Content {
			if b.Type == "tool_use" && b.ID == toolUseID {
				return b.Name
			}
		}
	}
	return ""
}

func codexContentBlocks(value any, role string) []ContentBlock {
	items, ok := value.([]any)
	if !ok {
		if s, ok := value.(string); ok && s != "" {
			return []ContentBlock{{Type: "text", Text: s}}
		}
		return nil
	}
	var blocks []ContentBlock
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind := stringValue(obj["type"])
		text := stringValue(obj["text"])
		if kind == "input_text" || kind == "output_text" {
			if text != "" {
				blocks = append(blocks, ContentBlock{Type: "text", Text: text})
			}
		} else if text != "" {
			blocks = append(blocks, ContentBlock{Type: "text", Text: text})
		}
	}
	return blocks
}

func piAssistantBlocks(value any) []ContentBlock {
	items, ok := value.([]any)
	if !ok {
		if s, ok := value.(string); ok && s != "" {
			return []ContentBlock{{Type: "text", Text: s}}
		}
		return nil
	}
	var blocks []ContentBlock
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind := stringValue(obj["type"])
		switch kind {
		case "thinking":
			blocks = append(blocks, ContentBlock{
				Type:      "thinking",
				Text:      stringValue(obj["thinking"]),
				Signature: stringValue(obj["thinkingSignature"]),
			})
		case "toolCall":
			id := stringValue(obj["id"])
			name := stringValue(obj["name"])
			var input any
			if args, ok := obj["arguments"].(map[string]any); ok {
				input = args
			}
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				ID:    id,
				Name:  name,
				Input: input,
			})
		case "text":
			blocks = append(blocks, ContentBlock{Type: "text", Text: stringValue(obj["text"])})
		default:
			if text := stringValue(obj["text"]); text != "" {
				blocks = append(blocks, ContentBlock{Type: "text", Text: text})
			}
		}
	}
	return blocks
}

func nonEmptyBlocks(blocks []ContentBlock) []ContentBlock {
	result := blocks[:0]
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) == "" {
			continue
		}
		result = append(result, b)
	}
	return result
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func timeValue(value any) string {
	switch item := value.(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, item); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano)
		}
		return item
	case float64:
		seconds := int64(item) / 1000
		nanos := (int64(item) % 1000) * int64(time.Millisecond)
		return time.Unix(seconds, nanos).UTC().Format(time.RFC3339Nano)
	case json.Number:
		value, _ := strconv.ParseInt(item.String(), 10, 64)
		return timeValue(float64(value))
	default:
		return ""
	}
}

func sortDetails(details []Detail) {
	sort.SliceStable(details, func(i, j int) bool { return details[i].UpdatedAt > details[j].UpdatedAt })
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func matchesWorkspace(detail Detail, workspace string) bool {
	if cleanPath(detail.Workspace) == workspace {
		return true
	}
	if detail.Provider != "gemini" || detail.Workspace != "" {
		return false
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(workspace)))
	return filepath.Base(filepath.Dir(filepath.Dir(detail.Source))) == hash
}

func oneLine(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max-1]) + "…"
}

func isSessionTitle(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.HasPrefix(value, "# AGENTS.md instructions") && !strings.HasPrefix(value, "<environment_context>")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nonEmpty(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

// ParseRecords parses raw records from a native agent format into a unified Detail.
// For JSONL agents (claude, codex, pi), records is the list of parsed JSON objects.
// For JSON agents (gemini), records should contain a single top-level object.
// For opencode, this returns false since opencode uses a file-per-message layout.
func ParseRecords(agent string, records []map[string]any) (Detail, bool) {
	switch agent {
	case "claude":
		return parseClaude("stdin", records)
	case "codex":
		return parseCodex("stdin", records)
	case "pi":
		return parsePi("stdin", records)
	case "gemini":
		if len(records) == 0 {
			return Detail{}, false
		}
		return parseGeminiRecord(records[0])
	default:
		return Detail{}, false
	}
}

func parseGeminiRecord(record map[string]any) (Detail, bool) {
	detail := newDetail("gemini", "stdin")
	detail.ID = stringValue(record["sessionId"])
	detail.IsSubagent = stringValue(record["kind"]) == "subagent"
	detail.Title = stringValue(record["summary"])
	detail.StartedAt = timeValue(record["startTime"])
	detail.UpdatedAt = timeValue(record["lastUpdated"])

	var lastID string
	values, _ := record["messages"].([]any)
	for _, value := range values {
		message, _ := value.(map[string]any)
		role := stringValue(message["type"])
		if role == "gemini" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" {
			continue
		}
		ts := timeValue(message["timestamp"])
		msgID := fmt.Sprintf("gemini-%d", len(detail.Messages))

		if role == "assistant" {
			var blocks []ContentBlock
			var toolMessages []Message
			if text := extractText(message["content"]); text != "" {
				blocks = append(blocks, ContentBlock{Type: "text", Text: text})
			}
			if toolCalls, ok := message["toolCalls"].([]any); ok {
				for _, tc := range toolCalls {
					call, _ := tc.(map[string]any)
					callID := stringValue(call["id"])
					callName := stringValue(call["name"])
					blocks = append(blocks, ContentBlock{Type: "tool_use", ID: callID, Name: callName, Input: call["args"]})
					if results, ok := call["result"].([]any); ok {
						for _, r := range results {
							resp, _ := r.(map[string]any)
							fr, _ := resp["functionResponse"].(map[string]any)
							toolMsg := Message{
								ParentID: msgID, Role: "tool",
								Result:    extractText(fr["response"]),
								Timestamp: ts, ToolUseID: callID, ToolName: callName,
							}
							toolMessages = append(toolMessages, toolMsg)
						}
					}
				}
			}
			if len(blocks) == 0 {
				continue
			}
			m := Message{ID: msgID, ParentID: lastID, Role: role, Content: blocks, Timestamp: ts}
			detail.Messages = append(detail.Messages, m)
			updateTimeBounds(&detail, ts)
			lastID = msgID
			for _, toolMessage := range toolMessages {
				toolMessage.ID = fmt.Sprintf("gemini-%d", len(detail.Messages))
				detail.Messages = append(detail.Messages, toolMessage)
				updateTimeBounds(&detail, ts)
				lastID = toolMessage.ID
			}
		} else {
			var blocks []ContentBlock
			if text := extractText(message["content"]); text != "" {
				blocks = append(blocks, ContentBlock{Type: "text", Text: text})
			}
			if len(blocks) == 0 {
				continue
			}
			m := Message{ID: msgID, ParentID: lastID, Role: role, Content: blocks, Timestamp: ts}
			detail.Messages = append(detail.Messages, m)
			updateTimeBounds(&detail, ts)
			lastID = msgID
		}
	}
	return finishDetail(detail)
}
