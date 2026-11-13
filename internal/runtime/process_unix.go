//go:build !windows

package runtime

import (
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {
	configureOwnProcessGroup(cmd)
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

func configureOwnProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// Detach puts a long-lived child into its own process group so host signals to the parent's
// group do not take it down; its own supervision decides when it dies.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Native tools may create their own process groups, so group cancellation alone leaves them alive.
func killProcessTree(cmd *exec.Cmd, debug bool, log io.Writer) {
	if cmd.Process == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := (ExecRunner{Debug: debug, Log: log}).Execute(ctx, CommandPlan{Executable: "ps", Args: []string{"-axo", "pid=,ppid="}}, ExecuteOptions{})
	if err == nil {
		children := map[int][]int{}
		for _, line := range strings.Split(result.Stdout, "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			pid, e1 := strconv.Atoi(fields[0])
			ppid, e2 := strconv.Atoi(fields[1])
			if e1 == nil && e2 == nil {
				children[ppid] = append(children[ppid], pid)
			}
		}
		var killChildren func(int)
		killChildren = func(parent int) {
			for _, pid := range children[parent] {
				killChildren(pid)
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		killChildren(cmd.Process.Pid)
	}
	killProcessGroup(cmd)
}
