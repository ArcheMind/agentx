package sessions

import (
	"encoding/json"
	"path/filepath"
)

// FileEntry represents a file to be written for file-per-message serializers.
type FileEntry struct {
	Path    string         `json:"path"`
	Content map[string]any `json:"content"`
}

// SerializeClaude converts a unified Detail back to Claude Code's native JSONL records.
func SerializeClaude(detail Detail) []map[string]any {
	var records []map[string]any
	for _, m := range detail.Messages {
		switch m.Role {
		case "user":
			records = append(records, map[string]any{
				"type":       "user",
				"sessionId":  detail.ID,
				"cwd":        detail.Workspace,
				"timestamp":  m.Timestamp,
				"uuid":       m.ID,
				"parentUuid": m.ParentID,
				"message": map[string]any{
					"role":    "user",
					"content": claudeSerializeUserContent(m.Content),
				},
			})
		case "assistant":
			var blocks []any
			for _, b := range m.Content {
				switch b.Type {
				case "thinking":
					block := map[string]any{"type": "thinking", "thinking": b.Text}
					if b.Signature != "" {
						block["signature"] = b.Signature
					}
					blocks = append(blocks, block)
				case "text":
					blocks = append(blocks, map[string]any{"type": "text", "text": b.Text})
				case "tool_use":
					block := map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name}
					if b.Input != nil {
						block["input"] = b.Input
					}
					blocks = append(blocks, block)
				}
			}
			msg := map[string]any{"role": "assistant", "content": blocks}
			if m.Model != "" {
				msg["model"] = m.Model
			}
			if m.StopReason != "" {
				msg["stop_reason"] = m.StopReason
			}
			records = append(records, map[string]any{
				"type":       "assistant",
				"sessionId":  detail.ID,
				"timestamp":  m.Timestamp,
				"uuid":       m.ID,
				"parentUuid": m.ParentID,
				"message":    msg,
			})
		case "tool":
			result := map[string]any{
				"type":        "tool_result",
				"tool_use_id": m.ToolUseID,
				"content":     textContent(m.Content),
			}
			if m.IsError {
				result["is_error"] = true
			}
			records = append(records, map[string]any{
				"type":       "user",
				"sessionId":  detail.ID,
				"timestamp":  m.Timestamp,
				"uuid":       m.ID,
				"parentUuid": m.ParentID,
				"message": map[string]any{
					"role":    "user",
					"content": []any{result},
				},
			})
		}
	}
	return records
}

func claudeSerializeUserContent(blocks []ContentBlock) any {
	var texts []string
	for _, b := range blocks {
		if b.Type == "text" {
			texts = append(texts, b.Text)
		}
	}
	if len(texts) == 1 {
		return texts[0]
	}
	var result []any
	for _, b := range blocks {
		if b.Type == "text" {
			result = append(result, map[string]any{"type": "text", "text": b.Text})
		}
	}
	return result
}

// SerializeCodex converts a unified Detail back to Codex CLI's native JSONL records.
func SerializeCodex(detail Detail) []map[string]any {
	meta := map[string]any{
		"id":        detail.ID,
		"cwd":       detail.Workspace,
		"timestamp": detail.StartedAt,
	}
	if detail.IsSubagent {
		meta["source"] = map[string]any{"subagent": map[string]any{}}
	}
	records := []map[string]any{{
		"type":      "session_meta",
		"timestamp": detail.StartedAt,
		"payload":   meta,
	}}
	for _, m := range detail.Messages {
		switch m.Role {
		case "user":
			var content []any
			for _, b := range m.Content {
				if b.Type == "text" {
					content = append(content, map[string]any{"type": "input_text", "text": b.Text})
				}
			}
			records = append(records, map[string]any{
				"type":      "response_item",
				"timestamp": m.Timestamp,
				"payload": map[string]any{
					"type":    "message",
					"role":    "user",
					"content": content,
				},
			})
		case "assistant":
			var textBlocks []any
			var toolBlocks []ContentBlock
			for _, b := range m.Content {
				switch b.Type {
				case "text":
					textBlocks = append(textBlocks, map[string]any{"type": "output_text", "text": b.Text})
				case "tool_use":
					toolBlocks = append(toolBlocks, b)
				}
			}
			if len(textBlocks) > 0 {
				records = append(records, map[string]any{
					"type":      "response_item",
					"timestamp": m.Timestamp,
					"payload": map[string]any{
						"type":    "message",
						"role":    "assistant",
						"id":      m.ID,
						"content": textBlocks,
					},
				})
			}
			for _, b := range toolBlocks {
				args := ""
				if b.Input != nil {
					data, _ := json.Marshal(b.Input)
					args = string(data)
				}
				records = append(records, map[string]any{
					"type":      "response_item",
					"timestamp": m.Timestamp,
					"payload": map[string]any{
						"type":      "function_call",
						"name":      b.Name,
						"arguments": args,
						"call_id":   b.ID,
					},
				})
			}
		case "tool":
			records = append(records, map[string]any{
				"type":      "response_item",
				"timestamp": m.Timestamp,
				"payload": map[string]any{
					"type":    "function_call_output",
					"call_id": m.ToolUseID,
					"output":  textContent(m.Content),
				},
			})
		}
	}
	return records
}

// SerializeGemini converts a unified Detail back to Gemini CLI's native JSON format.
func SerializeGemini(detail Detail) map[string]any {
	toolResults := map[string]Message{}
	consumed := map[string]bool{}
	for _, m := range detail.Messages {
		if m.Role == "tool" {
			toolResults[m.ToolUseID] = m
		}
		if m.Role == "assistant" {
			for _, b := range m.Content {
				if b.Type == "tool_use" {
					consumed[b.ID] = true
				}
			}
		}
	}

	var messages []any
	for _, m := range detail.Messages {
		switch m.Role {
		case "user":
			messages = append(messages, map[string]any{
				"type":      "user",
				"content":   textContent(m.Content),
				"timestamp": m.Timestamp,
			})
		case "assistant":
			msg := map[string]any{
				"type":      "gemini",
				"timestamp": m.Timestamp,
			}
			if text := textContent(m.Content); text != "" {
				msg["content"] = text
			}
			var toolCalls []any
			for _, b := range m.Content {
				if b.Type != "tool_use" {
					continue
				}
				tc := map[string]any{
					"id":     b.ID,
					"name":   b.Name,
					"args":   b.Input,
					"status": "success",
				}
				if result, ok := toolResults[b.ID]; ok {
					tc["result"] = []any{map[string]any{
						"functionResponse": map[string]any{
							"id":   b.ID,
							"name": b.Name,
							"response": map[string]any{
								"output": textContent(result.Content),
							},
						},
					}}
				}
				toolCalls = append(toolCalls, tc)
			}
			if len(toolCalls) > 0 {
				msg["toolCalls"] = toolCalls
			}
			messages = append(messages, msg)
		case "tool":
			if consumed[m.ToolUseID] {
				continue
			}
			messages = append(messages, map[string]any{
				"type":      "gemini",
				"content":   textContent(m.Content),
				"timestamp": m.Timestamp,
			})
		}
	}

	return map[string]any{
		"sessionId": detail.ID,
		"messages":  messages,
	}
}

// SerializePi converts a unified Detail back to Pi's native JSONL records.
// Fields (id, parentId, timestamp) are placed inside the "message" sub-object
// to match where parsePi reads them.
func SerializePi(detail Detail) []map[string]any {
	records := []map[string]any{{
		"type":      "session",
		"id":        detail.ID,
		"cwd":       detail.Workspace,
		"timestamp": detail.StartedAt,
	}}
	for _, m := range detail.Messages {
		switch m.Role {
		case "user":
			var content []any
			for _, b := range m.Content {
				if b.Type == "text" {
					content = append(content, map[string]any{"type": "text", "text": b.Text})
				}
			}
			records = append(records, map[string]any{
				"type": "message",
				"message": map[string]any{
					"id":        m.ID,
					"parentId":  m.ParentID,
					"role":      "user",
					"timestamp": m.Timestamp,
					"content":   content,
				},
			})
		case "assistant":
			var content []any
			for _, b := range m.Content {
				switch b.Type {
				case "thinking":
					block := map[string]any{"type": "thinking", "thinking": b.Text}
					if b.Signature != "" {
						block["thinkingSignature"] = b.Signature
					}
					content = append(content, block)
				case "text":
					content = append(content, map[string]any{"type": "text", "text": b.Text})
				case "tool_use":
					block := map[string]any{
						"type":       "toolCall",
						"toolCallId": b.ID,
						"toolName":   b.Name,
					}
					if b.Input != nil {
						block["input"] = b.Input
					}
					content = append(content, block)
				}
			}
			msg := map[string]any{
				"id":        m.ID,
				"parentId":  m.ParentID,
				"role":      "assistant",
				"timestamp": m.Timestamp,
				"content":   content,
			}
			if m.Model != "" {
				msg["model"] = m.Model
			}
			records = append(records, map[string]any{
				"type":    "message",
				"message": msg,
			})
		case "tool":
			var content []any
			for _, b := range m.Content {
				if b.Type == "text" {
					content = append(content, map[string]any{"type": "text", "text": b.Text})
				}
			}
			msg := map[string]any{
				"role":       "toolResult",
				"toolCallId": m.ToolUseID,
				"toolName":   m.ToolName,
				"timestamp":  m.Timestamp,
				"content":    content,
			}
			if m.IsError {
				msg["isError"] = true
			}
			records = append(records, map[string]any{
				"type":    "message",
				"message": msg,
			})
		}
	}
	return records
}

// SerializeOpenCode converts a unified Detail to OpenCode's file-per-message-per-part layout.
// Returns the session info object and a flat list of file entries to write.
// basePath is the session storage root (e.g. .../storage/session/).
func SerializeOpenCode(detail Detail, basePath string) (map[string]any, []FileEntry) {
	info := map[string]any{
		"id":    detail.ID,
		"title": detail.Title,
		"time": map[string]any{
			"created": detail.StartedAt,
			"updated": detail.UpdatedAt,
		},
	}

	// Map tool_use IDs to their parent assistant message index.
	toolUseOwner := map[string]int{}
	for i, m := range detail.Messages {
		if m.Role == "assistant" {
			for _, b := range m.Content {
				if b.Type == "tool_use" {
					toolUseOwner[b.ID] = i
				}
			}
		}
	}

	// Group step-finish tool results by their parent assistant message.
	stepFinish := map[int][]Message{}
	consumedTool := map[int]bool{}
	for i, m := range detail.Messages {
		if m.Role == "tool" && m.ToolName == "step" {
			if owner, ok := toolUseOwner[m.ToolUseID]; ok {
				stepFinish[owner] = append(stepFinish[owner], m)
				consumedTool[i] = true
			}
		}
	}

	var entries []FileEntry
	for i, m := range detail.Messages {
		if consumedTool[i] {
			continue
		}
		role := m.Role
		if role == "tool" {
			role = "assistant"
		}
		msgContent := map[string]any{
			"id":        m.ID,
			"sessionID": detail.ID,
			"role":      role,
			"time":      map[string]any{"created": m.Timestamp},
		}
		if detail.Workspace != "" {
			msgContent["path"] = map[string]any{"root": detail.Workspace}
		}
		entries = append(entries, FileEntry{
			Path:    filepath.Join(basePath, "message", detail.ID, m.ID+".json"),
			Content: msgContent,
		})

		partIdx := 0
		for _, b := range m.Content {
			partID := b.ID
			if partID == "" {
				partID = m.ID + "-p" + itoa(partIdx)
			}
			var pc map[string]any
			switch b.Type {
			case "text":
				pc = map[string]any{
					"id": partID, "messageID": m.ID, "sessionID": detail.ID,
					"type": "text", "text": b.Text,
				}
			case "tool_use":
				switch b.Name {
				case "step":
					pc = map[string]any{
						"id": partID, "messageID": m.ID, "sessionID": detail.ID,
						"type": "step-start",
					}
				case "patch":
					pc = map[string]any{
						"id": partID, "messageID": m.ID, "sessionID": detail.ID,
						"type": "patch",
					}
					if input, ok := b.Input.(map[string]any); ok {
						if h, ok := input["hash"].(string); ok {
							pc["hash"] = h
						}
						if files, ok := input["files"]; ok {
							pc["files"] = files
						}
					}
				}
			}
			if pc != nil {
				entries = append(entries, FileEntry{
					Path:    filepath.Join(basePath, "part", detail.ID, m.ID, partID+".json"),
					Content: pc,
				})
				partIdx++
			}
		}

		// Append step-finish parts for tool results consumed by this assistant message.
		for _, toolMsg := range stepFinish[i] {
			pc := map[string]any{
				"id": toolMsg.ToolUseID, "messageID": m.ID, "sessionID": detail.ID,
				"type": "step-finish",
			}
			text := textContent(toolMsg.Content)
			if text != "" {
				var tokens map[string]any
				if json.Unmarshal([]byte(text), &tokens) == nil {
					pc["tokens"] = tokens
				}
			}
			entries = append(entries, FileEntry{
				Path:    filepath.Join(basePath, "part", detail.ID, m.ID, toolMsg.ToolUseID+"-finish.json"),
				Content: pc,
			})
		}
	}

	return info, entries
}

func itoa(n int) string {
	const digits = "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	return itoa(n/10) + string(digits[n%10])
}
