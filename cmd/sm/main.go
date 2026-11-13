package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ArcheMind/agentx/internal/app"
	"github.com/ArcheMind/agentx/internal/update"
)

func main() {
	args := os.Args[1:]
	if handled, err := update.RunHelper(args, os.Stdout, os.Stderr); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	bare := len(args) == 0
	debug := os.Getenv("SM_LOG") == "debug"
	appArgs := make([]string, 0, len(args))
	for len(args) > 0 {
		switch args[0] {
		case "--verbose":
			debug = true
		case "--json", "--yaml":
			appArgs = append(appArgs, args[0])
		default:
			appArgs = append(appArgs, args...)
			args = nil
			continue
		}
		args = args[1:]
	}
	args = appArgs
	application := app.New(debug, os.Stdin, os.Stdout, os.Stderr)
	if bare {
		stop, err := application.CheckStartupUpdate(context.Background(), debug)
		if err != nil {
			_ = app.WriteError(os.Stderr, args, err)
			os.Exit(app.ExitCode(err))
		}
		if stop {
			return
		}
	}
	if err := application.Run(context.Background(), args); err != nil {
		_ = app.WriteError(os.Stderr, args, err)
		os.Exit(app.ExitCode(err))
	}
}
