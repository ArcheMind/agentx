package subagents

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed pi_observer.js
var piObserver []byte

const (
	piObserverEntry = "agentx-provider-request"
	// codexTelemetryFilter enables only Codex's trace-safe events, which omit account and email.
	codexTelemetryFilter = "codex_otel.trace_safe=info"
)

// Codex echoes the configured effort in its rollout even when it sends another level (gpt-5.6-luna:
// ultra is sent as max), so the executed level is read from each completed model response event.
var codexExecutedEffort = regexp.MustCompile(`event\.name="codex\.sse_event" event\.kind=response\.completed .*\bmodel_reasoning_effort="?([A-Za-z_-]+)"?`)

func codexExecutedEfforts(stderr string) []string {
	var efforts []string
	for _, match := range codexExecutedEffort.FindAllStringSubmatch(stderr, -1) {
		efforts = append(efforts, match[1])
	}
	return efforts
}

func withoutCodexTelemetry(stderr string) string {
	var lines []string
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.Contains(line, " codex_otel.") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// verifyEffort reports whether the native model requests of the turn ran at the preset effort.
func verifyEffort(requested string, executed []string, source string) (string, error) {
	if len(executed) == 0 {
		if requested == "" {
			return "", nil
		}
		return fmt.Sprintf("effort could not be verified: %s recorded no executed effort for this turn", source), nil
	}
	seen := map[string]bool{}
	var distinct []string
	var mismatched []string
	for _, effort := range executed {
		if effort == "" {
			effort = "none sent"
		}
		if requested != "" && effort != requested {
			if !seen[effort] {
				mismatched = append(mismatched, effort)
			}
		}
		if !seen[effort] {
			seen[effort] = true
			distinct = append(distinct, effort)
		}
	}
	summary := fmt.Sprintf("executed effort %s (%d model request(s), from %s)", strings.Join(distinct, ", "), len(executed), source)
	if len(mismatched) > 0 {
		summary += fmt.Sprintf("; WARNING: preset requested %s but native executed %s", requested, strings.Join(mismatched, ", "))
	}
	return summary, nil
}

func codexRollout(codexHome, thread string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(codexHome, "sessions", "*", "*", "*", "rollout-*-"+thread+".jsonl"))
	if err != nil || len(matches) != 1 {
		return "", fmt.Errorf("Codex rollout for thread %s not found exactly once under %s/sessions (found %d)", thread, codexHome, len(matches))
	}
	return matches[0], nil
}

// verifyCodexRole checks that the native session carries the role as a developer message; a missing role is a warning.
func verifyCodexRole(codexHome, thread, instructions string) (string, error) {
	path, err := codexRollout(codexHome, thread)
	if err != nil {
		return "", fmt.Errorf("role instructions could not be verified: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("role instructions could not be verified: %w", err)
	}
	defer file.Close()
	scan := bufio.NewScanner(file)
	scan.Buffer(make([]byte, 65536), 64*1024*1024)
	for scan.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if json.Unmarshal(scan.Bytes(), &entry) != nil || entry.Type != "response_item" || entry.Payload.Type != "message" || entry.Payload.Role != "developer" {
			continue
		}
		for _, part := range entry.Payload.Content {
			if part.Text == instructions {
				return "", nil
			}
		}
	}
	if err := scan.Err(); err != nil {
		return "", fmt.Errorf("role instructions could not be verified: %w", err)
	}
	return "WARNING: role instructions not found as developer message in rollout", nil
}

type piEvidence struct {
	thinking string
	efforts  []string
	roles    []bool
}

// readPiEvidence reads the thinking level in force and the observer records appended after offset.
func readPiEvidence(session string, offset int64) (piEvidence, error) {
	var evidence piEvidence
	file, err := os.Open(session)
	if err != nil {
		return evidence, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var position int64
	for {
		line, err := reader.ReadBytes('\n')
		start := position
		position += int64(len(line))
		if len(strings.TrimSpace(string(line))) > 0 {
			var entry struct {
				Type          string `json:"type"`
				ThinkingLevel string `json:"thinkingLevel"`
				CustomType    string `json:"customType"`
				Data          struct {
					Effort *string `json:"reasoning_effort"`
					Role   *bool   `json:"role_included"`
				} `json:"data"`
			}
			if jsonErr := json.Unmarshal(line, &entry); jsonErr != nil {
				return evidence, fmt.Errorf("invalid Pi session entry: %w", jsonErr)
			}
			switch {
			case entry.Type == "thinking_level_change":
				evidence.thinking = entry.ThinkingLevel
			case entry.Type == "custom" && entry.CustomType == piObserverEntry && start >= offset:
				effort := ""
				if entry.Data.Effort != nil {
					effort = *entry.Data.Effort
				}
				evidence.efforts = append(evidence.efforts, effort)
				if entry.Data.Role != nil {
					evidence.roles = append(evidence.roles, *entry.Data.Role)
				}
			}
		}
		if err == io.EOF {
			return evidence, nil
		}
		if err != nil {
			return evidence, err
		}
	}
}

// preparePiObserver writes the observer and the locked role snapshot next to the Pi session.
func preparePiObserver(a *ExternalSubagentState) ([]string, map[string]string, error) {
	dir := filepath.Dir(a.Session)
	observer := filepath.Join(dir, "agentx-observer.js")
	if err := os.WriteFile(observer, piObserver, 0o600); err != nil {
		return nil, nil, err
	}
	args := []string{"--extension", observer}
	env := map[string]string{}
	if a.Role.Instructions != "" {
		role := filepath.Join(dir, "role.md")
		if err := os.WriteFile(role, []byte(a.Role.Instructions), 0o600); err != nil {
			return nil, nil, err
		}
		args = append(args, "--append-system-prompt", role)
		env["AGENTX_PI_ROLE_FILE"] = role
	}
	if a.Option.Effort != "" {
		args = append(args, "--thinking", a.Option.Effort)
	}
	return args, env, nil
}

func verifyPi(a *ExternalSubagentState, offset int64) (string, error) {
	evidence, err := readPiEvidence(a.Session, offset)
	if err != nil {
		return "", fmt.Errorf("Pi session evidence unreadable: %w", err)
	}
	if len(evidence.efforts) == 0 {
		return "WARNING: the AgentX observer recorded no Pi provider request for this turn, so role and effort could not be verified", nil
	}
	var warnings []string
	if a.Option.Effort != "" && evidence.thinking != a.Option.Effort {
		warnings = append(warnings, fmt.Sprintf("WARNING: preset effort %s, but the Pi session records thinking level %q (Pi clamped it)", a.Option.Effort, evidence.thinking))
	}
	summary, err := verifyEffort(a.Option.Effort, evidence.efforts, "Pi provider requests recorded by the AgentX observer")
	if err != nil {
		return "", err
	}
	if a.Role.Instructions != "" {
		missing := len(evidence.roles) != len(evidence.efforts)
		for _, included := range evidence.roles {
			if !included {
				missing = true
			}
		}
		if missing {
			warnings = append(warnings, fmt.Sprintf("WARNING: role %s instructions missing or not recorded for every Pi provider request", a.Role.Name))
		} else {
			summary += fmt.Sprintf("; role instructions present in all %d provider request(s)", len(evidence.roles))
		}
	}
	return strings.Join(append([]string{summary}, warnings...), "; "), nil
}

func verifyCodex(a *ExternalSubagentState, stderr string) (string, error) {
	summary, err := verifyEffort(a.Option.Effort, codexExecutedEfforts(stderr), "Codex response.completed telemetry")
	if err != nil {
		return "", err
	}
	if a.Role.Instructions != "" {
		warning, err := verifyCodexRole(a.auth.Dir, a.Session, a.Role.Instructions)
		if err != nil {
			return "", err
		}
		if summary != "" {
			summary += "; "
		}
		if warning != "" {
			summary += warning
		} else {
			summary += "role instructions recorded as a Codex developer message"
		}
	}
	return summary, nil
}

// nativeFailure surfaces a failure the agent reported in its event stream (Codex puts a rejected
// effort's 400 there, not on stderr) when the process also exits non-zero.
func nativeFailure(a *ExternalSubagentState, stdout string) string {
	if strings.TrimSpace(stdout) == "" {
		return ""
	}
	if _, _, err := parseOutput(a, stdout); err != nil && strings.HasPrefix(err.Error(), "ExternalSubagent failed: ") {
		return "; native error: " + strings.TrimPrefix(err.Error(), "ExternalSubagent failed: ")
	}
	return ""
}
