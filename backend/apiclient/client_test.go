package apiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
