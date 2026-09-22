package skills

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
	"gopkg.in/yaml.v3"
)

type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Skill struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Scope       Scope    `json:"scope" yaml:"scope"`
	Source      string   `json:"source" yaml:"source"`
	Origin      string   `json:"origin" yaml:"origin"`
	Path        string   `json:"path" yaml:"path"`
	Enabled     bool     `json:"enabled" yaml:"enabled"`
	Agents      []string `json:"agents" yaml:"agents"`
}

type AgentSupport struct {
	Agent     string `json:"agent" yaml:"agent"`
	Supported bool   `json:"supported" yaml:"supported"`
}

type ListResult struct {
	Skills []Skill        `json:"skills" yaml:"skills"`
	Agents []AgentSupport `json:"agents" yaml:"agents"`
}

type InstallOptions struct {
	Source string
	Path   string
	Scope  Scope
	DryRun bool
}

type InstallPlan struct {
	Source       string   `json:"source" yaml:"source"`
	SourceType   string   `json:"source_type" yaml:"source_type"`
	Subpath      string   `json:"subpath,omitempty" yaml:"subpath,omitempty"`
	Scope        Scope    `json:"scope" yaml:"scope"`
	Name         string   `json:"name,omitempty" yaml:"name,omitempty"`
	Destination  string   `json:"destination,omitempty" yaml:"destination,omitempty"`
	Projection   string   `json:"claude_projection,omitempty" yaml:"claude_projection,omitempty"`
	Agents       []string `json:"agents" yaml:"agents"`
	WillValidate bool     `json:"will_validate" yaml:"will_validate"`
}

type Service struct {
	Home       string
	Project    string
	Runner     runtime.Runner
	TempParent string
}

type originRecord struct {
	Source string `yaml:"source"`
	Origin string `yaml:"origin"`
}

func New(runner runtime.Runner) Service {
	home, _ := os.UserHomeDir()
	project, _ := os.Getwd()
	return Service{Home: home, Project: project, Runner: runner}
}

func (s Service) List() (ListResult, error) {
	type root struct {
		path      string
		scope     Scope
		canonical bool
	}
	roots := []root{
		{filepath.Join(s.Home, ".agents", "skills"), ScopeUser, true},
		{filepath.Join(s.Project, ".agents", "skills"), ScopeProject, true},
		{filepath.Join(s.Home, ".claude", "skills"), ScopeUser, false},
		{filepath.Join(s.Project, ".claude", "skills"), ScopeProject, false},
	}
	byPhysicalPath := map[string]*Skill{}
	for _, candidate := range roots {
		origins, err := readOrigins(candidate.path)
		if err != nil {
			return ListResult{}, err
		}
		entries, err := os.ReadDir(candidate.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ListResult{}, fmt.Errorf("read skill directory %s: %w", candidate.path, err)
		}
		for _, entry := range entries {
			path := filepath.Join(candidate.path, entry.Name())
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() {
				continue
			}
			metadata, err := readMetadata(path)
			if err != nil {
				continue
			}
			physical, err := filepath.EvalSymlinks(path)
			if err != nil {
				physical = path
			}
			physical, _ = filepath.Abs(physical)
			item := byPhysicalPath[physical]
			if item == nil {
				agents := []string{"claude"}
				if candidate.canonical {
					agents = []string{"codex", "gemini", "pi"}
				}
				source, origin := "filesystem", physical
				if record, ok := origins[metadata.Name]; ok && candidate.canonical {
					source, origin = record.Source, record.Origin
				}
				item = &Skill{Name: metadata.Name, Description: metadata.Description, Scope: candidate.scope, Source: source, Origin: origin, Path: path, Enabled: true, Agents: agents}
				byPhysicalPath[physical] = item
			} else if candidate.canonical {
				item.Path = path
				item.Scope = candidate.scope
				if record, ok := origins[metadata.Name]; ok {
					item.Source, item.Origin = record.Source, record.Origin
				}
				item.Agents = appendUnique(item.Agents, "codex", "gemini", "pi")
			} else {
				item.Agents = appendUnique(item.Agents, "claude")
			}
		}
	}
	items := make([]Skill, 0, len(byPhysicalPath))
	for _, item := range byPhysicalPath {
		sort.Strings(item.Agents)
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].Path < items[j].Path
		}
		return items[i].Name < items[j].Name
	})
	return ListResult{Skills: items, Agents: supportMatrix()}, nil
}

func (s Service) Show(name string) (Skill, error) {
	result, err := s.List()
	if err != nil {
		return Skill{}, err
	}
	var matches []Skill
	for _, item := range result.Skills {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return Skill{}, fmt.Errorf("skill %q is not installed", name)
	}
	if len(matches) > 1 {
		return Skill{}, fmt.Errorf("skill %q is ambiguous across scopes", name)
	}
	return matches[0], nil
}

func (s Service) Install(ctx context.Context, options InstallOptions) (InstallPlan, error) {
	if options.Scope == "" {
		options.Scope = ScopeProject
	}
	if options.Scope != ScopeUser && options.Scope != ScopeProject {
		return InstallPlan{}, fmt.Errorf("invalid skill scope %q; expected user or project", options.Scope)
	}
	if options.Source == "" {
		return InstallPlan{}, errors.New("skill source is required")
	}
	plan := InstallPlan{Source: options.Source, Subpath: options.Path, Scope: options.Scope, Agents: []string{"claude", "codex", "gemini", "pi"}, WillValidate: true}
	root, sourceType, cleanup, err := s.resolveSource(ctx, options.Source, options.Path)
	if err != nil {
		return plan, err
	}
	defer cleanup()
	plan.SourceType = sourceType
	metadata, err := validate(root)
	if err != nil {
		return plan, err
	}
	plan.Name = metadata.Name
	canonicalRoot, claudeRoot := s.roots(options.Scope)
	plan.Destination = filepath.Join(canonicalRoot, metadata.Name)
	plan.Projection = filepath.Join(claudeRoot, metadata.Name)
	if err := ensureAvailable(plan.Destination, plan.Projection); err != nil {
		return plan, err
	}
	if options.DryRun {
		return plan, nil
	}
	if err := os.MkdirAll(canonicalRoot, 0o755); err != nil {
		return plan, fmt.Errorf("create canonical skill directory: %w", err)
	}
	staging, err := os.MkdirTemp(canonicalRoot, ".agentx-install-")
	if err != nil {
		return plan, fmt.Errorf("create skill staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := copyDirectory(root, staging); err != nil {
		return plan, err
	}
	if err := os.Rename(staging, plan.Destination); err != nil {
		return plan, fmt.Errorf("install skill %s: %w", metadata.Name, err)
	}
	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		_ = os.RemoveAll(plan.Destination)
		return plan, fmt.Errorf("create Claude skill directory: %w", err)
	}
	relative, err := filepath.Rel(claudeRoot, plan.Destination)
	if err != nil {
		_ = os.RemoveAll(plan.Destination)
		return plan, fmt.Errorf("resolve Claude projection: %w", err)
	}
	if err := os.Symlink(relative, plan.Projection); err != nil {
		_ = os.RemoveAll(plan.Destination)
		return plan, fmt.Errorf("create Claude skill projection: %w", err)
	}
	origin := options.Source
	if sourceType == "local" {
		origin, _ = filepath.Abs(root)
	} else if options.Path != "" {
		origin += "#" + filepath.ToSlash(options.Path)
	}
	if err := writeOrigin(canonicalRoot, metadata.Name, originRecord{Source: sourceType, Origin: origin}); err != nil {
		_ = os.Remove(plan.Projection)
		_ = os.RemoveAll(plan.Destination)
		return plan, err
	}
	return plan, nil
}

func (s Service) roots(scope Scope) (string, string) {
	base := s.Project
	if scope == ScopeUser {
		base = s.Home
	}
	return filepath.Join(base, ".agents", "skills"), filepath.Join(base, ".claude", "skills")
}

func (s Service) resolveSource(ctx context.Context, source, subpath string) (string, string, func(), error) {
	localSource := source
	if !filepath.IsAbs(localSource) {
		localSource = filepath.Join(s.Project, localSource)
	}
	if info, err := os.Stat(localSource); err == nil {
		if !info.IsDir() {
			return "", "local", func() {}, fmt.Errorf("skill source %s is not a directory", localSource)
		}
		root := filepath.Join(localSource, filepath.Clean(subpath))
		if subpath == "" {
			root = localSource
		}
		if !isWithin(localSource, root) {
			return "", "local", func() {}, fmt.Errorf("skill subpath %q escapes its source", subpath)
		}
		return root, "local", func() {}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "local", func() {}, fmt.Errorf("read skill source %s: %w", localSource, err)
	}
	if s.Runner == nil {
		return "", "git", func() {}, errors.New("git skill installation is unavailable")
	}
	temporary, err := os.MkdirTemp(s.TempParent, "agentx-skill-")
	if err != nil {
		return "", "git", func() {}, fmt.Errorf("create git skill staging directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	result, err := s.Runner.Execute(ctx, runtime.CommandPlan{Executable: "git", Args: []string{"clone", "--depth", "1", "--", source, temporary}}, runtime.ExecuteOptions{})
	if err != nil {
		cleanup()
		return "", "git", func() {}, fmt.Errorf("clone skill source: %w: %s", err, strings.TrimSpace(result.Stderr))
	}
	root := temporary
	if subpath != "" {
		root = filepath.Join(temporary, filepath.Clean(subpath))
	}
	if !isWithin(temporary, root) {
		cleanup()
		return "", "git", func() {}, fmt.Errorf("skill subpath %q escapes its repository", subpath)
	}
	return root, "git", cleanup, nil
}

type metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func readMetadata(root string) (metadata, error) {
	file, err := os.Open(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return metadata{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || scanner.Text() != "---" {
		return metadata{}, errors.New("SKILL.md must begin with YAML frontmatter")
	}
	var frontmatter strings.Builder
	foundEnd := false
	for scanner.Scan() {
		if scanner.Text() == "---" {
			foundEnd = true
			break
		}
		frontmatter.WriteString(scanner.Text())
		frontmatter.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return metadata{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	if !foundEnd {
		return metadata{}, errors.New("SKILL.md frontmatter is not closed")
	}
	var value metadata
	if err := yaml.Unmarshal([]byte(frontmatter.String()), &value); err != nil {
		return metadata{}, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	return value, nil
}

func validate(root string) (metadata, error) {
	info, err := os.Stat(root)
	if err != nil {
		return metadata{}, fmt.Errorf("read skill source: %w", err)
	}
	if !info.IsDir() {
		return metadata{}, errors.New("skill source is not a directory")
	}
	value, err := readMetadata(root)
	if err != nil {
		return metadata{}, fmt.Errorf("invalid skill: %w", err)
	}
	if len(value.Name) > 64 || !skillNamePattern.MatchString(value.Name) {
		return metadata{}, fmt.Errorf("invalid skill name %q; use lowercase letters, numbers, and single hyphens", value.Name)
	}
	if strings.TrimSpace(value.Description) == "" {
		return metadata{}, errors.New("invalid skill: description is required")
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill resource %s is a symbolic link", path)
		}
		return nil
	})
	if err != nil {
		return metadata{}, fmt.Errorf("validate skill resources: %w", err)
	}
	return value, nil
}

func ensureAvailable(paths ...string) error {
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("skill destination already exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect skill destination %s: %w", path, err)
		}
	}
	return nil
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return closeErr
	})
}

func isWithin(parent, child string) bool {
	parentAbsolute, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	childAbsolute, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(parentAbsolute, childAbsolute)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func appendUnique(values []string, candidates ...string) []string {
	for _, candidate := range candidates {
		found := false
		for _, value := range values {
			if value == candidate {
				found = true
				break
			}
		}
		if !found {
			values = append(values, candidate)
		}
	}
	return values
}

func supportMatrix() []AgentSupport {
	return []AgentSupport{
		{Agent: "claude", Supported: true},
		{Agent: "codex", Supported: true},
		{Agent: "dsh", Supported: false},
		{Agent: "gemini", Supported: true},
		{Agent: "pi", Supported: true},
	}
}

func readOrigins(root string) (map[string]originRecord, error) {
	content, err := os.ReadFile(filepath.Join(root, ".agentx-origins.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]originRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skill origins in %s: %w", root, err)
	}
	values := map[string]originRecord{}
	if err := yaml.Unmarshal(content, &values); err != nil {
		return nil, fmt.Errorf("parse skill origins in %s: %w", root, err)
	}
	return values, nil
}

func writeOrigin(root, name string, record originRecord) error {
	values, err := readOrigins(root)
	if err != nil {
		return err
	}
	values[name] = record
	content, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode skill origin: %w", err)
	}
	temporary, err := os.CreateTemp(root, ".agentx-origins-")
	if err != nil {
		return fmt.Errorf("create skill origin staging file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set skill origin permissions: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write skill origin: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close skill origin: %w", err)
	}
	destination := filepath.Join(root, ".agentx-origins.yaml")
	if goruntime.GOOS == "windows" {
		if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("replace skill origin: %w", err)
		}
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return fmt.Errorf("save skill origin: %w", err)
	}
	return nil
}
