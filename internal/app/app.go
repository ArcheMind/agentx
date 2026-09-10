package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"agentx/internal/drivers"
	"agentx/internal/runtime"
)

const Version = "0.1.0"

type App struct {
	Registry drivers.Registry
	Runner   runtime.Runner
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

func New(debug bool, stdin io.Reader, stdout, stderr io.Writer) App {
	return App{
		Registry: drivers.NewRegistry(),
		Runner:   runtime.ExecRunner{Debug: debug, Log: stderr},
		Stdin:    stdin, Stdout: stdout, Stderr: stderr,
	}
}

func (a App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.printHelp()
		return nil
	}
	switch args[0] {
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	case "version", "-v", "--version":
		fmt.Fprintf(a.Stdout, "agentx %s\n", Version)
		return nil
	case "list", "agents":
		return a.list(ctx, args[1:])
	case "which":
		return a.which(args[1:])
	case "install":
		return a.install(ctx, args[1:])
	case "models":
		return a.models(ctx, args[1:])
	case "run":
		return a.runAgent(ctx, args[1:])
	case "session", "sessions":
		return a.sessions(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q; run ax help", args[0])
	}
}

func (a App) list(ctx context.Context, args []string) error {
	jsonOutput, err := onlyJSONFlag(args)
	if err != nil {
		return err
	}
	detections := make([]runtime.Detection, 0)
	for _, agent := range a.Registry.All() {
		detection := runtime.Detection{ID: agent.ID, Name: agent.Name, Capabilities: agent.Capabilities}
		path, lookupErr := exec.LookPath(agent.Binary)
		if lookupErr == nil {
			detection.Installed = true
			detection.Path = path
			result, versionErr := a.Runner.Execute(ctx, runtime.CommandPlan{Executable: path, Args: []string{"--version"}}, runtime.ExecuteOptions{})
			if versionErr == nil {
				detection.Version = firstLine(result.Stdout + result.Stderr)
			}
		}
		detections = append(detections, detection)
	}
	if jsonOutput {
		return writeJSON(a.Stdout, detections)
	}
	for _, item := range detections {
		status := "not installed"
		if item.Installed {
			status = item.Path
			if item.Version != "" {
				status += " (" + item.Version + ")"
			}
		}
		fmt.Fprintf(a.Stdout, "%-10s %s\n", item.ID, status)
	}
	return nil
}

func (a App) which(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ax which <agent>")
	}
	agent, err := a.Registry.Get(args[0])
	if err != nil {
		return err
	}
	path, err := exec.LookPath(agent.Binary)
	if err != nil {
		return fmt.Errorf("%s is not installed; run ax install %s", agent.Name, agent.ID)
	}
	fmt.Fprintln(a.Stdout, path)
	return nil
}

func (a App) install(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax install <agent> [--version <version>] [--dry-run]")
	}
	agent, err := a.Registry.Get(args[0])
	if err != nil {
		return err
	}
	if agent.Package == nil {
		return fmt.Errorf("%s has no package driver", agent.Name)
	}
	var version string
	dryRun := false
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--version":
			index++
			if index >= len(args) {
				return fmt.Errorf("--version requires a value")
			}
			version = args[index]
		case "--dry-run":
			dryRun = true
		default:
			return fmt.Errorf("unknown install option %q", args[index])
		}
	}
	plan, err := agent.Package.PlanInstall(version)
	if err != nil {
		return err
	}
	if dryRun {
		return writeJSON(a.Stdout, plan)
	}
	_, err = a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	return err
}

func (a App) models(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax models <agent> [--json]")
	}
	agent, err := a.Registry.Get(args[0])
	if err != nil {
		return err
	}
	jsonOutput, err := onlyJSONFlag(args[1:])
	if err != nil {
		return err
	}
	if agent.Models == nil {
		return fmt.Errorf("%s has no model driver", agent.Name)
	}
	models, err := agent.Models.ListModels(ctx, a.Runner)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(a.Stdout, models)
	}
	for _, model := range models {
		if model.DisplayName != "" && model.DisplayName != model.ID {
			fmt.Fprintf(a.Stdout, "%s\t%s\n", model.ID, model.DisplayName)
		} else {
			fmt.Fprintln(a.Stdout, model.ID)
		}
	}
	return nil
}

func (a App) runAgent(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax run <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]")
	}
	agent, err := a.Registry.Get(args[0])
	if err != nil {
		return err
	}
	if agent.Launch == nil {
		return fmt.Errorf("%s cannot be launched", agent.Name)
	}
	request := runtime.RunRequest{}
	dryRun := false
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--":
			request.PassthroughArgs = append(request.PassthroughArgs, args[index+1:]...)
			index = len(args)
		case "--model":
			index++
			if index >= len(args) {
				return fmt.Errorf("--model requires a value")
			}
			request.Model = args[index]
		case "--cwd":
			index++
			if index >= len(args) {
				return fmt.Errorf("--cwd requires a value")
			}
			request.Cwd = args[index]
		case "--dry-run":
			dryRun = true
		default:
			return fmt.Errorf("unknown run option %q; native arguments must follow --", args[index])
		}
	}
	if request.Cwd == "" {
		request.Cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("read working directory: %w", err)
		}
	}
	request.Cwd, err = filepath.Abs(request.Cwd)
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	info, err := os.Stat(request.Cwd)
	if err != nil {
		return fmt.Errorf("read working directory %s: %w", request.Cwd, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory %s is not a directory", request.Cwd)
	}
	plan, err := agent.Launch.PlanRun(request)
	if err != nil {
		return err
	}
	if dryRun {
		return writeJSON(a.Stdout, plan)
	}
	if _, err := exec.LookPath(agent.Binary); err != nil {
		return fmt.Errorf("%s is not installed; run ax install %s", agent.Name, agent.ID)
	}
	result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	if err != nil {
		return &ExitError{Code: result.ExitCode, Err: err}
	}
	return nil
}

func (a App) sessions(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax session <providers|list|info|resume> [args...]")
	}
	if _, err := exec.LookPath("casr"); err != nil {
		return fmt.Errorf("casr is not installed; run ax install casr")
	}
	plan, err := (drivers.CASRSessions{}).Plan(args)
	if err != nil {
		return err
	}
	result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	if err != nil {
		return &ExitError{Code: result.ExitCode, Err: err}
	}
	return nil
}

func (a App) printHelp() {
	fmt.Fprint(a.Stdout, `agentx manages native AI coding-agent runtimes.

Usage:
  ax list [--json]
  ax which <agent>
  ax install <agent> [--version <version>] [--dry-run]
  ax models <agent> [--json]
  ax run <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]
  ax session <providers|list|info|resume> [args...]
  ax version

Agents: claude, codex, gemini, opencode, pi, casr

Set AX_LOG=debug or pass --verbose before the command to log raw external input,
output, and errors as JSON on stderr.
`)
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

func ExitCode(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Code > 0 {
		return exitErr.Code
	}
	return 1
}

func onlyJSONFlag(args []string) (bool, error) {
	jsonOutput := false
	for _, arg := range args {
		if arg != "--json" {
			return false, fmt.Errorf("unknown option %q", arg)
		}
		jsonOutput = true
	}
	return jsonOutput, nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if line, _, ok := strings.Cut(value, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return value
}
