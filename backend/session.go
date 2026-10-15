package backend

import (
	"os"
	"sync"

	"vera-app/backend/auth"
)

// Session token storage for the desktop client. The token lives in memory
// and is mirrored to the OS keychain so sessions survive restarts. Tests set
// VERA_NO_KEYRING=1 to keep everything in memory.
var (
	sessionMu    sync.RWMutex
	sessionToken string
)

// The OS-keychain calls this file makes, indirected through package-level
// vars so a test can swap them for recorders. Which of them ran is the whole
// difference between a remembered sign-in and an unremembered one — a
// remembered login writes the keychain, an unremembered one must delete what
// an earlier remembered login left there — and a CI machine has no keychain
// to ask.
var (
	keyringSave   = auth.SaveSessionToken
	keyringLoad   = auth.LoadSessionToken
	keyringDelete = auth.DeleteSessionToken
)

// keyringDisabled reports whether VERA_NO_KEYRING=1 keeps every path in
// memory, as it does on a machine with no keychain.
func keyringDisabled() bool {
	return os.Getenv("VERA_NO_KEYRING") == "1"
}

// SetSessionToken remembers a session token in memory and the OS keychain,
// so the session survives a restart. This is the remembered path: the user
// asked to stay signed in, so the token earns a home beyond this process.
func SetSessionToken(token string) error {
	setSessionTokenMemory(token)
	if keyringDisabled() {
		return nil
	}
	return keyringSave(token)
}

// setSessionTokenMemory stores the token for this process only: no keychain
// write, and no keychain delete either. It is the storage half of an
// unremembered sign-in, and the caller must pair it with
// forgetKeyringToken — on its own it leaves any previously remembered
// token sitting in the keychain, where the next launch's SessionToken
// fallback would restore it.
func setSessionTokenMemory(token string) {
	sessionMu.Lock()
	sessionToken = token
	sessionMu.Unlock()
}

// forgetKeyringToken deletes whatever token the OS keychain still holds, so
// the keychain cannot outlive the session it belongs to.
// VERA_NO_KEYRING=1 keeps everything memory-only, so there is nothing there.
func forgetKeyringToken() error {
	if keyringDisabled() {
		return nil
	}
	return keyringDelete()
}

// SessionToken returns the in-memory session token, falling back to the OS
// keychain (which also repopulates memory).
func SessionToken() string {
	sessionMu.RLock()
	token := sessionToken
	sessionMu.RUnlock()
	if token != "" {
		return token
	}
	if keyringDisabled() {
		return ""
	}
	stored, err := keyringLoad()
	if err != nil || stored == "" {
		return ""
	}
	sessionMu.Lock()
	sessionToken = stored
	sessionMu.Unlock()
	return stored
}

// ClearSessionToken forgets the session in memory and the OS keychain.
func ClearSessionToken() error {
	setSessionTokenMemory("")
	if keyringDisabled() {
		return nil
	}
	return keyringDelete()
}
