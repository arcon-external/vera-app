package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

const (
	// IdleTimeout signs the user out after inactivity (e.g. overnight).
	IdleTimeout = 8 * time.Hour
	// AbsoluteLifetime bounds a session even with continuous use.
	AbsoluteLifetime = 7 * 24 * time.Hour
	tokenBytes       = 32
)

// NewSessionToken generates a random session token and returns the token
// (given to the client) plus its SHA-256 hash (stored server-side).
func NewSessionToken() (token, hash string, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashSessionToken(token), nil
}

// HashSessionToken hashes a token for storage or lookup.
func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// IsSessionExpired reports whether the expiry time has passed.
func IsSessionExpired(expiresAt, now time.Time) bool {
	return !expiresAt.After(now)
}
