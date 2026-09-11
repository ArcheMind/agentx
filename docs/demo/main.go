package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ArcheMind/agentx/internal/app"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
)

func main() {
	workingDirectory, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	demoDirectory := filepath.Join(workingDirectory, "docs", "demo")
	if err := os.Chdir(filepath.Join(demoDirectory, "workspace")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	application := app.New(false, &demoInput{keys: []demoKey{
		{value: "j", pause: 700 * time.Millisecond},
		{value: "\r", pause: 500 * time.Millisecond},
		{value: "j", pause: 700 * time.Millisecond},
		{value: "\r", pause: 300 * time.Millisecond},
		{value: "j", pause: 600 * time.Millisecond},
	}}, os.Stdout, os.Stderr)
	application.Color = true
	application.Sessions = sessions.Service{HomeDir: filepath.Join(demoDirectory, "home")}
	application.Runner = demoRunner{}
	if err := application.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type demoInput struct {
	keys []demoKey
}

type demoKey struct {
	value string
	pause time.Duration
}

func (input *demoInput) Read(buffer []byte) (int, error) {
	if len(input.keys) == 0 {
		for {
			time.Sleep(time.Hour)
		}
	}
	key := input.keys[0]
	input.keys = input.keys[1:]
	time.Sleep(key.pause)
	return copy(buffer, key.value), nil
}

type demoRunner struct{}

func (demoRunner) Execute(_ context.Context, plan runtime.CommandPlan, options runtime.ExecuteOptions) (runtime.CommandResult, error) {
	if plan.Executable == "opencode" && len(plan.Args) == 1 && plan.Args[0] == "models" {
		return runtime.CommandResult{Stdout: "openai/gpt-5.4\nanthropic/claude-sonnet-4-6\ndeepseek/deepseek-chat\n"}, nil
	}
	if options.Interactive {
		model := "native default"
		for index, arg := range plan.Args {
			if arg == "--model" && index+1 < len(plan.Args) {
				model = plan.Args[index+1]
				break
			}
		}
		fmt.Fprintf(options.Stdout, "\nLaunching %s with %s...\n", plan.Executable, model)
		fmt.Fprintln(options.Stdout, "Cross-Agent session handed off to the native agent.")
	}
	return runtime.CommandResult{}, nil
}
