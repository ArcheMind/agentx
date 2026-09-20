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
	Registry   drivers.Registry
	Sessions   sessions.Service
	Runner     runtime.Runner
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Output     OutputFormat
	Color      bool
	termHeight int
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
		Color: colorEnabled(stdout),
	}
}

func (a App) Run(ctx context.Context, args []string) error {
	output, args, err := parseOutputFormat(args)
	if err != nil {
		return err
	}
	a.Output = output
	includeSubagents := false
	if len(args) > 0 && args[0] == "--include-subagents" {
		includeSubagents = true
		args = args[1:]
	}
	if len(args) == 0 {
		if err := a.rejectStructured("interactive session resume"); err != nil {
			return err
		}
		return a.interactiveResume(ctx, includeSubagents)
	}
	if includeSubagents {
		return fmt.Errorf("--include-subagents is only supported by interactive session resume or session list")
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
	case "list":
		return a.overview(ctx, args[1:])
	case "agent":
		return a.agent(ctx, args[1:])
	case "auth":
		return a.auth(ctx, args[1:])
	case "session":
		return a.sessions(ctx, args[1:])
	case "convert":
		return a.convert(args[1:])
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
	case "show", "which", "models":
		return a.agentShow(ctx, args[0], args[1:])
	case "install":
		return a.install(ctx, args[1:])
	case "run":
		return a.runAgent(ctx, args[1:])
	default:
		return agentUsageError()
	}
}

func agentUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] agent <list|show|install|run> [args...]")
}

func (a App) list(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: ax [--json|--yaml] agent list")
	}
	detections := make([]runtime.Detection, 0)
	for _, agent := range a.Registry.All() {
		detections = append(detections, a.detectAgent(ctx, agent))
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

type agentShowResult struct {
	ID           string               `json:"id" yaml:"id"`
	Name         string               `json:"name" yaml:"name"`
	Binary       string               `json:"binary" yaml:"binary"`
	Path         string               `json:"path,omitempty" yaml:"path,omitempty"`
	Capabilities []runtime.Capability `json:"capabilities" yaml:"capabilities"`
	Models       []runtime.Model      `json:"models,omitempty" yaml:"models,omitempty"`
}

func (a App) agentShow(ctx context.Context, verb string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ax [--json|--yaml] agent show <agent>")
	}
	agent, err := a.Registry.Get(args[0])
	if err != nil {
		return err
	}

	result := agentShowResult{
		ID:           agent.ID,
		Name:         agent.Name,
		Binary:       agent.Binary,
		Capabilities: agent.Capabilities,
	}

	path, pathErr := exec.LookPath(agent.Binary)
	if pathErr == nil {
		result.Path = path
	}

	// Legacy alias: "which" outputs only the path
	if verb == "which" {
		if err := a.rejectStructured("agent which"); err != nil {
			return err
		}
		if pathErr != nil {
			return fmt.Errorf("%s is not installed; run ax agent install %s", agent.Name, agent.ID)
		}
		fmt.Fprintln(a.Stdout, path)
		return nil
	}

	// Legacy alias: "models" outputs only the model list
	if verb == "models" {
		return a.models(ctx, args)
	}

	if agent.Models != nil {
		if _, unsupported := agent.Models.(drivers.UnsupportedModels); !unsupported {
			if models, modelErr := agent.Models.ListModels(ctx, a.Runner); modelErr == nil {
				result.Models = models
			}
		}
	}

	if a.Output != OutputText {
		return writeStructured(a.Stdout, result, a.Output)
	}

	fmt.Fprintf(a.Stdout, "Agent: %s (%s)\n", result.Name, result.ID)
	fmt.Fprintf(a.Stdout, "Binary: %s\n", result.Binary)
	if result.Path != "" {
		fmt.Fprintf(a.Stdout, "Path: %s\n", result.Path)
	} else {
		fmt.Fprintf(a.Stdout, "Path: not installed\n")
	}
	if len(result.Capabilities) > 0 {
		caps := make([]string, len(result.Capabilities))
		for i, c := range result.Capabilities {
			caps[i] = string(c)
		}
		fmt.Fprintf(a.Stdout, "Capabilities: %s\n", strings.Join(caps, ", "))
	}
	if len(result.Models) > 0 {
		fmt.Fprintln(a.Stdout, "Models:")
		for _, model := range result.Models {
			if model.DisplayName != "" && model.DisplayName != model.ID {
				fmt.Fprintf(a.Stdout, "  %s\t%s\n", model.ID, model.DisplayName)
			} else {
				fmt.Fprintf(a.Stdout, "  %s\n", model.ID)
			}
		}
	}
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
	if len(args) == 0 {
		return authUsageError()
	}
	// "auth list" requires no agent argument
	if args[0] == "list" {
		return a.authList(ctx, args[1:])
	}
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
	if args[0] == "show" || args[0] == "status" {
		if len(args) != 2 {
			return fmt.Errorf("usage: ax [--json|--yaml] auth show <agent>")
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
	options := runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr}
	var result runtime.CommandResult
	if args[0] == "login" {
		if executor, ok := agent.Auth.(drivers.LoginExecutor); ok {
			result, err = executor.Login(ctx, a.Runner, options)
		} else {
			result, err = a.Runner.Execute(ctx, plan.Command, options)
		}
	} else {
		result, err = a.Runner.Execute(ctx, plan.Command, options)
	}
	if err != nil {
		return &ExitError{Code: result.ExitCode, Err: err}
	}
	return nil
}

func (a App) authList(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: ax [--json|--yaml] auth list")
	}
	type authListEntry struct {
		Agent     string                 `json:"agent" yaml:"agent"`
		Installed bool                   `json:"installed" yaml:"installed"`
		Status    string                 `json:"status" yaml:"status"`
		Providers []runtime.AuthProvider `json:"providers,omitempty" yaml:"providers,omitempty"`
	}
	var entries []authListEntry
	for _, agent := range a.Registry.All() {
		entry := authListEntry{Agent: agent.ID}
		if _, err := exec.LookPath(agent.Binary); err != nil {
			entry.Status = "not_installed"
			entries = append(entries, entry)
			continue
		}
		entry.Installed = true
		if agent.Auth == nil || !agent.Auth.SupportsStatus() {
			entry.Status = "unknown"
			entries = append(entries, entry)
			continue
		}
		status, err := agent.Auth.Status(ctx, a.Runner)
		if err != nil {
			entry.Status = "error"
			entries = append(entries, entry)
			continue
		}
		if len(status.Providers) == 0 {
			entry.Status = "not_logged_in"
		} else {
			entry.Status = "logged_in"
			entry.Providers = status.Providers
		}
		entries = append(entries, entry)
	}
	if a.Output != OutputText {
		return writeStructured(a.Stdout, entries, a.Output)
	}
	for _, entry := range entries {
		if len(entry.Providers) > 0 {
			for _, p := range entry.Providers {
				details := []string{}
				if p.Method != "" {
					details = append(details, p.Method)
				}
				if p.Subscription != "" {
					details = append(details, p.Subscription)
				}
				line := fmt.Sprintf("%-10s %s", entry.Agent, p.ID)
				if len(details) > 0 {
					line += " (" + strings.Join(details, ", ") + ")"
				}
				fmt.Fprintln(a.Stdout, line)
			}
		} else {
			fmt.Fprintf(a.Stdout, "%-10s %s\n", entry.Agent, entry.Status)
		}
	}
	return nil
}

func authUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] auth <list|show|login|logout> [args...]")
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
		return fmt.Errorf("session providers has been removed; use ax list instead")
	case "list":
		return a.sessionList(args[1:])
	case "show", "info":
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
		case "--include-subagents":
			options.IncludeSubagents = true
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
		return fmt.Errorf("usage: ax [--json|--yaml] session show <session-id> [--source <provider>] [--peek] [--peek-lines <n>]")
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
			return fmt.Errorf("unknown session show option %q", args[index])
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
			fmt.Fprintf(a.Stdout, "[%s] %s\n", message.Role, singleLine(sessions.TextContent(message.Content), 200))
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

func (a App) interactiveResume(ctx context.Context, includeSubagents bool) error {
	reader := bufio.NewReader(a.Stdin)
	fmt.Fprint(a.Stdout, "Loading recent sessions...\r")
	groups, err := a.recentSessionGroups(includeSubagents)
	if err != nil {
		return err
	}
	fmt.Fprint(a.Stdout, "\x1b[2K\r")
	selected, err := a.chooseSession(reader, groups)
	if err != nil {
		return err
	}
	detail, err := a.Sessions.Info(selected.ID, selected.Provider)
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
	labels := make([]string, len(agents))
	for index, agent := range agents {
		labels[index] = agent.ID
	}
	agentIndex, err := a.chooseOptions(reader, "Choose an agent:", labels)
	if err != nil {
		return err
	}
	agent := agents[agentIndex]
	model, err := a.chooseModel(ctx, reader, agent)
	if err != nil {
		return err
	}
	return a.resumeSession(ctx, agent, detail, "", model, false)
}

func (a App) chooseModel(ctx context.Context, reader *bufio.Reader, agent runtime.Agent) (string, error) {
	if _, unsupported := agent.Models.(drivers.UnsupportedModels); unsupported || !hasCapability(agent.Capabilities, runtime.CapabilityModelSelect) {
		_, err := a.chooseOptions(reader, "Choose a model:", []string{"Native default"})
		return "", err
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
	choice, err := a.chooseOptions(reader, "Choose a model:", labels)
	if err != nil {
		return "", err
	}
	if choice == 0 {
		return "", nil
	}
	return models[choice-1].ID, nil
}

func (a App) prompt(reader *bufio.Reader, label string, allowBlank bool) (string, error) {
	fmt.Fprint(a.Stdout, label)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read interactive input: %w", err)
	}
	if errors.Is(err, io.EOF) && value == "" {
		return "", errors.New("interactive input closed")
	}
	value = strings.TrimSpace(value)
	if value == "" && !allowBlank {
		return "", errors.New("a selection is required")
	}
	return value, nil
}

func sessionUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] session <list|show|resume> [args...]")
}

func singleLine(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max-1]) + "…"
}

func (a App) printHelp() {
	fmt.Fprint(a.Stdout, `agentx manages native AI coding-agent runtimes.

Usage:
  ax [--include-subagents]
  ax <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]
  ax [--json|--yaml] list

  ax [--json|--yaml] agent <list|show|install|run> [args...]
  ax [--json|--yaml] auth <list|show|login|logout> [args...]
  ax [--json|--yaml] session <list|show|resume> [args...]

  ax convert --to <provider> [< unified.json]
  ax convert --from <provider> [< native.jsonl]

  ax version

Running ax without arguments starts an interactive session resume. The ax <agent>
shortcut launches an installed agent directly.

Agents: claude, codex, dsh, gemini, opencode, pi

Set AX_LOG=debug or pass --verbose before the command to log raw external input,
output, and errors as JSON on stderr.
`)
}

var convertProviders = []string{"claude", "codex", "gemini", "opencode", "pi"}

func isConvertProvider(id string) bool {
	for _, p := range convertProviders {
		if p == id {
			return true
		}
	}
	return false
}

func (a App) convert(args []string) error {
	var toProvider, fromProvider string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--to":
			i++
			if i >= len(args) {
				return fmt.Errorf("--to requires a value")
			}
			toProvider = args[i]
		case "--from":
			i++
			if i >= len(args) {
				return fmt.Errorf("--from requires a value")
			}
			fromProvider = args[i]
		default:
			return fmt.Errorf("unknown convert option %q", args[i])
		}
	}
	if toProvider == "" && fromProvider == "" {
		return fmt.Errorf("usage: ax convert --to <provider> or ax convert --from <provider>")
	}
	if toProvider != "" && fromProvider != "" {
		return fmt.Errorf("--to and --from are mutually exclusive")
	}

	if toProvider != "" {
		return a.convertTo(toProvider)
	}
	return a.convertFrom(fromProvider)
}

func (a App) convertTo(provider string) error {
	if !isConvertProvider(provider) {
		return fmt.Errorf("unknown convert provider %q; supported: %s", provider, strings.Join(convertProviders, ", "))
	}
	data, err := io.ReadAll(a.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	var detail sessions.Detail
	if err := json.Unmarshal(data, &detail); err != nil {
		return fmt.Errorf("parse unified JSON from stdin: %w", err)
	}

	switch provider {
	case "claude":
		return writeJSONL(a.Stdout, sessions.SerializeClaude(detail))
	case "codex":
		return writeJSONL(a.Stdout, sessions.SerializeCodex(detail))
	case "pi":
		return writeJSONL(a.Stdout, sessions.SerializePi(detail))
	case "gemini":
		encoder := json.NewEncoder(a.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(sessions.SerializeGemini(detail))
	case "opencode":
		return fmt.Errorf("opencode requires a directory target; use session resume instead")
	}
	return nil
}

func (a App) convertFrom(provider string) error {
	if !isConvertProvider(provider) {
		return fmt.Errorf("unknown convert provider %q; supported: %s", provider, strings.Join(convertProviders, ", "))
	}
	data, err := io.ReadAll(a.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	var records []map[string]any
	switch provider {
	case "gemini":
		var single map[string]any
		if err := json.Unmarshal(data, &single); err != nil {
			return fmt.Errorf("parse %s JSON from stdin: %w", provider, err)
		}
		records = []map[string]any{single}
	default:
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				return fmt.Errorf("parse %s JSONL from stdin: %w", provider, err)
			}
			records = append(records, record)
		}
	}

	detail, ok := sessions.ParseRecords(provider, records)
	if !ok {
		return fmt.Errorf("no valid session found in %s input", provider)
	}

	output := a.Output
	if output == OutputText {
		output = OutputJSON
	}
	return writeStructured(a.Stdout, detail, output)
}

func writeJSONL(writer io.Writer, records []map[string]any) error {
	encoder := json.NewEncoder(writer)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
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
