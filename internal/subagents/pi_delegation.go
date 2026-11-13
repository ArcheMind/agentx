package subagents

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ArcheMind/agentx/internal/runtime"
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

// injectPiDelegation adds the delegation bridge extension and depth env vars for a dispatched
// Pi ExternalSubagent. Stops at MaxDelegationDepth.
func (s *Server) injectPiDelegation(plan *runtime.CommandPlan, a *ExternalSubagentState) error {
	if s.axBinary == "" || s.delegationDepth >= MaxDelegationDepth {
		return nil
	}
	path, err := WritePiBridge(s.Home)
	if err != nil {
		return err
	}
	plan.Args = append(plan.Args, "--extension", path)
	plan.Env["SM_BINARY"] = s.axBinary
	plan.Env["SM_DELEGATION_DEPTH"] = strconv.Itoa(s.delegationDepth + 1)
	return nil
}
