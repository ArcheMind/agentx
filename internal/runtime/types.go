package runtime

import (
	"context"
	"io"
)

type Capability string

const (
	CapabilityLaunch       Capability = "launch"
	CapabilityInstall      Capability = "install"
	CapabilityModelList    Capability = "model_list"
	CapabilityModelSelect  Capability = "model_select"
	CapabilitySessionList  Capability = "session_list"
	CapabilitySessionRead  Capability = "session_read"
	CapabilitySessionWrite Capability = "session_write"
)

type CommandPlan struct {
	Executable string            `json:"executable"`
	Args       []string          `json:"args"`
	Cwd        string            `json:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
}

type Detection struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Installed    bool         `json:"installed"`
	Path         string       `json:"path,omitempty"`
	Version      string       `json:"version,omitempty"`
	Capabilities []Capability `json:"capabilities"`
}

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
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
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
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

type SessionDriver interface {
	Plan(args []string) (CommandPlan, error)
}

type Agent struct {
	ID           string
	Name         string
	Binary       string
	Launch       LaunchDriver
	Package      PackageDriver
	Models       ModelDriver
	Capabilities []Capability
}
