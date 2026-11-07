package sessions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ParseNativeSessionFile reads one native JSONL session file for a continue
// turn's prefix/result accounting. Only the JSONL providers are supported.
func ParseNativeSessionFile(agentID, path string) (Session, error) {
	records, err := readJSONL(path)
	if err != nil {
		return Session{}, fmt.Errorf("read %s session %s: %w", agentID, path, err)
	}
	detail, ok := ParseRecords(agentID, records)
	if !ok {
		return Session{}, fmt.Errorf("parse %s session %s: not a recognizable session", agentID, path)
	}
	return detail, nil
}

// ClaudeProjectFile is where Claude stores the session with this id for this
// workspace; the encoding must match WriteSessionFile's.
func ClaudeProjectFile(homeDir, workspace, sessionID string) string {
	encoded := strings.ReplaceAll(workspace, "/", "-")
	return filepath.Join(homeDir, ".claude", "projects", encoded, sessionID+".jsonl")
}

// FindCodexRollout locates the rollout file for a Codex thread id, newest
// first: Codex names rollout files rollout-<timestamp>-<uuid>.jsonl.
func FindCodexRollout(codexHome, threadID string) (string, error) {
	pattern := filepath.Join(codexHome, "sessions", "*", "*", "*", "*-"+threadID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no Codex rollout found for thread %s", threadID)
	}
	newest, newestTime := "", int64(0)
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		if modified := info.ModTime().UnixNano(); newest == "" || modified > newestTime {
			newest, newestTime = match, modified
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no readable Codex rollout for thread %s", threadID)
	}
	return newest, nil
}

// RechainMessages rewrites the parent chain positionally for a continue
// import. The transcript's message order is the conversation; Claude and Pi
// reconstruct a session as the newest record's ancestor chain, so a dangling
// or empty parentId (normal in delta-appended or recall-assembled
// transcripts) would orphan all history before it. Export/import fidelity is
// untouched: the serializers keep writing parentId verbatim.
func RechainMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	previous := ""
	for i, message := range messages {
		message.ParentID = previous
		previous = message.ID
		out[i] = message
	}
	return out
}

// Native APIs constrain tool ids: Anthropic requires ^[a-zA-Z0-9_-]+$ for
// tool_use.id, the OpenAI Responses API caps call_id at 64 characters, and
// Pi forwards to the same provider families. One rule satisfies all three.
var safeToolID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// SanitizeToolIDs rewrites tool ids the native APIs would reject (legacy
// harness-scoped ids like "native-<uuid>:<raw>" or composite "call_x|fc_y")
// into deterministic API-safe ids, for a continue import. The mapping is a
// pure hash of the raw id, so the same id maps identically in every turn
// (idempotent across round trips), tool_use blocks and their tool_result
// messages stay paired, and ids that are already API-safe pass untouched.
// Like RechainMessages this lives on the continue path only: the serializers
// keep ids verbatim because export/import fidelity requires it, and delta
// output keeps the agent's own ids, which its API just accepted.
func SanitizeToolIDs(messages []Message) []Message {
	out := make([]Message, len(messages))
	for i, message := range messages {
		if message.ToolUseID != "" && !safeToolID.MatchString(message.ToolUseID) {
			message.ToolUseID = sanitizedToolID(message.ToolUseID)
		}
		rewritten := false
		blocks := make([]ContentBlock, len(message.Content))
		for j, block := range message.Content {
			if block.Type == "tool_use" && block.ID != "" && !safeToolID.MatchString(block.ID) {
				block.ID = sanitizedToolID(block.ID)
				rewritten = true
			}
			blocks[j] = block
		}
		if rewritten {
			message.Content = blocks
		}
		out[i] = message
	}
	return out
}

// Codex sends a message's id as input[].id, which the Responses API requires
// to start with "msg" and cap at 64 chars. Namespaced delta ids
// ("<runId>:msg_...", 91 chars) and other agents' ids (uuids) violate it.
var codexMessageID = regexp.MustCompile(`^msg[a-zA-Z0-9_-]{0,61}$`)

// SanitizeMessageIDs maps message ids Codex would reject to "msg_" + 60 hex of
// their SHA-256, rewriting parentId with the same pure hash so the chain stays
// intact and the mapping is deterministic and idempotent across turns. No
// reverse mapping is needed: deltas are cut positionally from the native
// file, never by id. Continue path only, for the same reason as SanitizeToolIDs.
func SanitizeMessageIDs(messages []Message) []Message {
	out := make([]Message, len(messages))
	for i, message := range messages {
		message.ID = sanitizedMessageID(message.ID)
		message.ParentID = sanitizedMessageID(message.ParentID)
		out[i] = message
	}
	return out
}

func sanitizedMessageID(raw string) string {
	if raw == "" || codexMessageID.MatchString(raw) {
		return raw
	}
	sum := sha256.Sum256([]byte(raw))
	return "msg_" + hex.EncodeToString(sum[:30])
}

func sanitizedToolID(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "tid-" + hex.EncodeToString(sum[:12])
}
