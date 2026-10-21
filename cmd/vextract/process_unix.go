//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup puts pdftotext in its own process group so a timeout
// kills a child it started (font helpers, for example), not only the direct
// process.
func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(command *exec.Cmd) {
	if command.Process == nil {
		return
	}
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	_ = command.Process.Kill()
}
