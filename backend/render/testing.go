package render

import (
	"os"
	"testing"
)

// RequireUsable skips a test when the build selected a renderer this machine
// cannot run.
//
// The native engine works anywhere. The browser engine needs a Chrome
// installation, so a test that renders has two possible outcomes depending on
// how the binary was built -- and under `-tags chromedp` on a machine without a
// browser, the honest answer is to skip rather than to fail. A failure there
// reads as a broken test when the truth is a missing browser, which sends the
// reader looking in the wrong place.
//
// This lives beside the engines rather than in each test because every rendering
// test needs the same rule, and three copies of it would drift. It follows
// backend/auth/testing.go, which is the same idea: a test helper the production
// package owns because it knows the thing being tested.
//
// Set VERA_RUN_CHROME_TESTS=1 to opt in on a machine that has Chrome, matching
// the convention the renderer-comparison tests already use.
func RequireUsable(t *testing.T) {
	t.Helper()
	if Active.Name() != "chromedp" {
		return
	}
	if os.Getenv("VERA_RUN_CHROME_TESTS") == "" {
		t.Skip("this build carries the browser renderer and there is no Chrome to " +
			"drive; set VERA_RUN_CHROME_TESTS=1 on a machine that has one")
	}
}
