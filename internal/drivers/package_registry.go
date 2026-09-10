package drivers

import (
	"fmt"
	"sort"

	"agentx/internal/runtime"
)

type PackageRegistry struct {
	packages map[string]runtime.Package
}

func NewPackageRegistry() PackageRegistry {
	packages := []runtime.Package{
		{ID: "claude", Name: "Claude Code", Install: NPMPackage{Package: "@anthropic-ai/claude-code"}},
		{ID: "codex", Name: "Codex CLI", Install: NPMPackage{Package: "@openai/codex"}},
		{ID: "gemini", Name: "Gemini CLI", Install: NPMPackage{Package: "@google/gemini-cli"}},
		{ID: "opencode", Name: "OpenCode", Install: NPMPackage{Package: "opencode-ai"}},
		{ID: "pi", Name: "Pi Coding Agent", Install: NPMPackage{Package: "@mariozechner/pi-coding-agent"}},
		{ID: "casr", Name: "Cross Agent Session Resumer", Install: CASRPackage{}},
	}
	items := make(map[string]runtime.Package, len(packages))
	for _, item := range packages {
		items[item.ID] = item
	}
	return PackageRegistry{packages: items}
}

func (r PackageRegistry) Get(id string) (runtime.Package, error) {
	item, ok := r.packages[id]
	if !ok {
		return runtime.Package{}, fmt.Errorf("unknown package %q", id)
	}
	return item, nil
}

func (r PackageRegistry) All() []runtime.Package {
	items := make([]runtime.Package, 0, len(r.packages))
	for _, item := range r.packages {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}
