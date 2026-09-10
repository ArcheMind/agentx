package drivers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArcheMind/agentx/internal/runtime"
	"gopkg.in/yaml.v3"
)

const dshProviderID = "deepseek-official"

// DSHAuthProviders reports only the presence of a native credential.
type DSHAuthProviders struct {
	Path string
	Env  func(string) string
}

func (s DSHAuthProviders) ListLoggedInProviders(context.Context) ([]string, error) {
	getenv := s.Env
	if getenv == nil {
		getenv = os.Getenv
	}
	if getenv("DEEPSEEK_API_KEY") != "" {
		return []string{dshProviderID}, nil
	}
	path, err := s.credentialsPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read DSH credential state %s: %w", path, err)
	}
	var document struct {
		Refs map[string]string `yaml:"refs"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse DSH credential state %s: %w", path, err)
	}
	if document.Refs["DEEPSEEK_API_KEY"] == "" {
		return []string{}, nil
	}
	return []string{dshProviderID}, nil
}

func (s DSHAuthProviders) credentialsPath() (string, error) {
	if s.Path != "" {
		return s.Path, nil
	}
	home := os.Getenv("DSH_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home for DSH credential state: %w", err)
		}
		home = filepath.Join(userHome, ".dsh")
	}
	return filepath.Join(home, ".credentials.yaml"), nil
}

type DSHModels struct{}

func (DSHModels) ListModels(context.Context, runtime.Runner) ([]runtime.Model, error) {
	return []runtime.Model{
		{ID: "deepseek-flash", DisplayName: "DeepSeek-V41-Flash", Source: "dsh bundled catalog"},
		{ID: "deepseek-v4-flash", DisplayName: "DeepSeek-V4-Flash", Description: "Fast, efficient, and economical; suited to focused, routine, or parallel tasks.", Source: "dsh bundled catalog"},
		{ID: "deepseek-v4-pro", DisplayName: "DeepSeek-V4-Pro", Description: "Stronger agentic coding, knowledge, and difficult reasoning; suited to complex or quality-critical tasks at higher cost.", Source: "dsh bundled catalog"},
		{ID: "deepseek-v4-flash-vision-exp", DisplayName: "DeepSeek-V4-Flash-Vision-Exp", Source: "dsh bundled catalog"},
	}, nil
}
