package apiclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func stubServer(t *testing.T, handler http.HandlerFunc) (*Client, *[]http.Request) {
	t.Helper()
	var seen []http.Request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, *request.Clone(request.Context()))
		handler(writer, request)
	}))
	t.Cleanup(server.Close)
	return New(server.URL, func() string { return "test-token" }), &seen
}

func TestClientSendsBearerToken(t *testing.T) {
	client, seen := stubServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	})
	if err := client.Health(); err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("requests = %d; want 1", len(*seen))
	}
	if got := (*seen)[0].Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q; want Bearer test-token", got)
	}
}

func TestClientClearsSessionOn401(t *testing.T) {
	cleared := false
	client, _ := stubServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":"invalid or expired session"}`))
	})
	client.OnUnauthorized = func() { cleared = true }
	if err := client.Health(); err == nil {
		t.Fatal("expected error on 401")
	}
	if !cleared {
		t.Error("401 did not trigger the session-clear hook")
	}
}

func TestClientSurfacesFriendlyNetworkError(t *testing.T) {
	client := New("http://127.0.0.1:1", func() string { return "" })
	if err := client.Health(); err == nil {
		t.Fatal("expected network error")
	} else if got := err.Error(); got == "" || len(got) > 200 {
		t.Errorf("error = %q; want short friendly message", got)
	}
}

func TestClientDecodesTypedResponses(t *testing.T) {
	client, _ := stubServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id": 7, "quoteNumber": "15020007", "revision": 1, "grandTotal": 261000,
		})
	})
	var response struct {
		ID          int64  `json:"id"`
		QuoteNumber string `json:"quoteNumber"`
		Revision    int16  `json:"revision"`
		GrandTotal  uint64 `json:"grandTotal"`
	}
	if err := client.Get("/v1/quotations/7", &response); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if response.QuoteNumber != "15020007" || response.GrandTotal != 261000 {
		t.Errorf("response = %+v; want quote 15020007 grand 261000", response)
	}
}

// The tests below cover what the CLI added to this package. StatusError exists
// so a caller can tell a 404 from a 500 without matching English, and its
// MarshalJSON exists so the desktop keeps working -- the frontend reads an
// error's `message` off whatever Wails hands it (getErrorMessage in App.tsx), so
// a typed error must not arrive as an object it cannot read.

func TestStatusOfReadsTheCodeBack(t *testing.T) {
	client, _ := stubServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"error":"quotation not found"}`))
	})

	err := client.Get("/v1/quotations/9", nil)
	if err == nil {
		t.Fatal("expected an error on 404")
	}
	if got := StatusOf(err); got != http.StatusNotFound {
		t.Errorf("StatusOf = %d; want %d", got, http.StatusNotFound)
	}
	if err.Error() != "quotation not found" {
		t.Errorf("error = %q; want the server's message", err.Error())
	}
}

// TestStatusOfIsZeroWithoutAResponse pins the distinction the CLI's exit codes
// depend on: a status means the server answered, and zero means it never did.
// Mapping a network failure to 404 would report "no such quotation" for a
// machine that simply could not reach the server.
func TestStatusOfIsZeroWithoutAResponse(t *testing.T) {
	client := New("http://127.0.0.1:1", func() string { return "" })
	err := client.Get("/v1/quotations/9", nil)
	if err == nil {
		t.Fatal("expected a network error")
	}
	if got := StatusOf(err); got != 0 {
		t.Errorf("StatusOf = %d; want 0 when the server never answered", got)
	}
}

// TestStatusOfSurvivesWrapping checks errors.As reaches the type through a
// wrapped error, which is how callers add context without losing the code.
func TestStatusOfSurvivesWrapping(t *testing.T) {
	wrapped := fmt.Errorf("export quotation: %w", &StatusError{
		StatusCode: http.StatusUnprocessableEntity,
		Message:    "subject is required",
	})
	if got := StatusOf(wrapped); got != http.StatusUnprocessableEntity {
		t.Errorf("StatusOf(wrapped) = %d; want %d", got, http.StatusUnprocessableEntity)
	}
}

// TestStatusErrorMarshalsAsTheDesktopReadsIt is the one that matters most.
//
// The desktop's getErrorMessage accepts a string, an Error, or an object with a
// `message` property. If this type marshalled to its exported fields instead --
// {StatusCode, Message} -- the frontend would find no `message`, fall back to a
// generic string, and quietly lose the server's explanation. Nobody would notice
// until a user reported an unhelpful error message.
func TestStatusErrorMarshalsAsTheDesktopReadsIt(t *testing.T) {
	raw, err := json.Marshal(&StatusError{StatusCode: 404, Message: "quotation not found"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var asObject map[string]any
	if err := json.Unmarshal(raw, &asObject); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if asObject["message"] != "quotation not found" {
		t.Errorf("marshalled as %s; the desktop reads `message` and would find %v",
			raw, asObject["message"])
	}
	// The status code is for Go callers. Putting it on the wire would be a
	// second vocabulary for something StatusOf already answers.
	if _, present := asObject["StatusCode"]; present {
		t.Errorf("marshalled as %s; the status code belongs to Go callers, not the wire", raw)
	}
}

// TestPostFormPostsAForm covers the method the verification portal needs: the
// one endpoint that is neither JSON in nor JSON out.
func TestPostFormPostsAForm(t *testing.T) {
	// The form is read inside the handler rather than from the recorded clone:
	// a clone shares the request body, and by the time the test looks at it the
	// server has consumed it.
	var (
		contentType string
		auth        string
		form        url.Values
	)
	client, _ := stubServer(t, func(writer http.ResponseWriter, request *http.Request) {
		contentType = request.Header.Get("Content-Type")
		auth = request.Header.Get("Authorization")
		_ = request.ParseForm()
		form = request.PostForm
		_, _ = writer.Write([]byte(`<div class="verdict verdict-valid">`))
	})

	body, err := client.PostForm("/verify", map[string]string{"id": "abc123", "amount": "62100000"})
	if err != nil {
		t.Fatalf("PostForm: %v", err)
	}
	if !strings.Contains(body, "verdict-valid") {
		t.Errorf("body = %q; want the raw page back, not a decoded object", body)
	}
	if contentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q; want form encoding", contentType)
	}
	if auth != "Bearer test-token" {
		t.Errorf("Authorization = %q; want the token attached like every other call", auth)
	}
	if form.Get("id") != "abc123" || form.Get("amount") != "62100000" {
		t.Errorf("form = %v; want both fields sent", form)
	}
}

// TestPostFormReadsTheNotFoundPage is the portal's real shape: "no document
// matches that code" arrives as a 404 whose page still has to be read. Treating
// it as a failure would throw away the answer.
func TestPostFormReadsTheNotFoundPage(t *testing.T) {
	client, _ := stubServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`<div class="verdict verdict-notfound">`))
	})

	body, err := client.PostForm("/verify", map[string]string{"id": "00000000"})
	if err != nil {
		t.Fatalf("PostForm returned an error for a page it should have read: %v", err)
	}
	if !strings.Contains(body, "verdict-notfound") {
		t.Errorf("body = %q; want the not-found page", body)
	}
}
