package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

type overviewResult struct {
	Agents []agentOverview `json:"agents" yaml:"agents"`
}

type agentOverview struct {
	ID        string         `json:"id" yaml:"id"`
	Name      string         `json:"name" yaml:"name"`
	Installed bool           `json:"installed" yaml:"installed"`
	Path      string         `json:"path,omitempty" yaml:"path,omitempty"`
	Version   string         `json:"version,omitempty" yaml:"version,omitempty"`
	Auth      authOverview   `json:"auth" yaml:"auth"`
	Models    modelsOverview `json:"models" yaml:"models"`
}

type authOverview struct {
	Supported bool                   `json:"supported" yaml:"supported"`
	Providers []runtime.AuthProvider `json:"providers" yaml:"providers"`
	Error     string                 `json:"error,omitempty" yaml:"error,omitempty"`
}

type modelsOverview struct {
	Supported bool            `json:"supported" yaml:"supported"`
	Items     []runtime.Model `json:"items" yaml:"items"`
	Error     string          `json:"error,omitempty" yaml:"error,omitempty"`
}

func (a App) overview(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: ax [--json|--yaml] list")
	}
	result := overviewResult{Agents: make([]agentOverview, 0, len(a.Registry.All()))}
	for _, agent := range a.Registry.All() {
		detection := a.detectAgent(ctx, agent)
		result.Agents = append(result.Agents, a.queryAgentOverview(ctx, agent, detection))
	}
	if a.Output != OutputText {
		return writeStructured(a.Stdout, result, a.Output)
	}
	return writeOverviewTree(a.Stdout, result)
}

func (a App) detectAgent(ctx context.Context, agent runtime.Agent) runtime.Detection {
	detection := runtime.Detection{ID: agent.ID, Name: agent.Name, Capabilities: agent.Capabilities}
	path, err := exec.LookPath(agent.Binary)
	if err != nil {
		return detection
	}
	detection.Installed = true
	detection.Path = path
	result, err := a.Runner.Execute(ctx, runtime.CommandPlan{Executable: path, Args: []string{"--version"}}, runtime.ExecuteOptions{})
	if err == nil {
		detection.Version = firstLine(result.Stdout + result.Stderr)
	}
	return detection
}

func (a App) queryAgentOverview(ctx context.Context, agent runtime.Agent, detection runtime.Detection) agentOverview {
	item := agentOverview{
		ID: detection.ID, Name: detection.Name, Installed: detection.Installed,
		Path: detection.Path, Version: detection.Version,
		Auth:   authOverview{Supported: hasCapability(agent.Capabilities, runtime.CapabilityAuthStatus), Providers: []runtime.AuthProvider{}},
		Models: modelsOverview{Supported: hasCapability(agent.Capabilities, runtime.CapabilityModelList), Items: []runtime.Model{}},
	}
	if item.Auth.Supported {
		if !item.Installed {
			item.Auth.Error = "agent is not installed"
		} else {
			status, err := agent.Auth.Status(ctx, a.Runner)
			if err != nil {
				item.Auth.Error = err.Error()
			} else {
				item.Auth.Providers = status.Providers
			}
		}
	}
	if item.Models.Supported {
		models, err := agent.Models.ListModels(ctx, a.Runner)
		if err != nil {
			item.Models.Error = err.Error()
		} else {
			item.Models.Items = models
		}
	}
	return item
}

func hasCapability(capabilities []runtime.Capability, expected runtime.Capability) bool {
	for _, capability := range capabilities {
		if capability == expected {
			return true
		}
	}
	return false
}

type stringWriter interface {
	Write([]byte) (int, error)
}

func writeOverviewTree(writer stringWriter, result overviewResult) error {
	if _, err := fmt.Fprintln(writer, "agents"); err != nil {
		return err
	}
	for index, item := range result.Agents {
		lastAgent := index == len(result.Agents)-1
		branch, continuation := "├──", "│   "
		if lastAgent {
			branch, continuation = "└──", "    "
		}
		if _, err := fmt.Fprintf(writer, "%s %s — %s\n", branch, item.ID, item.Name); err != nil {
			return err
		}
		installed := "no"
		if item.Installed {
			installed = "yes"
			if item.Path != "" {
				installed += " (" + item.Path + ")"
			}
		}
		version := item.Version
		if version == "" {
			version = "unavailable"
		}
		if _, err := fmt.Fprintf(writer, "%s├── installed: %s\n%s├── version: %s\n", continuation, installed, continuation, version); err != nil {
			return err
		}
		if err := writeAuthTree(writer, continuation, item.Auth); err != nil {
			return err
		}
		if err := writeModelsTree(writer, continuation, item.Models); err != nil {
			return err
		}
	}
	return nil
}

func writeAuthTree(writer stringWriter, prefix string, auth authOverview) error {
	if !auth.Supported {
		_, err := fmt.Fprintf(writer, "%s├── auth: unsupported\n", prefix)
		return err
	}
	if auth.Error != "" {
		_, err := fmt.Fprintf(writer, "%s├── auth: error: %s\n", prefix, singleLine(auth.Error, 120))
		return err
	}
	if len(auth.Providers) == 0 {
		_, err := fmt.Fprintf(writer, "%s├── auth: not logged in\n", prefix)
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s├── auth\n", prefix); err != nil {
		return err
	}
	for index, provider := range auth.Providers {
		branch := "├──"
		if index == len(auth.Providers)-1 {
			branch = "└──"
		}
		details := make([]string, 0, 2)
		if provider.Method != "" {
			details = append(details, provider.Method)
		}
		if provider.Subscription != "" {
			details = append(details, provider.Subscription)
		}
		label := provider.ID + ": logged in"
		if len(details) > 0 {
			label += " (" + strings.Join(details, ", ") + ")"
		}
		if _, err := fmt.Fprintf(writer, "%s│   %s %s\n", prefix, branch, label); err != nil {
			return err
		}
	}
	return nil
}

func writeModelsTree(writer stringWriter, prefix string, models modelsOverview) error {
	if !models.Supported {
		_, err := fmt.Fprintf(writer, "%s└── models: unsupported\n", prefix)
		return err
	}
	if models.Error != "" {
		_, err := fmt.Fprintf(writer, "%s└── models: error: %s\n", prefix, singleLine(models.Error, 120))
		return err
	}
	if len(models.Items) == 0 {
		_, err := fmt.Fprintf(writer, "%s└── models: none\n", prefix)
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s└── models\n", prefix); err != nil {
		return err
	}
	for index, model := range models.Items {
		branch := "├──"
		if index == len(models.Items)-1 {
			branch = "└──"
		}
		label := model.ID
		if model.DisplayName != "" && model.DisplayName != model.ID {
			label += " — " + model.DisplayName
		}
		if _, err := fmt.Fprintf(writer, "%s    %s %s\n", prefix, branch, label); err != nil {
			return err
		}
	}
	return nil
}
