package invariant

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
)

const moduleRoot = "agentx/"

// Equal returns nil when actual matches expected, otherwise an error whose text
// is a self-contained prompt a user can paste to a coding agent.
func Equal(name string, actual, expected, context any) error {
	if reflect.DeepEqual(actual, expected) {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SessionMan internal invariant violated. Copy this whole message to a coding agent on this machine.\n\n")
	fmt.Fprintf(&b, "Invariant: %s\n", name)
	fmt.Fprintf(&b, "Location:  %s\n", callerLocation())
	fmt.Fprintf(&b, "Command:   %s\n", strings.Join(os.Args, " "))
	fmt.Fprintf(&b, "Actual:\n%s\n", render(actual))
	fmt.Fprintf(&b, "Expected:\n%s\n", render(expected))
	fmt.Fprintf(&b, "Context:\n%s\n\n", render(context))
	b.WriteString("Task: run `sm version` to get the installed SessionMan version, read the files referenced above on this machine, find the root cause using the SessionMan source at the matching tag (https://github.com/ArcheMind/agentx), then file an issue at https://github.com/ArcheMind/agentx/issues with the evidence and root cause.")
	return errors.New(b.String())
}

func callerLocation() string {
	// Skip Equal itself: the violation belongs to Equal's caller.
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	if i := strings.LastIndex(file, moduleRoot); i >= 0 {
		file = file[i+len(moduleRoot):]
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func render(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return string(data)
}
