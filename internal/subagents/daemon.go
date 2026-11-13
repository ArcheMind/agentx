package subagents

import (
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

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
)

const DefaultLinger = 60 * time.Second

// nativeProc is the one live native-protocol process a daemon owns; adapter-specific state
// (codexState, piState) hangs off the Daemon beside it.
type nativeProc struct {
	stream *runtime.Stream
	done   chan struct{}
}

// Daemon owns one ExternalSubagent: it holds the live native session, serializes rounds,
// delivers steered and queued messages, and appends the agent's event stream.
type Daemon struct {
	Home   string
	Dir    string
	Runner runtime.Runner
	Log    io.Writer
	Linger time.Duration

	mu         sync.Mutex
	meta       AgentMeta
	agent      *ExternalSubagentState
	proc       *nativeProc
	codex      *codexState
	pi         *piState
	agy        *agyState
	procAuth   Auth
	running    bool
	stopping   bool
	queue      []string
	lastStatus string
	linger     *time.Timer
	quit       chan struct{}
	quitOnce   sync.Once
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
	proc := d.proc
	d.mu.Unlock()
	if proc != nil {
		_ = proc.stream.CloseStdin()
		select {
		case <-proc.done:
		case <-time.After(5 * time.Second):
			proc.stream.Kill()
			<-proc.done
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
			if d.proc != nil {
				d.proc.stream.Kill()
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
	response := d.sendAdapterLocked(message, delivery)
	if response.Error != "" && !d.running {
		d.appendReportLocked(Event{Status: "failed", Failure: response.Error})
	}
	return response
}

func (d *Daemon) sendAdapterLocked(message, delivery string) controlResponse {
	if !d.running {
		// A new round re-checks the isolation worktree; a live turn already runs inside it.
		if err := checkWorktree(d.agent); err != nil {
			return controlResponse{Error: err.Error()}
		}
	}
	switch d.agent.Option.Agent {
	case "claude":
		return d.sendClaudeLocked(message, delivery)
	case "codex":
		return d.sendCodexLocked(message, delivery)
	case "pi":
		return d.sendPiLocked(message, delivery)
	case "agy":
		return d.sendAgyLocked(message, delivery)
	default:
		return controlResponse{Error: fmt.Sprintf("%s has no native subagent execution adapter", d.agent.Option.Agent)}
	}
}

func (d *Daemon) stop() controlResponse {
	d.mu.Lock()
	d.stopping = true
	d.queue = nil
	hadWork := d.running
	var wait chan struct{}
	if d.proc != nil {
		d.proc.stream.Kill()
		wait = d.proc.done
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
	if event.Auth == "" && d.procAuth.Key != "" {
		event.Auth = d.procAuth.String()
	}
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

// deliverQueue starts the next round from queued messages after a settle. The first message
// opens the round; any further held messages steer into it.
func (d *Daemon) deliverQueue() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopping || d.running || len(d.queue) == 0 {
		d.resetLingerLocked()
		return
	}
	messages := d.queue
	d.queue = nil
	if response := d.sendAdapterLocked(messages[0], "steer"); response.Error != "" {
		d.appendReportLocked(Event{Status: "failed", Failure: response.Error})
		d.queue = append(messages[1:], d.queue...)
		go d.deliverQueue()
		return
	}
	for _, message := range messages[1:] {
		if response := d.sendAdapterLocked(message, "steer"); response.Error != "" {
			d.appendReportLocked(Event{Status: "failed", Failure: "Queued message delivery failed: " + response.Error})
		}
	}
}

// preStartLocked runs the shared pre-round checks before a native process spawn and returns
// the role instructions.
func (d *Daemon) preStartLocked() (string, error) {
	a := d.agent
	if current, err := presets.Read(d.Home, a.Option.Name); err != nil || !current.Equal(a.Option) {
		return "", fmt.Errorf("original preset %s changed or was removed before execution; no replacement selected", a.Option.Name)
	}
	if err := checkWorktree(a); err != nil {
		return "", err
	}
	if a.Option.Description != "" {
		return "Role: " + a.Option.Name + "\n" + a.Option.Description, nil
	}
	return "", nil
}

func (d *Daemon) startStream(plan runtime.CommandPlan) (*runtime.Stream, error) {
	starter, ok := d.Runner.(runtime.StreamStarter)
	if !ok {
		return nil, fmt.Errorf("%s streaming requires a stream-capable runner", d.agent.Option.Agent)
	}
	return starter.StartStream(plan)
}

// procExit settles the daemon after the native process died: a cancelled report when a stop
// killed mid-run work, a failed one when the process ended with a turn still open.
func (d *Daemon) procExit(proc *nativeProc, label string) {
	err := proc.stream.Wait()
	stderr := strings.TrimSpace(proc.stream.Stderr())
	remaining := d.worktreeState()
	d.mu.Lock()
	wasRunning, stopping := d.running, d.stopping
	d.proc, d.codex, d.pi, d.agy = nil, nil, nil, nil
	d.running = false
	if wasRunning {
		if stopping {
			d.appendReportLocked(Event{Status: "cancelled", Failure: "Native execution terminated after cancellation", Worktree: remaining})
		} else {
			failure := label + " ended before the turn settled"
			if err != nil {
				failure = label + " failed: " + runtime.RedactSecrets(err.Error())
			}
			if stderr != "" {
				failure += "; stderr: " + runtime.RedactSecrets(withoutCodexTelemetry(stderr))
			}
			d.appendReportLocked(Event{Status: "failed", Failure: failure, Worktree: remaining})
		}
	}
	d.mu.Unlock()
	close(proc.done)
	d.deliverQueue()
}

func (d *Daemon) worktreeState() string {
	if d.agent.Worktree == nil {
		return ""
	}
	return worktreeRemaining(d.Runner, d.agent.Worktree)
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
