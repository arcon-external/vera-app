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

// TestKeyringDevOperationsUseDevAccount checks the operations, not just the
// constants: a hardcoded production account in Save/Load/Delete must fail.
func TestKeyringDevOperationsUseDevAccount(t *testing.T) {
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
		if call.service != "vera-app" || call.account != "session.dev" {
			t.Fatalf("%s used %q/%q; want vera-app/session.dev", call.operation, call.service, call.account)
		}
	}
}
