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

// TestKeyringPackagedOperationsUseProductionAccount checks Save/Load/Delete
// use the same constants, so a hardcoded operation cannot drift from them.
func TestKeyringPackagedOperationsUseProductionAccount(t *testing.T) {
	savedSet, savedGet, savedDelete := keyringSet, keyringGet, keyringDelete
	t.Cleanup(func() { keyringSet, keyringGet, keyringDelete = savedSet, savedGet, savedDelete })

	type call struct{ operation, service, account string }
	var calls []call
	record := func(operation, service, account string) {
		calls = append(calls, call{operation: operation, service: service, account: account})
	}
	keyringSet = func(service, account, secret string) error {
		record("set", service, account)
		return nil
	}
	keyringGet = func(service, account string) (string, error) {
		record("get", service, account)
		return "stored-token", nil
	}
	keyringDelete = func(service, account string) error {
		record("delete", service, account)
		return nil
	}

	if err := SaveSessionToken("token"); err != nil {
		t.Fatalf("SaveSessionToken error: %v", err)
	}
	if token, err := LoadSessionToken(); err != nil || token != "stored-token" {
		t.Fatalf("LoadSessionToken = %q, %v", token, err)
	}
	if err := DeleteSessionToken(); err != nil {
		t.Fatalf("DeleteSessionToken error: %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("keyring calls = %d; want 3 (set, get, delete)", len(calls))
	}
	for _, call := range calls {
		if call.service != "vera-app" || call.account != "session" {
			t.Fatalf("%s used %q/%q; want vera-app/session", call.operation, call.service, call.account)
		}
	}
}
