package sessions

import (
	"fmt"
	"os"
	"path/filepath"
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
func FindCodexRollout(homeDir, threadID string) (string, error) {
	pattern := filepath.Join(homeDir, ".codex", "sessions", "*", "*", "*", "*-"+threadID+".jsonl")
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
