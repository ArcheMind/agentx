package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

var (
	portableEvents  = []string{"SessionStart", "SessionEnd", "UserPromptSubmit", "Stop"}
	portableTargets = []string{"claude", "codex", "gemini", "pi"}
)

type Hook struct {
	Scope    Scope    `json:"scope" yaml:"scope"`
	Source   string   `json:"source" yaml:"source"`
	Event    string   `json:"event" yaml:"event"`
	Command  string   `json:"command" yaml:"command"`
	Portable bool     `json:"portable" yaml:"portable"`
	Targets  []string `json:"targets" yaml:"targets"`
	Warnings []string `json:"warnings" yaml:"warnings"`
}

type Warning struct {
	Source  string `json:"source" yaml:"source"`
	Event   string `json:"event,omitempty" yaml:"event,omitempty"`
	Message string `json:"message" yaml:"message"`
}

type ListResult struct {
	Hooks    []Hook    `json:"hooks" yaml:"hooks"`
	Warnings []Warning `json:"warnings" yaml:"warnings"`
}

type Service struct {
	Home    string
	Project string
}

func New() Service {
	home, _ := os.UserHomeDir()
	project, _ := os.Getwd()
	return Service{Home: home, Project: project}
}

func (s Service) List() (ListResult, error) {
	result := ListResult{Hooks: []Hook{}, Warnings: []Warning{}}
	for _, source := range []struct {
		path  string
		scope Scope
	}{
		{filepath.Join(s.Home, ".agents", "hooks.json"), ScopeUser},
		{filepath.Join(s.Project, ".agents", "hooks.json"), ScopeProject},
	} {
		items, warnings, err := readConfig(source.path, source.scope)
		if err != nil {
			return ListResult{}, err
		}
		result.Hooks = append(result.Hooks, items...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	return result, nil
}

func readConfig(path string, scope Scope) ([]Hook, []Warning, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read hook configuration %s: %w", path, err)
	}
	var root map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&root); err != nil {
		return nil, nil, fmt.Errorf("parse hook configuration %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, nil, fmt.Errorf("parse hook configuration %s: multiple JSON values", path)
	}
	hooksValue, ok := root["hooks"]
	if !ok {
		return nil, nil, fmt.Errorf("invalid hook configuration %s: hooks is required", path)
	}
	var events map[string]json.RawMessage
	if err := json.Unmarshal(hooksValue, &events); err != nil || events == nil {
		return nil, nil, fmt.Errorf("invalid hook configuration %s: hooks must be an object", path)
	}

	warnings := []Warning{}
	for key := range root {
		if key != "description" && key != "hooks" {
			warnings = append(warnings, Warning{Source: path, Message: fmt.Sprintf("top-level field %s is outside the portable hook profile; ignored", key)})
		}
	}
	for event := range events {
		if !isPortableEvent(event) {
			warnings = append(warnings, Warning{Source: path, Event: event, Message: "outside the portable hook profile; not projected"})
		}
	}
	sort.SliceStable(warnings, func(i, j int) bool {
		if warnings[i].Event == warnings[j].Event {
			return warnings[i].Message < warnings[j].Message
		}
		return warnings[i].Event < warnings[j].Event
	})

	items := []Hook{}
	for _, event := range portableEvents {
		value, ok := events[event]
		if !ok {
			continue
		}
		eventItems, eventWarnings, err := readEvent(path, scope, event, value)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, eventItems...)
		warnings = append(warnings, eventWarnings...)
	}
	return items, warnings, nil
}

func readEvent(path string, scope Scope, event string, value json.RawMessage) ([]Hook, []Warning, error) {
	var groups []map[string]json.RawMessage
	if err := json.Unmarshal(value, &groups); err != nil || groups == nil {
		return nil, nil, fmt.Errorf("invalid hook configuration %s: %s must be an array", path, event)
	}
	items := []Hook{}
	warnings := []Warning{}
	for groupIndex, group := range groups {
		matcher, err := optionalString(group, "matcher")
		if err != nil {
			return nil, nil, fmt.Errorf("invalid hook configuration %s: %s[%d].matcher must be a string", path, event, groupIndex)
		}
		if matcher != "" && matcher != "*" {
			if event == "UserPromptSubmit" || event == "Stop" {
				warnings = append(warnings, Warning{Source: path, Event: event, Message: "matcher has no portable subject; ignored"})
			} else if !portableMatcher(matcher) {
				warnings = append(warnings, Warning{Source: path, Event: event, Message: fmt.Sprintf("matcher %q is outside the portable hook profile; group not projected", matcher)})
				continue
			}
		}
		for key := range group {
			if key != "matcher" && key != "hooks" {
				warnings = append(warnings, Warning{Source: path, Event: event, Message: fmt.Sprintf("matcher-group field %s is outside the portable hook profile; ignored", key)})
			}
		}
		handlersValue, ok := group["hooks"]
		if !ok {
			return nil, nil, fmt.Errorf("invalid hook configuration %s: %s[%d].hooks is required", path, event, groupIndex)
		}
		var handlers []map[string]json.RawMessage
		if err := json.Unmarshal(handlersValue, &handlers); err != nil || len(handlers) == 0 {
			return nil, nil, fmt.Errorf("invalid hook configuration %s: %s[%d].hooks must be a non-empty array", path, event, groupIndex)
		}
		for handlerIndex, handler := range handlers {
			item, handlerWarnings, include, err := readHandler(path, scope, event, groupIndex, handlerIndex, handler)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, handlerWarnings...)
			if include {
				for _, warning := range handlerWarnings {
					item.Warnings = append(item.Warnings, warning.Message)
				}
				items = append(items, item)
			}
		}
	}
	return items, warnings, nil
}

func readHandler(path string, scope Scope, event string, groupIndex, handlerIndex int, handler map[string]json.RawMessage) (Hook, []Warning, bool, error) {
	location := fmt.Sprintf("%s[%d].hooks[%d]", event, groupIndex, handlerIndex)
	typeName, err := requiredString(handler, "type")
	if err != nil {
		return Hook{}, nil, false, fmt.Errorf("invalid hook configuration %s: %s.type must be a non-empty string", path, location)
	}
	if typeName != "command" {
		return Hook{}, []Warning{{Source: path, Event: event, Message: fmt.Sprintf("handler type %s is outside the portable hook profile; handler not projected", typeName)}}, false, nil
	}
	if _, ok := handler["async"]; ok {
		return Hook{}, []Warning{{Source: path, Event: event, Message: "handler field async changes execution semantics; handler not projected"}}, false, nil
	}
	command, err := requiredString(handler, "command")
	if err != nil {
		return Hook{}, nil, false, fmt.Errorf("invalid hook configuration %s: %s.command must be a non-empty string", path, location)
	}
	if value, ok := handler["timeout"]; ok {
		var timeout float64
		if err := json.Unmarshal(value, &timeout); err != nil || timeout <= 0 {
			return Hook{}, nil, false, fmt.Errorf("invalid hook configuration %s: %s.timeout must be a positive number", path, location)
		}
	}
	warnings := []Warning{}
	for key := range handler {
		if key != "type" && key != "command" && key != "timeout" {
			warnings = append(warnings, Warning{Source: path, Event: event, Message: fmt.Sprintf("handler field %s is outside the portable hook profile; ignored", key)})
		}
	}
	return Hook{Scope: scope, Source: path, Event: event, Command: command, Portable: true, Targets: append([]string(nil), portableTargets...), Warnings: []string{}}, warnings, true, nil
}

func requiredString(value map[string]json.RawMessage, key string) (string, error) {
	text, err := optionalString(value, key)
	if err != nil || strings.TrimSpace(text) == "" {
		return "", errors.New("required non-empty string")
	}
	return text, nil
}

func optionalString(value map[string]json.RawMessage, key string) (string, error) {
	raw, ok := value[key]
	if !ok {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", err
	}
	return text, nil
}

func isPortableEvent(event string) bool {
	for _, candidate := range portableEvents {
		if event == candidate {
			return true
		}
	}
	return false
}

func portableMatcher(value string) bool {
	value = strings.ReplaceAll(value, ".*", "")
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		switch character {
		case '_', '-', '.', '|', '^', '$':
		default:
			return false
		}
	}
	return !strings.Contains(value, "*")
}
