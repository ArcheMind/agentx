package drivers

import (
	"context"
	"fmt"
	goruntime "runtime"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

func agyAgent() runtime.Agent {
	return runtime.Agent{
		ID: "agy", Name: "Antigravity CLI", Binary: "agy",
		Launch: NativeLaunch{Binary: "agy", ModelFlag: "--model"},
		Models: AGYModels{},
		Auth: AGYAuth{NativeAuth: NativeAuth{
			Command:           runtime.CommandPlan{Executable: "agy", Args: []string{"acp", "--login"}},
			Instruction:       "Sign in using Antigravity CLI's native login. Authentication is shared through the OS keyring.",
			LogoutCommand:     runtime.CommandPlan{Executable: "agy"},
			LogoutInstruction: "Run /logout in Antigravity CLI. This affects its shared native authentication.",
		}},
		Install:      AGYInstaller{},
		Capabilities: []runtime.Capability{runtime.CapabilityLaunch, runtime.CapabilityAuthLogin, runtime.CapabilityAuthLogout, runtime.CapabilityAuthStatus, runtime.CapabilityModelList, runtime.CapabilityModelSelect},
	}
}

type AGYAuth struct {
	NativeAuth
}

func (AGYAuth) SupportsStatus() bool { return true }

func (AGYAuth) Status(ctx context.Context, runner runtime.Runner) (runtime.AuthStatus, error) {
	models, err := (AGYModels{}).ListModels(ctx, runner)
	if err != nil {
		return runtime.AuthStatus{}, err
	}
	return runtime.AuthStatus{
		Supported: true,
		Providers: authProviders(len(models) > 0, runtime.AuthProvider{ID: "agy"}),
	}, nil
}

type AGYModels struct{}

func (AGYModels) ListModels(ctx context.Context, runner runtime.Runner) ([]runtime.Model, error) {
	result, err := runner.Execute(ctx, runtime.CommandPlan{Executable: "agy", Args: []string{"models"}}, runtime.ExecuteOptions{})
	if err != nil {
		return nil, fmt.Errorf("read Antigravity account models: %w", err)
	}
	models := make([]runtime.Model, 0)
	seen := make(map[string]bool)
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(stripANSI(line))
		if line == "" || line == "Fetching available models..." {
			continue
		}
		id, name, ok := strings.Cut(line, "\t")
		if !ok || id == "" || strings.ContainsAny(id, " \t\r") || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("parse Antigravity models: expected native model ID and display name separated by a tab")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, runtime.Model{ID: id, DisplayName: strings.TrimSpace(name), Source: "agy models"})
	}
	return models, nil
}

type AGYInstaller struct{}

func (AGYInstaller) PlanInstall(version string) (runtime.CommandPlan, error) {
	return agyInstallPlan(version, goruntime.GOOS)
}

func agyInstallPlan(version, platform string) (runtime.CommandPlan, error) {
	if version != "" {
		return runtime.CommandPlan{}, fmt.Errorf("Antigravity's official installer does not support selecting a version")
	}
	if platform == "windows" {
		script := `$ErrorActionPreference = 'Stop'; Invoke-RestMethod -Uri 'https://antigravity.google/cli/install.ps1' | Invoke-Expression`
		return runtime.CommandPlan{Executable: "powershell", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script}}, nil
	}
	script := `set -euo pipefail
installer=$(mktemp)
trap 'rm -f "$installer"' EXIT
curl --fail --silent --show-error --location https://antigravity.google/cli/install.sh --output "$installer"
bash "$installer"`
	return runtime.CommandPlan{Executable: "bash", Args: []string{"-c", script}}, nil
}
