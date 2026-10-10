package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// command is one node of the CLI's tree: a group like `vera clients`, or a leaf
// like `vera clients add`. stdlib flag has no notion of subcommands, so the
// tree, the dispatch and the help output are all here -- deliberately small,
// because the whole point of choosing stdlib over cobra was to keep the module
// lean, not to hand-roll a framework.
type command struct {
	name    string
	summary string
	usage   string // one line; shown in help and when the arguments are wrong
	flags   func(*flag.FlagSet)
	run     func(*app) error
	sub     []*command

	// parent is set when the tree is assembled, so help can print the full path
	// ("vera clients add") rather than just the leaf's own name.
	parent *command
}

// path is the command's full name, e.g. "vera clients add".
func (c *command) path() string {
	if c.parent == nil {
		return "vera"
	}
	return c.parent.path() + " " + c.name
}

// find walks the tree by name, returning the deepest match and the arguments
// left over for it.
//
// Flags may appear before the verb (`vera --json clients list`), so a
// flag-looking token is stepped over rather than treated as a command name.
// Which globals take a separate value has to be known to do that: `--api URL
// clients list` must not mistake the URL for a noun, while `--json clients list`
// must not swallow the noun as a value. Values are collected in the order they
// came so the leaf's FlagSet still sees them.
func (c *command) find(args []string) (*command, []string) {
	var flags []string
	rest := args
	for len(rest) > 0 {
		if width := leadingFlagWidth(rest); width > 0 {
			flags = append(flags, rest[:width]...)
			rest = rest[width:]
			continue
		}
		matched := false
		for _, child := range c.sub {
			if child.name == rest[0] {
				c = child
				rest = rest[1:]
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	return c, append(flags, rest...)
}

// globalValueFlags are the global flags that take a separate value, so the
// dispatcher can walk past one without mistaking its value for a command name.
// A bool flag spelled --flag=value is self-contained and needs no entry.
var globalValueFlags = map[string]bool{"api": true, "token": true, "timeout": true}

// leadingFlagWidth reports how many tokens the flag at the front of args
// occupies, or 0 when args[0] is not a flag at all.
func leadingFlagWidth(args []string) int {
	token := args[0]
	if token == "-" || token == "--" || !strings.HasPrefix(token, "-") {
		return 0
	}
	if strings.HasPrefix(token, "--") {
		name := token[2:]
		if strings.Contains(name, "=") {
			return 1
		}
		if globalValueFlags[name] && len(args) > 1 {
			return 2
		}
		return 1
	}
	// This CLI defines no single-dash flags of its own.
	return 1
}

// app carries one invocation: the parsed globals, the command being run, its
// positional arguments, and where to write.
type app struct {
	stdout io.Writer
	stderr io.Writer

	// Globals, accepted on every command rather than only before the verb, so
	// `vera clients list --json` and `vera --json clients list` both work.
	apiURL    string
	token     string
	asJSON    bool
	quiet     bool
	assumeYes bool
	insecure  bool
	timeout   time.Duration

	fs   *flag.FlagSet
	cmd  *command
	args []string

	// session is loaded once, lazily, and cached: several commands need it and
	// reading the file once per invocation is enough.
	session     *Session
	sessionErr  error
	sessionDone bool
}

// run is the whole dispatch: assemble the tree, find the command, parse, run,
// and turn the result into an exit code.
func (a *app) run(args []string) int {
	root := a.tree()

	// `vera` alone, or `vera help`, prints the command list.
	if len(args) == 0 {
		a.printHelp(root)
		return exitOK
	}
	if args[0] == "help" {
		target, rest := root.find(args[1:])
		// find leaves behind anything it could not match, which for `help` is
		// exactly the part of the name that does not exist.
		if len(rest) > 0 {
			fmt.Fprintf(a.stderr, "vera: no such command %q\n", strings.Join(rest, " "))
			fmt.Fprintf(a.stderr, "run 'vera' for the command list\n")
			return exitUsage
		}
		a.printHelp(target)
		return exitOK
	}

	cmd, rest := root.find(args)
	if cmd == root && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		fmt.Fprintf(a.stderr, "vera: unknown command %q\n", rest[0])
		fmt.Fprintf(a.stderr, "run 'vera' for the command list, or 'vera help <command>'\n")
		return exitUsage
	}

	fs := flag.NewFlagSet(cmd.path(), flag.ContinueOnError)
	// flag's own output would print a bare usage block; the error text below is
	// better, so swallow its chatter and report it ourselves.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	a.registerGlobals(fs)
	if cmd.flags != nil {
		cmd.flags(fs)
	}
	a.fs = fs
	a.cmd = cmd

	positional, err := parseInterspersed(fs, rest)
	if err != nil {
		return a.fail(cmd, err)
	}
	a.args = positional

	if cmd.run == nil {
		// A group with no action of its own.
		if len(positional) > 0 {
			// Someone typed a verb this group does not have. Listing what it
			// does have is more useful than reprinting the whole help.
			fmt.Fprintf(a.stderr, "vera: %q is not a command under %q\n", positional[0], cmd.path())
			fmt.Fprintf(a.stderr, "available: %s\n", childNames(cmd))
			return exitUsage
		}
		a.printHelp(cmd)
		return exitOK
	}

	if err := cmd.run(a); err != nil {
		return a.fail(cmd, err)
	}
	return exitOK
}

// registerGlobals puts the flags every command accepts onto its FlagSet.
func (a *app) registerGlobals(fs *flag.FlagSet) {
	fs.StringVar(&a.apiURL, "api", "", "API base URL (default: the signed-in server, VERA_API_URL, or production)")
	fs.StringVar(&a.token, "token", "", "session token (default: the stored session, or VERA_TOKEN)")
	fs.BoolVar(&a.asJSON, "json", false, "print machine-readable JSON")
	fs.BoolVar(&a.quiet, "quiet", false, "print nothing on success")
	fs.BoolVar(&a.assumeYes, "yes", false, "do not prompt for confirmation")
	fs.BoolVar(&a.insecure, "insecure", false, "allow sending credentials over plain http to a remote host")
	fs.DurationVar(&a.timeout, "timeout", 60*time.Second, "per-request timeout")
}

// parseInterspersed parses flags that appear before, after, or between
// positional arguments.
//
// flag.FlagSet.Parse stops at the first non-flag token, which would make
// `vera quotations show 12 --revision 1` silently drop --revision and treat it
// as an item number. Parsing the remainder after each positional is the
// standard way round it. Everything after a bare `--` is positional, as usual.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		if rest[0] == "--" {
			return append(positional, rest[1:]...), nil
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
}

// fail prints an error the way the exit code says it should be read, and
// returns that code.
func (a *app) fail(cmd *command, err error) int {
	var usage *usageError
	if errors.As(err, &usage) {
		fmt.Fprintf(a.stderr, "vera: %s\n", usage.message)
		if usage.command != nil {
			fmt.Fprintf(a.stderr, "usage: %s\n", usage.command.usage)
			fmt.Fprintf(a.stderr, "run 'vera help %s'\n", strings.TrimPrefix(usage.command.path(), "vera "))
		}
		return exitUsage
	}

	code := exitCodeFor(err)
	fmt.Fprintf(a.stderr, "vera: %s\n", err)
	return code
}

// printHelp writes the command list, or one command's detail.
func (a *app) printHelp(cmd *command) {
	if len(cmd.sub) == 0 {
		a.printCommandHelp(cmd)
		return
	}

	if cmd.usage != "" {
		fmt.Fprintf(a.stdout, "%s\n\n", cmd.usage)
	}
	if cmd.summary != "" {
		fmt.Fprintf(a.stdout, "%s\n\n", cmd.summary)
	}

	if cmd.run != nil || cmd.usage != "" {
		fmt.Fprintf(a.stdout, "Usage:\n  %s\n\n", usageLine(cmd))
	}

	fmt.Fprintf(a.stdout, "Commands:\n")
	width := 0
	for _, child := range cmd.sub {
		if len(child.name) > width {
			width = len(child.name)
		}
	}
	for _, child := range cmd.sub {
		fmt.Fprintf(a.stdout, "  %-*s  %s\n", width, child.name, child.summary)
	}

	if cmd.parent == nil {
		fmt.Fprintf(a.stdout, `
Global flags (accepted by every command, before or after the verb):
  --api URL      API base URL
  --json         machine-readable output
  --quiet        print nothing on success
  --yes          skip confirmation prompts
  --timeout D    per-request timeout (default 60s)

Run 'vera help <command>' for one command, e.g. 'vera help quotations export'.
`)
	}
}

func (a *app) printCommandHelp(cmd *command) {
	if cmd.summary != "" {
		fmt.Fprintf(a.stdout, "%s\n\n", cmd.summary)
	}
	fmt.Fprintf(a.stdout, "Usage:\n  %s\n", usageLine(cmd))
	if cmd.flags != nil {
		fs := flag.NewFlagSet(cmd.path(), flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		cmd.flags(fs)
		var lines []string
		fs.VisitAll(func(f *flag.Flag) {
			lines = append(lines, fmt.Sprintf("  --%-14s %s", f.Name, f.Usage))
		})
		if len(lines) > 0 {
			fmt.Fprintf(a.stdout, "\nFlags:\n%s\n", strings.Join(lines, "\n"))
		}
	}
}

// childNames lists a group's verbs for the "not a command under" message.
func childNames(c *command) string {
	names := make([]string, 0, len(c.sub))
	for _, child := range c.sub {
		names = append(names, child.name)
	}
	return strings.Join(names, ", ")
}

// usageLine is the command's own usage if it declared one, else its path.
func usageLine(cmd *command) string {
	if cmd.usage != "" {
		return cmd.usage
	}
	return cmd.path()
}
