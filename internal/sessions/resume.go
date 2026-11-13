package sessions

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func WriteSessionFile(detail Session, agentID, dir, homeDir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create resume directory: %w", err)
	}
	id := detail.ID
	if id == "" {
		id = "resume"
	}

	switch agentID {
	case "claude":
		newID := newUUID()
		cwdEncoded := strings.ReplaceAll(detail.Workspace, "/", "-")
		claudeDir := filepath.Join(homeDir, ".claude", "projects", cwdEncoded)
		if err := os.MkdirAll(claudeDir, 0700); err != nil {
			return "", fmt.Errorf("create claude session directory: %w", err)
		}
		path := filepath.Join(claudeDir, newID+".jsonl")
		detail.ID = newID
		return newID, writeJSONLFile(path, SerializeClaude(detail))
	case "pi":
		newID := newUUID()
		workspace := detail.Workspace
		if workspace == "" {
			wd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("resolve pi session workspace: %w", err)
			}
			workspace = wd
		}
		piDir := filepath.Join(homeDir, ".pi", "agent", "sessions", piSessionDirName(workspace))
		if err := os.MkdirAll(piDir, 0700); err != nil {
			return "", fmt.Errorf("create pi session directory: %w", err)
		}
		path := filepath.Join(piDir, newID+".jsonl")
		detail.ID = newID
		detail.Workspace = workspace
		return path, writeJSONLFile(path, SerializePi(detail))
	case "gemini":
		path := filepath.Join(dir, newUUID()+".json")
		return path, writeJSONFile(path, SerializeGemini(detail))
	case "opencode":
		base := filepath.Join(dir, newUUID())
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

// piSessionDirName mirrors pi's getDefaultSessionDirPath (pi-coding-agent
// dist/core/session-manager.js) so pi's own session picker finds the file.
func piSessionDirName(workspace string) string {
	if len(workspace) > 0 && (workspace[0] == '/' || workspace[0] == '\\') {
		workspace = workspace[1:]
	}
	return "--" + strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(workspace) + "--"
}

func WriteCodexSessionFile(detail Session, codexHome, provider string) (string, error) {
	if strings.TrimSpace(provider) == "" {
		return "", fmt.Errorf("Codex model provider is required")
	}
	id := newUUID()
	now := time.Now()
	dir := filepath.Join(codexHome, "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create codex session directory: %w", err)
	}
	name := fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), id)
	detail.ID = id
	records := SerializeCodex(detail)
	records[0]["payload"].(map[string]any)["model_provider"] = provider
	return id, writeJSONLFile(filepath.Join(dir, name), records)
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
