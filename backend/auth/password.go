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
	argonTime    = 1
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
	salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return err
	}
	other := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, uint32(len(hash)))
	if subtle.ConstantTimeCompare(hash, other) != 1 {
		return fmt.Errorf("invalid email or password")
	}
	return nil
}

func decodeHash(encoded string) (salt, hash []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, fmt.Errorf("unsupported password hash format")
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, fmt.Errorf("decode password salt: %w", err)
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, fmt.Errorf("decode password hash: %w", err)
	}
	return salt, hash, nil
}
