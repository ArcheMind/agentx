package subagents

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed agentx_bridge.ts
var piDelegationBridge []byte

// WritePiBridge writes the delegation bridge extension to a stable path under home and returns it.
func WritePiBridge(home string) (string, error) {
	dir := filepath.Join(home, ".local", "state", "sessionman", "extensions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating bridge directory: %w", err)
	}
	path := filepath.Join(dir, "sessionman-bridge.ts")
	if err := os.WriteFile(path, piDelegationBridge, 0o600); err != nil {
		return "", fmt.Errorf("writing delegation bridge: %w", err)
	}
	return path, nil
}
