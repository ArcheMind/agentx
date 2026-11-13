package subagents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArcheMind/agentx/internal/agentparams"
	"github.com/ArcheMind/agentx/internal/runtime"
)

// piState is the per-round scratch beside the live `pi --mode rpc` process.
type piReply struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    struct {
		Disposition string `json:"disposition"`
	} `json:"data"`
}

type piState struct {
	replies chan piReply
	text    string
	usage   json.RawMessage
	failure string
	// offset is the session file size at round start; the observer's provider-request
	// records after it are this round's effort evidence.
	offset int64
}

func (d *Daemon) sendPiLocked(message, delivery string) controlResponse {
	if d.proc == nil {
		if err := d.startPiLocked(); err != nil {
			return controlResponse{Error: err.Error()}
		}
	}
	seq := LastEventSeq(d.Dir)
	if d.running {
		if delivery == "queued" {
			d.queue = append(d.queue, message)
			return controlResponse{Disposition: "queued", Seq: seq}
		}
		if err := d.writePiLocked("steer", steerPrefix+message); err != nil {
			return controlResponse{Error: err.Error()}
		}
		return controlResponse{Disposition: "steered", Seq: seq}
	}
	d.pi.text, d.pi.usage, d.pi.failure, d.pi.offset = "", nil, "", 0
	if info, err := os.Stat(d.agent.Session); err == nil {
		d.pi.offset = info.Size()
	}
	if err := d.writePiLocked("prompt", message); err != nil {
		return controlResponse{Error: err.Error()}
	}
	d.running = true
	return controlResponse{Disposition: "started", Seq: seq}
}

func (d *Daemon) writePiLocked(command, message string) error {
	line, err := json.Marshal(map[string]any{"type": command, "message": message})
	if err != nil {
		return err
	}
	if err := d.proc.stream.WriteLine(string(line)); err != nil {
		return err
	}
	select {
	case reply, ok := <-d.pi.replies:
		if !ok {
			return fmt.Errorf("pi exited while awaiting %s", command)
		}
		if reply.Command != command {
			return fmt.Errorf("pi acknowledged %s while awaiting %s", reply.Command, command)
		}
		if !reply.Success {
			return fmt.Errorf("pi %s failed: %s", command, runtime.RedactSecrets(reply.Error))
		}
		expected := "started"
		if command == "steer" {
			expected = "queued"
		}
		if reply.Data.Disposition != expected {
			return fmt.Errorf("pi %s disposition %q did not start or steer a turn", command, reply.Data.Disposition)
		}
		return nil
	case <-time.After(30 * time.Second):
		d.proc.stream.Kill()
		return fmt.Errorf("pi did not acknowledge %s within 30s", command)
	}
}

func (d *Daemon) startPiLocked() error {
	a := d.agent
	roleInstructions, err := d.preStartLocked()
	if err != nil {
		return err
	}
	if a.Session == "" {
		dir := filepath.Join(d.Home, ".local", "state", "sessionman", "subagents", a.ID)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		a.Session = filepath.Join(dir, "session.jsonl")
	} else if _, err := os.Stat(a.Session); err != nil {
		return fmt.Errorf("original Pi session unavailable: %w; no replacement created", err)
	}
	auth, err := DiscoverAuth("pi")
	if err != nil {
		return err
	}
	req, err := d.parameterRequest(roleInstructions)
	if err != nil {
		return err
	}
	mapped, err := agentparams.Build(a.Option.Agent, agentparams.RPC, req)
	if err != nil {
		return err
	}
	plan := runtime.CommandPlan{Executable: "pi", Args: mapped.Args, Cwd: req.Cwd}
	plan.Env, plan.UnsetEnv = AuthEnvironment(auth)
	if err := d.injectPiDelegation(&plan); err != nil {
		return err
	}
	if a.Option.Effort != "" {
		observerArgs, observerEnv, err := preparePiObserver(a)
		if err != nil {
			return err
		}
		plan.Args = append(plan.Args, observerArgs...)
		for key, value := range observerEnv {
			plan.Env[key] = value
		}
	}
	stream, err := d.startStream(plan)
	if err != nil {
		return err
	}
	proc := &nativeProc{stream: stream, done: make(chan struct{})}
	d.proc, d.pi, d.procAuth = proc, &piState{replies: make(chan piReply, 1)}, auth
	d.persistMetaLocked()
	notes := make(chan string, 256)
	go d.readPi(proc, d.pi, notes)
	go func() {
		for line := range notes {
			d.handlePiLine(line)
		}
		d.procExit(proc, "pi execution")
	}()
	return nil
}

func (d *Daemon) readPi(proc *nativeProc, pi *piState, notes chan<- string) {
	for line := range proc.stream.Lines() {
		var reply piReply
		if json.Unmarshal([]byte(line), &reply) == nil && reply.Type == "response" {
			pi.replies <- reply
		} else {
			notes <- line
		}
	}
	close(pi.replies)
	close(notes)
}

func (d *Daemon) handlePiLine(line string) {
	var event struct {
		Type    string          `json:"type"`
		ID      string          `json:"id"`
		Message json.RawMessage `json:"message"`
		Aborted bool            `json:"aborted"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		d.debugf("invalid pi event: %v; raw=%s", err, line)
		return
	}
	switch event.Type {
	case "session":
		d.mu.Lock()
		if event.ID != "" && d.agent.NativeID == "" {
			d.agent.NativeID = event.ID
			d.persistMetaLocked()
		} else if event.ID != "" && d.agent.NativeID != event.ID {
			d.stopping = true
			d.running = false
			d.appendReportLocked(Event{Status: "failed", Failure: "Pi returned a different native session; no replacement accepted"})
			if d.proc != nil {
				d.proc.stream.Kill()
			}
		}
		d.mu.Unlock()
	case "message_end":
		text, usage, failure, err := parsePiAssistant(event.Message, d.agent.Option.Model)
		if err != nil {
			return
		}
		d.mu.Lock()
		if d.pi != nil {
			d.pi.text, d.pi.usage, d.pi.failure = text, usage, failure
		}
		d.running = true
		d.mu.Unlock()
	case "agent_settled":
		remaining := d.worktreeState()
		d.mu.Lock()
		pi := d.pi
		if pi == nil {
			d.mu.Unlock()
			return
		}
		report := Event{Status: "completed", Report: pi.text, Usage: pi.usage, Worktree: remaining}
		if d.agent.Option.Effort != "" {
			var err error
			report.Evidence, err = verifyPi(d.agent, pi.offset)
			if err != nil {
				pi.failure = err.Error()
			}
		}
		failure := pi.failure
		if event.Aborted && failure == "" {
			failure = "pi run aborted"
		}
		switch {
		case failure != "":
			report.Status, report.Report = "failed", ""
			report.Failure = "ExternalSubagent failed: " + runtime.RedactSecrets(failure)
		case pi.text == "":
			report.Status, report.Report = "failed", ""
			report.Failure = "ExternalSubagent exited without a complete report and native session; no implicit retry"
		}
		pi.text, pi.usage, pi.failure = "", nil, ""
		d.running = false
		d.appendReportLocked(report)
		d.mu.Unlock()
		d.deliverQueue()
	}
}

// parsePiAssistant extracts this round's report fields from an assistant message_end; other
// roles return ok with nothing.
func parsePiAssistant(raw json.RawMessage, expectedModel string) (string, json.RawMessage, string, error) {
	var role struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &role); err != nil {
		return "", nil, "", err
	}
	if role.Role != "assistant" {
		return "", nil, "", fmt.Errorf("not an assistant message")
	}
	var message struct {
		Model        string `json:"model"`
		Provider     string `json:"provider"`
		StopReason   string `json:"stopReason"`
		ErrorMessage string `json:"errorMessage"`
		Content      []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return "", nil, "", err
	}
	if message.Provider+"/"+message.Model != expectedModel {
		return "", nil, fmt.Sprintf("Pi returned model %s/%s instead of %s", message.Provider, message.Model, expectedModel), nil
	}
	failure := ""
	if message.StopReason == "error" || message.StopReason == "aborted" {
		failure = message.ErrorMessage
		if failure == "" {
			failure = message.StopReason
		}
	}
	var parts []string
	for _, part := range message.Content {
		if part.Type == "text" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n"), message.Usage, failure, nil
}
