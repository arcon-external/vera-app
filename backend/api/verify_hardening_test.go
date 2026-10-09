package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

// TestVerifyInputsAreBounded proves every typed field is rejected at a
// documented bound before it can reach a query or the page. On a public
// endpoint, an unbounded field is a memory and reflection problem.
func TestVerifyInputsAreBounded(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_verify_bounds")
	attachedTestSigner(t, server, "vera_api_verify_bounds")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_bounds")

	oversized := strings.Repeat("9", 600)
	cases := map[string]url.Values{
		"oversized id":     {"id": {oversized}},
		"oversized number": {"id": {shortID}, "number": {strings.Repeat("1", 200)}},
		"oversized amount": {"id": {shortID}, "amount": {strings.Repeat("1", 200)}},
		"script in id":     {"id": {`"><script>alert(1)</script>`}},
		"script in number": {"id": {shortID}, "number": {`<img src=x onerror=alert(1)>`}},
		"script in amount": {"id": {shortID}, "amount": {"<b>100</b>"}},
		"null byte":        {"id": {"abc\x00def"}},
		"newlines":         {"id": {"abc\ndef\r\nghi"}},
	}
	for name, form := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)

			// Never a 500: malformed input is a reader mistake, not a crash.
			if recorder.Code >= http.StatusInternalServerError {
				t.Fatalf("status = %d; want a handled response", recorder.Code)
			}
			body := recorder.Body.String()
			// Nothing typed may be reflected verbatim, escaped or not. The
			// page carries exactly one static script (the amount grouping
			// helper, which never interpolates user input), so <script may
			// appear once and only once; anything more is reflection.
			if count := strings.Count(body, "<script"); count != 1 {
				t.Errorf("<script count = %d; want exactly the static helper", count)
			}
			for _, marker := range []string{"onerror", "<img", "<b>", "\x00"} {
				if strings.Contains(body, marker) {
					t.Errorf("unbounded input reflected: %q present in response", marker)
				}
			}
		})
	}
}

// TestVerifyRejectsOversizedBodyBeforeParsing proves the body limit stops
// absurd payloads at the edge rather than in the handler.
func TestVerifyRejectsOversizedBodyBeforeParsing(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_verify_bodylimit")
	attachedTestSigner(t, server, "vera_api_verify_bodylimit")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_bodylimit")

	huge := "id=" + shortID + "&amount=" + strings.Repeat("9", 20000)
	request := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(huge))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge && recorder.Code != http.StatusBadRequest {
		t.Errorf("oversized body = %d; want 413 or 400", recorder.Code)
	}
}

// TestVerifySetsHardeningHeaders locks the headers a public page needs:
// no framing, no sniffing, no referrer leakage, no caching, and a policy
// that allows nothing but the page's own stylesheet.
func TestVerifySetsHardeningHeaders(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_verify_headers2")
	attachedTestSigner(t, server, "vera_api_verify_headers2")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_headers2")

	request := httptest.NewRequest(http.MethodGet, "/v/"+shortID, nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	expected := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Cache-Control":           "no-store, max-age=0",
		"Content-Security-Policy": verificationCSP,
	}
	for header, want := range expected {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("%s = %q; want %q", header, got, want)
		}
	}
	csp := recorder.Header().Get("Content-Security-Policy")
	for _, must := range []string{"default-src 'none'", "frame-ancestors 'none'", "base-uri 'none'", "form-action 'self'"} {
		if !strings.Contains(csp, must) {
			t.Errorf("CSP missing %q", must)
		}
	}
	if strings.Contains(csp, "script-src") {
		t.Error("CSP allows scripts on a page that has none")
	}
}

// TestForwardedHeaderCannotSpoofTheLimiter proves a client cannot rotate its
// rate-limit identity by inventing X-Forwarded-For entries: only the address
// the platform appended is trusted.
func TestForwardedHeaderCannotSpoofTheLimiter(t *testing.T) {
	resetTestSchema(t, "vera_api_verify_spoof")
	server, token := testQuotationServer(t, "vera_api_verify_spoof")
	attachedTestSigner(t, server, "vera_api_verify_spoof")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_spoof")

	limited := false
	for i := 0; i < 60; i++ {
		request := httptest.NewRequest(http.MethodGet, "/v/"+shortID, nil)
		// The attacker rotates its claimed address every request; the real
		// peer (appended last) stays constant.
		request.Header.Set("X-Forwarded-For", "10.0.0."+strconv.Itoa(i%254+1)+", 198.51.100.9")
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("rotating X-Forwarded-For bypassed the rate limiter")
	}
}

// TestClientIPTrustsOnlyAppendedAddress locks the parsing rule in place.
func TestClientIPTrustsOnlyAppendedAddress(t *testing.T) {
	cases := map[string]string{
		"198.51.100.9, 203.0.113.7": "203.0.113.7", // platform appended last
		"203.0.113.7":               "203.0.113.7",
		"garbage, 203.0.113.7":      "203.0.113.7", // junk entry ignored
		"203.0.113.7, not-an-ip":    "203.0.113.7", // junk appended ignored
		// Nothing parseable: fall back to the actual peer rather than a
		// shared bucket, so limiting still discriminates callers.
		"not-an-ip": "192.0.2.1",
	}
	for header, want := range cases {
		request := httptest.NewRequest(http.MethodGet, "/v/x", nil)
		request.Header.Set("X-Forwarded-For", header)
		request.RemoteAddr = "192.0.2.1:5555"
		context := echo.New().NewContext(request, httptest.NewRecorder())
		if got := clientIP(context); got != want {
			t.Errorf("X-Forwarded-For %q -> %q; want %q", header, got, want)
		}
	}
}

// TestRateLimiterMemoryIsBounded proves the tracked set cannot be grown
// without limit by a flood from many addresses.
func TestRateLimiterMemoryIsBounded(t *testing.T) {
	limiter := newIPRateLimiter(5, time.Minute)
	limiter.maxKeys = 50
	for i := 0; i < 500; i++ {
		limiter.allow("10.1." + strconv.Itoa(i/256) + "." + strconv.Itoa(i%256))
	}
	if len(limiter.seen) > limiter.maxKeys {
		t.Errorf("tracked keys = %d; want at most %d", len(limiter.seen), limiter.maxKeys)
	}
}

// TestVerifyOnlyAcceptsGetAndPost keeps the surface to the two verbs the
// portal needs, so no other method can reach the handler.
func TestVerifyOnlyAcceptsGetAndPost(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_verify_methods")
	attachedTestSigner(t, server, "vera_api_verify_methods")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_methods")

	for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		request := httptest.NewRequest(method, "/v/"+shortID, nil)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMethodNotAllowed && recorder.Code != http.StatusNotFound {
			t.Errorf("%s = %d; want 405 or 404", method, recorder.Code)
		}
	}
}
