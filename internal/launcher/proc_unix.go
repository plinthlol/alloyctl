//go:build !windows

package launcher

import (
	"os/exec"
	"os/signal"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func ignoreSIGHUP() {
	signal.Ignore(syscall.SIGHUP)
}
