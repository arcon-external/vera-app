package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

func oldParameterHash(password string) string {
	salt := []byte("fixed-test-salt!")
	hash := argon2.IDKey([]byte(password), salt, 1, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=1,p=%d$%s$%s", argon2.Version, argonMemory, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}

func TestVerifyPasswordUsesStoredParameters(t *testing.T) {
	encoded := oldParameterHash("old password")
	if err := VerifyPassword(encoded, "old password"); err != nil {
		t.Fatalf("old-parameter hash rejected: %v", err)
	}
	if err := VerifyPassword(encoded, "wrong password"); err == nil {
		t.Fatal("old-parameter hash accepted a wrong password")
	}
}

func TestNeedsRehash(t *testing.T) {
	if !NeedsRehash(oldParameterHash("old password")) {
		t.Fatal("old Argon2 parameters should require rehash")
	}
	current, err := HashPassword("current password")
	if err != nil {
		t.Fatal(err)
	}
	if NeedsRehash(current) {
		t.Fatal("current parameters should not require rehash")
	}
	for _, changed := range []string{
		strings.Replace(current, "m=65536", "m=32768", 1),
		strings.Replace(current, "p=4", "p=2", 1),
		"invalid hash",
	} {
		if !NeedsRehash(changed) {
			t.Fatalf("changed or malformed parameters should require rehash: %q", changed)
		}
	}
}

func BenchmarkVerifyPassword(b *testing.B) {
	encoded, err := HashPassword("benchmark password")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := VerifyPassword(encoded, "benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestHashPasswordVerifiesCorrectPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password stored in plaintext")
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash = %q; want argon2id PHC format", hash)
	}
	if err := VerifyPassword(hash, "correct horse battery staple"); err != nil {
		t.Errorf("VerifyPassword rejected the correct password: %v", err)
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("the real password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if err := VerifyPassword(hash, "the wrong password"); err == nil {
		t.Fatal("VerifyPassword accepted the wrong password")
	}
}

func TestHashPasswordSaltsUniquely(t *testing.T) {
	first, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	second, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if first == second {
		t.Error("two hashes of the same password are identical; salt is missing")
	}
}

func TestNewSessionTokenIsUsableAndHashed(t *testing.T) {
	token, hash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken failed: %v", err)
	}
	if token == "" || hash == "" {
		t.Fatal("empty token or hash")
	}
	if token == hash {
		t.Error("raw token stored instead of a hash")
	}
	if HashSessionToken(token) != hash {
		t.Error("HashSessionToken does not reproduce the stored hash")
	}
	second, _, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken failed: %v", err)
	}
	if second == token {
		t.Error("two session tokens are identical; randomness is missing")
	}
}

func TestSessionExpiryBoundaries(t *testing.T) {
	now := time.Now()
	if !IsSessionExpired(now.Add(-time.Hour), now) {
		t.Error("past expiry not detected")
	}
	if IsSessionExpired(now.Add(time.Hour), now) {
		t.Error("future expiry reported as expired")
	}
}

func TestRateLimiterTripsAfterFiveFailures(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	key := "user@example.com"
	for i := 0; i < 5; i++ {
		if limiter.Blocked(key, time.Now()) {
			t.Fatalf("blocked before 5 failures (attempt %d)", i+1)
		}
		limiter.RecordFailure(key, time.Now())
	}
	if !limiter.Blocked(key, time.Now()) {
		t.Fatal("not blocked after 5 failures")
	}
	limiter.RecordSuccess(key)
	if limiter.Blocked(key, time.Now()) {
		t.Fatal("still blocked after a successful login reset the counter")
	}
}
