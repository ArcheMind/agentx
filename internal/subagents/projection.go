package subagents

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed claude/.claude-plugin/plugin.json claude/hooks/*
var plugin embed.FS

func Prepare(home, executable string, write bool) (string, error) {
	files := map[string][]byte{}
	hash := sha256.New()
	err := fs.WalkDir(plugin, "claude", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := plugin.ReadFile(path)
		if err != nil {
			return err
		}
		files[path[len("claude/"):]] = data
		fmt.Fprintf(hash, "%s\x00%s\x00", path, data)
		return nil
	})
	if err != nil {
		return "", err
	}
	config, err := json.Marshal(map[string]string{"executable": executable})
	if err != nil {
		return "", err
	}
	files["bridge.json"] = config
	hash.Write(config)
	root := filepath.Join(home, ".cache", "agentx", "subagents", fmt.Sprintf("%x", hash.Sum(nil)[:16]))
	if !write {
		return root, nil
	}
	if _, err := os.Stat(root); err == nil {
		for path, expected := range files {
			actual, err := os.ReadFile(filepath.Join(root, path))
			if err != nil || string(actual) != string(expected) {
				return "", fmt.Errorf("AgentX subagent projection differs from its content hash: %s", root)
			}
		}
		return root, nil
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(filepath.Dir(root), ".stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	for path, data := range files {
		target := filepath.Join(stage, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return "", err
		}
	}
	if err := os.Rename(stage, root); err != nil {
		if _, statErr := os.Stat(root); statErr != nil {
			return "", err
		}
		return Prepare(home, executable, true)
	}
	return root, nil
}
