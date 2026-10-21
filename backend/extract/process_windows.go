//go:build windows

package extract

import "os/exec"

// The API server runs on Linux. Keeping a Windows build compilable means the
// repository can still cross-compile its desktop and CLI targets; the timeout
// kills the direct child there, which is what Go supports without Job objects.
func configureProcessGroup(command *exec.Cmd) {}

func killProcessGroup(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
