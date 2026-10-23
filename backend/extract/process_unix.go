//go:build !windows

package extract

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the extractor in its own process group, so a
// timeout kills Poppler and anything Poppler started rather than only the
// direct child.
func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(command *exec.Cmd) {
	if command.Process == nil {
		return
	}
	// The negative pid addresses the group created by Setpgid.
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	_ = command.Process.Kill()
}
