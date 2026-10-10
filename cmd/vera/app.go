package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"

	"vera-app/backend/apiclient"
)

// productionAPI is where staff actually work: veraserver on Cloud Run, backed by
// Neon. It is the default so that a bare `vera quotations list` shows real data
// rather than failing for want of configuration -- and --api points elsewhere
// when you want the local stack.
const productionAPI = "https://api.arconpentapersada.co.id"

func newApp(stdout, stderr io.Writer) *app {
	return &app{stdout: stdout, stderr: stderr}
}

// API resolution, in order: --api, VERA_API_URL, whatever the stored session was
// signed in against, then production. The session is in the chain because a
// token belongs to one server: silently sending it to a different one would
// produce a confusing "signed out" rather than an obvious mistake.
func (a *app) apiBase() string {
	if a.apiURL != "" {
		return strings.TrimRight(a.apiURL, "/")
	}
	if env := strings.TrimSpace(os.Getenv("VERA_API_URL")); env != "" {
		return strings.TrimRight(env, "/")
	}
	if s := a.loadSession(); s != nil && s.API != "" {
		return strings.TrimRight(s.API, "/")
	}
	return productionAPI
}

// token resolution, in order: --token, VERA_TOKEN, the stored session.
func (a *app) tokenFor(base string) string {
	if a.token != "" {
		return a.token
	}
	if env := strings.TrimSpace(os.Getenv("VERA_TOKEN")); env != "" {
		return env
	}
	s := a.loadSession()
	if s == nil || s.Token == "" {
		return ""
	}
	// A session token is only meaningful to the server that issued it.
	if s.API != "" && strings.TrimRight(s.API, "/") != strings.TrimRight(base, "/") {
		return ""
	}
	return s.Token
}

// newClient builds an API client and reports the token it will carry. Every
// command goes through here, so none of them can bypass the auth-resolution
// rules above.
func (a *app) newClient() (*apiclient.Client, string, error) {
	base := a.apiBase()

	// A password or a bearer token crossing the open internet in clear is never
	// acceptable; over loopback it is ordinary local development.
	if err := checkTransport(base, a.insecure); err != nil {
		return nil, "", err
	}

	token := a.tokenFor(base)
	c := apiclient.New(base, func() string { return token })
	c.SetTimeout(a.timeout)
	return c, token, nil
}

// client builds a client for a route that needs a session, and refuses here
// when there is none.
//
// The server would answer 401 "missing session" anyway, but a round trip to
// learn something this machine already knows is a poor way to say it -- and on a
// slow connection it is the difference between an instant answer and a pause.
// Every unrelated problem reported as "signed out" is also how a session bug
// hides for a week.
func (a *app) client() (*apiclient.Client, error) {
	c, token, err := a.newClient()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errNotSignedIn
	}
	return c, nil
}

// anonClient builds a client for the routes that take no session at all:
// update checks, the changelog, signing in, redeeming an invite, and the public
// verification portal.
func (a *app) anonClient() (*apiclient.Client, error) {
	c, _, err := a.newClient()
	return c, err
}

func checkTransport(base string, insecure bool) error {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return usagef(nil, "%q is not a usable API URL", base)
	}
	if parsed.Scheme == "https" || insecure {
		return nil
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	return fmt.Errorf("%s is plain http on a remote host, so a password or token would cross the network in clear; use https or pass --insecure if you know the network is trusted", base)
}

// requireSession returns the stored session, or an error naming the fix.
func (a *app) requireSession() (*Session, error) {
	s := a.loadSession()
	if s == nil || s.Token == "" {
		return nil, errNotSignedIn
	}
	return s, nil
}

// errNotSignedIn is deliberately a plain error: the exit code for "signed out"
// comes from the message being recognisable, not from an HTTP status.
var errNotSignedIn = errors.New("not signed in; run 'vera login'")

// loadSession reads the session file once per invocation.
func (a *app) loadSession() *Session {
	if a.sessionDone {
		return a.session
	}
	a.sessionDone = true
	a.session, a.sessionErr = LoadSession()
	if a.sessionErr != nil {
		a.session = nil
	}
	return a.session
}

// exitCodeFor maps an error to the documented exit code.
func exitCodeFor(err error) int {
	// A command may already know better than the status does: `vera verify`
	// distinguishes superseded from forged, and both arrive as a 200.
	var explicit *exitCoder
	if errors.As(err, &explicit) {
		return explicit.code
	}
	switch status := apiclient.StatusOf(err); {
	case status == 401 || status == 403:
		return exitAuth
	case status == 404:
		return exitNotFound
	case status == 400 || status == 422:
		return exitValidation
	case status >= 500:
		return exitServer
	}
	if errors.Is(err, errNotSignedIn) {
		return exitAuth
	}
	// A network failure carries no status: the server was never reached.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return exitServer
	}
	return exitError
}

// confirm asks before something destructive, unless --yes was given.
//
// With no terminal to ask on -- a script or a pipe -- it refuses rather than
// assuming yes: an unattended run should have to say --yes explicitly.
func (a *app) confirm(format string, args ...any) error {
	if a.assumeYes {
		return nil
	}
	if !isTerminal(os.Stdin) {
		return usagef(a.cmd, "%s (pass --yes to confirm in a non-interactive run)", fmt.Sprintf(format, args...))
	}
	fmt.Fprintf(a.stderr, "%s [y/N] ", fmt.Sprintf(format, args...))
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return errors.New("cancelled")
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// readPassword reads a password from stdin when the caller asked to pipe one in.
//
// A flag would put the password in the process list and the shell history, and
// an interactive prompt cannot be automated; a pipe is the one form that is
// neither. `--password-stdin` is therefore the documented way for scripts.
func readPassword() (string, error) {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
	if err != nil {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	password := strings.TrimRight(string(data), "\r\n")
	if password == "" {
		return "", errors.New("no password on stdin")
	}
	return password, nil
}

// parseID reads a numeric argument, naming the command on failure.
func (a *app) parseID(what, value string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(value, "%d", &id); err != nil || id <= 0 {
		return 0, usagef(a.cmd, "%s must be a positive number, got %q", what, value)
	}
	return id, nil
}

// requireArgs checks the positional argument count.
func (a *app) requireArgs(count int, form string) error {
	if len(a.args) == count {
		return nil
	}
	if count == 0 {
		return usagef(a.cmd, "%s takes no arguments, got %d", a.cmd.path(), len(a.args))
	}
	return usagef(a.cmd, "expected %s", form)
}
