package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArcheMind/agentx/internal/skills"
)

func (a App) skill(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return skillUsageError()
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: ax [--json|--yaml] skill list")
		}
		result, err := a.Skills.List()
		if err != nil {
			return err
		}
		return a.writeSkillList(result)
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: ax [--json|--yaml] skill show <name>")
		}
		item, err := a.Skills.Show(args[1])
		if err != nil {
			return err
		}
		return a.writeSkill(item)
	case "install":
		return a.installSkill(ctx, args[1:])
	default:
		return skillUsageError()
	}
}

func (a App) installSkill(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax [--json|--yaml] skill install <source> [--path <subdir>] [--scope user|project] [--dry-run]")
	}
	options := skills.InstallOptions{Source: args[0], Scope: skills.ScopeProject}
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--path":
			index++
			if index >= len(args) {
				return fmt.Errorf("--path requires a value")
			}
			options.Path = args[index]
		case "--scope":
			index++
			if index >= len(args) {
				return fmt.Errorf("--scope requires a value")
			}
			options.Scope = skills.Scope(args[index])
		case "--dry-run":
			options.DryRun = true
		default:
			return fmt.Errorf("unknown skill install option %q", args[index])
		}
	}
	plan, err := a.Skills.Install(ctx, options)
	if err != nil {
		return err
	}
	if a.Output != OutputText || options.DryRun {
		return writeStructured(a.Stdout, plan, a.structuredDefault())
	}
	fmt.Fprintf(a.Stdout, "Installed %s to %s\n", plan.Name, plan.Destination)
	fmt.Fprintf(a.Stdout, "Projected to Claude at %s\n", plan.Projection)
	return nil
}

func (a App) writeSkillList(result skills.ListResult) error {
	if a.Output != OutputText {
		return writeStructured(a.Stdout, result, a.Output)
	}
	if len(result.Skills) == 0 {
		fmt.Fprintln(a.Stdout, "No skills installed.")
	} else {
		for _, item := range result.Skills {
			fmt.Fprintf(a.Stdout, "%-24s %-8s %s\n", item.Name, item.Scope, strings.Join(item.Agents, ","))
		}
	}
	fmt.Fprintln(a.Stdout, "dsh: unsupported")
	return nil
}

func (a App) writeSkill(item skills.Skill) error {
	if a.Output != OutputText {
		return writeStructured(a.Stdout, item, a.Output)
	}
	fmt.Fprintf(a.Stdout, "Skill: %s\n", item.Name)
	fmt.Fprintf(a.Stdout, "Description: %s\n", item.Description)
	fmt.Fprintf(a.Stdout, "Scope: %s\n", item.Scope)
	fmt.Fprintf(a.Stdout, "Path: %s\n", item.Path)
	fmt.Fprintf(a.Stdout, "Agents: %s\n", strings.Join(item.Agents, ", "))
	return nil
}

func skillUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] skill <list|show|install> [args...]")
}
