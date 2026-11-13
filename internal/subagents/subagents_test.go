package subagents

import (
	"github.com/ArcheMind/agentx/internal/agentparams"
	"strings"
	"testing"
)

func TestExternalPlansUseNativeReadOnlyAndExactResume(t *testing.T) {
	id := "0199a213-81c0-7800-8aa1-bbab2a035a53"
	for _, item := range []struct{ agent, policy, resume string }{
		{"pi", "--print --mode json", "--session " + id},
	} {
		plan, err := agentparams.Build(item.agent, agentparams.Exec, agentparams.Request{Cwd: "/child", Model: "selected-model", Session: id})
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Join(plan.Args, " ")
		if !strings.Contains(args, item.policy) || !strings.Contains(args, item.resume) || !strings.Contains(args, "--model selected-model") {
			t.Fatalf("unexpected plan: %#v", plan)
		}
		for _, invalid := range []string{"latest", "--continue", "partial-id"} {
			if _, err := agentparams.Build(item.agent, agentparams.Exec, agentparams.Request{Cwd: "/child", Session: invalid}); err == nil {
				t.Fatalf("accepted %s resume %s", item.agent, invalid)
			}
		}
	}
	for _, unsupported := range []string{"dsh", "opencode", "unknown"} {
		if _, err := agentparams.Build(unsupported, agentparams.Exec, agentparams.Request{Cwd: "/child"}); err == nil {
			t.Fatalf("accepted unsupported %s", unsupported)
		}
	}
}
