package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ArcheMind/agentx/internal/subagents"
)

// wait blocks until the ExternalSubagent's next event after --after exists, prints it and exits.
// The host arms exactly one wait per agent and re-arms it with the printed seq.
func (a App) wait(ctx context.Context, args []string) error {
	usage := fmt.Errorf("usage: sm wait <agent-id> --after <seq>")
	if len(args) == 0 {
		return usage
	}
	id := args[0]
	after := 0
	switch len(args) {
	case 1:
	case 3:
		if args[1] != "--after" {
			return usage
		}
		value, err := strconv.Atoi(args[2])
		if err != nil || value < 0 {
			return fmt.Errorf("--after requires a non-negative event seq")
		}
		after = value
	default:
		return usage
	}
	dir := subagents.AgentDir(a.presetHome(), id)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("unknown ExternalSubagent %s: %w", id, err)
	}
	for {
		event, err := subagents.NextEvent(dir, after)
		if err != nil {
			return err
		}
		if event != nil {
			fmt.Fprintf(a.Stdout, "SessionMan ExternalSubagent %s event seq=%d kind=%s\n%sRe-arm: sm wait %s --after %d\n", id, event.Seq, event.Kind, subagents.RenderEvent(event), id, event.Seq)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("ExternalSubagent %s was disposed while waiting", id)
		}
	}
}
