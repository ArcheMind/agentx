package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArcheMind/agentx/internal/inventory"
	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
)

type Service struct{ Home, Project string }

func (s Service) List() inventory.Result {
	sources := []inventory.Source{}
	for _, scope := range []string{"project", "user"} {
		base := s.Project
		if scope == "user" {
			base = s.Home
		}
		candidates := []struct{ path, agent, format string }{
			{filepath.Join(base, ".mcp.json"), "claude", "json"},
			{filepath.Join(base, ".gemini", "settings.json"), "gemini", "json"},
			{filepath.Join(base, ".codex", "config.toml"), "codex", "toml"},
		}
		openBase := filepath.Join(base, ".opencode")
		if scope == "user" {
			openBase = filepath.Join(base, ".config", "opencode")
		}
		for _, name := range []string{"opencode.json", "opencode.jsonc"} {
			candidates = append(candidates, struct{ path, agent, format string }{filepath.Join(openBase, name), "opencode", "jsonc"})
			if scope == "project" {
				candidates = append(candidates, struct{ path, agent, format string }{filepath.Join(base, name), "opencode", "jsonc"})
			}
		}
		if scope == "user" {
			candidates[0].path = filepath.Join(base, ".claude.json")
			if value := os.Getenv("CLAUDE_CONFIG_DIR"); value != "" {
				candidates[0].path = filepath.Join(value, ".claude.json")
			}
			if value := os.Getenv("CODEX_HOME"); value != "" {
				candidates[2].path = filepath.Join(value, "config.toml")
			}
		}
		if scope == "project" {
			for _, parent := range inventory.ProjectDirectories(s.Project)[1:] {
				for _, name := range []string{"opencode.json", "opencode.jsonc"} {
					for _, dir := range []string{parent, filepath.Join(parent, ".opencode")} {
						candidates = append(candidates, struct{ path, agent, format string }{filepath.Join(dir, name), "opencode", "jsonc"})
					}
				}

				candidates = append(candidates, struct{ path, agent, format string }{filepath.Join(parent, ".codex", "config.toml"), "codex", "toml"})
			}
			candidates = append(candidates, struct{ path, agent, format string }{filepath.Join(base, "config.toml"), "codex", "toml"})
		}

		if scope == "user" {
			candidates[1].path = filepath.Join(inventory.UserDirectory(base, "gemini"), "settings.json")

			for index := range candidates {
				candidate := &candidates[index]
				if candidate.agent == "opencode" {
					candidate.path = filepath.Join(inventory.UserDirectory(base, "opencode"), filepath.Base(candidate.path))
				}
			}
		}
		for _, candidate := range candidates {
			content, err := os.ReadFile(candidate.path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			source := inventory.Source{Scope: scope, Path: inventory.Physical(candidate.path), Agents: []string{candidate.agent}, Status: "ok"}
			if err != nil {
				source.Path = candidate.path
				source.Status = "failed"
				source.Error = err.Error()
				sources = append(sources, source)
				continue
			}
			var names []string
			if candidate.format == "json" || candidate.format == "jsonc" {
				if candidate.format == "jsonc" {
					content, err = hujson.Standardize(content)
				}
				var root map[string]json.RawMessage
				if err == nil {
					err = json.Unmarshal(content, &root)
				}
				if err == nil {
					var servers map[string]json.RawMessage
					key := "mcpServers"
					if candidate.agent == "opencode" {
						key = "mcp"
					}
					if data, ok := root[key]; ok {
						err = json.Unmarshal(data, &servers)
					}
					for name := range servers {
						names = append(names, name)
					}
					// Claude stores local project entries in the user configuration, keyed by absolute project path.
					if candidate.agent == "claude" && scope == "user" && err == nil {
						var projects map[string]struct {
							Servers map[string]json.RawMessage `json:"mcpServers"`
						}
						if data, ok := root["projects"]; ok {
							err = json.Unmarshal(data, &projects)
							if err == nil {
								if project, ok := projects[filepath.Clean(s.Project)]; ok {
									local := inventory.Source{Scope: "project", Path: source.Path, Agents: source.Agents, Status: "ok"}
									for name := range project.Servers {
										local.Resources = append(local.Resources, resource(name, source.Path, candidate.agent, candidate.path))
									}
									sources = append(sources, local)
								}
							}
						}
					}
				}
			} else {
				names, err = tomlNames(content)
			}
			if err != nil {
				source.Status = "failed"
				source.Error = err.Error()
			}
			for _, name := range names {
				source.Resources = append(source.Resources, resource(name, source.Path, candidate.agent, candidate.path))
			}
			sources = append(sources, source)
		}
	}
	for _, agent := range []string{"dsh", "pi"} {
		sources = append(sources, inventory.Source{Scope: "user", Path: agent + " MCP discovery", Agents: []string{agent}, Status: "unsupported"})
	}
	sources = append(sources, inventory.Source{Scope: "user", Path: "AX MCP common configuration (no defined storage contract)", Status: "unsupported"})
	for _, agent := range []string{"claude", "codex", "gemini", "opencode"} {
		sources = append(sources, inventory.Source{Scope: "user", Path: agent + " dynamic/plugin/managed/CLI MCP sources", Agents: []string{agent}, Status: "unsupported", Error: "Runtime and managed sources are not inspected"})
	}
	return inventory.Normalize(sources)
}
func resource(name, path, agent, projection string) inventory.Resource {
	item := inventory.Resource{Name: name, Path: path, Agents: []string{agent}, Evidence: inventory.Configured()}
	if projection != path {
		item.Projections = []inventory.Projection{{Path: projection, Agents: []string{agent}}}
	}
	return item
}

func tomlNames(content []byte) ([]string, error) {
	var root struct {
		Servers map[string]map[string]any `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(content, &root); err != nil {
		return nil, fmt.Errorf("parse MCP TOML: %w", err)
	}
	names := []string{}
	for name := range root.Servers {
		names = append(names, name)
	}
	return names, nil
}
