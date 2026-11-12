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

// injectPiDelegation writes the delegation bridge extension next to the Pi session and adds
// --extension plus the env vars the bridge reads at runtime. Stops at MaxDelegationDepth.
func (s *Server) injectPiDelegation(plan *runtime.CommandPlan, a *ExternalSubagentState) error {
	if s.axBinary == "" || s.delegationDepth >= MaxDelegationDepth {
		return nil
	}
	dir := filepath.Dir(a.Session)
	bridge := filepath.Join(dir, "agentx-bridge.ts")
	if err := os.WriteFile(bridge, piDelegationBridge, 0o600); err != nil {
		return fmt.Errorf("writing delegation bridge: %w", err)
	}
	plan.Args = append(plan.Args, "--extension", bridge)
	plan.Env["AX_BINARY"] = s.axBinary
	plan.Env["AX_DELEGATION_DEPTH"] = strconv.Itoa(s.delegationDepth + 1)
	return nil
}
