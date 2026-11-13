package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const agyOwner = "sessionman native agy plugin\n"

func PrepareAgyPlugin(home, executable string, write bool) (string, error) {
	root := filepath.Join(home, ".gemini", "config", "plugins", "sessionman")
	marker := filepath.Join(root, ".sessionman-owned")
	exists := false
	if info, err := os.Lstat(root); err == nil {
		exists = true
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing to overwrite non-SessionMan plugin %s", root)
		}
		data, err := os.ReadFile(marker)
		if err != nil || string(data) != agyOwner {
			return "", fmt.Errorf("refusing to overwrite non-SessionMan plugin %s", root)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if !write {
		return root, nil
	}
	command := "'" + strings.ReplaceAll(executable, "'", "'\"'\"'") + "' hook __agy_dispatch --event "
	events := map[string]any{}
	for _, event := range []string{"SessionStart", "Stop"} {
		events[event] = []any{map[string]any{"type": "command", "command": command + event}}
	}
	hookData, err := json.MarshalIndent(map[string]any{"sessionman": events}, "", "  ")
	if err != nil {
		return "", err
	}
	mcpData, err := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"sessionman": map[string]any{"command": executable, "args": []string{"mcp", "__agy_serve"}}}}, "", "  ")
	if err != nil {
		return "", err
	}
	files := map[string][]byte{"plugin.json": []byte("{\"name\":\"sessionman\",\"version\":\"1.0.0\",\"description\":\"SessionMan native MCP and portable hooks\"}\n"), "hooks.json": hookData, "mcp_config.json": mcpData, ".sessionman-owned": []byte(agyOwner)}
	if !exists {
		if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
			return "", err
		}
		staging, err := os.MkdirTemp(filepath.Dir(root), ".sessionman-plugin-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(staging)
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(staging, name), data, 0600); err != nil {
				return "", err
			}
		}
		if err := os.Rename(staging, root); err == nil {
			return root, nil
		}
		// A concurrent launch may have installed the same owned plugin first.
		data, err := os.ReadFile(marker)
		if err != nil || string(data) != agyOwner {
			return "", fmt.Errorf("refusing to overwrite non-SessionMan plugin %s", root)
		}
	}
	for name, data := range files {
		path := filepath.Join(root, name)
		// Atomic replacement keeps concurrent native launches from reading partial JSON.
		f, err := os.CreateTemp(root, ".sessionman-")
		if err != nil {
			return "", err
		}
		tmp := f.Name()
		if _, err = f.Write(data); err == nil {
			err = f.Close()
		} else {
			_ = f.Close()
		}
		if err == nil {
			err = os.Rename(tmp, path)
		}
		_ = os.Remove(tmp)
		if err != nil {
			return "", err
		}
	}
	return root, nil
}
