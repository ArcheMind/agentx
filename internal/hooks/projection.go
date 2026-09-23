package hooks

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const ownershipMarker = "agentx hooks projection\n"

type Projection struct {
	Args      []string  `json:"args" yaml:"args"`
	Artifacts []string  `json:"artifacts" yaml:"artifacts"`
	Warnings  []Warning `json:"warnings" yaml:"warnings"`
}

func (s Service) Prepare(provider, executable string, write bool) (Projection, error) {
	if !isPortableTarget(provider) {
		return Projection{Args: []string{}, Artifacts: []string{}, Warnings: []Warning{}}, nil
	}
	result, err := s.List()
	if err != nil {
		return Projection{}, err
	}
	projection := Projection{Args: []string{}, Artifacts: []string{}, Warnings: result.Warnings}
	if len(result.Hooks) == 0 {
		if write {
			if err := removeOwnedDirectory(projectionRoot(s.Home, provider)); err != nil {
				return Projection{}, err
			}
		}
		return projection, nil
	}
	events := configuredEvents(result.Hooks)
	switch provider {
	case "claude":
		root := projectionRoot(s.Home, provider)
		config, err := jsonHookConfig(provider, executable, s.Project, events, result.Hooks)
		if err != nil {
			return Projection{}, err
		}
		projection.Args = []string{"--plugin-dir", root}
		projection.Artifacts = []string{root}
		if err := validateOwnedDirectory(root); err != nil {
			return Projection{}, err
		}
		if write {
			err = writeOwnedDirectory(root, map[string][]byte{
				".claude-plugin/plugin.json": []byte("{\n  \"name\": \"agentx-hooks\"\n}\n"),
				"hooks/hooks.json":           config,
			})
		}
		return projection, err
	case "codex":
		for _, event := range events {
			groups := make([]string, 0)
			for _, item := range result.Hooks {
				if item.Event != event {
					continue
				}
				handler := projectedHandler(item, s.Project)
				command, err := bridgeCommand(executable, provider, item.Event, handler)
				if err != nil {
					return Projection{}, err
				}
				groups = append(groups, fmt.Sprintf("{hooks=[{type=\"command\",command=%s,timeout=%d}]}", tomlString(command), handlerBridgeTimeout(handler)))
			}
			value := fmt.Sprintf("hooks.%s=[%s]", event, strings.Join(groups, ","))
			projection.Args = append(projection.Args, "-c", value)
		}
		return projection, nil
	case "gemini":
		root := projectionRoot(s.Home, provider)
		config, err := jsonHookConfig(provider, executable, s.Project, events, result.Hooks)
		if err != nil {
			return Projection{}, err
		}
		projection.Artifacts = []string{root}
		if err := validateOwnedDirectory(root); err != nil {
			return Projection{}, err
		}
		if write {
			err = writeOwnedDirectory(root, map[string][]byte{
				"gemini-extension.json": []byte("{\n  \"name\": \"agentx-hooks\",\n  \"version\": \"1.0.0\",\n  \"description\": \"AgentX portable hook bridge\"\n}\n"),
				"hooks/hooks.json":      config,
			})
		}
		return projection, err
	case "pi":
		root := projectionRoot(s.Home, provider)
		path := filepath.Join(root, "agentx-hooks.ts")
		projection.Args = []string{"--extension", path}
		projection.Artifacts = []string{root}
		if err := validateOwnedDirectory(root); err != nil {
			return Projection{}, err
		}
		if write {
			err = writeOwnedDirectory(root, map[string][]byte{"agentx-hooks.ts": []byte(piExtension(executable, s.Project, result.Hooks))})
		}
		return projection, err
	default:
		panic("portable target was not handled")
	}
}

func configuredEvents(items []Hook) []string {
	present := map[string]bool{}
	for _, item := range items {
		present[item.Event] = true
	}
	events := make([]string, 0, len(present))
	for _, event := range portableEvents {
		if present[event] {
			events = append(events, event)
		}
	}
	return events
}

func isPortableTarget(provider string) bool {
	for _, target := range portableTargets {
		if provider == target {
			return true
		}
	}
	return false
}

func jsonHookConfig(provider, executable, project string, events []string, hooks []Hook) ([]byte, error) {
	type handler struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	type group struct {
		Hooks []handler `json:"hooks"`
	}
	root := struct {
		Description string             `json:"description"`
		Hooks       map[string][]group `json:"hooks"`
	}{Description: "AgentX portable hook bridge", Hooks: map[string][]group{}}
	for _, event := range events {
		native, err := providerEvent(provider, event)
		if err != nil {
			return nil, err
		}
		groups := make([]group, 0)
		for _, item := range hooks {
			if item.Event != event {
				continue
			}
			projected := projectedHandler(item, project)
			command, err := bridgeCommand(executable, provider, item.Event, projected)
			if err != nil {
				return nil, err
			}
			timeout := handlerBridgeTimeout(projected)
			if provider == "gemini" {
				timeout *= 1000
			}
			groups = append(groups, group{Hooks: []handler{{Type: "command", Command: command, Timeout: timeout}}})
		}
		root.Hooks[native] = groups
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s hook projection: %w", provider, err)
	}
	return append(data, '\n'), nil
}

func handlerBridgeTimeout(handler Handler) int {
	seconds := int(handler.Timeout)
	if handler.Timeout > float64(seconds) {
		seconds++
	}
	return seconds
}

func projectedHandler(item Hook, project string) Handler {
	timeout := item.timeout
	if timeout == 0 {
		timeout = 600
		if item.Event == "SessionEnd" {
			timeout = 3
		}
	}
	handler := Handler{Command: item.Command, Matcher: item.matcher, Timeout: timeout}
	if item.Scope == ScopeProject {
		handler.Project = project
	}
	return handler
}

func bridgeCommand(executable, provider, event string, handler Handler) (string, error) {
	data, err := json.Marshal(handler)
	if err != nil {
		return "", fmt.Errorf("encode %s %s hook handler: %w", provider, event, err)
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	return shellExecutable(executable) + " hook __dispatch --provider " + provider + " --event " + event + " --handler " + payload, nil
}

func shellExecutable(value string) string {
	if runtime.GOOS != "windows" {
		return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func tomlString(value string) string {
	return strconv.QuoteToGraphic(value)
}

func writeOwnedDirectory(root string, files map[string][]byte) error {
	if err := validateOwnedDirectory(root); err != nil {
		return err
	}
	if err := removeOwnedDirectory(root); err != nil {
		return err
	}
	marker := filepath.Join(root, ".agentx-owned")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create hook artifact %s: %w", root, err)
	}
	if err := os.WriteFile(marker, []byte(ownershipMarker), 0o600); err != nil {
		return fmt.Errorf("mark hook artifact %s: %w", root, err)
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create hook artifact directory %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, files[relative], 0o600); err != nil {
			return fmt.Errorf("write hook artifact %s: %w", path, err)
		}
	}
	return nil
}

func projectionRoot(home, provider string) string {
	switch provider {
	case "claude", "pi":
		return filepath.Join(home, ".agents", ".agentx", "hooks", provider)
	case "gemini":
		return filepath.Join(home, ".gemini", "extensions", "agentx-hooks")
	default:
		return ""
	}
}

func removeOwnedDirectory(root string) error {
	if root == "" {
		return nil
	}
	marker := filepath.Join(root, ".agentx-owned")
	data, err := os.ReadFile(marker)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect hook artifact %s: %w", root, err)
	}
	if string(data) != ownershipMarker {
		return nil
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove stale hook artifact %s: %w", root, err)
	}
	return nil
}

func validateOwnedDirectory(root string) error {
	marker := filepath.Join(root, ".agentx-owned")
	if _, err := os.Stat(root); err == nil {
		data, readErr := os.ReadFile(marker)
		if readErr != nil || string(data) != ownershipMarker {
			return fmt.Errorf("refusing to overwrite non-AgentX hook artifact %s", root)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect hook artifact %s: %w", root, err)
	}
	return nil
}

func providerEvent(provider, event string) (string, error) {
	mapping := map[string]map[string]string{
		"claude": {"SessionStart": "SessionStart", "SessionEnd": "SessionEnd", "UserPromptSubmit": "UserPromptSubmit", "Stop": "Stop"},
		"codex":  {"SessionStart": "SessionStart", "SessionEnd": "SessionEnd", "UserPromptSubmit": "UserPromptSubmit", "Stop": "Stop"},
		"gemini": {"SessionStart": "SessionStart", "SessionEnd": "SessionEnd", "UserPromptSubmit": "BeforeAgent", "Stop": "AfterAgent"},
		"pi":     {"SessionStart": "session_start", "SessionEnd": "session_shutdown", "UserPromptSubmit": "input", "Stop": "agent_settled"},
	}
	providerEvents, ok := mapping[provider]
	if !ok {
		return "", fmt.Errorf("unsupported hook provider %q", provider)
	}
	native, ok := providerEvents[event]
	if !ok {
		return "", fmt.Errorf("unsupported portable hook event %q", event)
	}
	return native, nil
}
