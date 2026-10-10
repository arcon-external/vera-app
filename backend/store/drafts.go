package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrDraftNotFound is returned when a draft does not exist, or does not belong
// to the caller. The two cases are deliberately indistinguishable: telling a
// caller that a draft id exists but is not theirs confirms the id against the
// owner's data.
var ErrDraftNotFound = errors.New("draft not found")

// ErrDraftCustomerUnknown is returned when a draft references a client that is
// not in the directory. The reference is kept only as a label, so a deleted
// client must not be silently resaved onto a draft.
var ErrDraftCustomerUnknown = errors.New("client not found")

// Draft is a saved, unexported quotation: editor state plus the labels needed
// to list it without decoding the payload.
type Draft struct {
	UpdatedAt    time.Time
	CreatedAt    time.Time
	Payload      json.RawMessage
	Label        string
	Subject      string
	CustomerName string
	ID           int64
	CustomerID   *int64
	ItemCount    int
}

// DraftSummary is one row of the drafts list. The payload is deliberately
// absent: listing a page of drafts should not pull every quotation's editor
// state through the wire.
type DraftSummary struct {
	UpdatedAt    time.Time
	Label        string
	Subject      string
	CustomerName string
	ID           int64
	ItemCount    int
}

// CreateDraft stores editor state as a new draft owned by userID.
func (s *Store) CreateDraft(
	ctx context.Context,
	userID int64,
	label, subject, customerName string,
	customerID *int64,
	itemCount int,
	payload json.RawMessage,
) (Draft, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return Draft{}, err
	}
	if len(payload) == 0 {
		return Draft{}, fmt.Errorf("draft payload is required")
	}
	if itemCount < 0 {
		return Draft{}, fmt.Errorf("draft item count cannot be negative")
	}
	// The customer reference is display-only, but a draft must not outlive a
	// pointer to a client row that is gone: the editor resolves the client by
	// id, and a dangling id would fail on resume rather than on save.
	if customerID != nil {
		if err := s.requireClient(ctx, *customerID); err != nil {
			return Draft{}, err
		}
	}

	var draft Draft
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_drafts
		       (user_id, label, subject, customer_name, customer_id, item_count, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, label, subject, customer_name, customer_id, item_count,
		          payload, created_at, updated_at`, s.configSchema()),
		userID, label, subject, customerName, customerID, itemCount, payload).
		Scan(&draft.ID, &draft.Label, &draft.Subject, &draft.CustomerName,
			&draft.CustomerID, &draft.ItemCount, &draft.Payload,
			&draft.CreatedAt, &draft.UpdatedAt)
	if err != nil {
		return Draft{}, fmt.Errorf("create draft: %w", err)
	}
	return draft, nil
}

// UpdateDraft replaces the state of a draft the caller owns. Saving again
// must not create a second row, or a user who saves twenty times ends up with
// twenty drafts to tidy up by hand.
func (s *Store) UpdateDraft(
	ctx context.Context,
	draftID, userID int64,
	label, subject, customerName string,
	customerID *int64,
	itemCount int,
	payload json.RawMessage,
) (Draft, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return Draft{}, err
	}
	if len(payload) == 0 {
		return Draft{}, fmt.Errorf("draft payload is required")
	}
	if itemCount < 0 {
		return Draft{}, fmt.Errorf("draft item count cannot be negative")
	}
	if customerID != nil {
		if err := s.requireClient(ctx, *customerID); err != nil {
			return Draft{}, err
		}
	}

	var draft Draft
	// user_id is in the WHERE clause, not just the SET list: ownership is the
	// access model, and it has to be enforced by the row that is written.
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_drafts
		SET label = $3, subject = $4, customer_name = $5, customer_id = $6,
		    item_count = $7, payload = $8, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, label, subject, customer_name, customer_id, item_count,
		          payload, created_at, updated_at`, s.configSchema()),
		draftID, userID, label, subject, customerName, customerID, itemCount, payload).
		Scan(&draft.ID, &draft.Label, &draft.Subject, &draft.CustomerName,
			&draft.CustomerID, &draft.ItemCount, &draft.Payload,
			&draft.CreatedAt, &draft.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, ErrDraftNotFound
	}
	if err != nil {
		return Draft{}, fmt.Errorf("update draft: %w", err)
	}
	return draft, nil
}

// ListDrafts returns the caller's own drafts, most recently changed first.
func (s *Store) ListDrafts(ctx context.Context, userID int64) ([]DraftSummary, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT id, label, subject, customer_name, item_count, updated_at
		FROM %s.quotation_drafts
		WHERE user_id = $1
		ORDER BY updated_at DESC, id DESC`, s.configSchema()), userID)
	if err != nil {
		return nil, fmt.Errorf("list drafts: %w", err)
	}
	defer rows.Close()

	drafts := make([]DraftSummary, 0, 16)
	for rows.Next() {
		var draft DraftSummary
		if err := rows.Scan(&draft.ID, &draft.Label, &draft.Subject,
			&draft.CustomerName, &draft.ItemCount, &draft.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan draft summary: %w", err)
		}
		drafts = append(drafts, draft)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate drafts: %w", err)
	}
	return drafts, nil
}

// GetDraft returns one draft the caller owns, payload included.
func (s *Store) GetDraft(ctx context.Context, draftID, userID int64) (Draft, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return Draft{}, err
	}
	var draft Draft
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, label, subject, customer_name, customer_id, item_count,
		       payload, created_at, updated_at
		FROM %s.quotation_drafts
		WHERE id = $1 AND user_id = $2`, s.configSchema()),
		draftID, userID).Scan(&draft.ID, &draft.Label, &draft.Subject,
		&draft.CustomerName, &draft.CustomerID, &draft.ItemCount, &draft.Payload,
		&draft.CreatedAt, &draft.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, ErrDraftNotFound
	}
	if err != nil {
		return Draft{}, fmt.Errorf("get draft: %w", err)
	}
	return draft, nil
}

// DeleteDraft removes one draft the caller owns. Deleting someone else's
// returns ErrDraftNotFound rather than an error about permissions, so an id
// belonging to another account is not confirmed by a different response.
func (s *Store) DeleteDraft(ctx context.Context, draftID, userID int64) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s.quotation_drafts WHERE id = $1 AND user_id = $2`,
		s.configSchema()), draftID, userID)
	if err != nil {
		return fmt.Errorf("delete draft: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrDraftNotFound
	}
	return nil
}

// DeleteUserDrafts removes every draft belonging to a user. Account deletion
// is a soft delete, so the cascade in 019 never fires for the normal path and
// this has to be called explicitly: a deleted account must not leave a
// recoverable copy of work that was never exported.
func (s *Store) DeleteUserDrafts(ctx context.Context, userID int64) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.quotation_drafts WHERE user_id = $1`, s.configSchema()),
		userID); err != nil {
		return fmt.Errorf("delete user drafts: %w", err)
	}
	return nil
}

// requireClient confirms a draft's client reference names a client.
//
// The value is the client's CODE, not its row id. It is the same number the
// editor's dropdown holds and the same one Prepare resolves through
// GetCustomerByCode, and the two must agree. Checking it against customers.id
// instead rejected a client whose code and id did not coincide -- telling the
// user their client was "no longer in the directory" while it sat in the
// dropdown in front of them -- and, worse, silently accepted a code that
// happened to match some other client's id, storing a reference to a client
// nobody chose.
func (s *Store) requireClient(ctx context.Context, customerCode int64) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	var exists bool
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT EXISTS (SELECT 1 FROM %s.customers WHERE customer_code = $1)", s.configSchema()),
		customerCode).Scan(&exists); err != nil {
		return fmt.Errorf("check client %d: %w", customerCode, err)
	}
	if !exists {
		return fmt.Errorf("%w: client %d", ErrDraftCustomerUnknown, customerCode)
	}
	return nil
}

// DraftDisplayName is the title a draft is listed under: the user's own label
// if they gave one, else the subject, else the client, else a placeholder.
// A draft with nothing filled in yet is exactly what someone is most likely to
// have saved, so it must still read as a row rather than a blank.
func DraftDisplayName(label, subject, customerName string) string {
	for _, candidate := range []string{label, subject, customerName} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return "Untitled draft"
}
