package backend

import (
	"testing"
)

func TestSessionTokenRoundTrip(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	defer ClearSessionToken()

	if got := SessionToken(); got != "" {
		t.Fatalf("initial token = %q; want empty", got)
	}
	if err := SetSessionToken("token-abc"); err != nil {
		t.Fatalf("SetSessionToken failed: %v", err)
	}
	if got := SessionToken(); got != "token-abc" {
		t.Errorf("SessionToken = %q; want token-abc", got)
	}
	if err := ClearSessionToken(); err != nil {
		t.Fatalf("ClearSessionToken failed: %v", err)
	}
	if got := SessionToken(); got != "" {
		t.Errorf("token after clear = %q; want empty", got)
	}
}
