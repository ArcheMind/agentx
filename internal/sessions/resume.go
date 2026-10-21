package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func WriteSessionFile(detail Detail, agentID, dir string) (string, error) {
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
		path := filepath.Join(dir, id+".jsonl")
		return path, writeJSONLFile(path, SerializeCodex(detail))
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

func ResumeArgs(agentID, sessionFile string) []string {
	switch agentID {
	case "pi":
		return []string{"--session", sessionFile}
	case "gemini":
		return []string{"--resume", sessionFile}
	case "opencode":
		return []string{"--prompt", ""}
	default:
		return []string{"--session", sessionFile}
	}
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
