package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// draftTestServer boots a server with a second account, so the ownership
// rules can be exercised through the real HTTP surface rather than by calling
// the store directly.
func draftTestServer(t *testing.T, schema string) (*Server, string, string, *store.Store) {
	t.Helper()
	resetVerifySchema(t, schema)
	server, ownerToken := testQuotationServer(t, schema)
	database := openTestStore(t, schema)

	hash, err := auth.HashPassword("draft-intruder-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := database.CreateUser(context.Background(),
		"draft-intruder@example.com", "Intruder", hash, "admin"); err != nil {
		t.Fatalf("create intruder: %v", err)
	}
	// An admin on purpose: the point is that the role buys no reach here.
	intruderToken, _, err := auth.NewService(database).Login(context.Background(),
		"draft-intruder@example.com", "draft-intruder-password")
	if err != nil {
		t.Fatalf("intruder login: %v", err)
	}
	return server, ownerToken, intruderToken, database
}

func draftSaveBody(payload string, itemCount int) map[string]any {
	return map[string]any{
		"subject":   "Roof works",
		"itemCount": itemCount,
		"payload":   json.RawMessage(payload),
	}
}

func saveDraft(t *testing.T, server *Server, token string, body map[string]any) int64 {
	t.Helper()
	code, response := doJSON(t, server, http.MethodPost, "/v1/drafts", token, body)
	if code != http.StatusOK {
		t.Fatalf("save draft = %d; want 200 (%v)", code, response)
	}
	id, ok := response["id"].(float64)
	if !ok {
		t.Fatalf("save draft returned no id: %v", response)
	}
	return int64(id)
}

// TestDraftsEndpoints covers the drafts HTTP surface end to end, including the
// isolation guarantee from the caller's side.
func TestDraftsEndpoints(t *testing.T) {
	server, ownerToken, intruderToken, _ := draftTestServer(t, "vera_api_drafts")
	payload := `{"Items":[{"Description":"Half-written line","Qty":2}]}`

	draftID := saveDraft(t, server, ownerToken, draftSaveBody(payload, 1))

	t.Run("the owner lists and sees their own draft", func(t *testing.T) {
		code, body := doJSON(t, server, http.MethodGet, "/v1/drafts", ownerToken, nil)
		if code != http.StatusOK {
			t.Fatalf("list = %d; want 200 (%v)", code, body)
		}
		drafts, _ := body["drafts"].([]any)
		if len(drafts) != 1 {
			t.Fatalf("owner holds %d drafts; want 1", len(drafts))
		}
		row, _ := drafts[0].(map[string]any)
		if row["id"] != float64(draftID) {
			t.Errorf("listed id = %v; want %d", row["id"], draftID)
		}
		if row["subject"] != "Roof works" {
			t.Errorf("listed subject = %v; want 'Roof works'", row["subject"])
		}
		if row["displayName"] != "Roof works" {
			t.Errorf("displayName = %v; want the subject when no label was given", row["displayName"])
		}
	})

	t.Run("the owner reloads the payload verbatim", func(t *testing.T) {
		code, body := doJSON(t, server, http.MethodGet,
			"/v1/drafts/"+strconv.FormatInt(draftID, 10), ownerToken, nil)
		if code != http.StatusOK {
			t.Fatalf("get = %d; want 200 (%v)", code, body)
		}
		stored, _ := body["payload"].(map[string]any)
		items, _ := stored["Items"].([]any)
		if len(items) != 1 {
			t.Fatalf("payload came back as %v; want the saved item list", body["payload"])
		}
		first, _ := items[0].(map[string]any)
		if first["Description"] != "Half-written line" {
			t.Errorf("item = %v; want the saved line back", first)
		}
	})

	t.Run("an admin intruder gets 404, not 403, on someone else's draft", func(t *testing.T) {
		// A 403 would confirm the id exists. Nobody but the owner may learn
		// that, so every cross-account path answers identically.
		for _, probe := range []struct {
			method string
			path   string
			body   any
		}{
			{http.MethodGet, "/v1/drafts/" + strconv.FormatInt(draftID, 10), nil},
			{http.MethodDelete, "/v1/drafts/" + strconv.FormatInt(draftID, 10), nil},
		} {
			code, response := doJSON(t, server, probe.method, probe.path, intruderToken, probe.body)
			if code != http.StatusNotFound {
				t.Errorf("%s %s as intruder = %d; want 404 (%v)", probe.method, probe.path, code, response)
			}
		}
	})

	t.Run("an admin intruder cannot overwrite it", func(t *testing.T) {
		body := draftSaveBody(`{"Items":[]}`, 0)
		body["id"] = draftID
		code, response := doJSON(t, server, http.MethodPost, "/v1/drafts", intruderToken, body)
		if code != http.StatusNotFound {
			t.Errorf("overwrite as intruder = %d; want 404 (%v)", code, response)
		}
	})

	t.Run("an admin intruder does not see it listed", func(t *testing.T) {
		code, body := doJSON(t, server, http.MethodGet, "/v1/drafts", intruderToken, nil)
		if code != http.StatusOK {
			t.Fatalf("list as intruder = %d; want 200 (%v)", code, body)
		}
		drafts, _ := body["drafts"].([]any)
		if len(drafts) != 0 {
			t.Errorf("intruder listed %d drafts; want 0", len(drafts))
		}
	})

	t.Run("drafts need a session", func(t *testing.T) {
		code, _ := doJSON(t, server, http.MethodGet, "/v1/drafts", "", nil)
		if code != http.StatusUnauthorized {
			t.Errorf("list without a token = %d; want 401", code)
		}
	})
}

// TestDraftSaveOverwritesInsteadOfDuplicating: "save" twice must be one draft.
func TestDraftSaveOverwritesInsteadOfDuplicating(t *testing.T) {
	server, token, _, _ := draftTestServer(t, "vera_api_drafts_overwrite")

	draftID := saveDraft(t, server, token, draftSaveBody(`{"v":1}`, 1))
	body := draftSaveBody(`{"v":2}`, 3)
	body["id"] = draftID
	body["label"] = "Renamed"
	code, response := doJSON(t, server, http.MethodPost, "/v1/drafts", token, body)
	if code != http.StatusOK {
		t.Fatalf("second save = %d; want 200 (%v)", code, response)
	}
	if response["id"] != float64(draftID) {
		t.Errorf("second save created draft %v; want the same %d", response["id"], draftID)
	}

	code, list := doJSON(t, server, http.MethodGet, "/v1/drafts", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d; want 200", code)
	}
	drafts, _ := list["drafts"].([]any)
	if len(drafts) != 1 {
		t.Fatalf("owner holds %d drafts after saving twice; want 1", len(drafts))
	}
	row, _ := drafts[0].(map[string]any)
	if row["label"] != "Renamed" || row["itemCount"] != 3.0 {
		t.Errorf("listed = %v; want the second save's state", row)
	}
}

// TestExportConsumesTheDraft: an exported draft has become a quotation and has
// nothing left to be.
func TestExportConsumesTheDraft(t *testing.T) {
	server, token, _, _ := draftTestServer(t, "vera_api_drafts_export")
	attachedTestSigner(t, server, "vera_api_drafts_export")

	draftID := saveDraft(t, server, token, draftSaveBody(`{"v":1}`, 1))

	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input":   quotationInput(),
		"draftId": draftID,
	})
	if code != http.StatusCreated {
		t.Fatalf("export = %d; want 201 (%v)", code, body)
	}

	code, list := doJSON(t, server, http.MethodGet, "/v1/drafts", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d; want 200", code)
	}
	drafts, _ := list["drafts"].([]any)
	if len(drafts) != 0 {
		t.Errorf("%d drafts survived the export; want 0", len(drafts))
	}
}

// TestFailedExportKeepsTheDraft is the other half of that: losing work to a
// part-finished export would be the worst possible bug in this feature.
func TestFailedExportKeepsTheDraft(t *testing.T) {
	server, token, _, _ := draftTestServer(t, "vera_api_drafts_export_fail")

	draftID := saveDraft(t, server, token, draftSaveBody(`{"v":1}`, 1))

	// An empty item list fails Prepare, so nothing is stored.
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input":   map[string]any{"Subject": "No items", "CustomerID": 2},
		"draftId": draftID,
	})
	if code == http.StatusCreated {
		t.Fatalf("export of an empty quotation succeeded; the test proves nothing (%v)", body)
	}

	code, list := doJSON(t, server, http.MethodGet, "/v1/drafts", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d; want 200", code)
	}
	drafts, _ := list["drafts"].([]any)
	if len(drafts) != 1 {
		t.Fatalf("a failed export took the draft with it; want it kept")
	}
}

// TestDraftsStoreUnvalidatedInput: saving is a scratchpad. Input that would
// never survive Prepare still saves, because refusing is the one thing a
// draft cannot do.
func TestDraftsStoreUnvalidatedInput(t *testing.T) {
	server, token, _, _ := draftTestServer(t, "vera_api_drafts_raw")

	draftID := saveDraft(t, server, token, draftSaveBody(
		`{"Items":"not an array","qty":"","marginPercent":"NaN","subject":null}`, 0))

	code, body := doJSON(t, server, http.MethodGet, "/v1/drafts/"+strconv.FormatInt(draftID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("get = %d; want 200 (%v)", code, body)
	}
	stored, _ := body["payload"].(map[string]any)
	if stored["Items"] != "not an array" || stored["qty"] != "" {
		t.Errorf("payload = %v; want the half-typed state back unchanged", body["payload"])
	}
}

// TestDraftRejectsUnknownClient guards a dangling reference: the editor
// resolves the client by id, so a draft must not pin one that is gone.
func TestDraftRejectsUnknownClient(t *testing.T) {
	server, token, _, _ := draftTestServer(t, "vera_api_drafts_client")

	body := draftSaveBody(`{"v":1}`, 1)
	body["customerId"] = 987654
	code, response := doJSON(t, server, http.MethodPost, "/v1/drafts", token, body)
	if code != http.StatusBadRequest {
		t.Fatalf("save with an unknown client = %d; want 400 (%v)", code, response)
	}
	if !strings.Contains(response["error"].(string), "client") {
		t.Errorf("error = %v; want it to name the client as the problem", response["error"])
	}
}
