package runtime

import (
	"context"
	"io"
)

type Capability string

const (
	CapabilityLaunch      Capability = "launch"
	CapabilityAuthLogin   Capability = "auth_login"
	CapabilityAuthStatus  Capability = "auth_status"
	CapabilityModelList   Capability = "model_list"
	CapabilityModelSelect Capability = "model_select"
)

type CommandPlan struct {
	Executable string            `json:"executable" yaml:"executable"`
	Args       []string          `json:"args" yaml:"args"`
	Cwd        string            `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

type Detection struct {
	ID           string       `json:"id" yaml:"id"`
	Name         string       `json:"name" yaml:"name"`
	Installed    bool         `json:"installed" yaml:"installed"`
	Path         string       `json:"path,omitempty" yaml:"path,omitempty"`
	Version      string       `json:"version,omitempty" yaml:"version,omitempty"`
	Capabilities []Capability `json:"capabilities" yaml:"capabilities"`
}

type Model struct {
	ID          string `json:"id" yaml:"id"`
	DisplayName string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Source      string `json:"source" yaml:"source"`
}

type RunRequest struct {
	Cwd             string
	Model           string
	PassthroughArgs []string
}

type ExecuteOptions struct {
	Interactive bool
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

type CommandResult struct {
	Stdout   string `json:"stdout" yaml:"stdout"`
	Stderr   string `json:"stderr" yaml:"stderr"`
	ExitCode int    `json:"exit_code" yaml:"exit_code"`
}

type AuthPlan struct {
	Command     CommandPlan `json:"command" yaml:"command"`
	Instruction string      `json:"instruction,omitempty" yaml:"instruction,omitempty"`
}

type AuthStatus struct {
	Agent        string `json:"agent" yaml:"agent"`
	Supported    bool   `json:"supported" yaml:"supported"`
	LoggedIn     bool   `json:"logged_in" yaml:"logged_in"`
	Method       string `json:"method,omitempty" yaml:"method,omitempty"`
	Subscription string `json:"subscription,omitempty" yaml:"subscription,omitempty"`
}

type Package struct {
	ID      string
	Name    string
	Install PackageDriver
}

type Runner interface {
	Execute(context.Context, CommandPlan, ExecuteOptions) (CommandResult, error)
}

type LaunchDriver interface {
	PlanRun(RunRequest) (CommandPlan, error)
}

type PackageDriver interface {
	PlanInstall(version string) (CommandPlan, error)
}

type ModelDriver interface {
	ListModels(context.Context, Runner) ([]Model, error)
}

type AuthDriver interface {
	PlanLogin() AuthPlan
	SupportsStatus() bool
	Status(context.Context, Runner) (AuthStatus, error)
}

type Agent struct {
	ID           string
	Name         string
	Binary       string
	Launch       LaunchDriver
	Models       ModelDriver
	Auth         AuthDriver
	Capabilities []Capability
}
