package drivers

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ArcheMind/agentx/internal/runtime"
)

//go:embed pi_sdk_login.mjs
var piSDKLoginScript string

type PiSDKAuth struct {
	ProviderAuth
}

func (d PiSDKAuth) PlanLogin() runtime.AuthPlan {
	return runtime.AuthPlan{Command: runtime.CommandPlan{Executable: "node", Args: []string{"<Pi SDK login adapter>"}}}
}

func (d PiSDKAuth) Login(ctx context.Context, runner runtime.Runner, options runtime.ExecuteOptions) (runtime.CommandResult, error) {
	sdkPath, err := piSDKPath()
	if err != nil {
		return runtime.CommandResult{ExitCode: -1}, err
	}
	script, err := os.CreateTemp("", "agentx-pi-login-*.mjs")
	if err != nil {
		return runtime.CommandResult{ExitCode: -1}, fmt.Errorf("create Pi SDK login adapter: %w", err)
	}
	path := script.Name()
	defer os.Remove(path)
	if _, err := script.WriteString(piSDKLoginScript); err != nil {
		script.Close()
		return runtime.CommandResult{ExitCode: -1}, fmt.Errorf("write Pi SDK login adapter: %w", err)
	}
	if err := script.Close(); err != nil {
		return runtime.CommandResult{ExitCode: -1}, fmt.Errorf("close Pi SDK login adapter: %w", err)
	}
	return runner.Execute(ctx, runtime.CommandPlan{
		Executable: "node",
		Args:       []string{path},
		Env:        map[string]string{"AX_PI_SDK": sdkPath},
	}, options)
}

func piSDKPath() (string, error) {
	binary, err := exec.LookPath("pi")
	if err != nil {
		return "", fmt.Errorf("locate Pi SDK: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return "", fmt.Errorf("resolve Pi executable %s: %w", binary, err)
	}
	sdkPath := filepath.Join(filepath.Dir(resolved), "..", "index.js")
	if _, err := os.Stat(sdkPath); err != nil {
		return "", fmt.Errorf("locate Pi SDK beside %s: %w", resolved, err)
	}
	return sdkPath, nil
}
