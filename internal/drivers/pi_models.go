package drivers

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

type LoggedInProviderSource interface {
	ListLoggedInProviders(context.Context) ([]string, error)
}

// PiModels reads `pi --list-models`, which Pi already restricts to providers with configured auth.
type PiModels struct {
	Plan runtime.CommandPlan
}

func (d PiModels) ListModels(ctx context.Context, runner runtime.Runner) ([]runtime.Model, error) {
	result, err := runner.Execute(ctx, d.Plan, runtime.ExecuteOptions{})
	if err != nil {
		return nil, fmt.Errorf("read Pi models: %w", err)
	}
	return parsePiModels(result.Stdout, d.Plan.Executable)
}

func (d PiModels) ListProviders(ctx context.Context, runner runtime.Runner) ([]string, error) {
	models, err := d.ListModels(ctx, runner)
	if err != nil {
		return nil, err
	}
	providers := make([]string, 0)
	for _, model := range models {
		provider, _, _ := strings.Cut(model.ID, "/")
		if !slices.Contains(providers, provider) {
			providers = append(providers, provider)
		}
	}
	slices.Sort(providers)
	return providers, nil
}

func parsePiModels(output, source string) ([]runtime.Model, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("parse Pi models: missing table header")
	}
	wantHeader := []string{"provider", "model", "context", "max-out", "thinking", "images"}
	if fields := strings.Fields(lines[0]); !slices.Equal(fields, wantHeader) {
		return nil, fmt.Errorf("parse Pi models: unexpected table header %q", strings.TrimSpace(lines[0]))
	}
	seen := make(map[string]bool)
	models := make([]runtime.Model, 0)
	for index, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != len(wantHeader) {
			return nil, fmt.Errorf("parse Pi models: row %d has %d columns, want %d", index+2, len(fields), len(wantHeader))
		}
		id := fields[0] + "/" + fields[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, runtime.Model{ID: id, Source: source})
	}
	return models, nil
}
