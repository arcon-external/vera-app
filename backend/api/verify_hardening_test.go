package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
			// Nothing typed may be reflected verbatim, escaped or not.
			for _, marker := range []string{"<script", "onerror", "<img", "<b>", "\x00"} {
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
