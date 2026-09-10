package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
	"gopkg.in/yaml.v3"
)

var Version = "0.1.0"

type App struct {
	Registry drivers.Registry
	Sessions sessions.Service
	Runner   runtime.Runner
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Output   OutputFormat
}

type OutputFormat string

const (
	OutputText OutputFormat = "text"
	OutputJSON OutputFormat = "json"
	OutputYAML OutputFormat = "yaml"
)

func New(debug bool, stdin io.Reader, stdout, stderr io.Writer) App {
	return App{
		Registry: drivers.NewRegistry(),
		Sessions: sessions.New(),
		Runner:   runtime.ExecRunner{Debug: debug, Log: stderr},
		Stdin:    stdin, Stdout: stdout, Stderr: stderr,
	}
}

func (a App) Run(ctx context.Context, args []string) error {
	output, args, err := parseOutputFormat(args)
	if err != nil {
		return err
	}
	a.Output = output
	if len(args) == 0 {
		if err := a.rejectStructured("interactive session resume"); err != nil {
			return err
		}
		return a.interactiveResume(ctx)
	}
	switch args[0] {
	case "help", "-h", "--help":
		if err := a.rejectStructured("help"); err != nil {
			return err
		}
		a.printHelp()
		return nil
	case "version", "-v", "--version":
		if a.Output != OutputText {
			return writeStructured(a.Stdout, map[string]string{"version": Version}, a.Output)
		}
		fmt.Fprintf(a.Stdout, "agentx %s\n", Version)
		return nil
	case "agent":
		return a.agent(ctx, args[1:])
	case "auth":
		return a.auth(ctx, args[1:])
	case "session":
		return a.sessions(ctx, args[1:])
	default:
		if _, err := a.Registry.Get(args[0]); err == nil {
			return a.runAgent(ctx, args)
		}
		return fmt.Errorf("unknown command %q; run ax help", args[0])
	}
}

func (a App) agent(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return agentUsageError()
	}
	switch args[0] {
	case "list":
		return a.list(ctx, args[1:])
	case "which":
		return a.which(args[1:])
	case "install":
		return a.install(ctx, args[1:])
	case "models":
		return a.models(ctx, args[1:])
	case "run":
		return a.runAgent(ctx, args[1:])
	default:
		return agentUsageError()
	}
}

func agentUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] agent <list|which|install|models|run> [args...]")
}

func (a App) list(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: ax [--json|--yaml] agent list")
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
	if a.Output != OutputText {
		return writeStructured(a.Stdout, detections, a.Output)
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
		return fmt.Errorf("usage: ax agent which <agent>")
	}
	if err := a.rejectStructured("agent which"); err != nil {
		return err
	}
	agent, err := a.Registry.Get(args[0])
	if err != nil {
		return err
	}
	path, err := exec.LookPath(agent.Binary)
	if err != nil {
		return fmt.Errorf("%s is not installed; run ax agent install %s", agent.Name, agent.ID)
	}
	fmt.Fprintln(a.Stdout, path)
	return nil
}

func (a App) install(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax [--json|--yaml] agent install <agent> [--version <version>] [--dry-run]")
	}
	item, err := a.Registry.Get(args[0])
	if err != nil {
		return err
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
	plan, err := item.Install.PlanInstall(version)
	if err != nil {
		return err
	}
	if dryRun {
		return writeStructured(a.Stdout, plan, a.structuredDefault())
	}
	if err := a.rejectStructuredNative("agent install", dryRun); err != nil {
		return err
	}
	_, err = a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	return err
}

func (a App) models(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ax [--json|--yaml] agent models <agent>")
	}
	agent, err := a.Registry.Get(args[0])
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
	if a.Output != OutputText {
		return writeStructured(a.Stdout, models, a.Output)
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
		return fmt.Errorf("usage: ax [--json|--yaml] agent run <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]")
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
		return writeStructured(a.Stdout, plan, a.structuredDefault())
	}
	if err := a.rejectStructuredNative("agent run", dryRun); err != nil {
		return err
	}
	if _, err := exec.LookPath(agent.Binary); err != nil {
		return fmt.Errorf("%s is not installed; run ax agent install %s", agent.Name, agent.ID)
	}
	result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	if err != nil {
		return &ExitError{Code: result.ExitCode, Err: err}
	}
	return nil
}

func (a App) auth(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return authUsageError()
	}
	agent, err := a.Registry.Get(args[1])
	if err != nil {
		return err
	}
	if agent.Auth == nil {
		return fmt.Errorf("%s has no auth driver", agent.Name)
	}
	if args[0] == "status" {
		if len(args) != 2 {
			return fmt.Errorf("usage: ax [--json|--yaml] auth status <agent>")
		}
		if !agent.Auth.SupportsStatus() {
			return a.writeAuthStatus(runtime.AuthStatus{Agent: agent.ID, Supported: false, Providers: []runtime.AuthProvider{}})
		}
		if _, err := exec.LookPath(agent.Binary); err != nil {
			return fmt.Errorf("%s is not installed; run ax agent install %s", agent.Name, agent.ID)
		}
		status, err := agent.Auth.Status(ctx, a.Runner)
		if err != nil {
			return err
		}
		status.Agent = agent.ID
		return a.writeAuthStatus(status)
	}
	if args[0] != "login" && args[0] != "logout" {
		return authUsageError()
	}
	dryRun := false
	for _, arg := range args[2:] {
		if arg != "--dry-run" {
			return fmt.Errorf("unknown auth option %q", arg)
		}
		dryRun = true
	}
	var plan runtime.AuthPlan
	if args[0] == "login" {
		plan = agent.Auth.PlanLogin()
	} else {
		if !agent.Auth.SupportsLogout() {
			return fmt.Errorf("%s does not expose a verified native logout flow", agent.Name)
		}
		plan = agent.Auth.PlanLogout()
	}
	if dryRun {
		return writeStructured(a.Stdout, plan, a.structuredDefault())
	}
	if err := a.rejectStructuredNative("auth "+args[0], dryRun); err != nil {
		return err
	}
	if _, err := exec.LookPath(agent.Binary); err != nil {
		return fmt.Errorf("%s is not installed; run ax agent install %s", agent.Name, agent.ID)
	}
	if plan.Instruction != "" {
		fmt.Fprintln(a.Stderr, plan.Instruction)
	}
	result, err := a.Runner.Execute(ctx, plan.Command, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	if err != nil {
		return &ExitError{Code: result.ExitCode, Err: err}
	}
	return nil
}

func authUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] auth <login|status|logout> <agent> [--dry-run]")
}

func (a App) writeAuthStatus(status runtime.AuthStatus) error {
	if a.Output != OutputText {
		return writeStructured(a.Stdout, status, a.Output)
	}
	if !status.Supported {
		fmt.Fprintf(a.Stdout, "%s\tunsupported\n", status.Agent)
		return nil
	}
	if len(status.Providers) == 0 {
		fmt.Fprintf(a.Stdout, "%s\tnot logged in\n", status.Agent)
		return nil
	}
	for _, provider := range status.Providers {
		details := []string{}
		if provider.Method != "" {
			details = append(details, provider.Method)
		}
		if provider.Subscription != "" {
			details = append(details, provider.Subscription)
		}
		fmt.Fprintf(a.Stdout, "%s\t%s", status.Agent, provider.ID)
		if len(details) > 0 {
			fmt.Fprintf(a.Stdout, " (%s)", strings.Join(details, ", "))
		}
		fmt.Fprintln(a.Stdout)
	}
	return nil
}

func (a App) sessions(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return sessionUsageError()
	}
	switch args[0] {
	case "providers":
		if len(args) != 1 {
			return fmt.Errorf("usage: ax [--json|--yaml] session providers")
		}
		providers := a.Sessions.Providers()
		if a.Output != OutputText {
			return writeStructured(a.Stdout, providers, a.Output)
		}
		for _, provider := range providers {
			status := "not installed"
			if provider.Installed {
				status = "installed"
			}
			fmt.Fprintf(a.Stdout, "%-10s %-13s %s\n", provider.ID, status, provider.Root)
		}
		return nil
	case "list":
		return a.sessionList(args[1:])
	case "info":
		return a.sessionInfo(args[1:])
	case "resume":
		return a.sessionResume(ctx, args[1:])
	default:
		return sessionUsageError()
	}
}

func (a App) sessionList(args []string) error {
	options := sessions.ListOptions{Limit: 10, Sort: "date"}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--source":
			index++
			if index >= len(args) {
				return fmt.Errorf("--source requires a value")
			}
			options.Provider = args[index]
		case "--workspace":
			index++
			if index >= len(args) {
				return fmt.Errorf("--workspace requires a value")
			}
			options.Workspace = args[index]
		case "--all":
			options.All = true
		case "--limit":
			index++
			if index >= len(args) {
				return fmt.Errorf("--limit requires a value")
			}
			limit, err := strconv.Atoi(args[index])
			if err != nil || limit < 0 {
				return fmt.Errorf("--limit must be a non-negative integer")
			}
			options.Limit = limit
		case "--sort":
			index++
			if index >= len(args) {
				return fmt.Errorf("--sort requires a value")
			}
			options.Sort = args[index]
		default:
			return fmt.Errorf("unknown session list option %q", args[index])
		}
	}
	if options.All && options.Workspace != "" {
		return fmt.Errorf("--all and --workspace are mutually exclusive")
	}
	items, err := a.Sessions.List(options)
	if err != nil {
		return err
	}
	if a.Output != OutputText {
		return writeStructured(a.Stdout, items, a.Output)
	}
	for _, item := range items {
		fmt.Fprintf(a.Stdout, "%-10s %-36s %5d  %s\n", item.Provider, item.ID, item.MessageCount, item.Title)
	}
	return nil
}

func (a App) sessionInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax [--json|--yaml] session info <session-id> [--source <provider>] [--peek] [--peek-lines <n>]")
	}
	id := args[0]
	var source string
	peekLines := 0
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--source":
			index++
			if index >= len(args) {
				return fmt.Errorf("--source requires a value")
			}
			source = args[index]
		case "--peek":
			peekLines = 5
		case "--peek-lines":
			index++
			if index >= len(args) {
				return fmt.Errorf("--peek-lines requires a value")
			}
			value, err := strconv.Atoi(args[index])
			if err != nil || value < 0 {
				return fmt.Errorf("--peek-lines must be a non-negative integer")
			}
			peekLines = value
		default:
			return fmt.Errorf("unknown session info option %q", args[index])
		}
	}
	detail, err := a.Sessions.Info(id, source)
	if err != nil {
		return err
	}
	if a.Output != OutputText {
		return writeStructured(a.Stdout, detail, a.Output)
	}
	fmt.Fprintf(a.Stdout, "ID: %s\nProvider: %s\n", detail.ID, detail.Provider)
	if detail.Title != "" {
		fmt.Fprintf(a.Stdout, "Title: %s\n", detail.Title)
	}
	if detail.Workspace != "" {
		fmt.Fprintf(a.Stdout, "Workspace: %s\n", detail.Workspace)
	}
	fmt.Fprintf(a.Stdout, "Messages: %d\nUpdated: %s\nSource: %s\n", detail.MessageCount, detail.UpdatedAt, detail.Source)
	if peekLines > len(detail.Messages) {
		peekLines = len(detail.Messages)
	}
	if peekLines > 0 {
		fmt.Fprintln(a.Stdout, "\nTranscript tail:")
		for _, message := range detail.Messages[len(detail.Messages)-peekLines:] {
			fmt.Fprintf(a.Stdout, "[%s] %s\n", message.Role, singleLine(message.Content, 200))
		}
	}
	return nil
}

func (a App) sessionResume(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ax [--json|--yaml] session resume <target-agent> <session-id> [--source <provider>] [--workspace <path>] [--dry-run]")
	}
	agent, err := a.Registry.Get(args[0])
	if err != nil {
		return err
	}
	id := args[1]
	var source, workspace string
	dryRun := false
	for index := 2; index < len(args); index++ {
		switch args[index] {
		case "--source":
			index++
			if index >= len(args) {
				return fmt.Errorf("--source requires a value")
			}
			source = args[index]
		case "--workspace":
			index++
			if index >= len(args) {
				return fmt.Errorf("--workspace requires a value")
			}
			workspace = args[index]
		case "--dry-run":
			dryRun = true
		default:
			return fmt.Errorf("unknown session resume option %q", args[index])
		}
	}
	detail, err := a.Sessions.Info(id, source)
	if err != nil {
		return err
	}
	return a.resumeSession(ctx, agent, detail, workspace, "", dryRun)
}

func (a App) resumeSession(ctx context.Context, agent runtime.Agent, detail sessions.Detail, workspace, model string, dryRun bool) error {
	var err error
	if workspace == "" {
		workspace = detail.Workspace
	}
	if workspace == "" {
		workspace, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("read working directory: %w", err)
		}
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("session workspace %s is not an accessible directory", workspace)
	}
	prompt := sessions.ResumePrompt(detail)
	request := runtime.RunRequest{Cwd: workspace, Model: model, PassthroughArgs: []string{prompt}}
	if agent.ID == "opencode" {
		request.PassthroughArgs = []string{"--prompt", prompt}
	}
	plan, err := agent.Launch.PlanRun(request)
	if err != nil {
		return err
	}
	if dryRun {
		return writeStructured(a.Stdout, plan, a.structuredDefault())
	}
	if err := a.rejectStructuredNative("session resume", dryRun); err != nil {
		return err
	}
	if _, err := exec.LookPath(agent.Binary); err != nil {
		return fmt.Errorf("%s is not installed; run ax agent install %s", agent.Name, agent.ID)
	}
	result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	if err != nil {
		return &ExitError{Code: result.ExitCode, Err: err}
	}
	return nil
}

func (a App) interactiveResume(ctx context.Context) error {
	scanner := bufio.NewScanner(a.Stdin)
	scope, err := a.choose(scanner, "Choose sessions:\n", []string{"Current workspace", "All workspaces"})
	if err != nil {
		return err
	}
	options := sessions.ListOptions{Limit: 10, Sort: "date"}
	if scope == 1 {
		options.All = true
	}
	items, err := a.Sessions.List(options)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no recent sessions found")
	}
	labels := make([]string, len(items))
	for index, item := range items {
		title := item.Title
		if title == "" {
			title = item.ID
		}
		labels[index] = fmt.Sprintf("%s  %s  %s", item.Provider, item.UpdatedAt, singleLine(title, 80))
	}
	sessionIndex, err := a.choose(scanner, "Choose a recent session:\n", labels)
	if err != nil {
		return err
	}
	detail, err := a.Sessions.Info(items[sessionIndex].ID, items[sessionIndex].Provider)
	if err != nil {
		return err
	}

	agents := make([]runtime.Agent, 0)
	for _, agent := range a.Registry.All() {
		if _, err := exec.LookPath(agent.Binary); err == nil {
			agents = append(agents, agent)
		}
	}
	if len(agents) == 0 {
		return fmt.Errorf("no supported agents are installed")
	}
	labels = make([]string, len(agents))
	for index, agent := range agents {
		labels[index] = agent.ID
	}
	agentIndex, err := a.choose(scanner, "Choose an agent:\n", labels)
	if err != nil {
		return err
	}
	agent := agents[agentIndex]
	model, err := a.chooseModel(ctx, scanner, agent)
	if err != nil {
		return err
	}
	return a.resumeSession(ctx, agent, detail, "", model, false)
}

func (a App) chooseModel(ctx context.Context, scanner *bufio.Scanner, agent runtime.Agent) (string, error) {
	if _, unsupported := agent.Models.(drivers.UnsupportedModels); unsupported {
		return a.prompt(scanner, fmt.Sprintf("Model for %s (leave blank for native default): ", agent.ID), true)
	}
	models, err := agent.Models.ListModels(ctx, a.Runner)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", fmt.Errorf("%s returned no models", agent.Name)
	}
	labels := make([]string, len(models)+1)
	labels[0] = "Native default"
	for index, model := range models {
		labels[index+1] = model.ID
		if model.DisplayName != "" && model.DisplayName != model.ID {
			labels[index+1] += "  " + model.DisplayName
		}
	}
	choice, err := a.choose(scanner, "Choose a model:\n", labels)
	if err != nil {
		return "", err
	}
	if choice == 0 {
		return "", nil
	}
	return models[choice-1].ID, nil
}

func (a App) choose(scanner *bufio.Scanner, prompt string, options []string) (int, error) {
	fmt.Fprint(a.Stdout, prompt)
	for index, option := range options {
		fmt.Fprintf(a.Stdout, "  %d. %s\n", index+1, option)
	}
	value, err := a.prompt(scanner, "Selection (or q to cancel): ", false)
	if err != nil {
		return 0, err
	}
	if strings.EqualFold(value, "q") {
		return 0, errors.New("interactive session resume cancelled")
	}
	choice, err := strconv.Atoi(value)
	if err != nil || choice < 1 || choice > len(options) {
		return 0, fmt.Errorf("selection must be a number from 1 to %d", len(options))
	}
	return choice - 1, nil
}

func (a App) prompt(scanner *bufio.Scanner, label string, allowBlank bool) (string, error) {
	fmt.Fprint(a.Stdout, label)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read interactive input: %w", err)
		}
		return "", errors.New("interactive input closed")
	}
	value := strings.TrimSpace(scanner.Text())
	if value == "" && !allowBlank {
		return "", errors.New("a selection is required")
	}
	return value, nil
}

func sessionUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] session <providers|list|info|resume> [args...]")
}

func singleLine(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}

func (a App) printHelp() {
	fmt.Fprint(a.Stdout, `agentx manages native AI coding-agent runtimes.

Usage:
  ax
  ax <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]
  ax [--json|--yaml] agent list
  ax agent which <agent>
  ax [--json|--yaml] agent install <agent> [--version <version>] [--dry-run]
  ax [--json|--yaml] agent models <agent>
  ax [--json|--yaml] agent run <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]
  ax [--json|--yaml] auth login <agent> [--dry-run]
  ax [--json|--yaml] auth status <agent>
  ax [--json|--yaml] auth logout <agent> [--dry-run]
  ax [--json|--yaml] session <providers|list|info|resume> [args...]
  ax version

Running ax without arguments starts an interactive session resume. The ax <agent>
shortcut launches an installed agent directly.

Agents: claude, codex, gemini, opencode, pi

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

func parseOutputFormat(args []string) (OutputFormat, []string, error) {
	format := OutputText
	for len(args) > 0 {
		var selected OutputFormat
		switch args[0] {
		case "--json":
			selected = OutputJSON
		case "--yaml":
			selected = OutputYAML
		default:
			return format, args, nil
		}
		if format != OutputText && format != selected {
			return OutputText, nil, fmt.Errorf("--json and --yaml are mutually exclusive")
		}
		format = selected
		args = args[1:]
	}
	return format, args, nil
}

func writeStructured(writer io.Writer, value any, format OutputFormat) error {
	switch format {
	case OutputJSON:
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case OutputYAML:
		encoder := yaml.NewEncoder(writer)
		encoder.SetIndent(2)
		defer encoder.Close()
		return encoder.Encode(value)
	default:
		return fmt.Errorf("unsupported structured output format %q", format)
	}
}

func (a App) rejectStructured(operation string) error {
	if a.Output == OutputText {
		return nil
	}
	return fmt.Errorf("--%s is not supported for %s", a.Output, operation)
}

func (a App) rejectStructuredNative(operation string, dryRun bool) error {
	if dryRun || a.Output == OutputText {
		return nil
	}
	return fmt.Errorf("--%s is only supported for %s with --dry-run because native output is passed through", a.Output, operation)
}

type errorEnvelope struct {
	Error structuredError `json:"error" yaml:"error"`
}

type structuredError struct {
	Code     string `json:"code" yaml:"code"`
	Message  string `json:"message" yaml:"message"`
	ExitCode int    `json:"exit_code" yaml:"exit_code"`
}

func WriteError(writer io.Writer, args []string, err error) error {
	format, _, formatErr := parseOutputFormat(args)
	if formatErr != nil || format == OutputText {
		_, writeErr := fmt.Fprintf(writer, "ax: %v\n", err)
		return writeErr
	}
	envelope := errorEnvelope{Error: structuredError{
		Code: classifyError(err), Message: err.Error(), ExitCode: ExitCode(err),
	}}
	return writeStructured(writer, envelope, format)
}

func classifyError(err error) string {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return "external_command"
	}
	message := err.Error()
	if strings.Contains(message, "no verified") || strings.Contains(message, "does not expose a verified") {
		return "unsupported_capability"
	}
	for _, marker := range []string{"usage:", "unknown command", "unknown ", "requires a value", "must be", "mutually exclusive", "is only supported"} {
		if strings.Contains(message, marker) {
			return "usage"
		}
	}
	return "operation_failed"
}

func (a App) structuredDefault() OutputFormat {
	if a.Output == OutputText {
		return OutputJSON
	}
	return a.Output
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if line, _, ok := strings.Cut(value, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return value
}
