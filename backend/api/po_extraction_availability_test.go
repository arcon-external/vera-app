package api

import (
	"net/http"
	"strconv"
	"testing"

	"vera-app/backend/extract"
)

// Issue #498: the desktop shows a clear unavailable banner and disables
// Extract from a typed answer. This probe is that answer: it reports the same
// Prober preflight Start uses, writes nothing, and never simulates success.
func extractionAvailability(t *testing.T, server *Server, token string) (int, map[string]any) {
	t.Helper()
	return doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/extraction/availability", token, nil)
}

func TestExtractionAvailabilityReportsThePreflightWithoutWriting(t *testing.T) {
	// The default server has no extractor. A deployment that never configured
	// isolation must say so before a button is pressed.
	t.Setenv("VERA_EXTRACTOR_CMD", "")
	server, token := testQuotationServer(t, "vera_api_po_extract_availability")
	if server.extractions.Engine != nil {
		t.Fatalf("default server configured an extraction engine: %+v", server.extractions.Engine)
	}

	if code, _ := extractionAvailability(t, server, ""); code != http.StatusUnauthorized {
		t.Fatalf("probe without a session = %d; want 401", code)
	}
	code, body := extractionAvailability(t, server, token)
	if code != http.StatusOK || body["available"] != false {
		t.Fatalf("unconfigured probe = %d %v; want 200 available=false", code, body)
	}

	// An engine without a Prober is assumed available, as Start assumes it.
	server.SetExtractor(&stubExtractionEngine{output: extractionFixture()})
	code, body = extractionAvailability(t, server, token)
	if code != http.StatusOK || body["available"] != true {
		t.Fatalf("configured probe = %d %v; want 200 available=true", code, body)
	}

	// A configured command that cannot be resolved is unavailable, not a
	// queued job that fails later.
	server.SetExtractor(&extract.CommandEngine{Command: []string{"vera-extractor-that-does-not-exist"}})
	code, body = extractionAvailability(t, server, token)
	if code != http.StatusOK || body["available"] != false {
		t.Fatalf("missing program probe = %d %v; want 200 available=false", code, body)
	}

	// The probe writes nothing: an untouched source-less draft still has no run.
	_, created := createBlankPODraft(t, server, token, nil)
	draft := decodePurchaseOrder(t, created)
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/extractions/latest", token, nil)
	if code != http.StatusOK {
		t.Fatalf("latest after probes = %d; want 200 (%v)", code, body)
	}
	if state := extractionState(t, body); state.Run != nil {
		t.Errorf("a probe queued run %+v; availability is read-only", state.Run)
	}
}
