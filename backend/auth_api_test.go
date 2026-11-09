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

// --- Issue #531: typed session check -------------------------------------

// issue531APIClient points the shared client at a URL and restores the
// previous one afterwards. Unlike pointAPIClientAt it takes a URL, so a test
// can close the server first and exercise the unreachable branch.
func issue531APIClient(t *testing.T, url string) {
	t.Helper()
	previous := apiClient
	ConfigureAPI(apiclient.New(url, SessionToken))
	t.Cleanup(func() { apiClient = previous })
}

// issue531Server answers /v1/auth/me with one fixed status and body.
func issue531Server(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/auth/me" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_, _ = writer.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// TestCheckSessionClassifiesEveryOutcome is the status matrix the desktop acts
// on. A status is data, never an error, so every expected outcome (including
// transport and malformed responses) has to arrive as a value.
func TestCheckSessionClassifiesEveryOutcome(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantState string
		wantCode  string
	}{
		{
			name:      "active",
			status:    http.StatusOK,
			body:      `{"id":7,"email":"staff@example.com","displayName":"Staff","role":"staff"}`,
			wantState: "active",
			wantCode:  "session_active",
		},
		{
			name:      "expired",
			status:    http.StatusUnauthorized,
			body:      `{"error":"session expired","code":"session_expired"}`,
			wantState: "expired",
			wantCode:  "session_expired",
		},
		{
			name:      "revoked",
			status:    http.StatusUnauthorized,
			body:      `{"error":"session was signed out","code":"session_revoked"}`,
			wantState: "revoked",
			wantCode:  "session_revoked",
		},
		{
			name:      "inactive",
			status:    http.StatusUnauthorized,
			body:      `{"error":"account is inactive","code":"session_inactive"}`,
			wantState: "inactive",
			wantCode:  "session_inactive",
		},
		{
			name:      "invalid",
			status:    http.StatusUnauthorized,
			body:      `{"error":"invalid session","code":"session_invalid"}`,
			wantState: "invalid",
			wantCode:  "session_invalid",
		},
		{
			name:      "legacy 401 without a code",
			status:    http.StatusUnauthorized,
			body:      `{"error":"invalid or expired session"}`,
			wantState: "invalid",
			wantCode:  "session_invalid",
		},
		{
			name:      "server error",
			status:    http.StatusServiceUnavailable,
			body:      `{"error":"session check failed","code":"session_check_failed"}`,
			wantState: "server_error",
			wantCode:  "session_check_failed",
		},
		{
			name:      "rate limited",
			status:    http.StatusTooManyRequests,
			body:      `{"error":"too many requests"}`,
			wantState: "rate_limited",
			wantCode:  "rate_limited",
		},
		{
			name:      "malformed response",
			status:    http.StatusOK,
			body:      `not-json`,
			wantState: "incompatible",
			wantCode:  "session_incompatible",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubSessionKeyring(t, &fakeKeyring{})
			issue531APIClient(t, issue531Server(t, tc.status, tc.body).URL)
			setSessionTokenMemory("test-token")

			got := (&AuthAPI{}).CheckSession()
			if got.State != tc.wantState {
				t.Errorf("State = %q; want %q (message %q)", got.State, tc.wantState, got.Message)
			}
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q; want %q", got.Code, tc.wantCode)
			}
			if tc.wantState == "active" && (got.User.ID != 7 || got.User.Email != "staff@example.com") {
				t.Errorf("User = %+v; want the server's user", got.User)
			}
		})
	}
}

// TestCheckSessionReportsUnreachable covers a request that never reaches the
// server: the laptop is offline, not signed out.
func TestCheckSessionReportsUnreachable(t *testing.T) {
	stubSessionKeyring(t, &fakeKeyring{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()
	issue531APIClient(t, url)
	setSessionTokenMemory("kept-token")

	got := (&AuthAPI{}).CheckSession()
	if got.State != "unreachable" {
		t.Fatalf("State = %q; want unreachable (message %q)", got.State, got.Message)
	}
	if SessionToken() != "kept-token" {
		t.Error("a transport failure cleared the session token")
	}
}

func TestCheckSessionWithoutATokenIsInvalid(t *testing.T) {
	stubSessionKeyring(t, &fakeKeyring{})
	forgetMemory()

	got := (&AuthAPI{}).CheckSession()
	if got.State != "invalid" {
		t.Fatalf("State = %q; want invalid", got.State)
	}
}

// TestCheckSessionClearsTheTokenOnlyOnARealSessionEnd pins the token
// lifecycle: a real session end is the only poll outcome allowed to clear the
// keychain. 5xx, 429 and transport failures keep it.
func TestCheckSessionClearsTheTokenOnlyOnARealSessionEnd(t *testing.T) {
	t.Run("expired clears", func(t *testing.T) {
		keyring := &fakeKeyring{}
		stubSessionKeyring(t, keyring)
		issue531APIClient(t, issue531Server(t, http.StatusUnauthorized,
			`{"error":"session expired","code":"session_expired"}`).URL)
		setSessionTokenMemory("dead-token")

		if got := (&AuthAPI{}).CheckSession(); got.State != "expired" {
			t.Fatalf("State = %q; want expired", got.State)
		}
		if SessionToken() != "" {
			t.Error("the expired session token survived")
		}
		if keyring.deleteCount() == 0 {
			t.Error("the keychain token was not deleted on a real session end")
		}
	})

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusServiceUnavailable, `{"error":"session check failed","code":"session_check_failed"}`},
		{"rate limited", http.StatusTooManyRequests, `{"error":"too many requests"}`},
	} {
		t.Run(tc.name+" keeps", func(t *testing.T) {
			keyring := &fakeKeyring{}
			stubSessionKeyring(t, keyring)
			issue531APIClient(t, issue531Server(t, tc.status, tc.body).URL)
			setSessionTokenMemory("kept-token")

			if got := (&AuthAPI{}).CheckSession(); got.State == "expired" {
				t.Fatalf("State = %q; want an operational state", got.State)
			}
			if SessionToken() != "kept-token" {
				t.Errorf("%s cleared the session token", tc.name)
			}
			if keyring.deleteCount() != 0 {
				t.Errorf("%s deleted the keychain token", tc.name)
			}
		})
	}
}

// TestChangePasswordWrongCurrentKeepsTheSession is the desktop half of the
// wrong-password contract: a 401 carrying current_password_incorrect is a
// refusal, not a session end, and the token must survive it.
func TestChangePasswordWrongCurrentKeepsTheSession(t *testing.T) {
	keyring := &fakeKeyring{}
	stubSessionKeyring(t, keyring)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/auth/password" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":"current password is incorrect","code":"current_password_incorrect"}`))
	}))
	t.Cleanup(server.Close)
	issue531APIClient(t, server.URL)
	setSessionTokenMemory("kept-token")

	err := (&AuthAPI{}).ChangePassword("wrong-password", "brand-new-password")
	if err == nil {
		t.Fatal("wrong current password was accepted")
	}
	if code := apiclient.CodeOf(err); code != "current_password_incorrect" {
		t.Errorf("CodeOf = %q; want current_password_incorrect", code)
	}
	if SessionToken() != "kept-token" {
		t.Error("a wrong current password cleared the session token")
	}
	if keyring.deleteCount() != 0 {
		t.Error("a wrong current password deleted the keychain token")
	}
}

// TestAnotherUnauthorizedDoesNotClearTheToken is the regression for the removed
// global "any 401 clears the token" hook. Only CheckSession's session-end path
// and a manual Logout may clear it.
func TestAnotherUnauthorizedDoesNotClearTheToken(t *testing.T) {
	keyring := &fakeKeyring{}
	stubSessionKeyring(t, keyring)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":"invalid or expired session"}`))
	}))
	t.Cleanup(server.Close)
	issue531APIClient(t, server.URL)
	setSessionTokenMemory("kept-token")

	if _, err := (&AuthAPI{}).GetCompany(); err == nil {
		t.Fatal("expected the 401 to surface")
	}
	if SessionToken() != "kept-token" {
		t.Error("an unrelated 401 cleared the session token")
	}
	if keyring.deleteCount() != 0 {
		t.Error("an unrelated 401 deleted the keychain token")
	}
}

// Ticket #536: the session check now carries the expiry the desktop needs for
// the "Stay signed in" window, and renewal is the same typed contract —
// data for every expected outcome, and a real session end is the only outcome
// that clears the token.

func sessionServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestCheckSessionCarriesTheExpiry(t *testing.T) {
	stubSessionKeyring(t, &fakeKeyring{})
	issue531APIClient(t, sessionServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/auth/me" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":7,"email":"staff@example.com","displayName":"Staff","role":"staff",
			"expiresAt":"2026-11-09T10:00:00Z",
			"absoluteExpiresAt":"2026-11-16T00:00:00Z",
			"canRenew":true,"remembered":false
		}`))
	}).URL)
	setSessionTokenMemory("live-token")

	got := (&AuthAPI{}).CheckSession()
	if got.State != SessionStateActive {
		t.Fatalf("State = %q; want active", got.State)
	}
	if got.ExpiresAt != "2026-11-09T10:00:00Z" {
		t.Errorf("ExpiresAt = %q; want the server's idle deadline", got.ExpiresAt)
	}
	if !got.CanRenew {
		t.Error("CanRenew = false; the server said the session is renewable")
	}
}

func TestRenewSessionReturnsTypedStatus(t *testing.T) {
	keyring := &fakeKeyring{}
	stubSessionKeyring(t, keyring)
	var renewals int
	issue531APIClient(t, sessionServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/auth/renew" || request.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", request.Method, request.URL.Path)
		}
		renewals++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":7,"email":"staff@example.com","displayName":"Staff","role":"staff",
			"expiresAt":"2026-11-09T18:00:00Z",
			"absoluteExpiresAt":"2026-11-16T00:00:00Z",
			"canRenew":true,"remembered":false
		}`))
	}).URL)
	setSessionTokenMemory("live-token")

	got := (&AuthAPI{}).RenewSession()
	if renewals != 1 {
		t.Fatalf("renewal requests = %d; want 1", renewals)
	}
	if got.State != SessionStateActive {
		t.Fatalf("State = %q; want active (message %q)", got.State, got.Message)
	}
	if got.ExpiresAt != "2026-11-09T18:00:00Z" {
		t.Errorf("ExpiresAt = %q; want the renewed deadline", got.ExpiresAt)
	}
	if got.User.ID != 7 {
		t.Errorf("User = %+v; want the server's user", got.User)
	}
	if SessionToken() != "live-token" {
		t.Error("a successful renewal changed the session token")
	}
	if keyring.deleteCount() != 0 {
		t.Error("a successful renewal deleted the keychain token")
	}
}

func TestRenewSessionOnARealEndClearsTheToken(t *testing.T) {
	keyring := &fakeKeyring{}
	stubSessionKeyring(t, keyring)
	issue531APIClient(t, sessionServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":"session expired","code":"session_expired"}`))
	}).URL)
	setSessionTokenMemory("dead-token")

	got := (&AuthAPI{}).RenewSession()
	if got.State != SessionStateExpired {
		t.Fatalf("State = %q; want expired", got.State)
	}
	if SessionToken() != "" {
		t.Error("the expired session token survived the renewal")
	}
	if keyring.deleteCount() == 0 {
		t.Error("the keychain token was not deleted on a real session end")
	}
}

// TestRenewSessionOperationalFailuresKeepTheSession is #531's poll rule
// applied to the renewal button: a 5xx, a 429, a transport failure or a
// rejected binding must never sign the user out.
func TestRenewSessionOperationalFailuresKeepTheSession(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusServiceUnavailable, `{"error":"session renewal failed","code":"session_check_failed"}`},
		{"rate limited", http.StatusTooManyRequests, `{"error":"too many requests"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keyring := &fakeKeyring{}
			stubSessionKeyring(t, keyring)
			issue531APIClient(t, sessionServer(t, func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(tc.status)
				_, _ = writer.Write([]byte(tc.body))
			}).URL)
			setSessionTokenMemory("kept-token")

			got := (&AuthAPI{}).RenewSession()
			if got.State == SessionStateActive || isSessionEndState(got.State) {
				t.Fatalf("State = %q; want an operational state", got.State)
			}
			if SessionToken() != "kept-token" {
				t.Errorf("%s cleared the session token", tc.name)
			}
			if keyring.deleteCount() != 0 {
				t.Errorf("%s deleted the keychain token", tc.name)
			}
		})
	}

	t.Run("transport", func(t *testing.T) {
		stubSessionKeyring(t, &fakeKeyring{})
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := server.URL
		server.Close()
		issue531APIClient(t, url)
		setSessionTokenMemory("kept-token")

		got := (&AuthAPI{}).RenewSession()
		if got.State != SessionStateUnreachable {
			t.Fatalf("State = %q; want unreachable", got.State)
		}
		if SessionToken() != "kept-token" {
			t.Error("a transport failure cleared the session token")
		}
	})
}

// isSessionEndState mirrors the frontend's isSessionEnd so the test states
// the contract rather than the absence of one status.
func isSessionEndState(state string) bool {
	switch state {
	case SessionStateExpired, SessionStateRevoked, SessionStateInactive, SessionStateInvalid:
		return true
	}
	return false
}
