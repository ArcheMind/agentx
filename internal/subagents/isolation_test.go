package subagents

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
)

// gitRunner runs git for real and hands every native agent plan to the fake, optionally writing a
// file into the agent working directory as the native agent would.
type gitRunner struct {
	*testRunner
	write string
	fail  bool
}

func (r *gitRunner) Execute(ctx context.Context, p runtime.CommandPlan, o runtime.ExecuteOptions) (runtime.CommandResult, error) {
	if p.Executable == "git" {
		return runtime.ExecRunner{}.Execute(ctx, p, o)
	}
	if _, ok := authCheck(p); !ok && r.write != "" {
		if err := os.WriteFile(filepath.Join(p.Cwd, r.write), []byte("agent output\n"), 0600); err != nil {
			return runtime.CommandResult{}, err
		}
	}
	if _, ok := authCheck(p); !ok && r.fail {
		return runtime.CommandResult{Stderr: "native failure"}, errors.New("exit status 1")
	}
	return r.testRunner.Execute(ctx, p, o)
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// dirtyRepo returns a committed repository whose checkout has a modified tracked file and an
// untracked file, with the server working directory in a tracked subdirectory.
func dirtyRepo(t *testing.T, s *Server) string {
	t.Helper()
	return dirtyRepoNamed(t, s, "Repo")
}

func dirtyRepoNamed(t *testing.T, s *Server, name string) string {
	t.Helper()
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	repo := filepath.Join(tmp, name)
	os.Mkdir(repo, 0700)
	gitCmd(t, repo, "init", "-q")
	os.MkdirAll(filepath.Join(repo, "sub"), 0700)
	os.WriteFile(filepath.Join(repo, "sub", "tracked.txt"), []byte("committed\n"), 0600)
	gitCmd(t, repo, "add", ".")
	gitCmd(t, repo, "commit", "-qm", "base")
	os.WriteFile(filepath.Join(repo, "sub", "tracked.txt"), []byte("uncommitted edit\n"), 0600)
	os.WriteFile(filepath.Join(repo, "draft.txt"), []byte("untracked draft\n"), 0600)
	s.Cwd = filepath.Join(repo, "sub")
	return repo
}

func TestWorktreeWriteScopeIsOnlyTheWorktree(t *testing.T) {
	r := &gitRunner{testRunner: &testRunner{}}
	s := testServer(t, r)
	repo := dirtyRepo(t, s)
	_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","isolation":"worktree","write":true,"writable_roots":["`+repo+`"]}`))
	if err == nil || !strings.Contains(err.Error(), "the worktree is the only writable root") {
		t.Fatal("writable_roots accepted with isolation", err)
	}
	if strings.Contains(gitCmd(t, repo, "worktree", "list"), "agentx/") || len(s.agents) != 0 {
		t.Fatal("rejected call created a worktree or agent")
	}
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "name": "v", "model": "review", "isolation": "worktree", "write": true, "run_in_background": false})
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "x", "prompt": "x", "name": "ro", "model": "review", "isolation": "worktree", "run_in_background": false})
	s.mu.Lock()
	a, _ := s.target("v")
	s.mu.Unlock()
	r.mu.Lock()
	writeArgs, readArgs := strings.Join(r.plans[0].Args, " "), strings.Join(r.plans[1].Args, " ")
	r.mu.Unlock()
	if !strings.Contains(writeArgs, `writable_roots=["`+a.Worktree.Path+`"] `) || strings.Contains(writeArgs, repo) || !strings.Contains(text, "Write scope: worktree only [\""+a.Worktree.Path+"\"]") {
		t.Fatal("isolated sandbox carries more than the worktree:", writeArgs, text)
	}
	if !strings.Contains(readArgs, `sandbox_mode="read-only"`) || strings.Contains(readArgs, "writable_roots") {
		t.Fatal("isolation without write was writable:", readArgs)
	}
}

func TestWorktreeIsolationRunsInWorktreeAndKeepsOriginalChanges(t *testing.T) {
	r := &gitRunner{testRunner: &testRunner{}, write: "agent.txt"}
	s := testServer(t, r)
	repo := dirtyRepo(t, s)
	before := gitCmd(t, repo, "status", "--porcelain")
	text := callTest(t, s, "ExternalSubagent", map[string]any{"description": "write", "prompt": "edit", "name": "iso", "model": "review", "isolation": "worktree", "write": true, "run_in_background": false})
	s.mu.Lock()
	a, _ := s.target("iso")
	w := a.Worktree
	s.mu.Unlock()
	if w == nil || w.Repo != repo || !strings.HasSuffix(w.Path, filepath.Join(".local", "state", "agentx", "worktrees", a.ID)) {
		t.Fatalf("worktree not recorded: %#v", w)
	}
	r.mu.Lock()
	plan := r.plans[0]
	r.mu.Unlock()
	args := strings.Join(plan.Args, " ")
	if plan.Cwd != filepath.Join(w.Path, "sub") || !strings.Contains(args, `writable_roots=["`+w.Path+`"] `) || strings.Contains(args, `"`+repo+`"`) {
		t.Fatalf("sandbox not bound to worktree: cwd=%s args=%s", plan.Cwd, args)
	}
	if after := gitCmd(t, repo, "status", "--porcelain"); after != before {
		t.Fatalf("original checkout changed:\n%s\n---\n%s", before, after)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "sub", "tracked.txt")); string(data) != "uncommitted edit\n" {
		t.Fatal("original uncommitted edit lost")
	}
	if _, err := os.Stat(filepath.Join(repo, "sub", "agent.txt")); err == nil {
		t.Fatal("agent output landed in original checkout")
	}
	if data, _ := os.ReadFile(filepath.Join(w.Path, "sub", "tracked.txt")); string(data) != "committed\n" {
		t.Fatal("worktree not at HEAD")
	}
	for _, want := range []string{"Isolation: worktree path=", w.Path, "branch=agentx/" + a.ID, "is not modified", "kept after completion, failure and cancellation", "?? sub/agent.txt", "worktree remove", "Write scope: worktree only"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
	if branch := gitCmd(t, w.Path, "branch", "--show-current"); branch != "agentx/"+a.ID {
		t.Fatal(branch)
	}
	r.write = ""
	callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "iso", "message": "again"})
	waitAgent(t, s, "iso")
	r.mu.Lock()
	if r.plans[1].Cwd != plan.Cwd || !strings.Contains(strings.Join(r.plans[1].Args, " "), w.Path) {
		t.Fatal("continuation rebound workspace", r.plans[1])
	}
	r.mu.Unlock()
	gitCmd(t, repo, "worktree", "remove", "--force", w.Path)
	callTest(t, s, "SendExternalSubagentMessage", map[string]any{"to": "iso", "message": "gone"})
	waitAgent(t, s, "iso")
	s.mu.Lock()
	failure := a.Jobs[len(a.Jobs)-1].Failure
	s.mu.Unlock()
	if !strings.Contains(failure, "unavailable; no replacement created") || len(r.plans) != 2 {
		t.Fatal("missing worktree did not fail explicitly:", failure)
	}
}

func TestWorktreeKeptOnFailureAndCancellation(t *testing.T) {
	r := &gitRunner{testRunner: &testRunner{}, write: "partial.txt", fail: true}
	s := testServer(t, r)
	repo := dirtyRepo(t, s)
	_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","isolation":"worktree","run_in_background":false}`))
	if err == nil || !strings.Contains(err.Error(), "kept after completion, failure and cancellation") || !strings.Contains(err.Error(), "?? sub/partial.txt") {
		t.Fatal(err)
	}
	r2 := &gitRunner{testRunner: &testRunner{started: make(chan int, 1), release: make(chan struct{})}}
	s.Runner = r2
	callTest(t, s, "ExternalSubagent", map[string]any{"description": "wait", "prompt": "wait", "name": "c", "model": "review", "isolation": "worktree"})
	<-r2.started
	text := callTest(t, s, "StopExternalSubagent", map[string]any{"task_id": "c"})
	s.mu.Lock()
	c, _ := s.target("c")
	path := c.Worktree.Path
	s.mu.Unlock()
	if !strings.Contains(text, "cancelled") || !strings.Contains(text, path) || !strings.Contains(text, "no uncommitted changes") {
		t.Fatal(text)
	}
	list := gitCmd(t, repo, "worktree", "list")
	if strings.Count(list, "agentx/ax-") != 2 {
		t.Fatal("worktrees not retained:", list)
	}
}

func TestWorktreeIsolationRejectsWithoutGitAndUnknownValues(t *testing.T) {
	r := &gitRunner{testRunner: &testRunner{}}
	s := testServer(t, r)
	if _, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","isolation":"worktree"}`)); err == nil || !strings.Contains(err.Error(), "inside a git work tree") {
		t.Fatal(err)
	}
	for _, value := range []string{"remote", "container"} {
		if _, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","isolation":"`+value+`"}`)); err == nil || err.Error() != `isolation "`+value+`" unsupported; supported: worktree; no task started` {
			t.Fatal(value, err)
		}
	}
	repo := dirtyRepo(t, s)
	untracked := filepath.Join(repo, "untracked-dir")
	os.Mkdir(untracked, 0700)
	s.Cwd = untracked
	if _, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","isolation":"worktree"}`)); err == nil || !strings.Contains(err.Error(), "not tracked at HEAD") {
		t.Fatal(err)
	}
	if list := gitCmd(t, repo, "worktree", "list"); strings.Contains(list, "agentx/") || strings.Contains(gitCmd(t, repo, "branch"), "agentx/") {
		t.Fatal("rejected call left a worktree:", list)
	}
	if len(r.plans) != 0 || len(s.agents) != 0 {
		t.Fatal("rejected isolation started work")
	}
}

func TestPiWorktreeIsolationChangesOnlyWorkingDirectory(t *testing.T) {
	r := &gitRunner{testRunner: &testRunner{}}
	s := testServer(t, r)
	dirtyRepo(t, s)
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	savePreset(t, s.Home, presets.Preset{Name: "pi-iso", Agent: "pi", Model: "provider/model", Description: "pi"})
	s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"pi-iso","isolation":"worktree","name":"p","run_in_background":false}`))
	s.mu.Lock()
	a, _ := s.target("p")
	s.mu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if a == nil || a.Worktree == nil || len(r.plans) != 1 || r.plans[0].Cwd != filepath.Join(a.Worktree.Path, "sub") || !strings.Contains(strings.Join(r.plans[0].Args, " "), "read,grep,find,ls") || a.Worktree.Roots != nil {
		t.Fatal("Pi worktree plan wrong", r.plans)
	}
}

func TestConcurrentWorktreeCreationInOneRepository(t *testing.T) {
	r := &gitRunner{testRunner: &testRunner{}}
	s := testServer(t, r)
	repo := dirtyRepo(t, s)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() {
			_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","isolation":"worktree","run_in_background":false}`))
			errs <- err
		}()
	}
	for i := 0; i < 16; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(gitCmd(t, repo, "worktree", "list"), "agentx/ax-"); n != 16 {
		t.Fatal("worktrees created:", n)
	}
}

type noLoginGitRunner struct{ *gitRunner }

func (r noLoginGitRunner) Execute(ctx context.Context, p runtime.CommandPlan, o runtime.ExecuteOptions) (runtime.CommandResult, error) {
	if len(p.Args) > 1 && p.Args[0] == "login" {
		return runtime.CommandResult{Stderr: "Not logged in", ExitCode: 1}, errors.New("exit status 1")
	}
	return r.gitRunner.Execute(ctx, p, o)
}

func TestWorktreeIsKeptAndNamedWhenAuthPreflightFails(t *testing.T) {
	inner := &gitRunner{testRunner: &testRunner{}}
	s := testServer(t, noLoginGitRunner{inner})
	dirtyRepo(t, s)
	_, err := s.call("ExternalSubagent", []byte(`{"description":"x","prompt":"x","model":"review","isolation":"worktree","name":"noauth","run_in_background":false}`))
	s.mu.Lock()
	a, _ := s.target("noauth")
	s.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "no fallback identity") || a == nil || a.Worktree == nil || !strings.Contains(err.Error(), "Isolation: worktree path=\""+a.Worktree.Path+"\"") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(a.Worktree.Path); statErr != nil || len(inner.plans) != 0 {
		t.Fatal("worktree not kept or native execution started after failed preflight", statErr, len(inner.plans))
	}
}
