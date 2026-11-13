package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

// Parsing is shared so every entry point supplies the same logical parameters.
func parseRunOption(args []string, index *int, request *runtime.RunRequest) (bool, error) {
	option := args[*index]
	switch option {
	case "--":
		request.Flags = append(request.Flags, args[*index+1:]...)
		*index = len(args)
		return true, nil
	case "--model", "--effort", "--mode", "--user-config", "--append-system-prompt", "--mcp-config":
	default:
		return false, nil
	}
	*index = *index + 1
	if *index >= len(args) || strings.TrimSpace(args[*index]) == "" {
		return true, fmt.Errorf("%s requires a value", option)
	}
	value := args[*index]
	switch option {
	case "--model":
		request.Model = value
	case "--effort":
		request.Effort = value
	case "--mode":
		request.Mode = value
	case "--append-system-prompt":
		request.SystemPrompt = value
	case "--user-config":
		if value != "true" && value != "false" {
			return true, fmt.Errorf("--user-config must be true or false")
		}
		enabled, _ := strconv.ParseBool(value)
		request.UserConfig = &enabled
	case "--mcp-config":
		config, err := readMCPConfig(value)
		if err != nil {
			return true, err
		}
		request.MCP = config
	}
	return true, nil
}
