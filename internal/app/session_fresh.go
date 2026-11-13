package app

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArcheMind/agentx/internal/sessions"
)

const (
	freshTokenThreshold = 250_000
	freshHeadUsers      = 2
	freshTailUsers      = 3
)

// estimateContextTokens approximates resume context as text bytes / 4; only
// the threshold decision needs it, not an exact count.
func estimateContextTokens(messages []sessions.Message) int {
	total := 0
	for _, message := range messages {
		for _, block := range message.Content {
			total += len(block.Text)
		}
		switch result := message.Result.(type) {
		case nil:
		case string:
			total += len(result)
		default:
			if encoded, err := json.Marshal(result); err == nil {
				total += len(encoded)
			}
		}
	}
	return total / 4
}

func exceedsFreshThreshold(messages []sessions.Message) (int, bool) {
	tokens := estimateContextTokens(messages)
	return tokens, tokens > freshTokenThreshold
}

// freshHeadEnd returns the index just past the last message belonging to the
// first n user turns (each user message plus its following non-user messages).
func freshHeadEnd(messages []sessions.Message, n int) int {
	seen := 0
	end := 0
	for i, m := range messages {
		if m.Role == "user" {
			seen++
			if seen > n {
				return end
			}
		}
		end = i + 1
	}
	return end
}

// freshTailStart is the index of the freshTailUsers-th user message from the
// end; with fewer user messages the whole transcript is kept.
func freshTailStart(messages []sessions.Message) int {
	remaining := freshTailUsers
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			remaining--
			if remaining == 0 {
				return i
			}
		}
	}
	return 0
}

// offloadToolResults keeps every tool entry (tool_use/tool_result pairing
// stays intact) but replaces its result with a stub pointing at the full
// history file.
func offloadToolResults(messages []sessions.Message, historyPath string) []sessions.Message {
	stub := "[offloaded: " + historyPath + "]"
	out := make([]sessions.Message, len(messages))
	for i, message := range messages {
		if message.Role == "tool" {
			message.Result = stub
		}
		out[i] = message
	}
	return out
}

func freshHistoryMessage(historyPath string) sessions.Message {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return sessions.Message{
		ID:   fmt.Sprintf("fresh-%x", buf),
		Role: "user",
		Content: []sessions.ContentBlock{{
			Type: "text",
			Text: fmt.Sprintf("Full conversation history: %s — read it when earlier context is needed.", historyPath),
		}},
	}
}

// freshSessionDetail offloads the full transcript to a history file and keeps
// only the recent window, with tool results stubbed and a pointer to the
// history injected up front.
func (a App) freshSessionDetail(detail sessions.Session) (sessions.Session, error) {
	dir := filepath.Join(a.Sessions.HomeDir, ".local", "state", "sessionman", "history")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return sessions.Session{}, fmt.Errorf("create history directory: %w", err)
	}
	id := detail.ID
	if id == "" {
		id = "session"
	}
	historyPath := filepath.Join(dir, strings.ReplaceAll(id, string(os.PathSeparator), "-")+".json")
	file, err := os.OpenFile(historyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return sessions.Session{}, fmt.Errorf("create history file: %w", err)
	}
	err = writeStructured(file, detail, OutputJSON)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return sessions.Session{}, fmt.Errorf("write history file: %w", err)
	}

	headEnd := freshHeadEnd(detail.Messages, freshHeadUsers)
	tailStart := freshTailStart(detail.Messages)
	if tailStart < headEnd {
		tailStart = headEnd
	}
	head := offloadToolResults(detail.Messages[:headEnd], historyPath)
	tail := offloadToolResults(detail.Messages[tailStart:], historyPath)
	messages := make([]sessions.Message, 0, len(head)+1+len(tail))
	messages = append(messages, head...)
	messages = append(messages, freshHistoryMessage(historyPath))
	messages = append(messages, tail...)
	detail.Messages = sessions.SanitizeMessageIDs(sessions.SanitizeToolIDs(sessions.RechainMessages(fillTimestamps(messages))))
	detail.MessageCount = len(detail.Messages)
	return detail, nil
}

// confirmLargeContext returns selectionFresh, selectionConfirm (full load) or
// selectionCancel.
func (a App) confirmLargeContext(reader *bufio.Reader, tokens int) (selectionKey, error) {
	fmt.Fprintf(a.Stdout, "Context ~%dK tokens — [f] fresh  [Enter] full  [q] cancel", tokens/1000)
	restore, err := makeRaw(a.Stdin)
	if err != nil {
		return selectionCancel, err
	}
	defer restore()
	for {
		key, err := readSelectionKey(reader)
		if err != nil {
			return selectionCancel, err
		}
		switch key {
		case selectionFresh, selectionConfirm, selectionCancel:
			fmt.Fprint(a.Stdout, "\r\n")
			return key, nil
		}
	}
}
