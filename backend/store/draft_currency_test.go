package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestDraftCurrencyTravelsWithTheDraft pins the B4 contract: the currency is
// stamped at creation, read back on load, and never moved by an update. The
// editor compares it against the live company profile before opening, so a
// draft saved in IDR that is opened under USD blocks instead of silently
// reinterpreting every stored integer.
func TestDraftCurrencyTravelsWithTheDraft(t *testing.T) {
	ctx := context.Background()
	database := draftTestStore(t)
	owner := draftTestUser(t, database, "draft-a@example.com")

	draft, err := database.CreateDraft(ctx, owner.ID, "USD draft", "Subject", "Client",
		nil, 1, "USD", json.RawMessage(`{"v":1}`))
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if draft.Currency != "USD" {
		t.Errorf("created currency = %q; want USD", draft.Currency)
	}

	loaded, err := database.GetDraft(ctx, draft.ID, owner.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	if loaded.Currency != "USD" {
		t.Errorf("loaded currency = %q; want USD", loaded.Currency)
	}

	// An update under a new company currency must not relabel old integers:
	// the stamp stays what creation wrote.
	updated, err := database.UpdateDraft(ctx, draft.ID, owner.ID, "USD draft",
		"Subject", "Client", nil, 1, json.RawMessage(`{"v":2}`))
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if updated.Currency != "USD" {
		t.Errorf("updated currency = %q; want the original USD stamp", updated.Currency)
	}
}

// TestDraftCurrencyDefaultsToIDR covers drafts that predate the column and
// callers that pass nothing: the migration default and the empty-write
// default agree, so both read back as what the company currency can only
// have been when no other code existed.
func TestDraftCurrencyDefaultsToIDR(t *testing.T) {
	ctx := context.Background()
	database := draftTestStore(t)
	owner := draftTestUser(t, database, "draft-a@example.com")

	for _, code := range []string{"", "  ", "idr", "Idr "} {
		draft, err := database.CreateDraft(ctx, owner.ID, "", "", "", nil, 0,
			code, json.RawMessage(`{"v":1}`))
		if err != nil {
			t.Fatalf("create draft with currency %q: %v", code, err)
		}
		if draft.Currency != "IDR" {
			t.Errorf("created currency = %q; want IDR for %q", draft.Currency, code)
		}
		loaded, err := database.GetDraft(ctx, draft.ID, owner.ID)
		if err != nil {
			t.Fatalf("get draft: %v", err)
		}
		if loaded.Currency != "IDR" {
			t.Errorf("loaded currency = %q; want IDR for %q", loaded.Currency, code)
		}
	}
}

// TestDraftCurrencyRejectsUnknownCodes is the catalogue validation on write:
// anything outside the shipped set fails naming the field, like the company
// profile's own check, rather than storing a code nothing can format.
func TestDraftCurrencyRejectsUnknownCodes(t *testing.T) {
	ctx := context.Background()
	database := draftTestStore(t)
	owner := draftTestUser(t, database, "draft-a@example.com")

	if _, err := database.CreateDraft(ctx, owner.ID, "", "", "", nil, 0,
		"XX", json.RawMessage(`{"v":1}`)); err == nil {
		t.Fatal("create draft with currency XX succeeded; want rejection")
	} else if !strings.Contains(err.Error(), "draft currency") {
		t.Errorf("error = %v; want it to name the draft currency as the problem", err)
	}
}
