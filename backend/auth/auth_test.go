package auth

import (
	"strings"
	"testing"
	"time"
)

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
