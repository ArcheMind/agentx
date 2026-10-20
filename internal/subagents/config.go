package subagents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

var NamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type Option struct {
	Name        string   `json:"name"`
	Agent       string   `json:"agent"`
	Model       string   `json:"model"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Efforts     []string `json:"efforts,omitempty"`
}

type Config struct {
	Enabled bool     `json:"enabled"`
	Default string   `json:"default,omitempty"`
	Options []Option `json:"options"`
}

func ConfigPath(home string) string {
	return filepath.Join(home, ".config", "agentx", "subagents.json")
}

func StrictDecode(data []byte, value any) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return fmt.Errorf("expected a JSON object")
	}
	for key, raw := range object {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s must not be null", key)
		}
	}
	typeOf := reflect.TypeOf(value).Elem()
	if typeOf.Kind() == reflect.Struct {
		allowed := map[string]bool{}
		for i := 0; i < typeOf.NumField(); i++ {
			key := strings.Split(typeOf.Field(i).Tag.Get("json"), ",")[0]
			allowed[key] = true
		}
		for key := range object {
			if !allowed[key] {
				return fmt.Errorf("unknown field %q", key)
			}
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}

func LoadConfig(home string) (Config, error) {
	var config Config
	data, err := os.ReadFile(ConfigPath(home))
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	if err := StrictDecode(data, &config); err != nil {
		return config, fmt.Errorf("invalid ExternalSubagent configuration: %w", err)
	}
	if err := config.Validate(); err != nil {
		return config, err
	}
	return config, nil
}

func (c Config) Validate() error {
	seen := map[string]bool{}
	for _, o := range c.Options {
		if !NamePattern.MatchString(o.Name) || seen[o.Name] {
			return fmt.Errorf("invalid or duplicate option name %q", o.Name)
		}
		seen[o.Name] = true
		if err := o.Validate(); err != nil {
			return fmt.Errorf("option %s: %w", o.Name, err)
		}
	}
	if c.Default != "" && !seen[c.Default] {
		return fmt.Errorf("default %q is not configured", c.Default)
	}
	return nil
}

func (o Option) Validate() error {
	if o.Agent != "codex" && o.Agent != "pi" {
		return fmt.Errorf("unsupported execution agent %q; supported: codex, pi", o.Agent)
	}
	if o.Model == "" || o.Description == "" {
		return fmt.Errorf("exact model and capability description are required")
	}
	if o.Agent == "pi" && !regexp.MustCompile(`^[A-Za-z0-9_-]+/[^*?:[:space:]]+$`).MatchString(o.Model) {
		return fmt.Errorf("Pi requires an exact provider/model ID without a thinking suffix or wildcard")
	}
	for _, effort := range o.Efforts {
		if o.Agent == "pi" {
			return fmt.Errorf("Pi effort selection is not supported: native runtime may clamp levels; omit efforts")
		}
		if !Contains([]string{"low", "medium", "high", "xhigh", "max"}, effort) {
			return fmt.Errorf("invalid effort %q", effort)
		}
		if o.Agent == "codex" && effort == "max" {
			return fmt.Errorf("Codex does not support max effort")
		}
	}
	return nil
}

func Contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func (o Option) Ready() error {
	if !o.Enabled {
		return fmt.Errorf("option is disabled")
	}
	if err := o.Validate(); err != nil {
		return err
	}
	if _, err := exec.LookPath(o.Agent); err != nil {
		return fmt.Errorf("%s executable unavailable: %w", o.Agent, err)
	}
	return nil
}

func SaveConfig(home string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := ConfigPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".subagents-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func Limits(o Option) string {
	if o.Agent == "pi" {
		return "Read-only tool allowlist (read, grep, find, ls); extensions, skills, prompt templates and project approval disabled. Not an OS sandbox. Native credentials; does not inherit Claude permissions. Role: general-purpose only. Isolation and effort selection: unsupported (Pi may silently clamp thinking levels). Exact provider/model required. Remote availability is unverified until execution."
	}
	return "Read-only native sandbox; approval_policy=never. Native credentials; does not inherit Claude permissions. Role: general-purpose only. Isolation: unsupported. Advertised efforts are user-verified model capabilities. Remote availability is unverified until execution."
}
