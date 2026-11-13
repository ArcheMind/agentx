package runtime

import (
	"os/exec"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {
	configureOwnProcessGroup(cmd)
}

func configureOwnProcessGroup(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func Detach(cmd *exec.Cmd) {}
