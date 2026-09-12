package drivers

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

//go:embed pi_sdk_login.mjs
var piSDKLoginScript string

//go:embed pi_sdk_providers.mjs
var piSDKProvidersScript string

type PiSDKAuth struct {
	ProviderAuth
	ConfiguredProviders PiConfiguredProviderSource
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

func (d PiSDKAuth) SupportsStatus() bool {
	return true
}

func (d PiSDKAuth) Status(ctx context.Context, runner runtime.Runner) (runtime.AuthStatus, error) {
	providerSource := d.ConfiguredProviders
	if providerSource == nil {
		providerSource = PiSDKConfiguredProviders{}
	}
	ids, err := providerSource.ListConfiguredProviders(ctx, runner)
	if err != nil {
		return runtime.AuthStatus{}, err
	}
	providers := make([]runtime.AuthProvider, 0, len(ids))
	for _, id := range ids {
		providers = append(providers, runtime.AuthProvider{ID: id})
	}
	return runtime.AuthStatus{Supported: true, Providers: providers}, nil
}

type PiSDKConfiguredProviders struct{}

func (PiSDKConfiguredProviders) ListConfiguredProviders(ctx context.Context, runner runtime.Runner) ([]string, error) {
	sdkPath, err := piSDKPath()
	if err != nil {
		return nil, err
	}
	script, err := os.CreateTemp("", "agentx-pi-providers-*.mjs")
	if err != nil {
		return nil, fmt.Errorf("create Pi SDK provider adapter: %w", err)
	}
	path := script.Name()
	defer os.Remove(path)
	if _, err := script.WriteString(piSDKProvidersScript); err != nil {
		script.Close()
		return nil, fmt.Errorf("write Pi SDK provider adapter: %w", err)
	}
	if err := script.Close(); err != nil {
		return nil, fmt.Errorf("close Pi SDK provider adapter: %w", err)
	}
	result, err := runner.Execute(ctx, runtime.CommandPlan{
		Executable: "node",
		Args:       []string{path},
		Env:        map[string]string{"AX_PI_SDK": sdkPath},
	}, runtime.ExecuteOptions{})
	if err != nil {
		return nil, fmt.Errorf("read configured Pi providers: %w", err)
	}
	return piProviderIDs(result.Stdout)
}

func piProviderIDs(output string) ([]string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return []string{}, nil
	}
	providers := make([]string, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	for index, line := range lines {
		id := strings.TrimSpace(line)
		if id == "" || strings.ContainsAny(id, " \t") {
			return nil, fmt.Errorf("parse configured Pi providers: invalid provider ID on row %d", index+1)
		}
		if !seen[id] {
			seen[id] = true
			providers = append(providers, id)
		}
	}
	return providers, nil
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
