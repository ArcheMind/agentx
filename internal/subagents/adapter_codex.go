package subagents

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ArcheMind/agentx/internal/agentparams"
	"github.com/ArcheMind/agentx/internal/runtime"
)

// codexState is the app-server protocol state beside the live codex process: JSON-RPC
// correlation plus per-turn scratch (JSON-RPC 2.0 over stdio, line-delimited, no jsonrpc header).
type codexState struct {
	closed chan struct{}

	rpcMu   sync.Mutex
	nextID  int
	pending map[int]chan codexReply

	turnID       string
	text         string
	usage        json.RawMessage
	stderrOffset int
}

type codexReply struct {
	result json.RawMessage
	err    *codexError
}

type codexError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *codexState) route(id int, reply codexReply) {
	c.rpcMu.Lock()
	waiter := c.pending[id]
	delete(c.pending, id)
	c.rpcMu.Unlock()
	if waiter != nil {
		waiter <- reply
	}
}

// call issues one app-server request and waits for its response. Responses are routed by the
// reader goroutine without the daemon lock, so callers may hold it.
func (c *codexState) call(stream *runtime.Stream, method string, params any, timeout time.Duration) (json.RawMessage, *codexError, error) {
	c.rpcMu.Lock()
	c.nextID++
	id := c.nextID
	waiter := make(chan codexReply, 1)
	c.pending[id] = waiter
	c.rpcMu.Unlock()
	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	defer func() { c.rpcMu.Lock(); delete(c.pending, id); c.rpcMu.Unlock() }()
	line, err := json.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	if err := stream.WriteLine(string(line)); err != nil {
		return nil, nil, fmt.Errorf("codex app-server write: %w", err)
	}
	select {
	case reply := <-waiter:
		return reply.result, reply.err, nil
	case <-c.closed:
		return nil, nil, fmt.Errorf("codex app-server exited while awaiting %s", method)
	case <-time.After(timeout):
		return nil, nil, fmt.Errorf("codex app-server did not answer %s within %s", method, timeout)
	}
}

func codexInput(text string) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}}
}

func (d *Daemon) sendCodexLocked(message, delivery string) controlResponse {
	if d.proc == nil {
		if err := d.startCodexLocked(); err != nil {
			return controlResponse{Error: err.Error()}
		}
	}
	seq := LastEventSeq(d.Dir)
	stream, cs := d.proc.stream, d.codex
	if d.running {
		if delivery == "queued" {
			d.queue = append(d.queue, message)
			return controlResponse{Disposition: "queued", Seq: seq}
		}
		_, rpcErr, err := cs.call(stream, "turn/steer", map[string]any{"threadId": d.agent.Session, "expectedTurnId": cs.turnID, "input": codexInput(steerPrefix + message)}, 30*time.Second)
		if err != nil {
			return controlResponse{Error: err.Error()}
		}
		if rpcErr != nil {
			if rpcErr.Code != -32600 || !strings.Contains(rpcErr.Message, "no active turn") {
				return controlResponse{Error: fmt.Sprintf("codex turn/steer failed (%d): %s", rpcErr.Code, runtime.RedactSecrets(rpcErr.Message))}
			}
			// A settled turn cannot consume the message; the daemon owns its next-round delivery.
			d.debugf("codex steer rejected (%d %s); queued instead", rpcErr.Code, rpcErr.Message)
			d.queue = append(d.queue, message)
			return controlResponse{Disposition: "queued", Seq: seq}
		}
		return controlResponse{Disposition: "steered", Seq: seq}
	}
	cs.stderrOffset = len(stream.Stderr())
	result, rpcErr, err := cs.call(stream, "turn/start", map[string]any{"threadId": d.agent.Session, "input": codexInput(message)}, 30*time.Second)
	if err != nil {
		return controlResponse{Error: err.Error()}
	}
	if rpcErr != nil {
		return controlResponse{Error: fmt.Sprintf("codex turn/start failed (%d): %s", rpcErr.Code, rpcErr.Message)}
	}
	var started struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	_ = json.Unmarshal(result, &started)
	cs.turnID = started.Turn.ID
	cs.text, cs.usage = "", nil
	d.running = true
	return controlResponse{Disposition: "started", Seq: seq}
}

func (d *Daemon) startCodexLocked() error {
	a := d.agent
	roleInstructions, err := d.preStartLocked()
	if err != nil {
		return err
	}
	req, err := d.parameterRequest(roleInstructions)
	if err != nil {
		return err
	}
	mapped, err := agentparams.Build(a.Option.Agent, agentparams.AppServer, req)
	if err != nil {
		return err
	}
	auth, err := DiscoverAuth("codex")
	if err != nil {
		return err
	}
	plan := runtime.CommandPlan{Executable: "codex", Args: mapped.Args, Cwd: req.Cwd}
	plan.Env, plan.UnsetEnv = AuthEnvironment(auth)
	plan.Env["RUST_LOG"] = codexTelemetryFilter
	stream, err := d.startStream(plan)
	if err != nil {
		return err
	}
	proc := &nativeProc{stream: stream, done: make(chan struct{})}
	cs := &codexState{pending: map[int]chan codexReply{}, closed: make(chan struct{})}
	d.proc, d.codex, d.procAuth = proc, cs, auth
	notes := make(chan string, 256)
	go d.readCodex(proc, cs, notes)
	go d.pumpCodex(proc, notes)
	fail := func(err error) error {
		stream.Kill()
		return err
	}
	if _, rpcErr, err := cs.call(stream, "initialize", map[string]any{"clientInfo": map[string]any{"name": "sessionman", "title": "SessionMan", "version": "1.0"}}, 30*time.Second); err != nil || rpcErr != nil {
		if err == nil {
			err = fmt.Errorf("codex initialize failed (%d): %s", rpcErr.Code, rpcErr.Message)
		}
		return fail(err)
	}
	params := mapped.Params
	method := "thread/start"
	if a.Session != "" {
		method = "thread/resume"
	}
	result, rpcErr, err := cs.call(stream, method, params, 60*time.Second)
	if err != nil {
		return fail(err)
	}
	if rpcErr != nil {
		return fail(fmt.Errorf("codex %s failed (%d): %s", method, rpcErr.Code, rpcErr.Message))
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(result, &thread)
	if thread.Thread.ID == "" || (a.Session != "" && a.Session != thread.Thread.ID) {
		return fail(fmt.Errorf("unexpected native session; no continuation accepted"))
	}
	a.Session = thread.Thread.ID
	d.persistMetaLocked()
	return nil
}

// readCodex classifies app-server lines: responses are routed lock-free so callers may wait
// under the daemon lock, server-initiated approvals are answered inline, notifications go to
// the pump.
func (d *Daemon) readCodex(proc *nativeProc, cs *codexState, notes chan<- string) {
	for line := range proc.stream.Lines() {
		var envelope struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *codexError     `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		switch {
		case envelope.ID != nil && envelope.Method == "":
			cs.route(*envelope.ID, codexReply{result: envelope.Result, err: envelope.Error})
		case envelope.ID != nil:
			d.answerCodexApproval(proc.stream, cs, *envelope.ID, envelope.Method)
		default:
			notes <- line
		}
	}
	close(cs.closed)
	close(notes)
}

func (d *Daemon) answerCodexApproval(stream *runtime.Stream, cs *codexState, id int, method string) {
	reply := map[string]any{"id": id}
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		reply["result"] = map[string]any{"decision": "decline"}
	default:
		reply["error"] = map[string]any{"code": -32601, "message": "interactive client request unavailable in an ExternalSubagent: " + method}
	}
	line, _ := json.Marshal(reply)
	_ = stream.WriteLine(string(line))
}

func (d *Daemon) pumpCodex(proc *nativeProc, notes <-chan string) {
	for line := range notes {
		d.handleCodexNote(line)
	}
	d.procExit(proc, "codex app-server execution")
}

func (d *Daemon) handleCodexNote(line string) {
	var note struct {
		Method string `json:"method"`
		Params struct {
			Turn struct {
				ID     string          `json:"id"`
				Status string          `json:"status"`
				Error  json.RawMessage `json:"error"`
			} `json:"turn"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
			TokenUsage struct {
				Last json.RawMessage `json:"last"`
			} `json:"tokenUsage"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(line), &note); err != nil {
		return
	}
	switch note.Method {
	case "turn/started":
		d.mu.Lock()
		if d.codex != nil {
			d.codex.turnID = note.Params.Turn.ID
		}
		d.running = true
		d.mu.Unlock()
	case "item/completed":
		if note.Params.Item.Type != "agentMessage" {
			return
		}
		d.mu.Lock()
		if d.codex != nil {
			d.codex.text = note.Params.Item.Text
		}
		d.mu.Unlock()
	case "thread/tokenUsage/updated":
		d.mu.Lock()
		if d.codex != nil && len(note.Params.TokenUsage.Last) > 0 {
			d.codex.usage = note.Params.TokenUsage.Last
		}
		d.mu.Unlock()
	case "turn/completed":
		remaining := d.worktreeState()
		d.mu.Lock()
		cs := d.codex
		if cs == nil {
			d.mu.Unlock()
			return
		}
		report := Event{Status: "completed", Report: cs.text, Usage: cs.usage, Worktree: remaining}
		stderr := d.proc.stream.Stderr()
		if cs.stderrOffset <= len(stderr) {
			stderr = stderr[cs.stderrOffset:]
		}
		report.Evidence, _ = verifyCodex(d.agent, stderr)
		switch {
		case note.Params.Turn.Status == "completed" && cs.text == "":
			report.Status, report.Report = "failed", ""
			report.Failure = "ExternalSubagent exited without a complete report and native session; no implicit retry"
		case note.Params.Turn.Status == "interrupted":
			report.Status, report.Report, report.Failure = "cancelled", "", "Native turn interrupted"
		case note.Params.Turn.Status != "completed":
			failure := string(note.Params.Turn.Error)
			if failure == "" || failure == "null" {
				failure = "turn " + note.Params.Turn.Status
			}
			report.Status, report.Report = "failed", ""
			report.Failure = "ExternalSubagent failed: " + runtime.RedactSecrets(failure)
		}
		cs.turnID, cs.text, cs.usage = "", "", nil
		d.running = false
		d.appendReportLocked(report)
		d.mu.Unlock()
		d.deliverQueue()
	}
}
