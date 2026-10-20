package skills

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArcheMind/agentx/internal/inventory"
)

type skillRoot struct {
	path      string
	agents    []string
	preferred bool
}

func (s Service) Inventory() inventory.Result {
	sources := []inventory.Source{}
	for _, scope := range []string{"project", "user"} {
		base := s.Project
		if scope == "user" {
			base = s.Home
		}
		roots := []skillRoot{
			{filepath.Join(base, ".agents", "skills"), []string{"codex", "dsh", "gemini", "opencode", "pi"}, true},
			{filepath.Join(base, ".claude", "skills"), []string{"claude", "opencode"}, false},
			{filepath.Join(base, ".codex", "skills"), []string{"codex"}, false},
			{filepath.Join(base, ".gemini", "skills"), []string{"gemini"}, false},
		}
		dshBase := filepath.Join(base, ".dsh")
		if scope == "user" {
			if value := os.Getenv("DSH_HOME"); value != "" {
				dshBase = value
			}
		}
		roots = append(roots, skillRoot{filepath.Join(dshBase, "skills"), []string{"dsh"}, false})
		openBase := filepath.Join(base, ".opencode")
		if scope == "user" {
			openBase = filepath.Join(base, ".config", "opencode")
		}
		for _, name := range []string{"skill", "skills"} {
			roots = append(roots, skillRoot{filepath.Join(openBase, name), []string{"opencode"}, false})
		}
		if scope == "user" {
			roots = append(roots, skillRoot{filepath.Join(base, ".pi", "agent", "skills"), []string{"pi"}, false})
		} else {
			roots = append(roots, skillRoot{filepath.Join(base, ".pi", "skills"), []string{"pi"}, false})
		}

		if scope == "user" {
			codexBase := filepath.Join(base, ".codex")
			if value := os.Getenv("CODEX_HOME"); value != "" {
				codexBase = value
			}
			roots[2].path = filepath.Join(codexBase, "skills")
			roots = append(roots, skillRoot{filepath.Join(codexBase, "skills", ".system"), []string{"codex"}, false})
			if value := os.Getenv("DSH_AGENTS_HOME"); value != "" {
				roots[0].agents = []string{"codex", "gemini", "opencode", "pi"}
				roots = append(roots, skillRoot{filepath.Join(value, "skills"), []string{"dsh"}, false})
			}
		} else {
			ancestors := inventory.ProjectDirectories(s.Project)
			for _, parent := range ancestors[1:] {
				for _, name := range []string{"skill", "skills"} {
					roots = append(roots, skillRoot{filepath.Join(parent, ".opencode", name), []string{"opencode"}, false})
				}
				roots = append(roots, skillRoot{filepath.Join(parent, ".claude", "skills"), []string{"opencode"}, false})

				roots = append(roots, skillRoot{filepath.Join(parent, ".agents", "skills"), []string{"codex", "opencode", "pi"}, true}, skillRoot{filepath.Join(parent, ".codex", "skills"), []string{"codex"}, false})
			}
			if len(ancestors) > 0 {
				root := ancestors[len(ancestors)-1]
				if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
					roots[4].path = filepath.Join(root, ".dsh", "skills")
					if root != s.Project {
						roots[0].agents = []string{"codex", "gemini", "opencode", "pi"}
						roots = append(roots, skillRoot{filepath.Join(root, ".agents", "skills"), []string{"dsh"}, true})
					}
				}
			}
		}

		if scope == "user" {
			if value := os.Getenv("GEMINI_CLI_HOME"); value != "" {
				agents := []string{}
				for _, agent := range roots[0].agents {
					if agent != "gemini" {
						agents = append(agents, agent)
					}
				}
				roots[0].agents = agents
				roots = append(roots, skillRoot{filepath.Join(value, ".agents", "skills"), []string{"gemini"}, false})
			}

			for index := range roots {
				root := &roots[index]
				switch root.path {
				case filepath.Join(base, ".gemini", "skills"):
					root.path = filepath.Join(inventory.UserDirectory(base, "gemini"), "skills")
				case filepath.Join(base, ".claude", "skills"):
					if os.Getenv("CLAUDE_CONFIG_DIR") != "" {
						root.agents = []string{"opencode"}
						roots = append(roots, skillRoot{filepath.Join(inventory.UserDirectory(base, "claude"), "skills"), []string{"claude"}, false})
					}
				case filepath.Join(base, ".pi", "agent", "skills"):
					root.path = filepath.Join(inventory.UserDirectory(base, "pi"), "skills")
				case filepath.Join(base, ".config", "opencode", "skill"):
					root.path = filepath.Join(inventory.UserDirectory(base, "opencode"), "skill")
				case filepath.Join(base, ".config", "opencode", "skills"):
					root.path = filepath.Join(inventory.UserDirectory(base, "opencode"), "skills")
				}
			}
		}
		for _, root := range roots {
			source := inventory.Source{Scope: scope, Path: inventory.Physical(root.path), Agents: root.agents, Preferred: root.preferred, Status: "ok"}
			seen := map[string]bool{}
			var visit func(string, bool)
			visit = func(path string, top bool) {
				physical := inventory.Physical(path)
				if seen[physical] {
					return
				}
				seen[physical] = true
				defer delete(seen, physical)
				entries, err := os.ReadDir(path)
				if errors.Is(err, os.ErrNotExist) && top {
					return
				}
				if err != nil {
					failed := source
					failed.Path = path
					failed.Status = "failed"
					failed.Error = err.Error()
					sources = append(sources, failed)
					return
				}
				if !top {
					if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
						metadata, err := readMetadata(path)
						if err != nil {
							failed := source
							failed.Path = path
							failed.Status = "failed"
							failed.Error = err.Error()
							sources = append(sources, failed)
							return
						}
						agents := []string{}
						relative, _ := filepath.Rel(root.path, path)
						for _, agent := range root.agents {
							if agent == "dsh" && strings.Contains(relative, string(filepath.Separator)) {
								continue
							}
							agents = append(agents, agent)
						}
						if len(agents) == 0 {
							return
						}
						item := inventory.Resource{Name: metadata.Name, Description: metadata.Description, Path: physical, Agents: agents, Evidence: inventory.Configured()}
						if filepath.Clean(path) != physical {
							item.Projections = []inventory.Projection{{Path: path, Agents: agents}}
						}
						block := source
						block.Path = filepath.Dir(physical)
						if relative, err := filepath.Rel(source.Path, physical); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
							block.Path = source.Path
						}
						block.Resources = []inventory.Resource{item}
						block.Preferred = root.preferred && block.Path == inventory.Physical(root.path)
						sources = append(sources, block)
						return
					} else if !errors.Is(err, os.ErrNotExist) {
						failed := source
						failed.Path = path
						failed.Status = "failed"
						failed.Error = err.Error()
						sources = append(sources, failed)
						return
					}
				}
				count := len(sources)
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".") {
						continue
					}
					child := filepath.Join(path, entry.Name())
					info, err := os.Stat(child)
					if err != nil {
						failed := source
						failed.Path = child
						failed.Status = "failed"
						failed.Error = err.Error()
						sources = append(sources, failed)
						continue
					}
					if info.IsDir() {
						visit(child, false)
					} else if strings.HasSuffix(strings.ToLower(child), ".md") && entry.Name() != "SKILL.md" {
						agents := []string{}
						for _, agent := range root.agents {
							if agent == "dsh" && top || agent == "pi" && (!top || strings.Contains(root.path, string(filepath.Separator)+".pi"+string(filepath.Separator))) {
								agents = append(agents, agent)
							}
						}
						if len(agents) == 0 {
							continue
						}
						metadata, err := readMetadataFile(child)
						if err != nil || metadata.Description == "" {
							continue
						}
						if metadata.Name == "" {
							metadata.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
						}
						actual := inventory.Physical(child)
						item := inventory.Resource{Name: metadata.Name, Description: metadata.Description, Path: actual, Agents: agents, Evidence: inventory.Configured()}
						if actual != child {
							item.Projections = []inventory.Projection{{Path: child, Agents: agents}}
						}
						block := source
						block.Path = filepath.Dir(actual)
						block.Resources = []inventory.Resource{item}
						sources = append(sources, block)
					}
				}
				if top && len(sources) == count {
					sources = append(sources, source)
				}
			}
			visit(root.path, true)
		}

	}
	for _, agent := range []string{"claude", "codex", "dsh", "gemini", "opencode", "pi"} {
		sources = append(sources, inventory.Source{Scope: "user", Path: agent + " dynamic/plugin/package/CLI/settings skill sources", Agents: []string{agent}, Status: "unsupported", Error: "Only documented filesystem roots are inventoried; custom settings, runtime and package sources are not inspected"})
	}
	return inventory.Normalize(sources)
}
