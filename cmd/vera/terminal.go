package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// stdin is the process's standard input.
//
// A function rather than a package variable holding os.Stdin, because a test
// that drives `vera login --password-stdin` has to be able to replace it, and a
// snapshot taken at init time cannot be replaced.
func stdin() *os.File { return os.Stdin }

// readPasswordMasked reads a line with the terminal's echo turned off.
//
// golang.org/x/term rather than a hand-rolled stty call: stty does not exist on
// Windows, and the CLI ships there. The module was already in the build graph as
// an indirect dependency, so this adds no new module to the build.
func readPasswordMasked(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	fmt.Fprint(os.Stderr, prompt)
	data, err := term.ReadPassword(fd)
	// The newline the user typed is swallowed with the echo, so add it back or
	// the next line of output lands on the prompt.
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no password entered")
	}
	return string(data), nil
}
