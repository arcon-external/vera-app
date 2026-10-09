package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// draftTestUser creates a throwaway account for the draft ownership tests.
// Drafts never log in, so any non-empty hash will do.
func draftTestUser(t *testing.T, database *Store, email string) AuthUser {
	t.Helper()
	user, err := database.CreateUser(context.Background(), email, "Draft Tester", "hash", "staff")
	if err != nil {
		t.Fatalf("create draft test user %s: %v", email, err)
	}
	t.Cleanup(func() { deleteTestUser(t, database, user.ID) })
	return user
}

// deleteTestUser removes a throwaway account. DeleteUser refuses an active
// user, so the deactivation the real flow does has to happen first.
func deleteTestUser(t *testing.T, database *Store, userID int64) {
	t.Helper()
	ctx := context.Background()
	if err := database.SetUserActive(ctx, userID, false); err != nil {
		t.Logf("deactivate test user: %v", err)
		return
	}
	if _, err := database.DeleteUser(ctx, userID); err != nil {
		t.Logf("delete test user: %v", err)
	}
}

func draftTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	database := openTestSchema(t, "vera_test_drafts")
	t.Cleanup(database.Close)
	// A warm database from a previous run would collide on the user emails.
	for _, email := range []string{"draft-a@example.com", "draft-b@example.com", "draft-admin@example.com"} {
		if existing, err := database.GetUserByEmail(context.Background(), email); err == nil {
			deleteTestUser(t, database, existing.ID)
		}
	}
	return database
}

// TestDraftsArePrivateToTheirOwner is the test that matters: ownership is the
// entire access model, and it has to hold in SQL rather than wherever a
// handler happened to remember to check.
func TestDraftsArePrivateToTheirOwner(t *testing.T) {
	ctx := context.Background()
	database := draftTestStore(t)
	owner := draftTestUser(t, database, "draft-a@example.com")
	intruder := draftTestUser(t, database, "draft-b@example.com")

	payload := json.RawMessage(`{"Items":[{"Description":"Half-written line"}]}`)
	draft, err := database.CreateDraft(ctx, owner.ID, "Mine", "Roof works",
		"Test Customer", nil, 1, payload)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	t.Run("intruder cannot list the owner's draft", func(t *testing.T) {
		drafts, err := database.ListDrafts(ctx, intruder.ID)
		if err != nil {
			t.Fatalf("list drafts: %v", err)
		}
		for _, listed := range drafts {
			if listed.ID == draft.ID {
				t.Fatalf("another user's draft appeared in the list: %+v", listed)
			}
		}
	})

	t.Run("intruder cannot read it", func(t *testing.T) {
		if _, err := database.GetDraft(ctx, draft.ID, intruder.ID); !errors.Is(err, ErrDraftNotFound) {
			t.Fatalf("get as intruder = %v; want ErrDraftNotFound", err)
		}
	})

	t.Run("intruder cannot overwrite it", func(t *testing.T) {
		_, err := database.UpdateDraft(ctx, draft.ID, intruder.ID, "Stolen", "", "", nil, 0,
			json.RawMessage(`{"Items":[]}`))
		if !errors.Is(err, ErrDraftNotFound) {
			t.Fatalf("update as intruder = %v; want ErrDraftNotFound", err)
		}
	})

	t.Run("intruder cannot delete it", func(t *testing.T) {
		if err := database.DeleteDraft(ctx, draft.ID, intruder.ID); !errors.Is(err, ErrDraftNotFound) {
			t.Fatalf("delete as intruder = %v; want ErrDraftNotFound", err)
		}
		if _, err := database.GetDraft(ctx, draft.ID, owner.ID); err != nil {
			t.Fatalf("owner lost the draft to an intruder's delete: %v", err)
		}
	})

	t.Run("an admin has no more reach than anyone else", func(t *testing.T) {
		// Promotion is asserted through the user row rather than a role
		// parameter: the point is that no role exists in the SQL at all.
		admin, err := database.CreateUser(ctx, "draft-admin@example.com", "Admin", "hash", "admin")
		if err != nil {
			t.Fatalf("create admin: %v", err)
		}
		defer deleteTestUser(t, database, admin.ID)
		if _, err := database.GetDraft(ctx, draft.ID, admin.ID); !errors.Is(err, ErrDraftNotFound) {
			t.Fatalf("admin read another user's draft: %v; want ErrDraftNotFound", err)
		}
		if err := database.DeleteDraft(ctx, draft.ID, admin.ID); !errors.Is(err, ErrDraftNotFound) {
			t.Fatalf("admin deleted another user's draft: %v; want ErrDraftNotFound", err)
		}
	})

	t.Run("the owner reads it back verbatim", func(t *testing.T) {
		loaded, err := database.GetDraft(ctx, draft.ID, owner.ID)
		if err != nil {
			t.Fatalf("get as owner: %v", err)
		}
		if loaded.Subject != "Roof works" || loaded.ItemCount != 1 {
			t.Errorf("loaded = %+v; want subject 'Roof works' and 1 item", loaded)
		}
		var decoded struct {
			Items []struct {
				Description string
			}
		}
		if err := json.Unmarshal(loaded.Payload, &decoded); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(decoded.Items) != 1 || decoded.Items[0].Description != "Half-written line" {
			t.Errorf("payload round trip = %+v; want the saved line back unchanged", decoded)
		}
	})
}

// TestDraftSaveUpdatesInPlace: pressing save twice must not leave two drafts.
func TestDraftSaveUpdatesInPlace(t *testing.T) {
	ctx := context.Background()
	database := draftTestStore(t)
	owner := draftTestUser(t, database, "draft-a@example.com")

	draft, err := database.CreateDraft(ctx, owner.ID, "First", "Subject", "Client", nil, 1,
		json.RawMessage(`{"v":1}`))
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	updated, err := database.UpdateDraft(ctx, draft.ID, owner.ID, "Second", "Other", "Client", nil, 4,
		json.RawMessage(`{"v":2}`))
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if !updated.CreatedAt.Equal(draft.CreatedAt) {
		t.Errorf("created_at moved on save: %v -> %v", draft.CreatedAt, updated.CreatedAt)
	}
	if updated.UpdatedAt.Before(draft.UpdatedAt) {
		t.Errorf("updated_at went backwards: %v -> %v", draft.UpdatedAt, updated.UpdatedAt)
	}

	drafts, err := database.ListDrafts(ctx, owner.ID)
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("owner holds %d drafts after saving twice; want 1", len(drafts))
	}
	if drafts[0].Label != "Second" || drafts[0].ItemCount != 4 {
		t.Errorf("listed = %+v; want the saved-through state", drafts[0])
	}
}

// TestDraftsStoreUnvalidatedState: a draft is a scratchpad. Half-typed,
// unbalanced, unpriceable state must survive a save/load round trip untouched,
// because being unable to save is the one thing a draft cannot do.
func TestDraftsStoreUnvalidatedState(t *testing.T) {
	ctx := context.Background()
	database := draftTestStore(t)
	owner := draftTestUser(t, database, "draft-a@example.com")

	garbage := json.RawMessage(`{"Items":"not an array","qty":"","margin":"NaN","notes":{"a":[1,2]}}`)
	draft, err := database.CreateDraft(ctx, owner.ID, "", "", "", nil, 0, garbage)
	if err != nil {
		t.Fatalf("saving a half-typed draft failed: %v", err)
	}
	loaded, err := database.GetDraft(ctx, draft.ID, owner.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	// JSONB reorders keys and normalises whitespace, so the guarantee is that
	// the editor state comes back identical, not that the bytes do.
	var stored, wanted map[string]any
	if err := json.Unmarshal(loaded.Payload, &stored); err != nil {
		t.Fatalf("decode stored payload %s: %v", loaded.Payload, err)
	}
	if err := json.Unmarshal(garbage, &wanted); err != nil {
		t.Fatalf("decode wanted payload: %v", err)
	}
	if !reflect.DeepEqual(stored, wanted) {
		t.Errorf("payload = %s; want %s back unchanged", loaded.Payload, garbage)
	}
	if name := DraftDisplayName(loaded.Label, loaded.Subject, loaded.CustomerName); name != "Untitled draft" {
		t.Errorf("untitled draft = %q; want a name rather than a blank row", name)
	}
}

func TestDraftRejectsUnknownClient(t *testing.T) {
	ctx := context.Background()
	database := draftTestStore(t)
	owner := draftTestUser(t, database, "draft-a@example.com")

	if _, err := database.CreateDraft(ctx, owner.ID, "", "", "Ghost", int64Pointer(987654),
		0, json.RawMessage(`{}`)); !errors.Is(err, ErrDraftCustomerUnknown) {
		t.Fatalf("create draft with unknown client = %v; want ErrDraftCustomerUnknown", err)
	}
}

// TestDraftsDieWithTheAccount: a deleted account must not leave a recoverable
// copy of work that was never exported.
func TestDraftsDieWithTheAccount(t *testing.T) {
	ctx := context.Background()
	database := draftTestStore(t)
	owner := draftTestUser(t, database, "draft-a@example.com")

	if _, err := database.CreateDraft(ctx, owner.ID, "Doomed", "Subject", "Client", nil, 1,
		json.RawMessage(`{"v":1}`)); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := database.DeleteUserDrafts(ctx, owner.ID); err != nil {
		t.Fatalf("delete user drafts: %v", err)
	}
	drafts, err := database.ListDrafts(ctx, owner.ID)
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	if len(drafts) != 0 {
		t.Fatalf("%d drafts survived account deletion; want 0", len(drafts))
	}
}

func int64Pointer(value int64) *int64 { return &value }
