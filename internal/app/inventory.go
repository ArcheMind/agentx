package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArcheMind/agentx/internal/inventory"
	"github.com/ArcheMind/agentx/internal/mcp"
)

func (a App) mcp(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "serve" {
		if len(args) != 1 {
			return fmt.Errorf("usage: ax mcp serve")
		}
		return a.mcpServe(ctx)
	}
	return a.inventoryCommand("mcp", args, (mcp.Service{Home: a.Skills.Home, Project: a.Skills.Project}).List())
}
func (a App) inventoryCommand(kind string, args []string, result inventory.Result) error {
	if len(args) == 1 && args[0] == "list" {
		return a.writeInventory(result)
	}
	if len(args) >= 2 && args[0] == "show" {
		selection := inventory.Selection{}
		for i := 2; i < len(args); i++ {
			option := args[i]
			if option != "--scope" && option != "--source" && option != "--path" {
				return fmt.Errorf("unknown %s show option %q", kind, option)
			}
			i++
			if i == len(args) {
				return fmt.Errorf("%s requires a value", option)
			}
			if option == "--scope" {
				selection.Scope = args[i]
				if selection.Scope != "project" && selection.Scope != "user" {
					return fmt.Errorf("invalid scope %q", selection.Scope)
				}
			} else if option == "--path" {
				selection.Path = args[i]
			} else {
				selection.Source = args[i]
			}
		}
		detail, err := result.Show(args[1], selection)
		if err != nil {
			return err
		}
		if a.Output != OutputText {
			return writeStructured(a.Stdout, detail, a.Output)
		}
		fmt.Fprintf(a.Stdout, "Name: %s\nScope: %s\nSource: %s\nPath: %s\nAgents: %s\n", detail.Resource.Name, detail.Source.Scope, detail.Source.Path, detail.Resource.Path, a.inventoryAgents(detail.Resource.Agents))
		for _, projection := range detail.Resource.Projections {
			fmt.Fprintf(a.Stdout, "Projection: %s (%s) -> %s\n", projection.Path, a.inventoryAgents(projection.Agents), detail.Resource.Path)
		}
		a.writeEvidence(detail.Resource.Evidence)
		if len(detail.Resource.Observations) > 1 {
			for _, observation := range detail.Resource.Observations {
				if observation.SessionID != "" {
					fmt.Fprintf(a.Stdout, "Observed session %s: %s (%s)\n", observation.SessionID, observation.Session, observation.SessionEvidence)
				}
			}
		}
		return nil
	}
	return fmt.Errorf("usage: ax [--json|--yaml] %s <list|show <name> [--scope project|user] [--source <path>] [--path <resource-path>]>", kind)
}
func (a App) inventoryAgents(agents []string) string {
	values := []string{}
	for _, agent := range agents {
		values = append(values, colorize(a.Color, agentColor(agent), agent))
	}
	return strings.Join(values, " · ")
}
func (a App) writeInventory(result inventory.Result) error {
	if a.Output != OutputText {
		return writeStructured(a.Stdout, result, a.Output)
	}
	for _, scope := range []string{"project", "user"} {
		title := "Project"
		if scope == "user" {
			title = "User"
		}
		fmt.Fprintln(a.Stdout, title)
		found := false
		for _, source := range result.Sources {
			if source.Scope != scope {
				continue
			}
			found = true
			fmt.Fprintln(a.Stdout)
			path := source.Path
			if source.Preferred {
				path = colorize(a.Color, ansiBold, path)
			}
			fmt.Fprintf(a.Stdout, "%s  %s\n", path, a.inventoryAgents(source.Agents))
			if source.Status == "failed" {
				fmt.Fprintf(a.Stdout, "  Read failed: %s\n", source.Error)
			} else if source.Status == "unsupported" {
				fmt.Fprintf(a.Stdout, "  Discovery unsupported: %s\n", source.Error)
			} else if len(source.Resources) == 0 {
				fmt.Fprintln(a.Stdout, "  No resources")
			}
			for _, item := range source.Resources {
				suffix := ""
				if strings.Join(item.Agents, ",") != strings.Join(source.Agents, ",") {
					suffix = " (" + a.inventoryAgents(item.Agents) + ")"
				}
				fmt.Fprintf(a.Stdout, "  %s%s\n", item.Name, suffix)
			}
		}
		if !found {
			fmt.Fprintln(a.Stdout, "\n  No configuration sources found")
		}
		fmt.Fprintln(a.Stdout)
	}
	return nil
}
func (a App) writeEvidence(e inventory.Evidence) {
	e = e.Verified()
	fmt.Fprintf(a.Stdout, "Configuration: %s\nConnection: %s\nCurrent session: %s\n", e.Configuration, e.Connection, e.Session)
	if e.ConnectionEvidence != "" {
		fmt.Fprintf(a.Stdout, "Connection evidence: %s\n", e.ConnectionEvidence)
	}
	if e.SessionID != "" {
		fmt.Fprintf(a.Stdout, "Session ID: %s\n", e.SessionID)
	}
	if e.SessionEvidence != "" {
		fmt.Fprintf(a.Stdout, "Session evidence: %s\n", e.SessionEvidence)
	}
}
