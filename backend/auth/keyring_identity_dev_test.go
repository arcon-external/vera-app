//go:build dev && !production

package auth

import "testing"

// TestKeyringIdentityDev proves a development build cannot read or overwrite
// the installed app's remembered token: same service, different account.
func TestKeyringIdentityDev(t *testing.T) {
	if keyringService != "vera-app" {
		t.Fatalf("keyringService = %q; want \"vera-app\"", keyringService)
	}
	if keyringAccount != "session.dev" {
		t.Fatalf("keyringAccount = %q; want \"session.dev\"", keyringAccount)
	}
	if keyringAccount == "session" {
		t.Fatal("development build still shares the production keychain account")
	}
}
