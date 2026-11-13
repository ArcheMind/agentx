package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
)

const DefaultLinger = 60 * time.Second

// Daemon owns one ExternalSubagent: it holds the live native session, serializes rounds,
// delivers steered and queued messages, and appends the agent's event stream.
type Daemon struct {
	Home   string
	Dir    string
	Runner runtime.Runner
	Log    io.Writer
	Linger time.Duration

	mu          sync.Mutex
	meta        AgentMeta
	agent       *ExternalSubagentState
	claude      *claudeAdapter
	roundCancel context.CancelFunc
	roundDone   chan struct{}
	running     bool
	stopping    bool
	queue       []string
	lastStatus  string
	linger      *time.Timer
	quit        chan struct{}
	quitOnce    sync.Once
}

// RunDaemon serves one agent until stop, linger expiry or owner death. It is run by the hidden
// `sm mcp __daemon` command and in-process by tests.
func RunDaemon(d *Daemon) error {
	meta, err := ReadMeta(d.Dir)
	if err != nil {
		return err
	}
	if d.Linger == 0 {
		d.Linger = DefaultLinger
	}
	d.meta = meta
	d.agent = &ExternalSubagentState{ID: meta.ID, Name: meta.Name, Option: meta.Preset, Mode: meta.Mode, Cwd: meta.Cwd, Session: meta.Session, NativeID: meta.NativeID, Worktree: meta.Worktree}
	d.quit = make(chan struct{})
	if err := os.WriteFile(filepath.Join(d.Dir, "daemon.pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		return err
	}
	_ = os.Remove(meta.Socket)
	listener, err := net.Listen("unix", meta.Socket)
	if err != nil {
		return fmt.Errorf("daemon control socket %s: %w", meta.Socket, err)
	}
	d.mu.Lock()
	if report := LastReport(d.Dir); report != nil {
		d.lastStatus = report.Status
	}
	d.resetLingerLocked()
	d.mu.Unlock()
	go d.watchOwner()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go d.serveConn(conn)
		}
	}()
	<-d.quit
	listener.Close()
	d.mu.Lock()
	claude := d.claude
	cancel := d.roundCancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
		<-d.roundDone
	}
	if claude != nil {
		_ = claude.stream.CloseStdin()
		select {
		case <-claude.done:
		case <-time.After(5 * time.Second):
			claude.stream.Kill()
			<-claude.done
		}
	}
	os.Remove(meta.Socket)
	os.Remove(filepath.Join(d.Dir, "daemon.pid"))
	return nil
}

func (d *Daemon) shutdown() { d.quitOnce.Do(func() { close(d.quit) }) }

func (d *Daemon) debugf(format string, args ...any) {
	if d.Log != nil {
		fmt.Fprintf(d.Log, "debug daemon "+format+"\n", args...)
	}
}

func pidAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	process.Release()
	return err == nil
}

// watchOwner is the orphan backstop: when the owning MCP server dies without cleanup, the
// daemon kills its native process and exits.
func (d *Daemon) watchOwner() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.quit:
			return
		case <-ticker.C:
			if pidAlive(d.meta.OwnerPID) {
				continue
			}
			d.debugf("owner pid %d gone; exiting", d.meta.OwnerPID)
			d.mu.Lock()
			d.stopping = true
			if d.claude != nil {
				d.claude.stream.Kill()
			}
			if d.roundCancel != nil {
				d.roundCancel()
			}
			d.mu.Unlock()
			d.shutdown()
			return
		}
	}
}

func (d *Daemon) serveConn(conn net.Conn) {
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	var request controlRequest
	if err := decoder.Decode(&request); err != nil {
		return
	}
	d.debugf("control request op=%s delivery=%s raw message=%s", request.Op, request.Delivery, request.Message)
	var response controlResponse
	exitAfter := false
	switch request.Op {
	case "send":
		response = d.send(request.Message, request.Delivery)
	case "stop":
		response = d.stop()
		exitAfter = true
	case "status":
		response = d.status()
	case "message":
		response = d.hostMessage(request.Message)
	default:
		response = controlResponse{Error: fmt.Sprintf("unknown daemon op %q", request.Op)}
	}
	response.OK = response.Error == ""
	data, _ := json.Marshal(response)
	d.debugf("control response raw=%s", string(data))
	_, _ = conn.Write(append(data, '\n'))
	if exitAfter {
		d.shutdown()
	}
}

func (d *Daemon) status() controlResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := "idle"
	switch {
	case d.running:
		state = "running"
	case d.lastStatus == "failed" || d.lastStatus == "cancelled":
		// A lingering daemon after a failed or cancelled settle still gates resume.
		state = d.lastStatus
	}
	return controlResponse{State: state, Seq: LastEventSeq(d.Dir)}
}

func (d *Daemon) hostMessage(text string) controlResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	seq, err := AppendEvent(d.Dir, Event{Kind: "message", Message: text})
	if err != nil {
		return controlResponse{Error: err.Error()}
	}
	return controlResponse{Seq: seq}
}

func (d *Daemon) send(message, delivery string) controlResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopping {
		return controlResponse{Error: "daemon is stopping; no message accepted"}
	}
	defer d.resetLingerLocked()
	if d.agent.Option.Agent == "claude" {
		return d.sendClaudeLocked(message, delivery)
	}
	if d.running {
		// One-shot adapters cannot inject mid-run; steer degrades to queued until Wave 2.
		d.queue = append(d.queue, message)
		return controlResponse{Disposition: "queued", Seq: LastEventSeq(d.Dir)}
	}
	d.startOneShotLocked(message)
	return controlResponse{Disposition: "started", Seq: LastEventSeq(d.Dir)}
}

func (d *Daemon) stop() controlResponse {
	d.mu.Lock()
	d.stopping = true
	d.queue = nil
	hadWork := d.running
	var wait chan struct{}
	if d.claude != nil {
		d.claude.stream.Kill()
		wait = d.claude.done
	}
	if d.roundCancel != nil {
		d.roundCancel()
		wait = d.roundDone
	}
	d.mu.Unlock()
	if wait != nil {
		<-wait
	}
	d.mu.Lock()
	status := d.lastStatus
	d.mu.Unlock()
	if status == "" {
		status = "idle"
	}
	return controlResponse{HadWork: hadWork, Cancelled: hadWork && status == "cancelled", Status: status}
}

func (d *Daemon) resetLingerLocked() {
	if d.linger != nil {
		d.linger.Stop()
	}
	d.linger = time.AfterFunc(d.Linger, d.lingerExpired)
}

func (d *Daemon) lingerExpired() {
	d.mu.Lock()
	if d.running || len(d.queue) > 0 || d.stopping {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	d.debugf("linger expired; exiting")
	d.shutdown()
}

// appendReportLocked writes a report event and records the terminal status of the last run.
func (d *Daemon) appendReportLocked(event Event) {
	event.Kind = "report"
	d.lastStatus = event.Status
	if _, err := AppendEvent(d.Dir, event); err != nil {
		d.debugf("append report failed: %v", err)
	}
	d.persistMetaLocked()
}

func (d *Daemon) persistMetaLocked() {
	d.meta.Session = d.agent.Session
	d.meta.NativeID = d.agent.NativeID
	if err := WriteMeta(d.Dir, d.meta); err != nil {
		d.debugf("persist meta failed: %v", err)
	}
}

// deliverQueue starts the next round from queued messages after a settle.
func (d *Daemon) deliverQueue() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopping || d.running || len(d.queue) == 0 {
		d.resetLingerLocked()
		return
	}
	if d.agent.Option.Agent == "claude" {
		messages := d.queue
		d.queue = nil
		if response := d.sendClaudeLocked(messages[0], "steer"); response.Error != "" {
			d.appendReportLocked(Event{Status: "failed", Failure: response.Error})
			return
		}
		for _, message := range messages[1:] {
			d.sendClaudeLocked(message, "steer")
		}
		return
	}
	message := d.queue[0]
	d.queue = d.queue[1:]
	d.startOneShotLocked(message)
}

func (d *Daemon) startOneShotLocked(prompt string) {
	ctx, cancel := context.WithCancel(context.Background())
	d.roundCancel = cancel
	done := make(chan struct{})
	d.roundDone = done
	d.running = true
	a := d.agent
	go func() {
		report, usage, err := d.executeRound(ctx, a, prompt)
		remaining := ""
		if a.Worktree != nil {
			remaining = worktreeRemaining(d.Runner, a.Worktree)
		}
		event := Event{Status: "completed", Report: report, Evidence: a.evidence, Worktree: remaining}
		if a.auth.Key != "" {
			event.Auth = a.auth.String()
		}
		if usage != nil {
			event.Usage, _ = json.Marshal(usage)
		}
		if err != nil {
			if ctx.Err() != nil {
				event.Status, event.Failure, event.Report = "cancelled", "Native execution terminated after cancellation", ""
			} else {
				event.Status, event.Failure, event.Report = "failed", err.Error(), ""
			}
		}
		cancel()
		d.mu.Lock()
		d.roundCancel = nil
		d.running = false
		d.appendReportLocked(event)
		d.mu.Unlock()
		close(done)
		d.deliverQueue()
	}()
}

// executeRound runs one one-shot native round (codex, pi). The claude adapter streams instead.
func (d *Daemon) executeRound(ctx context.Context, a *ExternalSubagentState, prompt string) (string, any, error) {
	if current, err := presets.Read(d.Home, a.Option.Name); err != nil || !current.Equal(a.Option) {
		return "", nil, fmt.Errorf("original preset %s changed or was removed before execution; no replacement selected", a.Option.Name)
	}
	a.evidence, a.auth = "", Auth{}
	if a.Session != "" && a.Option.Agent == "pi" {
		if _, err := os.Stat(a.Session); err != nil {
			return "", nil, fmt.Errorf("original Pi session unavailable: %w; no replacement created", err)
		}
	}
	if err := checkWorktree(a); err != nil {
		return "", nil, err
	}
	auth, err := DiscoverAuth(a.Option.Agent)
	if err != nil {
		return "", nil, err
	}
	a.auth = auth
	var roleInstructions string
	if a.Option.Description != "" {
		roleInstructions = "Role: " + a.Option.Name + "\n" + a.Option.Description
	}
	var plan runtime.CommandPlan
	switch a.Option.Agent {
	case "codex":
		plan, err = drivers.CodexPolicyExecPlan(a.Cwd, a.Option.Model, a.Session, a.Mode == ModeYolo, a.Option.Effort, a.Option.UserConfig, a.Option.Flags)
		if err != nil {
			return "", nil, err
		}
		if roleInstructions != "" {
			plan.Args = append(plan.Args[:len(plan.Args)-1], "-c", fmt.Sprintf("developer_instructions=%q", roleInstructions), "-")
		}
		d.injectCodexDelegation(&plan)
	default:
		plan, err = drivers.ExecPlan("pi", a.Cwd, a.Option.Model, "")
		if err != nil {
			return "", nil, err
		}
		if a.Session == "" {
			dir := filepath.Join(d.Home, ".local", "state", "sessionman", "subagents", a.ID)
			if err := os.MkdirAll(dir, 0700); err != nil {
				return "", nil, err
			}
			a.Session = filepath.Join(dir, "session.jsonl")
		}
		plan.Args = append(plan.Args, "--session", a.Session, "--no-approve")
		if a.Option.UserConfig != nil && !*a.Option.UserConfig {
			plan.Args = append(plan.Args, "--no-skills", "--no-prompt-templates")
		}
		if roleInstructions != "" {
			plan.Args = append(plan.Args, "--append-system-prompt", roleInstructions)
		}
		plan.Args = append(plan.Args, a.Option.Flags...)
	}
	plan.Env, plan.UnsetEnv = AuthEnvironment(auth)
	if a.Option.Agent == "pi" {
		if err := d.injectPiDelegation(&plan); err != nil {
			return "", nil, err
		}
	}
	observed := a.Option.Agent == "pi" && a.Option.Effort != ""
	var piOffset int64
	if observed {
		args, env, err := preparePiObserver(a)
		if err != nil {
			return "", nil, err
		}
		plan.Args = append(plan.Args, args...)
		for key, value := range env {
			plan.Env[key] = value
		}
		if info, err := os.Stat(a.Session); err == nil {
			piOffset = info.Size()
		}
	}
	if a.Option.Agent == "codex" {
		plan.Env["RUST_LOG"] = codexTelemetryFilter
	}
	result, err := d.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Stdin: strings.NewReader(prompt), CancelProcessGroup: true})
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, err
		}
		return "", nil, fmt.Errorf("%s execution failed: %s; stderr: %s%s", a.Option.Agent, runtime.RedactSecrets(err.Error()), runtime.RedactSecrets(withoutCodexTelemetry(result.Stderr)), nativeFailure(a, result.Stdout))
	}
	report, usage, err := parseOutput(a, result.Stdout)
	if err != nil {
		return report, usage, err
	}
	switch {
	case a.Option.Agent == "codex":
		a.evidence, err = verifyCodex(a, result.Stderr)
	case observed:
		a.evidence, err = verifyPi(a, piOffset)
	}
	return report, usage, err
}

// injectCodexDelegation adds SessionMan as an MCP server into a dispatched Codex so it can delegate
// and message its host. Stops at MaxDelegationDepth to prevent unbounded chains.
func (d *Daemon) injectCodexDelegation(plan *runtime.CommandPlan) {
	if d.meta.AxBinary == "" || d.meta.DelegationDepth >= MaxDelegationDepth {
		return
	}
	depth := strconv.Itoa(d.meta.DelegationDepth + 1)
	plan.Args = append(plan.Args,
		"-c", fmt.Sprintf("mcp_servers.sessionman.command=%q", d.meta.AxBinary),
		"-c", `mcp_servers.sessionman.args=["mcp","serve"]`,
		"-c", fmt.Sprintf("mcp_servers.sessionman.env.SM_DELEGATION_DEPTH=%q", depth),
		"-c", fmt.Sprintf("mcp_servers.sessionman.env.SM_HOST_AGENT_DIR=%q", d.Dir),
	)
	if home := os.Getenv("SM_SUBAGENT_HOME"); home != "" {
		plan.Args = append(plan.Args,
			"-c", fmt.Sprintf("mcp_servers.sessionman.env.SM_SUBAGENT_HOME=%q", home),
		)
	}
}

func (d *Daemon) injectPiDelegation(plan *runtime.CommandPlan) error {
	if d.meta.AxBinary == "" || d.meta.DelegationDepth >= MaxDelegationDepth {
		return nil
	}
	path, err := WritePiBridge(d.Home)
	if err != nil {
		return err
	}
	plan.Args = append(plan.Args, "--extension", path)
	plan.Env["SM_BINARY"] = d.meta.AxBinary
	plan.Env["SM_DELEGATION_DEPTH"] = strconv.Itoa(d.meta.DelegationDepth + 1)
	plan.Env["SM_HOST_AGENT_DIR"] = d.Dir
	return nil
}
