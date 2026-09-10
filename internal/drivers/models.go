package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentx/internal/runtime"
)

type CommandModels struct {
	Plan runtime.CommandPlan
}

func (d CommandModels) ListModels(ctx context.Context, runner runtime.Runner) ([]runtime.Model, error) {
	result, err := runner.Execute(ctx, d.Plan, runtime.ExecuteOptions{})
	if err != nil {
		return nil, fmt.Errorf("read models: %w: %s", err, strings.TrimSpace(result.Stderr))
	}
	return modelsFromLines(result.Stdout, d.Plan.Executable), nil
}

func modelsFromLines(output, source string) []runtime.Model {
	seen := make(map[string]bool)
	var models []runtime.Model
	for _, line := range strings.Split(output, "\n") {
		id := strings.TrimSpace(line)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, runtime.Model{ID: id, Source: source})
	}
	return models
}

type CodexCacheModels struct {
	Path string
}

type codexModelCache struct {
	Models []struct {
		Slug        string `json:"slug"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
	} `json:"models"`
}

func (d CodexCacheModels) ListModels(_ context.Context, _ runtime.Runner) ([]runtime.Model, error) {
	path := d.Path
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve user home: %w", err)
		}
		path = filepath.Join(home, ".codex", "models_cache.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Codex model cache %s: %w", path, err)
	}
	var cache codexModelCache
	if err := json.Unmarshal(raw, &cache); err != nil {
		return nil, fmt.Errorf("parse Codex model cache %s: %w", path, err)
	}
	models := make([]runtime.Model, 0, len(cache.Models))
	for _, item := range cache.Models {
		if item.Slug == "" {
			continue
		}
		models = append(models, runtime.Model{
			ID: item.Slug, DisplayName: item.DisplayName, Description: item.Description, Source: path,
		})
	}
	return models, nil
}

type UnsupportedModels struct {
	Agent string
}

func (d UnsupportedModels) ListModels(context.Context, runtime.Runner) ([]runtime.Model, error) {
	return nil, fmt.Errorf("%s supports model selection but exposes no verified local model-list source", d.Agent)
}
