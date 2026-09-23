package app

import (
	"fmt"
	"strings"
)

func (a App) hook(args []string) error {
	if len(args) != 1 || args[0] != "list" {
		return fmt.Errorf("usage: ax [--json|--yaml] hook list")
	}
	result, err := a.Hooks.List()
	if err != nil {
		return err
	}
	if a.Output != OutputText {
		return writeStructured(a.Stdout, result, a.Output)
	}
	if len(result.Hooks) == 0 {
		fmt.Fprintln(a.Stdout, "No portable hooks configured.")
	} else {
		fmt.Fprintln(a.Stdout, "SCOPE    EVENT             COMMAND                         TARGETS")
		for _, item := range result.Hooks {
			fmt.Fprintf(a.Stdout, "%-8s %-17s %-31s %s\n", item.Scope, item.Event, item.Command, strings.Join(item.Targets, ","))
		}
	}
	if len(result.Warnings) > 0 && len(result.Hooks) > 0 {
		fmt.Fprintln(a.Stdout)
	}
	for _, warning := range result.Warnings {
		if warning.Event == "" {
			fmt.Fprintf(a.Stdout, "warning: %s\n", warning.Message)
		} else {
			fmt.Fprintf(a.Stdout, "warning: %s %s\n", warning.Event, warning.Message)
		}
	}
	return nil
}
