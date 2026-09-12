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

// SetSessionToken remembers a session token in memory and the OS keychain.
func SetSessionToken(token string) error {
	sessionMu.Lock()
	sessionToken = token
	sessionMu.Unlock()
	if os.Getenv("VERA_NO_KEYRING") == "1" {
		return nil
	}
	return auth.SaveSessionToken(token)
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
	if os.Getenv("VERA_NO_KEYRING") == "1" {
		return ""
	}
	stored, err := auth.LoadSessionToken()
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
	sessionMu.Lock()
	sessionToken = ""
	sessionMu.Unlock()
	if os.Getenv("VERA_NO_KEYRING") == "1" {
		return nil
	}
	return auth.DeleteSessionToken()
}
