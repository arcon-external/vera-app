package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PO reconciliation history (#502, ticket 06 / #518 B1).
//
// A saved record keeps every generation it ever reconciled against: the active
// one and the ones a source repair or a relink retired. LoadPOReconciliation
// answers only for the active session because that is the live workbench; this
// read is the archive, so it returns all of them in one consistent snapshot.
//
// The snapshot is repeatable-read on purpose. A repair commits a PO version
// bump, a deactivation and a new generation together, so a read that took the
// PO row and then the generations at two different instants could show a
// current version that belongs to a generation the caller never receives (or
// the reverse). One transaction, one snapshot, one answer.

// POReconciliationSessions is every reconciliation generation recorded for one
// purchase order, newest first.
type POReconciliationSessions struct {
	PurchaseOrderID int64
	// POVersion is the record's current optimistic-concurrency version at this
	// read. A nested Session.POVersion is the stored audit version it was
	// recorded at; they are expected to differ after a correction.
	POVersion int64
	// Sessions is never nil: a known record with no session is an empty slice.
	Sessions []POReconciliationSessionRecord
}

// POReconciliationSessionRecord is one generation with the context the archive
// needs to render it without pretending it is current.
type POReconciliationSessionRecord struct {
	Generation int
	// SourceAvailable reports whether the exact revision row still exists. The
	// immutable source pair and the retained items are readable either way.
	SourceAvailable bool
	// RepairCorrectionID links the generation to the source repair that
	// installed it, and therefore to a GetPurchaseOrderHistory row. Null for
	// create/link generations and for every session written before #518.
	RepairCorrectionID *int64
	Session            POReconciliationSession
}

// ListPOReconciliationSessions returns the current PO version and every
// reconciliation generation for one record, sorted by generation and ID
// descending. It is a read: no version argument, no source prerequisite, and
// no write of any kind.
func (s *Store) ListPOReconciliationSessions(ctx context.Context, poID int64) (POReconciliationSessions, error) {
	if poID < 1 {
		return POReconciliationSessions{}, ErrPurchaseOrderNotFound
	}
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return POReconciliationSessions{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return POReconciliationSessions{}, err
	}
	schema := s.configSchema()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return POReconciliationSessions{}, fmt.Errorf("begin reconciliation history read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var version int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT version FROM %s.purchase_orders WHERE id = $1 AND org_id = $2`, schema),
		poID, org.ID).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return POReconciliationSessions{}, ErrPurchaseOrderNotFound
		}
		return POReconciliationSessions{}, fmt.Errorf("read purchase order %d for reconciliation history: %w", poID, err)
	}

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT s.id, s.source_quotation_id, s.source_revision,
		       s.source_revision_id IS NOT NULL,
		       s.generation, s.active, s.readonly, s.po_version,
		       s.created_by, s.created_at, s.updated_at,
		       s.line_identities, s.repair_correction_id
		FROM %s.purchase_order_reconciliation_sessions s
		WHERE s.purchase_order_id = $1 AND s.org_id = $2
		ORDER BY s.generation DESC, s.id DESC`, schema), poID, org.ID)
	if err != nil {
		return POReconciliationSessions{}, fmt.Errorf("read reconciliation generations for purchase order %d: %w", poID, err)
	}
	result := POReconciliationSessions{
		PurchaseOrderID: poID,
		POVersion:       version,
		Sessions:        []POReconciliationSessionRecord{},
	}
	for rows.Next() {
		var record POReconciliationSessionRecord
		var session POReconciliationSession
		var sourceRevision int16
		var createdAt, updatedAt time.Time
		var lineIdentitiesRaw []byte
		if err := rows.Scan(&session.ID, &session.SourceQuotationID, &sourceRevision,
			&record.SourceAvailable, &record.Generation, &session.Active,
			&session.ReadOnly, &session.POVersion, &session.CreatedBy,
			&createdAt, &updatedAt, &lineIdentitiesRaw,
			&record.RepairCorrectionID); err != nil {
			rows.Close()
			return POReconciliationSessions{}, fmt.Errorf("scan reconciliation generation for purchase order %d: %w", poID, err)
		}
		session.PurchaseOrderID = poID
		session.SourceRevision = sourceRevision
		session.CreatedAt = createdAt.UTC()
		session.UpdatedAt = updatedAt.UTC()
		if len(lineIdentitiesRaw) > 0 {
			if err := decodeReconciliationLineIdentities(lineIdentitiesRaw, &session); err != nil {
				rows.Close()
				return POReconciliationSessions{}, fmt.Errorf("decode reconciliation line identities for purchase order %d: %w", poID, err)
			}
		}
		record.Session = session
		result.Sessions = append(result.Sessions, record)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return POReconciliationSessions{}, fmt.Errorf("iterate reconciliation generations for purchase order %d: %w", poID, err)
	}
	rows.Close()

	// Items are read inside the same snapshot; one query per generation keeps
	// the existing scan and value-normalisation path in one place.
	for index := range result.Sessions {
		items, err := loadReconciliationItems(ctx, tx, schema, org.ID, result.Sessions[index].Session.ID)
		if err != nil {
			return POReconciliationSessions{}, err
		}
		result.Sessions[index].Session.Items = items
		normalizeReconciliationSlices(&result.Sessions[index].Session)
	}

	if err := tx.Commit(ctx); err != nil {
		return POReconciliationSessions{}, fmt.Errorf("close reconciliation history read: %w", err)
	}
	return result, nil
}

// decodeReconciliationLineIdentities is the one decode path both the active
// load and the history read use, so a malformed row fails the same way.
func decodeReconciliationLineIdentities(raw []byte, session *POReconciliationSession) error {
	if len(raw) == 0 {
		session.LineIdentities = []POReconciliationLineIdentity{}
		return nil
	}
	if err := json.Unmarshal(raw, &session.LineIdentities); err != nil {
		return err
	}
	if session.LineIdentities == nil {
		session.LineIdentities = []POReconciliationLineIdentity{}
	}
	return nil
}
