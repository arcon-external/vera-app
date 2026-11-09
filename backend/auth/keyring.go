package auth

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

// The OS-keychain calls are indirected through package vars so a test can
// record which service/account every operation actually uses. A test that only
// compared the constants would miss a hardcoded call.
var (
	keyringSet    = keyring.Set
	keyringGet    = keyring.Get
	keyringDelete = keyring.Delete
)

// SaveSessionToken persists the session token in the OS keychain.
func SaveSessionToken(token string) error {
	if err := keyringSet(keyringService, keyringAccount, token); err != nil {
		return fmt.Errorf("save session token: %w", err)
	}
	return nil
}

// LoadSessionToken reads the session token from the OS keychain.
// It returns an empty string when no session is stored.
func LoadSessionToken() (string, error) {
	token, err := keyringGet(keyringService, keyringAccount)
	if err != nil {
		if err == keyring.ErrNotFound {
			return "", nil
		}
		return "", fmt.Errorf("load session token: %w", err)
	}
	return token, nil
}

// DeleteSessionToken removes the session token from the OS keychain.
func DeleteSessionToken() error {
	if err := keyringDelete(keyringService, keyringAccount); err != nil {
		if err == keyring.ErrNotFound {
			return nil
		}
		return fmt.Errorf("delete session token: %w", err)
	}
	return nil
}
