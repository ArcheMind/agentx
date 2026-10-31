package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
	"github.com/ArcheMind/agentx/internal/subagents"
)

const sessionContinueUsage = "usage: ax [--json|--yaml] session continue <target-agent> [file] [--model <model>] [--effort <level>] [--append-system-prompt <text>] [--mcp-config <file|json>] [--mode <normal|yolo>] [--workspace <path>]"

// sessionContinue runs one headless turn on a unified transcript: import the
// history into the target agent's native store, resume it with the trailing
// user message as the prompt, and print the newly recorded messages as a
// transcript delta. The caller stays the owner of the full transcript.
func (a App) sessionContinue(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(sessionContinueUsage)
	}
	agent, err := a.Registry.Get(args[0])
	if err != nil {
		return err
	}
	var file, model, effort, systemPrompt, mcpValue, workspace string
	mode := "normal"
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--model":
			index++
			if index >= len(args) {
				return fmt.Errorf("--model requires a value")
			}
			model = args[index]
		case "--effort":
			index++
			if index >= len(args) {
				return fmt.Errorf("--effort requires a value")
			}
			effort = args[index]
		case "--append-system-prompt":
			index++
			if index >= len(args) {
				return fmt.Errorf("--append-system-prompt requires a value")
			}
			systemPrompt = args[index]
		case "--mcp-config":
			index++
			if index >= len(args) {
				return fmt.Errorf("--mcp-config requires a value")
			}
			mcpValue = args[index]
		case "--mode":
			index++
			if index >= len(args) {
				return fmt.Errorf("--mode requires a value")
			}
			mode = args[index]
			if mode != "normal" && mode != "yolo" {
				return fmt.Errorf("--mode must be normal or yolo")
			}
		case "--workspace":
			index++
			if index >= len(args) {
				return fmt.Errorf("--workspace requires a value")
			}
			workspace = args[index]
		default:
			if file == "" && !strings.HasPrefix(args[index], "--") {
				file = args[index]
				continue
			}
			return fmt.Errorf("unknown session continue option %q", args[index])
		}
	}
	if agent.ID != "claude" && agent.ID != "codex" && agent.ID != "pi" {
		return fmt.Errorf("%s has no headless continue adapter", agent.ID)
	}
	// The caller owns this process (a host harness); SIGTERM/SIGINT must cancel
	// the native turn's whole process group instead of orphaning it.
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if agent.ID == "pi" && model == "" {
		return fmt.Errorf("--model is required for pi (provider/model)")
	}
	mcp := drivers.MCPConfig{}
	if mcpValue != "" {
		mcp, err = readMCPConfig(mcpValue)
		if err != nil {
			return err
		}
	}
	detail, err := a.readTranscript(file)
	if err != nil {
		return err
	}
	prompt, history, err := splitContinuePrompt(detail.Messages)
	if err != nil {
		return err
	}
	workspace, err = resolveContinueWorkspace(workspace, detail.Workspace)
	if err != nil {
		return err
	}
	detail.Workspace = workspace

	stateDir := filepath.Join(a.Sessions.HomeDir, ".local", "state", "agentx", "resume")
	token := ""
	prefix := 0
	// Pi always gets a session file (even an empty one): --session is how pi
	// both resumes and records, and the recorded file is the delta source.
	if len(history) > 0 || agent.ID == "pi" {
		imported := detail
		// Claude's session scanner rejects records without a parseable
		// timestamp ("No conversation found"), so empty ones are filled.
		imported.Messages = fillTimestamps(history)
		imported.MessageCount = len(history)
		token, err = sessions.WriteSessionFile(imported, agent.ID, stateDir, a.Sessions.HomeDir)
		if err != nil {
			return err
		}
		importPath, err := a.continueSessionFile(agent.ID, workspace, token)
		if err != nil {
			return err
		}
		before, err := sessions.ParseNativeSessionFile(agent.ID, importPath)
		if err != nil {
			return err
		}
		prefix = len(before.Messages)
	}

	auth, err := subagents.DiscoverAuth(ctx, a.Runner, agent.ID, model)
	if err != nil {
		return err
	}
	plan, err := drivers.ContinueExecPlan(agent.ID, workspace, model, token, effort, systemPrompt, mode == "yolo", mcp)
	if err != nil {
		return err
	}
	plan.Env, plan.UnsetEnv = subagents.AuthEnvironment(auth)
	result, execErr := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Stdin: strings.NewReader(prompt), CancelProcessGroup: true})
	if ctx.Err() != nil {
		return fmt.Errorf("session continue cancelled: %w", ctx.Err())
	}
	run, parseErr := parseContinueStream(agent.ID, model, expectedSession(agent.ID, token), result.Stdout)
	if run.failure != "" {
		return fmt.Errorf("%s turn failed: %s", agent.ID, runtime.RedactSecrets(run.failure))
	}
	if execErr != nil {
		return fmt.Errorf("%s execution failed: %s; stderr: %s", agent.ID, runtime.RedactSecrets(execErr.Error()), runtime.RedactSecrets(strings.TrimSpace(result.Stderr)))
	}
	if parseErr != nil {
		return parseErr
	}
	if !run.completed {
		return fmt.Errorf("%s exited without completing the turn; stderr: %s", agent.ID, runtime.RedactSecrets(strings.TrimSpace(result.Stderr)))
	}
	if run.sessionID == "" && agent.ID != "pi" {
		return fmt.Errorf("%s reported no native session for the turn", agent.ID)
	}

	resultToken := token
	if agent.ID != "pi" {
		resultToken = run.sessionID
	}
	resultPath, err := a.continueSessionFile(agent.ID, workspace, resultToken)
	if err != nil {
		return err
	}
	full, err := sessions.ParseNativeSessionFile(agent.ID, resultPath)
	if err != nil {
		return err
	}
	if len(full.Messages) < prefix {
		return fmt.Errorf("%s recorded fewer messages (%d) than the imported history (%d)", agent.ID, len(full.Messages), prefix)
	}
	delta := full.Messages[prefix:]
	// Everything a turn records before its first assistant message is setup
	// the caller already owns or the agent re-creates per turn: the echoed
	// prompt, and per-turn context like Codex instruction records. Keeping it
	// would duplicate and snowball across turns.
	for len(delta) > 0 && delta[0].Role != "assistant" {
		delta = delta[1:]
	}
	if len(delta) == 0 {
		return fmt.Errorf("%s completed the turn but recorded no new messages", agent.ID)
	}
	delta = namespaceDelta(delta, continueRunID(agent.ID, token, run.sessionID))

	out := sessions.Session{
		ID:           run.sessionID,
		Provider:     agent.ID,
		Workspace:    workspace,
		StartedAt:    full.StartedAt,
		UpdatedAt:    full.UpdatedAt,
		MessageCount: len(delta),
		Source:       "continue",
		Messages:     delta,
	}
	if out.ID == "" {
		out.ID = full.ID
	}
	return writeStructured(a.Stdout, out, a.structuredDefault())
}

func fillTimestamps(messages []sessions.Message) []sessions.Message {
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]sessions.Message, len(messages))
	for i, message := range messages {
		if message.Timestamp == "" {
			message.Timestamp = message.RecordedAt
		}
		if message.Timestamp == "" {
			message.Timestamp = now
		}
		out[i] = message
	}
	return out
}

// splitContinuePrompt peels the trailing user message off the transcript; its
// text is the native prompt and everything before it is the imported history.
func splitContinuePrompt(messages []sessions.Message) (string, []sessions.Message, error) {
	if len(messages) == 0 {
		return "", nil, fmt.Errorf("transcript has no messages; nothing to continue")
	}
	last := messages[len(messages)-1]
	if last.Role != "user" {
		return "", nil, fmt.Errorf("transcript must end with a user message; got role %q", last.Role)
	}
	var parts []string
	for _, block := range last.Content {
		if block.Type != "text" {
			return "", nil, fmt.Errorf("the trailing user message allows only text blocks; got %q", block.Type)
		}
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	prompt := strings.Join(parts, "\n")
	if strings.TrimSpace(prompt) == "" {
		return "", nil, fmt.Errorf("the trailing user message has no text to send")
	}
	return prompt, messages[:len(messages)-1], nil
}

// resolveContinueWorkspace resolves symlinks because Claude keys its project
// store by the resolved working directory.
func resolveContinueWorkspace(flagValue, transcriptValue string) (string, error) {
	workspace := flagValue
	if workspace == "" {
		workspace = transcriptValue
	}
	if workspace == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		workspace = cwd
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("workspace %s is not usable: %w", absolute, err)
	}
	return resolved, nil
}

// continueSessionFile maps a session token to the native file that records it.
func (a App) continueSessionFile(agentID, workspace, token string) (string, error) {
	switch agentID {
	case "claude":
		return sessions.ClaudeProjectFile(a.Sessions.HomeDir, workspace, token), nil
	case "codex":
		return sessions.FindCodexRollout(a.Sessions.HomeDir, token)
	default:
		return token, nil
	}
}

func expectedSession(agentID, token string) string {
	if agentID == "pi" {
		return ""
	}
	return token
}

// continueRunID is a fresh-per-call namespace that keeps delta message ids
// unique in the caller's accumulated transcript across turns.
func continueRunID(agentID, token, sessionID string) string {
	if agentID == "pi" {
		return strings.TrimSuffix(filepath.Base(token), ".jsonl")
	}
	if token != "" {
		return token
	}
	return sessionID
}

func namespaceDelta(messages []sessions.Message, runID string) []sessions.Message {
	local := make(map[string]string, len(messages))
	for _, message := range messages {
		if message.ID != "" {
			local[message.ID] = runID + ":" + message.ID
		}
	}
	out := make([]sessions.Message, len(messages))
	for i, message := range messages {
		if mapped, ok := local[message.ID]; ok {
			message.ID = mapped
		}
		if mapped, ok := local[message.ParentID]; ok {
			message.ParentID = mapped
		}
		out[i] = message
	}
	return out
}

func readMCPConfig(value string) (drivers.MCPConfig, error) {
	data := []byte(value)
	if !strings.HasPrefix(strings.TrimSpace(value), "{") {
		loaded, err := os.ReadFile(value)
		if err != nil {
			return drivers.MCPConfig{}, fmt.Errorf("read MCP config: %w", err)
		}
		data = loaded
	}
	var config drivers.MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return drivers.MCPConfig{}, fmt.Errorf("parse MCP config JSON: %w", err)
	}
	if len(config.Servers) == 0 {
		return drivers.MCPConfig{}, fmt.Errorf("MCP config must declare at least one server under mcpServers")
	}
	for name, server := range config.Servers {
		if server.Command == "" {
			return drivers.MCPConfig{}, fmt.Errorf("MCP server %q must declare a command", name)
		}
	}
	return config, nil
}

type continueRun struct {
	sessionID string
	completed bool
	failure   string
}

// parseContinueStream reads the native headless event stream for the turn's
// session identity and completion; the content itself comes from the native
// store afterwards, not from the stream.
func parseContinueStream(agentID, model, expectSession, output string) (continueRun, error) {
	run := continueRun{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event struct {
			Type    string          `json:"type"`
			Subtype string          `json:"subtype"`
			Thread  string          `json:"thread_id"`
			ID      string          `json:"id"`
			Session string          `json:"session_id"`
			Result  string          `json:"result"`
			IsError bool            `json:"is_error"`
			Error   json.RawMessage `json:"error"`
			Message json.RawMessage `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return run, fmt.Errorf("invalid %s event: %w", agentID, err)
		}
		switch agentID {
		case "claude":
			switch event.Type {
			case "system":
				if event.Subtype == "init" && event.Session != "" {
					if expectSession != "" && event.Session != expectSession {
						return run, fmt.Errorf("claude continued session %s instead of %s", event.Session, expectSession)
					}
					run.sessionID = event.Session
				}
			case "result":
				if event.Session != "" && run.sessionID == "" {
					run.sessionID = event.Session
				}
				if event.IsError {
					run.failure = event.Result
					if run.failure == "" {
						run.failure = event.Subtype
					}
				} else {
					run.completed = true
				}
			}
		case "codex":
			switch event.Type {
			case "thread.started":
				if expectSession != "" && event.Thread != expectSession {
					return run, fmt.Errorf("codex continued thread %s instead of %s", event.Thread, expectSession)
				}
				run.sessionID = event.Thread
			case "turn.completed":
				run.completed = true
			case "error", "turn.failed":
				failure := string(event.Error)
				if failure == "" || failure == "null" {
					failure = string(event.Message)
				}
				run.failure = failure
			}
		default:
			switch event.Type {
			case "session":
				run.sessionID = event.ID
			case "message_end":
				var message struct {
					Role         string `json:"role"`
					Model        string `json:"model"`
					Provider     string `json:"provider"`
					StopReason   string `json:"stopReason"`
					ErrorMessage string `json:"errorMessage"`
				}
				if err := json.Unmarshal(event.Message, &message); err != nil {
					return run, err
				}
				if message.Role == "assistant" {
					if model != "" && message.Provider+"/"+message.Model != model {
						return run, fmt.Errorf("pi answered with model %s/%s instead of %s", message.Provider, message.Model, model)
					}
					if message.StopReason == "error" || message.StopReason == "aborted" {
						run.failure = message.ErrorMessage
						if run.failure == "" {
							run.failure = message.StopReason
						}
					}
				}
			case "agent_end":
				run.completed = true
			}
		}
	}
	return run, nil
}
