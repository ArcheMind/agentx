package app

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

type overviewStatus string

const (
	overviewReady        overviewStatus = "ready"
	overviewNotInstalled overviewStatus = "not_installed"
	overviewNotLoggedIn  overviewStatus = "not_logged_in"
	overviewUnknown      overviewStatus = "unknown"
)

type modelStatus string

const (
	modelsAvailable modelStatus = "available"
	modelsNone      modelStatus = "none_available"
	modelsUnknown   modelStatus = "unknown"
)

type overviewResult struct {
	Agents []agentOverview `json:"agents" yaml:"agents"`
}

type agentOverview struct {
	ID       string                 `json:"id" yaml:"id"`
	Name     string                 `json:"name" yaml:"name"`
	Status   overviewStatus         `json:"status" yaml:"status"`
	Accounts []runtime.AuthProvider `json:"accounts,omitempty" yaml:"accounts,omitempty"`
	Models   *modelsOverview        `json:"models,omitempty" yaml:"models,omitempty"`
}

type modelsOverview struct {
	Status modelStatus     `json:"status" yaml:"status"`
	Items  []overviewModel `json:"items,omitempty" yaml:"items,omitempty"`
}

type overviewModel struct {
	ID          string `json:"id" yaml:"id"`
	DisplayName string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
}

func (a App) overview(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: ax [--json|--yaml] list")
	}
	agents := a.Registry.All()
	result := overviewResult{Agents: make([]agentOverview, 0, len(agents))}
	for _, agent := range agents {
		detection := detectInstalled(agent)
		result.Agents = append(result.Agents, a.queryAgentOverview(ctx, agent, detection))
	}
	if a.Output != OutputText {
		return writeStructured(a.Stdout, result, a.Output)
	}
	return writeOverviewTree(a.Stdout, result, a.Color)
}

func (a App) detectAgent(ctx context.Context, agent runtime.Agent) runtime.Detection {
	detection := detectInstalled(agent)
	if !detection.Installed {
		return detection
	}
	result, err := a.Runner.Execute(ctx, runtime.CommandPlan{Executable: detection.Path, Args: []string{"--version"}}, runtime.ExecuteOptions{})
	if err == nil {
		detection.Version = firstLine(result.Stdout + result.Stderr)
	}
	return detection
}

func detectInstalled(agent runtime.Agent) runtime.Detection {
	detection := runtime.Detection{ID: agent.ID, Name: agent.Name, Capabilities: agent.Capabilities}
	path, err := exec.LookPath(agent.Binary)
	if err != nil {
		return detection
	}
	detection.Installed = true
	detection.Path = path
	return detection
}

func (a App) queryAgentOverview(ctx context.Context, agent runtime.Agent, detection runtime.Detection) agentOverview {
	item := agentOverview{ID: detection.ID, Name: detection.Name}
	if !detection.Installed {
		item.Status = overviewNotInstalled
		return item
	}
	if !hasCapability(agent.Capabilities, runtime.CapabilityAuthStatus) {
		item.Status = overviewUnknown
		return item
	}
	status, err := agent.Auth.Status(ctx, a.Runner)
	if err != nil {
		item.Status = overviewUnknown
		return item
	}
	if len(status.Providers) == 0 {
		item.Status = overviewNotLoggedIn
		return item
	}
	item.Status = overviewReady
	item.Accounts = status.Providers
	item.Models = a.queryModels(ctx, agent)
	return item
}

func (a App) queryModels(ctx context.Context, agent runtime.Agent) *modelsOverview {
	if !hasCapability(agent.Capabilities, runtime.CapabilityModelList) {
		return &modelsOverview{Status: modelsUnknown}
	}
	models, err := agent.Models.ListModels(ctx, a.Runner)
	if err != nil {
		return &modelsOverview{Status: modelsUnknown}
	}
	if len(models) == 0 {
		return &modelsOverview{Status: modelsNone}
	}
	items := make([]overviewModel, 0, len(models))
	for _, model := range models {
		items = append(items, overviewModel{ID: model.ID, DisplayName: model.DisplayName})
	}
	return &modelsOverview{Status: modelsAvailable, Items: items}
}

func hasCapability(capabilities []runtime.Capability, expected runtime.Capability) bool {
	for _, capability := range capabilities {
		if capability == expected {
			return true
		}
	}
	return false
}

func writeOverviewTree(writer io.Writer, result overviewResult, color bool) error {
	if _, err := fmt.Fprintln(writer, colorize(color, ansiBold, "agents")); err != nil {
		return err
	}
	for index, item := range result.Agents {
		lastAgent := index == len(result.Agents)-1
		branch, continuation := "├──", "│   "
		if lastAgent {
			branch, continuation = "└──", "    "
		}
		label := item.ID
		if item.Status == overviewReady {
			label = colorize(color, ansiGreen, label)
		} else {
			label += " " + formatOverviewStatus(item.Status, color)
		}
		if _, err := fmt.Fprintf(writer, "%s %s\n", colorize(color, ansiDim, branch), label); err != nil {
			return err
		}
		if item.Status != overviewReady {
			continue
		}
		if err := writeAccountsTree(writer, continuation, item.Accounts, color); err != nil {
			return err
		}
		if err := writeModelsTree(writer, continuation, item.Models, color); err != nil {
			return err
		}
	}
	return nil
}

func formatOverviewStatus(status overviewStatus, color bool) string {
	switch status {
	case overviewNotInstalled:
		return colorize(color, ansiDim, "(not installed)")
	case overviewNotLoggedIn:
		return colorize(color, ansiYellow, "(not logged in)")
	case overviewUnknown:
		return colorize(color, ansiRed, "(status unknown)")
	default:
		return ""
	}
}

func writeAccountsTree(writer io.Writer, prefix string, accounts []runtime.AuthProvider, color bool) error {
	branch := colorize(color, ansiDim, prefix+"├──")
	if len(accounts) == 1 {
		_, err := fmt.Fprintf(writer, "%s account: %s\n", branch, colorize(color, ansiCyan, formatAccount(accounts[0])))
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s accounts\n", branch); err != nil {
		return err
	}
	for index, account := range accounts {
		accountBranch := "├──"
		if index == len(accounts)-1 {
			accountBranch = "└──"
		}
		if _, err := fmt.Fprintf(writer, "%s %s\n", colorize(color, ansiDim, prefix+"│   "+accountBranch), colorize(color, ansiCyan, formatAccount(account))); err != nil {
			return err
		}
	}
	return nil
}

func formatAccount(account runtime.AuthProvider) string {
	details := make([]string, 0, 2)
	if account.Method != "" {
		details = append(details, account.Method)
	}
	if account.Subscription != "" {
		details = append(details, account.Subscription)
	}
	if len(details) == 0 {
		return account.ID
	}
	return account.ID + " (" + strings.Join(details, ", ") + ")"
}

func writeModelsTree(writer io.Writer, prefix string, models *modelsOverview, color bool) error {
	branch := colorize(color, ansiDim, prefix+"└──")
	if models == nil || models.Status == modelsUnknown {
		_, err := fmt.Fprintf(writer, "%s models %s\n", branch, colorize(color, ansiRed, "(status unknown)"))
		return err
	}
	if models.Status == modelsNone {
		_, err := fmt.Fprintf(writer, "%s models %s\n", branch, colorize(color, ansiYellow, "(none available)"))
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s models\n", branch); err != nil {
		return err
	}
	for index, model := range models.Items {
		modelBranch := "├──"
		if index == len(models.Items)-1 {
			modelBranch = "└──"
		}
		label := model.ID
		if model.DisplayName != "" && model.DisplayName != model.ID {
			label += " — " + model.DisplayName
		}
		if _, err := fmt.Fprintf(writer, "%s %s\n", colorize(color, ansiDim, prefix+"    "+modelBranch), colorize(color, ansiCyan, label)); err != nil {
			return err
		}
	}
	return nil
}
