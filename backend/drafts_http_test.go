package backend

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestGetDraftCarriesCurrency pins the desktop side of the B4 contract: the
// currency key the server stamps on the draft row reaches the DTO the editor
// compares against the live profile, without going through the shared
// archive contract.
func TestGetDraftCarriesCurrency(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":        7,
			"label":     "Roof works",
			"subject":   "Roof works",
			"itemCount": 1,
			"currency":  "USD",
			"payload":   json.RawMessage(`{"v":1}`),
			"updatedAt": "2026-10-01T00:00:00Z",
		})
	})

	q := newTestQuotation()
	draft, err := q.GetDraft(7)
	if err != nil {
		t.Fatalf("GetDraft failed: %v", err)
	}
	if draft.Currency != "USD" {
		t.Errorf("draft currency = %q; want USD", draft.Currency)
	}
	if draft.Label != "Roof works" || draft.ItemCount != 1 {
		t.Errorf("draft = %+v; want the row fields beside the currency", draft)
	}
	if draft.Payload != `{"v":1}` {
		t.Errorf("payload = %q; want it back verbatim", draft.Payload)
	}
}

// TestGetDraftWithoutCurrencyReadsBlank covers a server that predates the
// key: the DTO carries nothing and the editor's normaliser reads it as the
// default, so old servers fail closed to IDR rather than crashing the open.
func TestGetDraftWithoutCurrencyReadsBlank(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":        8,
			"itemCount": 0,
			"payload":   json.RawMessage(`{}`),
			"updatedAt": "2026-10-01T00:00:00Z",
		})
	})

	q := newTestQuotation()
	draft, err := q.GetDraft(8)
	if err != nil {
		t.Fatalf("GetDraft failed: %v", err)
	}
	if draft.Currency != "" {
		t.Errorf("draft currency = %q; want blank (the editor defaults it)", draft.Currency)
	}
}
