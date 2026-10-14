package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/subagents"
)

func (a App) prepareSubagents(request *runtime.RunRequest, write bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root, err := subagents.Prepare(a.Hooks.Home, executable, write)
	if err != nil {
		return err
	}
	request.PassthroughArgs = append([]string{"--plugin-dir", root}, request.PassthroughArgs...)
	return nil
}

func (a App) execAgent(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax agent exec <codex|pi|gemini> [--model <model>] [--resume <session-id>] [--cwd <path>] [--dry-run] < prompt.txt")
	}
	model, resume, cwd := "", "", ""
	dryRun := false
	for i := 1; i < len(args); i++ {
		option := args[i]
		switch option {
		case "--dry-run":
			dryRun = true
		case "--model", "--resume", "--cwd":
			i++
			if i == len(args) || strings.TrimSpace(args[i]) == "" {
				return fmt.Errorf("%s requires a value", option)
			}
			switch option {
			case "--model":
				model = args[i]
			case "--resume":
				resume = args[i]
			case "--cwd":
				cwd = args[i]
			}
		default:
			return fmt.Errorf("unknown exec option %q", option)
		}
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return err
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory %s is not a directory", cwd)
	}
	plan, err := drivers.ExecPlan(args[0], cwd, model, resume)
	if err != nil {
		return err
	}
	if dryRun {
		return writeStructured(a.Stdout, plan, a.structuredDefault())
	}
	if err := a.rejectStructuredNative("agent exec", false); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr, CancelProcessGroup: true})
	if err != nil {
		return &ExitError{Code: result.ExitCode, Err: err}
	}
	return nil
}
