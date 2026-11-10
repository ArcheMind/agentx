package subagents

import (
	"github.com/ArcheMind/agentx/internal/drivers"
	"strings"
	"testing"
)

func TestExternalPlansUseNativeReadOnlyAndExactResume(t *testing.T) {
	id := "0199a213-81c0-7800-8aa1-bbab2a035a53"
	for _, item := range []struct{ agent, policy, resume string }{
		{"pi", "--print --mode json", "--session " + id},
	} {
		plan, err := drivers.ExecPlan(item.agent, "/child", "selected-model", id)
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Join(plan.Args, " ")
		if plan.Executable != item.agent || plan.Cwd != "/child" || !strings.Contains(args, item.policy) || !strings.Contains(args, item.resume) || !strings.Contains(args, "--model selected-model") {
			t.Fatalf("unexpected plan: %#v", plan)
		}
		for _, invalid := range []string{"latest", "--continue", "partial-id"} {
			if _, err := drivers.ExecPlan(item.agent, "/child", "", invalid); err == nil {
				t.Fatalf("accepted %s resume %s", item.agent, invalid)
			}
		}
	}
	for _, unsupported := range []string{"dsh", "opencode", "unknown"} {
		if _, err := drivers.ExecPlan(unsupported, "/child", "", ""); err == nil {
			t.Fatalf("accepted unsupported %s", unsupported)
		}
	}
}
