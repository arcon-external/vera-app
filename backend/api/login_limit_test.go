package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The login endpoint now has two limiters and they defend different things:
// backend/auth's, keyed by the email in the request body, protects one account;
// this one, keyed by address, bounds the endpoint. A flood of distinct addresses
// passes the first and not the second, which is the whole point.

// TestLoginLimiterRefusesBeforeDoingAnyWork is the property that matters most.
//
// The limiter is checked before the body is read and before the service is
// called, so a flood is refused as cheaply as possible. This server has a nil
// service, which would panic the moment the handler got that far -- so reaching
// a 429 is itself the proof that nothing downstream ran.
func TestLoginLimiterRefusesBeforeDoingAnyWork(t *testing.T) {
	server := NewServer(nil, nil, nil)

	// Fill one address's budget directly, so the test does not need a database.
	const address = "203.0.113.7"
	for i := 0; i < loginAttemptsPerMinute; i++ {
		if !server.loginLimiter.allow(address) {
			t.Fatalf("limiter refused attempt %d of %d; the budget should cover all of them",
				i+1, loginAttemptsPerMinute)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	request.Header.Set("X-Forwarded-For", address)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("GET /v1/auth/login from an exhausted address = %d; want %d",
			recorder.Code, http.StatusTooManyRequests)
	}
}

// TestLoginLimiterIsKeyedByAddressNotByEmail is the distinction from the
// per-account limiter, stated as a test.
//
// Two different addresses, each inside its own budget, must both be let through
// even though they carry the same email in the body -- otherwise this would just
// be the email limiter again with extra steps.
func TestLoginLimiterIsKeyedByAddressNotByEmail(t *testing.T) {
	server := NewServer(nil, nil, nil)

	for i := 0; i < loginAttemptsPerMinute; i++ {
		server.loginLimiter.allow("203.0.113.7")
	}

	// The exhausted address is refused; a different one still has its budget.
	if server.loginLimiter.allow("203.0.113.7") {
		t.Fatal("an exhausted address was allowed again")
	}
	if !server.loginLimiter.allow("198.51.100.9") {
		t.Fatal("a second address was refused; the limit is not per-address")
	}
}

// TestLoginLimiterRejectsSpoofedForwardedFor checks the bucket cannot be chosen
// by the caller.
//
// clientIP trusts only the *last* X-Forwarded-For entry, because the platform
// appends the real peer after anything the client sent. So a client that prepends
// a fake address still lands in its own bucket -- which is what makes this limiter
// worth having at all.
func TestLoginLimiterRejectsSpoofedForwardedFor(t *testing.T) {
	server := NewServer(nil, nil, nil)

	const realAddress = "203.0.113.7"
	for i := 0; i < loginAttemptsPerMinute; i++ {
		server.loginLimiter.allow(realAddress)
	}

	// A client-supplied prefix must not become the bucket key.
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	request.Header.Set("X-Forwarded-For", "10.0.0.1, "+realAddress)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("a spoofed prefix changed the bucket: got %d, want %d",
			recorder.Code, http.StatusTooManyRequests)
	}
}
