package main

import (
	"context"
	"os"

	"github.com/ArcheMind/agentx/internal/app"
)

func main() {
	args := os.Args[1:]
	debug := os.Getenv("AX_LOG") == "debug"
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
	if err := application.Run(context.Background(), args); err != nil {
		_ = app.WriteError(os.Stderr, args, err)
		os.Exit(app.ExitCode(err))
	}
}
