package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"agentx/internal/runtime"
)

const piAgentDirEnv = "PI_CODING_AGENT_DIR"

type LoggedInProviderSource interface {
	ListLoggedInProviders(context.Context) ([]string, error)
}

type PiAuthFileProviders struct {
	Path string
}

type piCredential struct {
	Type string `json:"type"`
}

func (s PiAuthFileProviders) ListLoggedInProviders(_ context.Context) ([]string, error) {
	path, err := s.authPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Pi auth state %s: %w", path, err)
	}
	providers := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &providers); err != nil {
		return nil, fmt.Errorf("parse Pi auth state %s: %w", path, err)
	}
	if providers == nil {
		return nil, fmt.Errorf("parse Pi auth state %s: expected an object", path)
	}
	loggedIn := make([]string, 0, len(providers))
	for provider, rawCredential := range providers {
		var credential piCredential
		if err := json.Unmarshal(rawCredential, &credential); err != nil {
			return nil, fmt.Errorf("parse Pi auth state %s for provider %q: %w", path, provider, err)
		}
		if provider == "" {
			return nil, fmt.Errorf("parse Pi auth state %s: provider ID is empty", path)
		}
		if credential.Type != "oauth" && credential.Type != "api_key" {
			return nil, fmt.Errorf("parse Pi auth state %s for provider %q: unsupported credential type %q", path, provider, credential.Type)
		}
		loggedIn = append(loggedIn, provider)
	}
	return loggedIn, nil
}

func (s PiAuthFileProviders) authPath() (string, error) {
	if s.Path != "" {
		return s.Path, nil
	}
	agentDir := os.Getenv(piAgentDirEnv)
	if agentDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home for Pi auth state: %w", err)
		}
		agentDir = filepath.Join(home, ".pi", "agent")
	} else if agentDir == "~" || strings.HasPrefix(agentDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand %s for Pi auth state: %w", piAgentDirEnv, err)
		}
		agentDir = filepath.Join(home, strings.TrimPrefix(agentDir, "~/"))
	}
	return filepath.Join(agentDir, "auth.json"), nil
}

type PiModels struct {
	Plan      runtime.CommandPlan
	Providers LoggedInProviderSource
}

func (d PiModels) ListModels(ctx context.Context, runner runtime.Runner) ([]runtime.Model, error) {
	providerSource := d.Providers
	if providerSource == nil {
		providerSource = PiAuthFileProviders{}
	}
	providers, err := providerSource.ListLoggedInProviders(ctx)
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
