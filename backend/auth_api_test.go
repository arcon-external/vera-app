package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"vera-app/backend/apiclient"
)

// The desktop sign-in storage contract (issue #434).
//
// A remembered sign-in earns a session that outlives the process, so the
// token is written to the OS keychain. An unremembered one must not be:
// leaving a previously remembered token in the keychain would restore that
// stale 30-day session on the next launch through SessionToken's keychain
// fallback — "keep me signed in" unchecked would still keep the machine
// signed in, on a token this sign-in never issued.
//
// The keychain is reached through the package vars in session.go (keyringSave,
// keyringLoad, keyringDelete), which default to the real auth.* calls and are
// swapped here for recorders: whether the keychain was written is exactly
// what these tests assert, and the machine running them has no keychain.

// fakeKeyring stands in for the OS keychain and records every call.
type fakeKeyring struct {
	mu      sync.Mutex
	stored  string
	saves   []string
	deletes int
	loads   int
}

func (f *fakeKeyring) save(token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stored = token
	f.saves = append(f.saves, token)
	return nil
}

func (f *fakeKeyring) load() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	return f.stored, nil
}

func (f *fakeKeyring) del() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	f.stored = ""
	return nil
}

func (f *fakeKeyring) saveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.saves)
}

func (f *fakeKeyring) deleteCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deletes
}

func (f *fakeKeyring) storedToken() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stored
}

// stubSessionKeyring points the package's keychain seam at the fake, with the
// keyring path enabled (VERA_NO_KEYRING unset would do, but an environment
// that sets it must not silently turn these into memory-only tests).
func stubSessionKeyring(t *testing.T, fake *fakeKeyring) {
	t.Helper()
	t.Setenv("VERA_NO_KEYRING", "0")
	previousSave, previousLoad, previousDelete := keyringSave, keyringLoad, keyringDelete
	keyringSave, keyringLoad, keyringDelete = fake.save, fake.load, fake.del
	t.Cleanup(func() {
		_ = ClearSessionToken()
		fake.mu.Lock()
		fake.stored = ""
		fake.mu.Unlock()
		keyringSave, keyringLoad, keyringDelete = previousSave, previousLoad, previousDelete
	})
}

// loginRequestBody is what the desktop posts to /v1/auth/login, as the server
// decodes it. Remember is the field W1 added; an absent field decodes false,
// which is why the desktop may always send it.
type loginRequestBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// loginServer answers the two endpoints a sign-in touches and records the
// login body into seen.
func loginServer(t *testing.T, seen *loginRequestBody, token string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/login":
			if err := json.NewDecoder(request.Body).Decode(seen); err != nil {
				t.Errorf("decode login body: %v", err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"token": token,
				"user": map[string]any{
					"id":          7,
					"email":       "staff@example.com",
					"displayName": "Staff",
					"role":        "staff",
				},
			})
		case "/v1/auth/logout":
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func pointAPIClientAt(t *testing.T, server *httptest.Server) {
	t.Helper()
	previous := apiClient
	ConfigureAPI(apiclient.New(server.URL, SessionToken))
	t.Cleanup(func() { apiClient = previous })
}

// forgetMemory simulates the next launch: the process starts with an empty
// in-memory token, so only the keychain can still hold a session.
func forgetMemory() {
	sessionMu.Lock()
	sessionToken = ""
	sessionMu.Unlock()
}

func TestLoginRememberedWritesTheKeychain(t *testing.T) {
	keyring := &fakeKeyring{}
	stubSessionKeyring(t, keyring)
	var seen loginRequestBody
	pointAPIClientAt(t, loginServer(t, &seen, "srv-remembered-token"))

	user, err := (&AuthAPI{}).Login("staff@example.com", "correct horse", true)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !seen.Remember {
		t.Error("the login body did not ask to be remembered")
	}
	if keyring.saveCount() != 1 || keyring.storedToken() != "srv-remembered-token" {
		t.Errorf("keychain saves = %v, stored = %q; want the new token written once",
			keyring.saves, keyring.storedToken())
	}
	if keyring.deleteCount() != 0 {
		t.Errorf("keychain deletes = %d; want 0 on the remembered path", keyring.deleteCount())
	}
	if got := SessionToken(); got != "srv-remembered-token" {
		t.Errorf("SessionToken = %q; want the new token", got)
	}
	if user.Email != "staff@example.com" || user.ID != 7 {
		t.Errorf("user = %+v; want the server's user", user)
	}
}

func TestLoginUnrememberedSetsMemoryAndDeletesTheStoredToken(t *testing.T) {
	keyring := &fakeKeyring{stored: "stale-30d-token"}
	stubSessionKeyring(t, keyring)
	var seen loginRequestBody
	pointAPIClientAt(t, loginServer(t, &seen, "srv-standard-token"))

	if _, err := (&AuthAPI{}).Login("staff@example.com", "correct horse", false); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if seen.Remember {
		t.Error("the login body asked to be remembered on an unchecked sign-in")
	}
	if keyring.saveCount() != 0 {
		t.Errorf("keychain saves = %v; want none — the session must not survive a restart", keyring.saves)
	}
	if keyring.deleteCount() != 1 {
		t.Errorf("keychain deletes = %d; want 1 — the remembered token from the previous sign-in is stale now", keyring.deleteCount())
	}
	if keyring.storedToken() != "" {
		t.Errorf("keychain still holds %q; want empty", keyring.storedToken())
	}
	if got := SessionToken(); got != "srv-standard-token" {
		t.Errorf("SessionToken = %q; want the new token held in memory", got)
	}
}

// The stale-token trap, end to end: an unchecked sign-in after a remembered
// one must leave the next launch with nothing to restore.
func TestUncheckedSignInLeavesNothingForTheNextLaunchToRestore(t *testing.T) {
	keyring := &fakeKeyring{stored: "stale-30d-token"}
	stubSessionKeyring(t, keyring)
	var seen loginRequestBody
	pointAPIClientAt(t, loginServer(t, &seen, "srv-standard-token"))

	if _, err := (&AuthAPI{}).Login("staff@example.com", "correct horse", false); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	forgetMemory()
	if got := SessionToken(); got != "" {
		t.Errorf("next launch restored %q; the unchecked sign-in left the remembered token in the keychain", got)
	}
}

func TestRememberedSignInSurvivesTheNextLaunch(t *testing.T) {
	keyring := &fakeKeyring{}
	stubSessionKeyring(t, keyring)
	var seen loginRequestBody
	pointAPIClientAt(t, loginServer(t, &seen, "srv-remembered-token"))

	if _, err := (&AuthAPI{}).Login("staff@example.com", "correct horse", true); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	forgetMemory()
	if got := SessionToken(); got != "srv-remembered-token" {
		t.Errorf("next launch got %q; want the remembered token back from the keychain", got)
	}
}

func TestLogoutClearsMemoryAndTheKeychainAfterBothSignIns(t *testing.T) {
	for _, remember := range []bool{true, false} {
		keyring := &fakeKeyring{stored: "stale-30d-token"}
		stubSessionKeyring(t, keyring)
		var seen loginRequestBody
		pointAPIClientAt(t, loginServer(t, &seen, "srv-token"))

		if _, err := (&AuthAPI{}).Login("staff@example.com", "correct horse", remember); err != nil {
			t.Fatalf("Login(remember=%v) failed: %v", remember, err)
		}
		if err := (&AuthAPI{}).Logout(); err != nil {
			t.Fatalf("Logout failed: %v", err)
		}
		if got := SessionToken(); got != "" {
			t.Errorf("remember=%v: SessionToken after logout = %q; want empty", remember, got)
		}
		if keyring.storedToken() != "" {
			t.Errorf("remember=%v: keychain still holds %q after logout; want empty", remember, keyring.storedToken())
		}
	}
}
