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
