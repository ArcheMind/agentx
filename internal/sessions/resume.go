package sessions

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func WriteSessionFile(detail Detail, agentID, dir, homeDir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create resume directory: %w", err)
	}
	id := detail.ID
	if id == "" {
		id = "resume"
	}

	switch agentID {
	case "claude":
		path := filepath.Join(dir, id+".jsonl")
		return path, writeJSONLFile(path, SerializeClaude(detail))
	case "codex":
		newID := newUUID()
		now := time.Now()
		codexDir := filepath.Join(homeDir, ".codex", "sessions",
			now.Format("2006"), now.Format("01"), now.Format("02"))
		if err := os.MkdirAll(codexDir, 0700); err != nil {
			return "", fmt.Errorf("create codex session directory: %w", err)
		}
		name := fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), newID)
		path := filepath.Join(codexDir, name)
		detail.ID = newID
		return newID, writeJSONLFile(path, SerializeCodex(detail))
	case "pi":
		path := filepath.Join(dir, id+".jsonl")
		return path, writeJSONLFile(path, SerializePi(detail))
	case "gemini":
		path := filepath.Join(dir, id+".json")
		return path, writeJSONFile(path, SerializeGemini(detail))
	case "opencode":
		base := filepath.Join(dir, id)
		info, entries := SerializeOpenCode(detail, base)
		infoPath := filepath.Join(base, "session.json")
		if err := os.MkdirAll(filepath.Dir(infoPath), 0700); err != nil {
			return "", err
		}
		if err := writeJSONFile(infoPath, info); err != nil {
			return "", err
		}
		for _, entry := range entries {
			if err := os.MkdirAll(filepath.Dir(entry.Path), 0700); err != nil {
				return "", err
			}
			if err := writeJSONFile(entry.Path, entry.Content); err != nil {
				return "", err
			}
		}
		return base, nil
	default:
		return "", fmt.Errorf("unsupported resume target %q", agentID)
	}
}

func ResumeArgs(agentID, token string) []string {
	switch agentID {
	case "codex":
		return []string{"resume", token}
	case "claude":
		return []string{"--resume", token}
	case "pi":
		return []string{"--session", token}
	case "gemini":
		return []string{"--resume", token}
	case "opencode":
		return []string{"--prompt", ""}
	default:
		return []string{"--resume", token}
	}
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func writeJSONLFile(path string, records []map[string]any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONFile(path string, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}
