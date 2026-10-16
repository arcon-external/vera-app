package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword hashes a password with Argon2id and returns the PHC-encoded
// string. The plaintext password is never stored.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash)), nil
}

// VerifyPassword reports whether the password matches the stored hash using a
// constant-time comparison.
func VerifyPassword(encodedHash, password string) error {
	salt, hash, memory, iterations, threads, err := decodeHash(encodedHash)
	if err != nil {
		return err
	}
	other := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(hash)))
	if subtle.ConstantTimeCompare(hash, other) != 1 {
		return fmt.Errorf("invalid email or password")
	}
	return nil
}

// NeedsRehash reports whether a stored hash uses different Argon2 parameters.
// An invalid hash cannot be verified and is treated as needing replacement.
func NeedsRehash(encoded string) bool {
	_, _, memory, iterations, threads, err := decodeHash(encoded)
	return err != nil || memory != argonMemory || iterations != argonTime || threads != argonThreads
}

func decodeHash(encoded string) (salt, hash []byte, memory, iterations uint32, threads uint8, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return nil, nil, 0, 0, 0, fmt.Errorf("unsupported password hash format")
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil || memory == 0 || iterations == 0 || threads == 0 || parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, threads) {
		return nil, nil, 0, 0, 0, fmt.Errorf("invalid password hash parameters")
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("decode password salt: %w", err)
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("decode password hash: %w", err)
	}
	if len(salt) == 0 || len(hash) == 0 {
		return nil, nil, 0, 0, 0, fmt.Errorf("invalid password hash encoding")
	}
	return salt, hash, memory, iterations, threads, nil
}
