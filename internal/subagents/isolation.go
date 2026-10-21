package subagents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/runtime"
)

const (
	IsolationWorktree = "worktree"
	IsolationRemote   = "remote"
)

// RemoteUnsupported is returned for isolation "remote": no installed runtime offers a remote
// environment whose identity, result retrieval and cancellation AgentX can verify.
const RemoteUnsupported = `isolation "remote" unsupported; no task started and nothing runs locally instead. No installed runtime provides a verifiable remote execution environment with cancellation: Codex 0.163 "codex cloud" can submit, poll, diff and apply Codex Cloud tasks but has no cancel command and runs against a Codex Cloud environment bound to a pushed GitHub branch, not this working directory; "codex exec-server --remote" requires a remote host the user provisions; Pi has no remote execution`

// Worktree is the git worktree an ExternalSubagent runs in for its whole lifetime. AgentX never
// removes it, because continuation reuses it and its uncommitted work belongs to the user.
type Worktree struct {
	Repo, Path, Branch, Base, Original string
	Roots                              []string
	Remaining                          string
}

func WorktreeDir(home, id string) string {
	return filepath.Join(home, ".local", "state", "agentx", "worktrees", id)
}

func (s *Server) git(ctx context.Context, dir string, args ...string) (string, error) {
	plan := runtime.CommandPlan{Executable: "git", Args: append([]string{"-C", dir}, args...)}
	result, err := s.Runner.Execute(ctx, plan, runtime.ExecuteOptions{})
	if err != nil {
		return "", fmt.Errorf("git %s: %v; stderr: %s", strings.Join(args, " "), err, strings.TrimSpace(result.Stderr))
	}
	return strings.TrimSpace(result.Stdout), nil
}

func within(base, path string) (string, bool) {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// rebaseRoots maps writable roots onto the worktree so the original checkout never becomes
// writable: roots inside the repository move into the worktree, roots containing the repository
// shrink to the worktree, unrelated roots stay as granted.
func rebaseRoots(roots []string, repo, path string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, root := range roots {
		mapped := root
		if rel, ok := within(repo, root); ok {
			mapped = filepath.Join(path, rel)
		} else if _, ok := within(root, repo); ok {
			mapped = path
		}
		if !seen[mapped] {
			seen[mapped] = true
			out = append(out, mapped)
		}
	}
	return out
}

// createWorktree adds a new worktree at HEAD of the repository containing cwd and returns it with
// the agent working directory inside it. A rejected call leaves no worktree behind.
func (s *Server) createWorktree(ctx context.Context, id, cwd string, g Grant) (*Worktree, string, error) {
	roots, err := g.ResolvedRoots(cwd)
	if err != nil {
		return nil, "", err
	}
	realCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, "", err
	}
	top, err := s.git(ctx, realCwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, "", fmt.Errorf("isolation %q requires the working directory %s to be inside a git work tree; no task started: %w", IsolationWorktree, cwd, err)
	}
	if top, err = filepath.EvalSymlinks(top); err != nil {
		return nil, "", err
	}
	base, err := s.git(ctx, top, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, "", fmt.Errorf("isolation %q requires a commit at HEAD of %s to base the worktree on; no task started: %w", IsolationWorktree, top, err)
	}
	rel, _ := within(top, realCwd)
	parent := filepath.Dir(WorktreeDir(s.Home, id))
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, "", err
	}
	if parent, err = filepath.EvalSymlinks(parent); err != nil {
		return nil, "", err
	}
	path := filepath.Join(parent, id)
	if _, inside := within(top, path); inside {
		return nil, "", fmt.Errorf("worktree location %s is inside repository %s; no task started", path, top)
	}
	w := &Worktree{Repo: top, Path: path, Branch: "agentx/" + id, Base: base, Original: realCwd}
	if _, err := s.git(ctx, top, "worktree", "add", "-b", w.Branch, path, base); err != nil {
		return nil, "", fmt.Errorf("creating worktree %s failed; no task started: %w", path, err)
	}
	reject := func(err error) (*Worktree, string, error) {
		if _, cleanupErr := s.git(context.Background(), top, "worktree", "remove", path); cleanupErr != nil {
			return nil, "", fmt.Errorf("%w; the new worktree %s could not be removed: %v", err, path, cleanupErr)
		}
		if _, cleanupErr := s.git(context.Background(), top, "branch", "-D", w.Branch); cleanupErr != nil {
			return nil, "", fmt.Errorf("%w; branch %s could not be removed: %v", err, w.Branch, cleanupErr)
		}
		return nil, "", err
	}
	agentCwd := filepath.Join(w.Path, rel)
	if info, err := os.Stat(agentCwd); err != nil || !info.IsDir() {
		return reject(fmt.Errorf("working directory %s is not tracked at HEAD %s, so it does not exist in the worktree; no task started", cwd, base))
	}
	if g.Execution.Policy == drivers.PolicyBoundedWrite {
		w.Roots = rebaseRoots(roots, top, w.Path)
		for _, root := range w.Roots {
			if info, err := os.Stat(root); err != nil || !info.IsDir() {
				return reject(fmt.Errorf("writable root rebased to %s does not exist in the worktree (not tracked at HEAD %s); no task started", root, base))
			}
		}
	}
	return w, agentCwd, nil
}

func (s *Server) executionRoots(a *ExternalSubagentState) ([]string, error) {
	if a.Worktree == nil {
		return a.Grant.ResolvedRoots(a.Cwd)
	}
	if info, err := os.Stat(a.Cwd); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("isolation worktree directory %s unavailable; no replacement created and nothing runs in the original directory", a.Cwd)
	}
	return a.Worktree.Roots, nil
}

func (s *Server) worktreeRemaining(w *Worktree) string {
	if _, err := os.Stat(w.Path); err != nil {
		return "worktree directory missing: " + err.Error()
	}
	status, err := s.git(context.Background(), w.Path, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "unknown: " + err.Error()
	}
	head, err := s.git(context.Background(), w.Path, "rev-parse", "HEAD")
	if err != nil {
		return "unknown: " + err.Error()
	}
	text := "HEAD unchanged at base"
	if head != w.Base {
		text = "HEAD moved to " + head
	}
	if status == "" {
		return text + "; no uncommitted changes"
	}
	lines := strings.Split(status, "\n")
	if len(lines) > 50 {
		lines = append(lines[:50], fmt.Sprintf("... %d more", len(lines)-50))
	}
	return text + "; uncommitted changes (git status --porcelain):\n" + strings.Join(lines, "\n")
}

func (w *Worktree) describe(agentCwd string) string {
	text := fmt.Sprintf("Isolation: worktree path=%q branch=%s base=%s repository=%q; agent cwd=%q. The original working directory %q is not modified; its uncommitted changes are not copied into the worktree.\n", w.Path, w.Branch, w.Base, w.Repo, agentCwd, w.Original)
	if len(w.Roots) > 0 {
		text += fmt.Sprintf("Effective writable roots (rebased into the worktree): %q\n", w.Roots)
	}
	remaining := w.Remaining
	if remaining == "" {
		remaining = "not recorded yet"
	}
	return text + fmt.Sprintf("Worktree retention: kept after completion, failure and cancellation; AgentX never removes it. State after last task: %s\nRemove when done: git -C %q worktree remove %q (add --force to discard its changes) && git -C %q branch -D %s\n", remaining, w.Repo, w.Path, w.Repo, w.Branch)
}

func IsolationLimits(agent string) string {
	text := "Isolation: worktree supported: a new git worktree (branch agentx/<id>) of the repository containing the MCP working directory, at HEAD, under ~/.local/state/agentx/worktrees/<id>; the original checkout is not modified and its uncommitted changes are not copied; the worktree is kept after completion, failure and cancellation. "
	if agent == "pi" {
		text += "For Pi this only changes the working directory; reads are not path-restricted, so the original checkout stays readable. "
	} else {
		text += "bounded-write roots inside or containing the repository are rebased into the worktree, so the original checkout is not writable; reads are not path-restricted. "
	}
	return text + "Remote: rejected (no verifiable remote execution environment with cancellation). "
}
