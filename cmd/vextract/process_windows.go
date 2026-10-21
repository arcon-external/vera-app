//go:build windows

package main

import "os/exec"

func configureProcessGroup(command *exec.Cmd) {}

func killProcessGroup(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
