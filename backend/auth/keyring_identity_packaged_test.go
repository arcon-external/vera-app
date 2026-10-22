//go:build !dev || production

package auth

import "testing"

// TestKeyringIdentityPackaged pins the installed app's remembered sign-in:
// these exact bytes are what every existing production keychain item lives
// under, so changing either constant silently signs every staff member out.
func TestKeyringIdentityPackaged(t *testing.T) {
	if keyringService != "vera-app" {
		t.Fatalf("keyringService = %q; want \"vera-app\"", keyringService)
	}
	if keyringAccount != "session" {
		t.Fatalf("keyringAccount = %q; want \"session\"", keyringAccount)
	}
}
