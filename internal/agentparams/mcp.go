package agentparams

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type MCPServer struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	EnvPassthrough []string          `json:"envPassthrough,omitempty"`
}
type MCPConfig struct {
	Servers map[string]MCPServer `json:"mcpServers"`
}
type AgyMCPServer struct {
	Name    string      `json:"name"`
	Command string      `json:"command"`
	Args    []string    `json:"args"`
	Env     []AgyMCPEnv `json:"env"`
}
type AgyMCPEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func serverNames(m MCPConfig) []string {
	names := make([]string, 0, len(m.Servers))
	for name := range m.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func AgyMCPServers(config MCPConfig) []AgyMCPServer {
	result := make([]AgyMCPServer, 0, len(config.Servers))
	for _, name := range serverNames(config) {
		server := config.Servers[name]
		env := map[string]string{}
		for _, key := range server.EnvPassthrough {
			if value, ok := os.LookupEnv(key); ok {
				env[key] = value
			}
		}
		for key, value := range server.Env {
			env[key] = value
		}
		entry := AgyMCPServer{Name: name, Command: server.Command, Args: append([]string{}, server.Args...), Env: []AgyMCPEnv{}}
		for _, key := range sortedKeys(env) {
			entry.Env = append(entry.Env, AgyMCPEnv{Name: key, Value: env[key]})
		}
		result = append(result, entry)
	}
	return result
}

func claudeMCP(m MCPConfig) (string, error) {
	servers := map[string]any{}
	for _, name := range serverNames(m) {
		s := m.Servers[name]
		entry := map[string]any{"type": "stdio", "command": s.Command}
		if len(s.Args) > 0 {
			entry["args"] = s.Args
		}
		if len(s.Env) > 0 {
			entry["env"] = s.Env
		}
		servers[name] = entry
	}
	raw, err := json.Marshal(map[string]any{"mcpServers": servers})
	return string(raw), err
}
func codexMCPArgs(m MCPConfig) ([]string, error) {
	var args []string
	for _, name := range serverNames(m) {
		s := m.Servers[name]
		args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.command=%q", name, s.Command))
		if len(s.Args) > 0 {
			raw, _ := json.Marshal(s.Args)
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.args=%s", name, raw))
		}
		for _, key := range sortedKeys(s.Env) {
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.env.%s=%q", name, key, s.Env[key]))
		}
		if len(s.EnvPassthrough) > 0 {
			raw, _ := json.Marshal(s.EnvPassthrough)
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.env_vars=%s", name, raw))
		}
	}
	return args, nil
}
func codexMCPConfig(m MCPConfig) map[string]any {
	servers := map[string]any{}
	for _, name := range serverNames(m) {
		s := m.Servers[name]
		entry := map[string]any{"command": s.Command}
		if len(s.Args) > 0 {
			entry["args"] = s.Args
		}
		if len(s.Env) > 0 {
			entry["env"] = s.Env
		}
		if len(s.EnvPassthrough) > 0 {
			entry["env_vars"] = s.EnvPassthrough
		}
		servers[name] = entry
	}
	return servers
}
