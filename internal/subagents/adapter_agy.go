package subagents

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ArcheMind/agentx/internal/agentparams"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
)

type agyError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type agyReply struct {
	result json.RawMessage
	err    *agyError
}

// A single prompt retains native usage; merged turns preserve each RPC's raw usage.
type agyPromptResult struct {
	ID         int             `json:"id"`
	StopReason string          `json:"stopReason"`
	Usage      json.RawMessage `json:"usage"`
}

type agyState struct {
	rpcMu     sync.Mutex
	nextID    int
	pending   map[int]chan agyReply
	closed    chan struct{}
	live      bool
	prompts   map[int]bool
	text      string
	results   []agyPromptResult
	terminal  string
	failure   string
	cancelled bool
	role      string
}

func (a *agyState) request(stream *runtime.Stream, method string, params any, wait bool) (int, <-chan agyReply, error) {
	a.rpcMu.Lock()
	a.nextID++
	id := a.nextID
	var ch chan agyReply
	if wait {
		ch = make(chan agyReply, 1)
		a.pending[id] = ch
	}
	a.rpcMu.Unlock()
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err == nil {
		err = stream.WriteLine(string(raw))
	}
	if err != nil {
		a.rpcMu.Lock()
		delete(a.pending, id)
		a.rpcMu.Unlock()
	}
	return id, ch, err
}

func (a *agyState) call(stream *runtime.Stream, method string, params any) (json.RawMessage, error) {
	id, ch, err := a.request(stream, method, params, true)
	if err != nil {
		return nil, err
	}
	defer func() { a.rpcMu.Lock(); delete(a.pending, id); a.rpcMu.Unlock() }()
	select {
	case reply := <-ch:
		if reply.err != nil {
			return nil, fmt.Errorf("agy %s failed (%d): %s", method, reply.err.Code, runtime.RedactSecrets(reply.err.Message))
		}
		return reply.result, nil
	case <-a.closed:
		return nil, fmt.Errorf("agy exited while awaiting %s", method)
	case <-time.After(60 * time.Second):
		return nil, fmt.Errorf("agy did not answer %s within 60s", method)
	}
}

func (d *Daemon) sendAgyLocked(message, delivery string) controlResponse {
	if d.proc == nil {
		if err := d.startAgyLocked(); err != nil {
			return controlResponse{Error: err.Error()}
		}
	}
	seq := LastEventSeq(d.Dir)
	state := d.agy
	disposition := "started"
	if d.running {
		if delivery == "queued" {
			d.queue = append(d.queue, message)
			return controlResponse{Disposition: "queued", Seq: seq}
		}
		disposition, message = "steered", steerPrefix+message
	} else {
		state.text, state.results, state.terminal, state.failure, state.cancelled = "", nil, "", "", false
		if state.role != "" {
			if body, marked := strings.CutPrefix(message, sessions.SubagentMarker+"\n"); marked {
				message = sessions.SubagentMarker + "\n" + state.role + "\n\n" + body
			} else {
				message = state.role + "\n\n" + message
			}
		}
	}
	state.rpcMu.Lock()
	state.live = true
	state.rpcMu.Unlock()
	id, _, err := state.request(d.proc.stream, "session/prompt", map[string]any{"sessionId": d.agent.Session, "prompt": []map[string]any{{"type": "text", "text": message}}}, false)
	if err != nil {
		return controlResponse{Error: err.Error()}
	}
	state.prompts[id] = true
	d.running = true
	return controlResponse{Disposition: disposition, Seq: seq}
}

func (d *Daemon) startAgyLocked() error {
	a := d.agent
	role, err := d.preStartLocked()
	if err != nil {
		return err
	}
	req, err := d.parameterRequest(role)
	if err != nil {
		return err
	}
	mapped, err := agentparams.Build(a.Option.Agent, agentparams.ACP, req)
	if err != nil {
		return err
	}
	stream, err := d.startStream(runtime.CommandPlan{Executable: "agy", Args: mapped.Args, Cwd: req.Cwd, Env: d.agyEnvironment()})
	if err != nil {
		return err
	}
	proc := &nativeProc{stream: stream, done: make(chan struct{})}
	state := &agyState{pending: map[int]chan agyReply{}, closed: make(chan struct{}), prompts: map[int]bool{}, role: mapped.PromptPrefix}
	d.proc, d.agy, d.procAuth = proc, state, Auth{}
	notes := make(chan string, 256)
	go d.readAgy(proc, state, notes)
	go func() {
		for line := range notes {
			d.handleAgyLine(state, line)
		}
		d.procExit(proc, "agy ACP execution")
	}()
	fail := func(err error) error { stream.Kill(); return err }
	if _, err := state.call(stream, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}, "clientInfo": map[string]any{"name": "sessionman"}}); err != nil {
		return fail(err)
	}
	params := mapped.Params
	method := "session/new"
	if a.Session != "" {
		method = "session/load"
	}
	result, err := state.call(stream, method, params)
	if err != nil {
		return fail(err)
	}
	var session struct {
		ID string `json:"sessionId"`
	}
	if err := json.Unmarshal(result, &session); err != nil {
		return fail(fmt.Errorf("invalid agy session response: %w", err))
	}
	if a.Session == "" {
		if session.ID == "" {
			return fail(fmt.Errorf("agy returned no native session; no replacement accepted"))
		}
		a.Session = session.ID
	} else if session.ID != "" && session.ID != a.Session {
		return fail(fmt.Errorf("agy returned a different native session; no replacement accepted"))
	}
	a.NativeID = a.Session
	for _, setting := range mapped.Settings {
		setting.Params["sessionId"] = a.Session
		if _, err := state.call(stream, setting.Method, setting.Params); err != nil {
			return fail(err)
		}
	}
	d.persistMetaLocked()
	return nil
}

func (d *Daemon) agyEnvironment() map[string]string {
	env := map[string]string{"SM_MCP_CWD": d.agent.Cwd, "SM_HOST_AGENT_DIR": d.Dir, "SM_DELEGATION_DEPTH": fmt.Sprint(d.meta.DelegationDepth + 1)}
	if home := os.Getenv("SM_SUBAGENT_HOME"); home != "" {
		env["SM_SUBAGENT_HOME"] = home
	}
	return env
}

func (d *Daemon) readAgy(proc *nativeProc, state *agyState, notes chan<- string) {
	for line := range proc.stream.Lines() {
		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *agyError       `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			d.debugf("invalid agy event: %v; raw=%s", err, line)
			continue
		}
		if len(envelope.ID) > 0 && envelope.Method != "" {
			d.answerAgyRequest(proc.stream, envelope.ID, envelope.Method)
			continue
		}
		if len(envelope.ID) > 0 {
			var id int
			if json.Unmarshal(envelope.ID, &id) != nil {
				continue
			}
			state.rpcMu.Lock()
			ch := state.pending[id]
			delete(state.pending, id)
			state.rpcMu.Unlock()
			if ch != nil {
				ch <- agyReply{result: envelope.Result, err: envelope.Error}
				continue
			}
		}
		state.rpcMu.Lock()
		live := state.live
		state.rpcMu.Unlock()
		if live {
			notes <- line
		}
	}
	close(state.closed)
	close(notes)
}

func (d *Daemon) answerAgyRequest(stream *runtime.Stream, id json.RawMessage, method string) {
	reply := map[string]any{"jsonrpc": "2.0", "id": id}
	if method == "session/request_permission" {
		reply["result"] = map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	} else {
		reply["error"] = map[string]any{"code": -32601, "message": "interactive client request unavailable in an ExternalSubagent: " + method}
	}
	raw, _ := json.Marshal(reply)
	if err := stream.WriteLine(string(raw)); err != nil {
		d.debugf("agy client response failed: %v", err)
	}
}

func (d *Daemon) handleAgyLine(state *agyState, line string) {
	var event struct {
		ID     int       `json:"id"`
		Method string    `json:"method"`
		Error  *agyError `json:"error"`
		Result struct {
			StopReason string          `json:"stopReason"`
			Usage      json.RawMessage `json:"usage"`
		} `json:"result"`
		Params struct {
			Session string `json:"sessionId"`
			Update  struct {
				Type    string `json:"sessionUpdate"`
				Content struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"update"`
		} `json:"params"`
	}
	if json.Unmarshal([]byte(line), &event) != nil {
		return
	}
	d.mu.Lock()
	if d.agy != state || d.stopping {
		d.mu.Unlock()
		return
	}
	if event.Method == "session/update" {
		if event.Params.Session == d.agent.Session && event.Params.Update.Type == "agent_message_chunk" && event.Params.Update.Content.Type == "text" {
			state.text += event.Params.Update.Content.Text
		}
		d.mu.Unlock()
		return
	}
	if !state.prompts[event.ID] {
		d.mu.Unlock()
		return
	}
	delete(state.prompts, event.ID)
	if event.Error != nil {
		state.failure = fmt.Sprintf("agy session/prompt failed (%d): %s", event.Error.Code, runtime.RedactSecrets(event.Error.Message))
	}
	state.results = append(state.results, agyPromptResult{ID: event.ID, StopReason: event.Result.StopReason, Usage: event.Result.Usage})
	if event.Error == nil {
		switch event.Result.StopReason {
		case "end_turn":
		case "cancelled":
			state.cancelled = true
		case "max_tokens", "max_turn_requests", "refusal":
			state.terminal = fmt.Sprintf("agy prompt RPC %d stopped with native stopReason=%s; no implicit retry", event.ID, event.Result.StopReason)
		default:
			state.failure = fmt.Sprintf("agy prompt RPC %d returned invalid native stopReason %q; no implicit retry", event.ID, event.Result.StopReason)
		}
	}
	if len(state.prompts) > 0 {
		d.mu.Unlock()
		return
	}
	var usage json.RawMessage
	if len(state.results) == 1 {
		usage = state.results[0].Usage
	} else {
		usage, _ = json.Marshal(struct {
			Prompts []agyPromptResult `json:"prompts"`
		}{state.results})
	}
	report := Event{Status: "completed", Report: state.text, Usage: usage, Auth: "agy native credential resolution (no identity override)"}
	if d.agent.Option.Effort != "" {
		report.Evidence = "WARNING: agy accepted the thought_level request=" + d.agent.Option.Effort + "; native provider request effort was not recorded and is unverified"
	}
	switch {
	case state.failure != "":
		report.Status, report.Report, report.Failure = "failed", "", state.failure
	case state.cancelled:
		report.Status, report.Report, report.Failure = "cancelled", "", "Native turn cancelled"
	case state.terminal != "":
		report.Status, report.Failure = "failed", state.terminal
	case state.text == "":
		report.Status, report.Failure = "failed", "ExternalSubagent exited without a complete report; no implicit retry"
	}
	d.running = false
	report.Worktree = d.worktreeState()
	d.appendReportLocked(report)
	d.mu.Unlock()
	d.deliverQueue()
}
