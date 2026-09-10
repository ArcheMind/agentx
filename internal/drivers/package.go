package drivers

import (
	"fmt"
	"strings"

	"agentx/internal/runtime"
)

type NPMPackage struct {
	Package string
}

func (d NPMPackage) PlanInstall(version string) (runtime.CommandPlan, error) {
	spec := d.Package
	if version != "" {
		if strings.ContainsAny(version, " \t\r\n") {
			return runtime.CommandPlan{}, fmt.Errorf("version must not contain whitespace")
		}
		spec += "@" + version
	}
	return runtime.CommandPlan{Executable: "npm", Args: []string{"install", "--global", spec}}, nil
}

type CASRPackage struct{}

func (CASRPackage) PlanInstall(version string) (runtime.CommandPlan, error) {
	if version != "" && version != "latest" {
		return runtime.CommandPlan{}, fmt.Errorf("casr installer only supports the latest release")
	}
	return runtime.CommandPlan{
		Executable: "bash",
		Args: []string{
			"-c",
			`set -o pipefail; curl -fsSL "https://raw.githubusercontent.com/Dicklesworthstone/cross_agent_session_resumer/main/install.sh?$(date +%s)" | bash`,
		},
	}, nil
}
