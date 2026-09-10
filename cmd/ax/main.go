package main

import (
	"context"
	"fmt"
	"os"

	"agentx/internal/app"
)

func main() {
	args := os.Args[1:]
	debug := os.Getenv("AX_LOG") == "debug"
	if len(args) > 0 && args[0] == "--verbose" {
		debug = true
		args = args[1:]
	}
	application := app.New(debug, os.Stdin, os.Stdout, os.Stderr)
	if err := application.Run(context.Background(), args); err != nil {
		fmt.Fprintf(os.Stderr, "ax: %v\n", err)
		os.Exit(app.ExitCode(err))
	}
}
