package drivers

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

type PiConfiguredProviderSource interface {
	ListConfiguredProviders(context.Context, runtime.Runner) ([]string, error)
}

type LoggedInProviderSource interface {
	ListLoggedInProviders(context.Context) ([]string, error)
}

type PiModels struct {
	Plan      runtime.CommandPlan
	Providers PiConfiguredProviderSource
}

func (d PiModels) ListModels(ctx context.Context, runner runtime.Runner) ([]runtime.Model, error) {
	providerSource := d.Providers
	if providerSource == nil {
		providerSource = PiSDKConfiguredProviders{}
	}
	providers, err := providerSource.ListConfiguredProviders(ctx, runner)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return []runtime.Model{}, nil
	}
	result, err := runner.Execute(ctx, d.Plan, runtime.ExecuteOptions{})
	if err != nil {
		return nil, fmt.Errorf("read Pi models: %w", err)
	}
	return parsePiModels(result.Stdout, d.Plan.Executable, providers)
}

func parsePiModels(output, source string, providers []string) ([]runtime.Model, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("parse Pi models: missing table header")
	}
	wantHeader := []string{"provider", "model", "context", "max-out", "thinking", "images"}
	if fields := strings.Fields(lines[0]); !equalStrings(fields, wantHeader) {
		return nil, fmt.Errorf("parse Pi models: unexpected table header %q", strings.TrimSpace(lines[0]))
	}
	allowed := make(map[string]bool, len(providers))
	for _, provider := range providers {
		allowed[provider] = true
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
		provider, modelID := fields[0], fields[1]
		if !allowed[provider] {
			continue
		}
		id := provider + "/" + modelID
		if seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, runtime.Model{ID: id, Source: source})
	}
	return models, nil
}

func equalStrings(left, right []string) bool {
	return slices.Equal(left, right)
}
