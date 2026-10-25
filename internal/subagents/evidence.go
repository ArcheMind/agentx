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

// verifyEffort reports whether the native model requests of the turn ran at the profile effort.
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
		summary += fmt.Sprintf("; WARNING: profile requested %s but native executed %s", requested, strings.Join(mismatched, ", "))
	}
	return summary, nil
}

type piEvidence struct {
	thinking string
	efforts  []string
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

// preparePiObserver writes the observer extension next to the Pi session.
func preparePiObserver(a *ExternalSubagentState) ([]string, map[string]string, error) {
	dir := filepath.Dir(a.Session)
	observer := filepath.Join(dir, "agentx-observer.js")
	if err := os.WriteFile(observer, piObserver, 0o600); err != nil {
		return nil, nil, err
	}
	args := []string{"--extension", observer}
	env := map[string]string{}
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
		return "WARNING: the AgentX observer recorded no Pi provider request for this turn, so effort could not be verified", nil
	}
	var warnings []string
	if a.Option.Effort != "" && evidence.thinking != a.Option.Effort {
		warnings = append(warnings, fmt.Sprintf("WARNING: profile effort %s, but the Pi session records thinking level %q (Pi clamped it)", a.Option.Effort, evidence.thinking))
	}
	summary, err := verifyEffort(a.Option.Effort, evidence.efforts, "Pi provider requests recorded by the AgentX observer")
	if err != nil {
		return "", err
	}
	return strings.Join(append([]string{summary}, warnings...), "; "), nil
}

func verifyCodex(a *ExternalSubagentState, stderr string) (string, error) {
	return verifyEffort(a.Option.Effort, codexExecutedEfforts(stderr), "Codex response.completed telemetry")
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
