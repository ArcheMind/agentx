package subagents

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/runtime"
)

// steerPrefix frames mid-turn text so the model attributes it to its coordinator instead of
// treating it as injection inside command output (verified hazard: unframed steers are ignored).
const steerPrefix = "Message from your coordinator (via sessionman Send):\n"

// sendClaudeLocked delivers one message into the live claude session, spawning the streaming
// process when none is alive (warm start and cold resume are the same path).
func (d *Daemon) sendClaudeLocked(message, delivery string) controlResponse {
	if d.proc == nil {
		if err := d.startClaudeLocked(); err != nil {
			return controlResponse{Error: err.Error()}
		}
	}
	seq := LastEventSeq(d.Dir)
	if d.running {
		if delivery == "queued" {
			d.queue = append(d.queue, message)
			return controlResponse{Disposition: "queued", Seq: seq}
		}
		if err := d.writeUserLocked(steerPrefix + message); err != nil {
			return controlResponse{Error: err.Error()}
		}
		return controlResponse{Disposition: "steered", Seq: seq}
	}
	if err := d.writeUserLocked(message); err != nil {
		return controlResponse{Error: err.Error()}
	}
	d.running = true
	return controlResponse{Disposition: "started", Seq: seq}
}

func (d *Daemon) writeUserLocked(text string) error {
	line, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": text}}}})
	if err != nil {
		return err
	}
	return d.proc.stream.WriteLine(string(line))
}

func (d *Daemon) startClaudeLocked() error {
	a := d.agent
	roleInstructions, err := d.preStartLocked()
	if err != nil {
		return err
	}
	plan, err := drivers.ClaudePolicyExecPlan(a.Cwd, a.Option.Model, a.Session, a.Mode == ModeYolo, a.Option.Effort, roleInstructions, a.Option.Flags)
	if err != nil {
		return err
	}
	d.injectClaudeDelegation(&plan)
	d.procAuth = Auth{}
	plan.Env, plan.UnsetEnv = AuthEnvironment(d.procAuth)
	stream, err := d.startStream(plan)
	if err != nil {
		return err
	}
	proc := &nativeProc{stream: stream, done: make(chan struct{})}
	d.proc = proc
	go d.readClaude(proc)
	return nil
}

func (d *Daemon) readClaude(proc *nativeProc) {
	for line := range proc.stream.Lines() {
		d.handleClaudeLine(line)
	}
	d.procExit(proc, "claude execution")
}

func (d *Daemon) handleClaudeLine(line string) {
	var event struct {
		Type    string          `json:"type"`
		Subtype string          `json:"subtype"`
		Session string          `json:"session_id"`
		IsError bool            `json:"is_error"`
		Result  string          `json:"result"`
		Usage   json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return
	}
	switch event.Type {
	case "system":
		if event.Subtype != "init" || event.Session == "" {
			return
		}
		d.mu.Lock()
		switch {
		case d.agent.Session == "":
			d.agent.Session = event.Session
			d.persistMetaLocked()
		case d.agent.Session != event.Session:
			d.stopping = true
			d.running = false
			d.appendReportLocked(Event{Status: "failed", Failure: "unexpected Claude session; no continuation accepted"})
			if d.proc != nil {
				d.proc.stream.Kill()
			}
		}
		d.mu.Unlock()
	case "assistant":
		// Background work inside claude can open turns the daemon did not initiate.
		d.mu.Lock()
		d.running = true
		d.mu.Unlock()
	case "result":
		remaining := d.worktreeState()
		report := Event{Status: "completed", Report: event.Result, Usage: event.Usage, Worktree: remaining}
		if event.IsError {
			report.Status, report.Report = "failed", ""
			report.Failure = event.Result
			if report.Failure == "" {
				report.Failure = event.Subtype
			}
		}
		d.mu.Lock()
		d.running = false
		d.appendReportLocked(report)
		d.mu.Unlock()
		d.deliverQueue()
	}
}

func (d *Daemon) injectClaudeDelegation(plan *runtime.CommandPlan) {
	if d.meta.AxBinary == "" || d.meta.DelegationDepth >= MaxDelegationDepth {
		return
	}
	env := map[string]string{
		"SM_DELEGATION_DEPTH": strconv.Itoa(d.meta.DelegationDepth + 1),
		"SM_HOST_AGENT_DIR":   d.Dir,
	}
	if home := os.Getenv("SM_SUBAGENT_HOME"); home != "" {
		env["SM_SUBAGENT_HOME"] = home
	}
	entry, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"sessionman": map[string]any{
		"type": "stdio", "command": d.meta.AxBinary, "args": []string{"mcp", "serve"}, "env": env,
	}}})
	plan.Args = append(plan.Args,
		"--mcp-config", string(entry),
		"--allowedTools", "mcp__sessionman__StartExternalSubagent", "mcp__sessionman__SendExternalSubagentMessage", "mcp__sessionman__StopExternalSubagent", "mcp__sessionman__ListExternalSubagentSessions", "mcp__sessionman__MessageHost",
	)
}
