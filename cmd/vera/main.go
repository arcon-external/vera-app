// Command vera is the command-line interface to Vera.
//
// It talks to veraserver over HTTP exactly as the desktop does -- the same
// apiclient, the same routes, the same errors -- and it imports the application
// itself so it can do the two things a pure HTTP client cannot: validate a
// quotation's pricing and render its PDF. Rendering is pure Go (backend/render
// uses gpdf), so this works on a machine with no browser and no display.
//
// That is what makes it end to end: `vera quotations export` prepares, stores a
// signed revision, renders the document the customer receives, attaches it, and
// the result verifies through the same public portal a customer would use.
//
// Run `vera help` for the command list, or `vera help <command>` for one.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

// Run dispatches one invocation and returns the process exit code.
//
// Kept separate from main, and against io.Writer rather than *os.File, so tests
// can drive the whole CLI in-process and read what it printed instead of
// spawning a binary and scraping stdout.
func Run(args []string, stdout, stderr io.Writer) int {
	return newApp(stdout, stderr).run(args)
}

// Exit codes. Scripts and tests branch on these rather than on the wording of a
// message, so the numbers are part of the interface: keep them stable.
const (
	exitOK         = 0
	exitError      = 1 // something went wrong that fits nothing below
	exitUsage      = 2 // the command line itself was wrong
	exitAuth       = 3 // signed out, or not allowed to do this
	exitNotFound   = 4 // the thing named does not exist
	exitValidation = 5 // the server refused the input
	exitServer     = 6 // the server or the network failed
)

// usageError marks a mistake in how the command was called, so the dispatcher
// can print the usage line and return exit 2 rather than treating it as a
// runtime failure.
type usageError struct {
	message string
	command *command
}

func (e *usageError) Error() string { return e.message }

func usagef(cmd *command, format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...), command: cmd}
}
