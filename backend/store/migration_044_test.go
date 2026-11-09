package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration 044 installs the one construction window a saved record's
// reconciliation history is allowed to grow through (#518, B1).
//
// These tests pin the boundary, not the repair feature:
//
//   - a session row for a saved (or read-only) PO exists only because a real
//     source repair created it, inside the transaction that wrote the repair
//     audit and the new PO version;
//   - the terminal items of that session are written in the same transaction,
//     by the same actor, already resolved, and never again afterwards.
//
// Everything here drives raw SQL on purpose. The Go repair path is B2; these
// tests must survive any rewrite of it.

func isSQLState(t *testing.T, err error, code string) bool {
	t.Helper()
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != code {
		t.Logf("SQLSTATE = %s (%s); want %s", pgErr.Code, pgErr.Message, code)
		return false
	}
	return true
}

// po518HasColumn reports whether a migration has already added a column, so one
// test body can assert pre- and post-migration behaviour.
func po518HasColumn(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, schema, table, column string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2 AND column_name = $3)`,
		schema, table, column).Scan(&exists); err != nil {
		t.Fatalf("read column %s.%s: %v", table, column, err)
	}
	return exists
}

// po518Querier is the read side both a pool and a transaction satisfy.
type po518Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// po518RevisionID resolves the exact revision row for one source pair.
func po518RevisionID(t *testing.T, q po518Querier, schema string, quotationID int64, revision int16) int64 {
	t.Helper()
	var id int64
	if err := q.QueryRow(context.Background(), fmt.Sprintf(`
		SELECT id FROM %s.quotation_revisions
		WHERE quotation_id = $1 AND revision_number = $2`, schema),
		quotationID, revision).Scan(&id); err != nil {
		t.Fatalf("read revision %d/%d: %v", quotationID, revision, err)
	}
	return id
}

// po518SavedFixture finalizes one real PO from an issued revision, which leaves
// exactly one active, read-only reconciliation session for a saved record.
func po518SavedFixture(t *testing.T, schema, number string) (*Store, context.Context, PurchaseOrder, QuotationRecord, AuthUser, string, int64) {
	t.Helper()
	database := openTestSchema(t, schema)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "B1 guard")
	staff := poTestUser(t, database)
	saved := savedPurchaseOrder(t, database, ctx, staff, quotation, number)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var orgID string
	var revisionID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT po.org_id::text, r.id
		FROM %s.purchase_orders po
		JOIN %s.quotation_revisions r
		  ON r.quotation_id = po.quotation_id AND r.revision_number = po.source_revision
		WHERE po.id = $1`, database.configSchema(), database.configSchema()),
		saved.ID).Scan(&orgID, &revisionID); err != nil {
		t.Fatalf("read saved PO identity: %v", err)
	}
	return database, ctx, saved, quotation, staff, orgID, revisionID
}

// po518NextRevision issues revision 1 of the same quotation. A repair must move
// the link (036), and a revision move keeps the customer and tax basis while
// proving the new-pair checks without a second quotation's occupancy.
func po518NextRevision(t *testing.T, database *Store, first QuotationRecord) QuotationRecord {
	t.Helper()
	return po518RevisionN(t, database, first, 1)
}

// po518RevisionN issues revision n of the same quotation.
func po518RevisionN(t *testing.T, database *Store, first QuotationRecord, revision int16) QuotationRecord {
	t.Helper()
	record, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:     int16(2),
		CustomerName:     "PT. Buyer",
		CustomerAddress:  "Jl. Billing 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          "B1 guard",
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		TaxZone:          "FTZ",
		TaxType:          "Goods",
		Currency:         "USD",
		HTMLSnapshot:     fmt.Sprintf("<html>revision %d</html>", revision),
		QuoteNumber:      first.QuoteNumber,
		RevisionNumber:   revision,
		Items: []RevisionItem{
			{
				ItemNo:                1,
				Description:           "Priced widget",
				Qty:                   2,
				UoM:                   "Pc",
				UnitPrice:             100,
				OriginalTotal:         200,
				MarginalizedUnitPrice: 100,
				MarginalizedTotal:     200,
				ListUnitPrice:         100,
			},
			{
				ItemNo:                3,
				Description:           "Zero-value line",
				Qty:                   1,
				UoM:                   "Lot",
				UnitPrice:             0,
				OriginalTotal:         0,
				MarginalizedUnitPrice: 0,
				MarginalizedTotal:     0,
				ListUnitPrice:         0,
			},
		},
		Totals: RevisionTotals{
			OriginalSubtotal:          200,
			OriginalGrand:             200,
			MarginalizedSubtotal:      200,
			MarginalizedGrand:         200,
			MarginalizedGrandAfterTax: 200,
			CustomerGrandTotal:        200,
		},
	})
	if err != nil {
		t.Fatalf("issue revision %d: %v", revision, err)
	}
	return record
}

// po518Repair is the state a mid-transaction repair holds after moving the
// record: enough for any guard case to attempt the terminal write.
type po518Repair struct {
	Tx               pgx.Tx
	CorrectionID     int64
	Target           QuotationRecord
	TargetRevisionID int64
	Version          int64
	Generation       int
	Actor            AuthUser
	OrgID            string
	POID             int64
}

// po518BeginRepair performs the audit half of a real repair in one transaction
// and leaves the terminal session uninstalled, so a test can install it with
// exactly one thing wrong.
func po518BeginRepair(t *testing.T, database *Store, ctx context.Context, saved PurchaseOrder,
	target QuotationRecord, orgID string) po518Repair {
	return po518BeginRepairStamped(t, database, ctx, saved, target, orgID, nil)
}

// po518BeginRepairStamped also lets a test choose the transaction stamp the
// satellite carries. A nil override lets the column default write the current
// transaction; "" writes NULL; any other value is the literal stamp.
func po518BeginRepairStamped(t *testing.T, database *Store, ctx context.Context, saved PurchaseOrder,
	target QuotationRecord, orgID string, stamp *string) po518Repair {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	// An admin repair. The stored role is what the guard re-reads.
	admin := poRepairAdmin(t, database)
	correctionID, err := insertPurchaseOrderCorrection(ctx, tx, schema, saved, orgID,
		PurchaseOrderCorrectionKindSourceRepair, "bootstrap proof", admin.ID)
	if err != nil {
		t.Fatalf("correction snapshot: %v", err)
	}
	source, err := loadPurchaseOrderSourceDetail(ctx, tx, schema,
		target.ID, target.RevisionNumber)
	if err != nil {
		t.Fatalf("load target source: %v", err)
	}
	if stamp == nil {
		if err := insertPurchaseOrderSourceRepair(ctx, tx, schema, correctionID, saved,
			source, PurchaseOrderComparison{}, nil); err != nil {
			t.Fatalf("source repair satellite: %v", err)
		}
	} else {
		var stampArg any
		if *stamp != "" {
			stampArg = *stamp
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_source_repairs (
				correction_id, previous_quotation_id, previous_source_revision,
				previous_quote_number, previous_customer_id, previous_customer_name,
				new_quotation_id, new_source_revision, new_quote_number,
				new_customer_id, new_customer_name, reconciliation_seed_xid
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, schema),
			correctionID, saved.QuotationID, saved.SourceRevision,
			saved.QuoteNumber, saved.CustomerID, saved.CustomerName,
			source.QuotationID, source.RevisionNumber, source.QuoteNumber,
			source.CustomerID, source.CustomerName, stampArg); err != nil {
			t.Fatalf("source repair satellite with stamp: %v", err)
		}
	}
	var version int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET quotation_id = $3, source_revision = $4, updated_by = $5, updated_at = NOW()
		WHERE id = $1 AND org_id = $2
		RETURNING version`, schema),
		saved.ID, orgID, target.ID, target.RevisionNumber, admin.ID).Scan(&version); err != nil {
		t.Fatalf("advance saved PO version: %v", err)
	}
	if err := deactivateReconciliationSessionsTx(ctx, tx, schema, orgID, saved.ID); err != nil {
		t.Fatalf("deactivate old generation: %v", err)
	}
	var generation int
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(MAX(generation), 0) + 1
		FROM %s.purchase_order_reconciliation_sessions
		WHERE purchase_order_id = $1`, schema), saved.ID).Scan(&generation); err != nil {
		t.Fatalf("next generation: %v", err)
	}
	return po518Repair{
		Tx:               tx,
		CorrectionID:     correctionID,
		Target:           target,
		TargetRevisionID: po518RevisionID(t, tx, schema, target.ID, target.RevisionNumber),
		Version:          version,
		Generation:       generation,
		Actor:            admin,
		OrgID:            orgID,
		POID:             saved.ID,
	}
}

// po518SessionParams is one attempt to install a terminal session. A nil pointer
// means "the genuine value"; tests override one field at a time.
type po518SessionParams struct {
	QuotationID        *int64
	Revision           *int16
	RevisionID         *int64
	NullRevisionID     bool
	POVersion          *int64
	CreatedBy          *int64
	Active             *bool
	ReadOnly           *bool
	RepairCorrectionID *int64
	NullRepairLink     bool
}

func po518InstallSession(ctx context.Context, schema string, q po518Querier,
	repair po518Repair, p po518SessionParams) (int64, error) {
	quotationID := repair.Target.ID
	if p.QuotationID != nil {
		quotationID = *p.QuotationID
	}
	revision := repair.Target.RevisionNumber
	if p.Revision != nil {
		revision = *p.Revision
	}
	var revisionID any = repair.TargetRevisionID
	if p.NullRevisionID {
		revisionID = nil
	} else if p.RevisionID != nil {
		revisionID = *p.RevisionID
	}
	version := repair.Version
	if p.POVersion != nil {
		version = *p.POVersion
	}
	createdBy := repair.Actor.ID
	if p.CreatedBy != nil {
		createdBy = *p.CreatedBy
	}
	active := true
	if p.Active != nil {
		active = *p.Active
	}
	readOnly := true
	if p.ReadOnly != nil {
		readOnly = *p.ReadOnly
	}
	var correctionID any = repair.CorrectionID
	if p.NullRepairLink {
		correctionID = nil
	} else if p.RepairCorrectionID != nil {
		correctionID = *p.RepairCorrectionID
	}
	var sessionID int64
	err := q.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_sessions (
			org_id, purchase_order_id, source_quotation_id, source_revision,
			source_revision_id, generation, active, readonly, po_version,
			line_identities, created_by, repair_correction_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'[]',$10,$11)
		RETURNING id`, schema),
		repair.OrgID, repair.POID, quotationID, revision, revisionID,
		repair.Generation, active, readOnly, version, createdBy, correctionID).Scan(&sessionID)
	return sessionID, err
}

func po518InstallItem(ctx context.Context, schema string, q po518Querier,
	repair po518Repair, sessionID int64, actorID int64, status, resolution string) error {
	var resolutionArg any
	if resolution != "" {
		resolutionArg = resolution
	}
	var actorArg any
	if actorID > 0 {
		actorArg = actorID
	}
	_, err := q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_items (
			org_id, session_id, purchase_order_id, key, field, kind,
			label, po_value, source_value, status, resolution, actor_id, history
		) VALUES ($1,$2,$3,'header:paymentTerms','header:paymentTerms','difference',
			'Payment terms','"Net 30"','"Net 60"',$4,$5,$6,
			jsonb_build_array(jsonb_build_object(
				'Kind','difference','SourceLineRef',NULL,
				'POValue','"Net 30"'::jsonb,'SourceValue','"Net 60"'::jsonb,
				'Status',$4::text,'Resolution',$5::text,'ActorID',$6::bigint,
				'At','2026-11-08T00:00:00Z')))`, schema),
		repair.OrgID, sessionID, repair.POID, status, resolutionArg, actorArg)
	return err
}

// TestMigration044SavedParentSessionInsertRequiresRepairSeed is B1's first red
// proof. At ecbd3b3 migration 041's session INSERT guard only re-checks that the
// source revision exists, so a hand-written session row for a saved PO is
// accepted. After 044 the same statement must raise 23514.
func TestMigration044SavedParentSessionInsertRequiresRepairSeed(t *testing.T) {
	database, ctx, saved, _, staff, orgID, revisionID := po518SavedFixture(
		t, "vera_po_518_guard", "PO-518-GUARD")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	var active, readOnly int
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FILTER (WHERE active), COUNT(*) FILTER (WHERE readonly)
		FROM %s.purchase_order_reconciliation_sessions
		WHERE purchase_order_id = $1`, schema), saved.ID).Scan(&active, &readOnly); err != nil {
		t.Fatalf("read seeded session: %v", err)
	}
	if active != 1 || readOnly != 1 {
		t.Fatalf("seeded sessions = %d active / %d read-only; want 1/1", active, readOnly)
	}
	var generation int
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(MAX(generation), 0) + 1
		FROM %s.purchase_order_reconciliation_sessions
		WHERE purchase_order_id = $1`, schema), saved.ID).Scan(&generation); err != nil {
		t.Fatalf("next generation: %v", err)
	}

	// A forged historical generation: inactive so it cannot collide with the
	// one-active-per-PO index, read-only, with a real source pointer and a real
	// actor. It is exactly what the history read would otherwise surface as a
	// truthful generation.
	_, err = pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_sessions (
			org_id, purchase_order_id, source_quotation_id, source_revision,
			source_revision_id, generation, active, readonly, po_version,
			line_identities, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,FALSE,TRUE,$7,'[]',$8)`, schema),
		orgID, saved.ID, saved.QuotationID, saved.SourceRevision, revisionID,
		generation, saved.Version, staff.ID)
	if err == nil {
		t.Fatalf("a raw session INSERT for saved PO %d succeeded; migration 044's saved-parent guard is missing", saved.ID)
	}
	if !isSQLState(t, err, "23514") {
		t.Fatalf("raw saved-parent session INSERT error = %v; want SQLSTATE 23514", err)
	}
}

// TestMigration044SeedRefusals holds the predicate's shape: every near-miss of
// a genuine repair is refused, not merely the obvious forgery.
func TestMigration044SeedRefusals(t *testing.T) {
	database, ctx, saved, seed, staff, orgID, _ := po518SavedFixture(
		t, "vera_po_518_seed", "PO-518-SEED")
	schema := database.configSchema()
	target := po518NextRevision(t, database, seed)

	cases := []struct {
		name  string
		stamp *string
		build func(t *testing.T, repair po518Repair) po518SessionParams
	}{
		{name: "no repair link", build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{NullRepairLink: true}
		}},
		{name: "null source pointer", build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{NullRevisionID: true}
		}},
		{name: "unknown source pointer", build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{RevisionID: ptrInt64(999999)}
		}},
		{name: "another actor", build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{CreatedBy: &staff.ID}
		}},
		{name: "not read-only", build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{ReadOnly: ptrBool(false)}
		}},
		{name: "not active", build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{Active: ptrBool(false)}
		}},
		{name: "wrong version", build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{POVersion: ptrInt64(repair.Version + 1)}
		}},
		{name: "wrong source revision", build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{Revision: ptrInt16(0)}
		}},
		{name: "stale transaction stamp", stamp: strPtr("1"), build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{}
		}},
		{name: "null transaction stamp", stamp: strPtr(""), build: func(t *testing.T, repair po518Repair) po518SessionParams {
			return po518SessionParams{}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repair := po518BeginRepairStamped(t, database, ctx, saved, target, orgID, tc.stamp)
			params := tc.build(t, repair)
			if _, err := po518InstallSession(ctx, schema, repair.Tx, repair, params); err == nil {
				t.Fatalf("a near-miss repair seed was accepted")
			} else if !isSQLState(t, err, "23514") {
				t.Fatalf("seed refusal error = %v; want SQLSTATE 23514", err)
			}
		})
	}
}

// TestMigration044OrdinaryCorrectionCannotSeed proves the link is not enough:
// the audit must be a source repair, so an ordinary correction of the saved
// record cannot install a session even in its own transaction.
func TestMigration044OrdinaryCorrectionCannotSeed(t *testing.T) {
	database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
		t, "vera_po_518_kind", "PO-518-KIND")
	target_record := po518NextRevision(t, database, seed)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	admin := poRepairAdmin(t, database)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	correctionID, err := insertPurchaseOrderCorrection(ctx, tx, schema, saved, orgID,
		PurchaseOrderCorrectionKindCorrection, "ordinary correction", admin.ID)
	if err != nil {
		t.Fatalf("correction snapshot: %v", err)
	}
	source, err := loadPurchaseOrderSourceDetail(ctx, tx, schema, target_record.ID, target_record.RevisionNumber)
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	if err := insertPurchaseOrderSourceRepair(ctx, tx, schema, correctionID, saved,
		source, PurchaseOrderComparison{}, nil); err != nil {
		t.Fatalf("satellite: %v", err)
	}
	var version int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET quotation_id = $3, source_revision = $4, updated_by = $5, updated_at = NOW()
		WHERE id = $1 AND org_id = $2 RETURNING version`, schema),
		saved.ID, orgID, target_record.ID, target_record.RevisionNumber, admin.ID).Scan(&version); err != nil {
		t.Fatalf("bump version: %v", err)
	}
	if err := deactivateReconciliationSessionsTx(ctx, tx, schema, orgID, saved.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	var generation int
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(MAX(generation), 0) + 1 FROM %s.purchase_order_reconciliation_sessions
		WHERE purchase_order_id = $1`, schema), saved.ID).Scan(&generation); err != nil {
		t.Fatalf("generation: %v", err)
	}
	repair := po518Repair{
		Tx:               tx,
		CorrectionID:     correctionID,
		Target:           target_record,
		TargetRevisionID: po518RevisionID(t, tx, schema, target_record.ID, target_record.RevisionNumber),
		Version:          version,
		Generation:       generation,
		Actor:            admin,
		OrgID:            orgID,
		POID:             saved.ID,
	}
	if _, err := po518InstallSession(ctx, schema, tx, repair, po518SessionParams{}); err == nil {
		t.Fatalf("an ordinary correction installed a terminal session")
	} else if !isSQLState(t, err, "23514") {
		t.Fatalf("ordinary-correction seed error = %v; want 23514", err)
	}
}

// TestMigration044ItemBootstrapInsideRepairTransaction is the item half of the
// red proof. At ecbd3b3 the session insert above is accepted and the item
// insert is then refused by 041 because the session is read-only. After 044 the
// same repair transaction may write its terminal items once, and a later
// transaction may not.
func TestMigration044ItemBootstrapInsideRepairTransaction(t *testing.T) {
	database, ctx, saved, seed, staff, orgID, _ := po518SavedFixture(
		t, "vera_po_518_bootstrap", "PO-518-BOOT")
	schema := database.configSchema()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	repair := po518BeginRepair(t, database, ctx, saved,
		po518NextRevision(t, database, seed), orgID)

	// `repair_correction_id` only exists once 044 has been applied. The column
	// is optional in this INSERT so the *assertion* below is the same on both
	// sides of the migration: before 044 the terminal item is refused by 041
	// (this test fails), after 044 it is accepted inside the repair transaction.
	columns := `org_id, purchase_order_id, source_quotation_id, source_revision,
			source_revision_id, generation, active, readonly, po_version,
			line_identities, created_by`
	values := `$1,$2,$3,$4,$5,$6,TRUE,TRUE,$7,'[]',$8`
	args := []any{repair.OrgID, repair.POID, repair.Target.ID,
		repair.Target.RevisionNumber, repair.TargetRevisionID,
		repair.Generation, repair.Version, repair.Actor.ID}
	if po518HasColumn(t, pool, database.config.Schema,
		"purchase_order_reconciliation_sessions", "repair_correction_id") {
		columns += `, repair_correction_id`
		values += `,$9`
		args = append(args, repair.CorrectionID)
	}
	var sessionID int64
	if err := repair.Tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_sessions (%s)
		VALUES (%s)
		RETURNING id`, schema, columns, values), args...).Scan(&sessionID); err != nil {
		t.Fatalf("install terminal session inside the repair transaction: %v", err)
	}
	if err := po518InstallItem(ctx, schema, repair.Tx, repair, sessionID,
		repair.Actor.ID, "resolved", "kept_po"); err != nil {
		t.Fatalf("terminal item insert inside the repair transaction: %v", err)
	}
	if err := repair.Tx.Commit(ctx); err != nil {
		t.Fatalf("commit repair fixture: %v", err)
	}

	// The construction window is this transaction only.
	if err := po518InstallItem(ctx, schema, pool, repair, sessionID,
		repair.Actor.ID, "resolved", "kept_po"); err == nil {
		t.Fatalf("a later transaction appended to a terminal session; the window did not close")
	} else if !isSQLState(t, err, "23514") {
		t.Fatalf("post-commit item insert error = %v; want SQLSTATE 23514", err)
	}

	// The seed authorises construction, not authorship: the same transaction
	// cannot borrow another actor or leave an item unresolved. Each case gets a
	// fresh record so it starts from a saved PO with a new target revision.
	for index, tc := range []struct {
		name       string
		actorID    int64
		status     string
		resolution string
	}{
		{name: "another actor", actorID: -1, status: "resolved", resolution: "kept_po"},
		{name: "unresolved item", actorID: 0, status: "unresolved", resolution: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer",
				fmt.Sprintf("B1 item refusal %d", index))
			order := savedPurchaseOrder(t, database, ctx, staff, quotation,
				fmt.Sprintf("PO-518-ITEM-%d", index))
			var org string
			if err := pool.QueryRow(ctx, fmt.Sprintf(
				`SELECT org_id::text FROM %s.purchase_orders WHERE id = $1`, schema),
				order.ID).Scan(&org); err != nil {
				t.Fatalf("read org: %v", err)
			}
			third := po518BeginRepair(t, database, ctx, order,
				po518NextRevision(t, database, quotation), org)
			actor := tc.actorID
			if actor == 0 {
				actor = third.Actor.ID
			}
			if actor == -1 {
				actor = staff.ID
			}
			id, err := po518InstallSession(ctx, schema, third.Tx, third, po518SessionParams{})
			if err != nil {
				t.Fatalf("install terminal session: %v", err)
			}
			if err := po518InstallItem(ctx, schema, third.Tx, third, id,
				actor, tc.status, tc.resolution); err == nil {
				t.Fatalf("044 accepted a terminal item with %s", tc.name)
			} else if !isSQLState(t, err, "23514") {
				t.Fatalf("item refusal error = %v; want 23514", err)
			}
		})
	}
}

func strPtr(value string) *string { return &value }

func ptrInt64(value int64) *int64 { return &value }
func ptrInt16(value int16) *int16 { return &value }
func ptrBool(value bool) *bool    { return &value }

// po518Unmigrated opens a schema without running Migrate, so a test can build a
// real historical schema by applying the production migrations itself.
func po518Unmigrated(t *testing.T, schema string) *Store {
	t.Helper()
	resetTestSchema(t, testDSN(t), schema)
	pool, err := pgxpool.New(context.Background(), testDSN(t))
	if err != nil {
		t.Fatalf("connect for a pre-migration schema: %v", err)
	}
	t.Cleanup(pool.Close)
	return &Store{pool: pool, config: Config{DSN: testDSN(t), Schema: schema}}
}

// po518ApplyThrough applies the real migrations up to and including maxVersion,
// exactly as Migrate does: one transaction per file and one ledger row.
func po518ApplyThrough(t *testing.T, database *Store, maxVersion int) {
	t.Helper()
	ctx := context.Background()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, schema)); err != nil {
		t.Fatalf("create migration ledger: %v", err)
	}
	migrations, err := loadMigrations(database.config.Schema)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.version > maxVersion {
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin migration %d: %v", migration.version, err)
		}
		if _, err := tx.Exec(ctx, migration.sql); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply migration %d: %v", migration.version, err)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s.schema_migrations (version, name) VALUES ($1, $2)`, schema),
			migration.version, migration.name); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("record migration %d: %v", migration.version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit migration %d: %v", migration.version, err)
		}
	}
}

// po518RetainedJSON captures the complete retained history content for one PO,
// excluding only the columns 044 adds, so a pre/post upgrade comparison proves
// 044 rewrote nothing.
func po518RetainedJSON(t *testing.T, ctx context.Context, database *Store, poID int64) map[string]string {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	queries := map[string]string{
		"sessions": `
			SELECT coalesce(jsonb_agg(to_jsonb(t) - 'repair_correction_id' ORDER BY t.id)::text, '[]')
			FROM %s.purchase_order_reconciliation_sessions t WHERE t.purchase_order_id = $1`,
		"session_items": `
			SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.session_id, t.key)::text, '[]')
			FROM %s.purchase_order_reconciliation_items t WHERE t.purchase_order_id = $1`,
		"corrections": `
			SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.id)::text, '[]')
			FROM %s.purchase_order_corrections t WHERE t.purchase_order_id = $1`,
		"correction_items": `
			SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.correction_id, t.line_number)::text, '[]')
			FROM %s.purchase_order_correction_items t
			WHERE t.correction_id IN (
				SELECT id FROM %s.purchase_order_corrections WHERE purchase_order_id = $1)`,
		"source_repairs": `
			SELECT coalesce(jsonb_agg(to_jsonb(t) - 'reconciliation_seed_xid' ORDER BY t.correction_id)::text, '[]')
			FROM %s.purchase_order_source_repairs t
			WHERE t.correction_id IN (
				SELECT id FROM %s.purchase_order_corrections WHERE purchase_order_id = $1)`,
		"source_repair_decisions": `
			SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.correction_id, t.key)::text, '[]')
			FROM %s.purchase_order_source_repair_decisions t
			WHERE t.correction_id IN (
				SELECT id FROM %s.purchase_order_corrections WHERE purchase_order_id = $1)`,
	}
	out := make(map[string]string, len(queries))
	for name, query := range queries {
		placeholders := strings.Count(query, "%s")
		args := make([]any, placeholders)
		for index := range args {
			args[index] = schema
		}
		var value string
		if err := pool.QueryRow(ctx, fmt.Sprintf(query, args...), poID).Scan(&value); err != nil {
			t.Fatalf("retained %s: %v", name, err)
		}
		out[name] = value
	}
	return out
}

// TestMigration044Genuine043Upgrade builds a real pre-044 schema by applying the
// production migrations 1..43, writes the old-shape history rows, then lets
// Migrate apply 044. It is not a downgrade simulation: every object below 044
// is created by its own migration file.
func TestMigration044Genuine043Upgrade(t *testing.T) {
	database := po518Unmigrated(t, "vera_po_518_upgrade")
	po518ApplyThrough(t, database, 43)
	ctx := poContext(t, database)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	seedRecipientCompanyColumnOnLegacySchema(t, pool, database.config.Schema)
	if po518HasColumn(t, pool, database.config.Schema,
		"purchase_order_reconciliation_sessions", "repair_correction_id") {
		t.Fatalf("the pre-044 schema already has 044's column")
	}

	// Real 043-era data: a saved record with its session, a repair audit and a
	// satellite row.
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "B1 upgrade")
	target := po518RevisionN(t, database, original, 1)
	saved := savedPurchaseOrder(t, database, ctx, staff, original, "PO-518-UPG")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	var orgID string
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT org_id::text FROM %s.purchase_orders WHERE id = $1`, schema), saved.ID).Scan(&orgID); err != nil {
		t.Fatalf("read org: %v", err)
	}
	correctionID, err := insertPurchaseOrderCorrection(ctx, tx, schema, saved, orgID,
		PurchaseOrderCorrectionKindSourceRepair, "legacy repair", admin.ID)
	if err != nil {
		t.Fatalf("legacy correction: %v", err)
	}
	source, err := loadPurchaseOrderSourceDetail(ctx, tx, schema, target.ID, target.RevisionNumber)
	if err != nil {
		t.Fatalf("legacy source: %v", err)
	}
	if err := insertPurchaseOrderSourceRepair(ctx, tx, schema, correctionID, saved,
		source, PurchaseOrderComparison{}, nil); err != nil {
		t.Fatalf("legacy satellite: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit legacy audit: %v", err)
	}
	var sessionIDBefore int64
	var versionBefore int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM %s.purchase_order_reconciliation_sessions
		WHERE purchase_order_id = $1 AND active`, schema), saved.ID).Scan(&sessionIDBefore); err != nil {
		t.Fatalf("read legacy session: %v", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT version FROM %s.purchase_orders WHERE id = $1`, schema), saved.ID).Scan(&versionBefore); err != nil {
		t.Fatalf("read version: %v", err)
	}

	// Complete retained content before the upgrade, excluding only 044's new
	// columns: the migration must rewrite nothing.
	retainedBefore := po518RetainedJSON(t, ctx, database, saved.ID)

	// The migration under test.
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("apply 044 to a real 043 schema: %v", err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("repeat Migrate: %v", err)
	}

	if !po518HasColumn(t, pool, database.config.Schema,
		"purchase_order_reconciliation_sessions", "repair_correction_id") {
		t.Fatalf("044's session column is missing after the upgrade")
	}
	var link *int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT repair_correction_id FROM %s.purchase_order_reconciliation_sessions
		WHERE id = $1`, schema), sessionIDBefore).Scan(&link); err != nil {
		t.Fatalf("read upgraded session: %v", err)
	}
	if link != nil {
		t.Fatalf("the upgrade backfilled a repair link on a pre-044 session: %d", *link)
	}
	var stamp *string
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT reconciliation_seed_xid::text FROM %s.purchase_order_source_repairs
		WHERE correction_id = $1`, schema), correctionID).Scan(&stamp); err != nil {
		t.Fatalf("read upgraded satellite: %v", err)
	}
	if stamp != nil {
		t.Fatalf("the upgrade backfilled a transaction stamp on a pre-044 repair: %q", *stamp)
	}
	var versionAfter int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT version FROM %s.purchase_orders WHERE id = $1`, schema), saved.ID).Scan(&versionAfter); err != nil {
		t.Fatalf("read version after upgrade: %v", err)
	}
	if versionAfter != versionBefore {
		t.Fatalf("the upgrade moved PO version %d -> %d", versionBefore, versionAfter)
	}

	retainedAfter := po518RetainedJSON(t, ctx, database, saved.ID)
	for name, before := range retainedBefore {
		if after := retainedAfter[name]; after != before {
			t.Errorf("%s changed across the upgrade:\n  before: %s\n  after:  %s", name, before, after)
		}
	}

	// The guards are live on the upgraded schema.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_sessions (
			org_id, purchase_order_id, source_quotation_id, source_revision,
			source_revision_id, generation, active, readonly, po_version,
			line_identities, created_by
		) VALUES ($1,$2,$3,$4,$5,99,FALSE,TRUE,$6,'[]',$7)`, schema),
		orgID, saved.ID, saved.QuotationID, saved.SourceRevision, 0, saved.Version,
		admin.ID); err == nil {
		t.Fatalf("the upgraded schema accepted a raw saved-parent session insert")
	} else if !isSQLState(t, err, "23514") {
		t.Fatalf("upgraded-schema guard error = %v; want 23514", err)
	}
}

// TestMigration044RepairChainCascadesWithWholePO proves the deferred foreign
// keys do not break #480's whole-PO deletion: both chains leave together.
func TestMigration044RepairChainCascadesWithWholePO(t *testing.T) {
	database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
		t, "vera_po_518_cascade", "PO-518-CASC")
	schema := database.configSchema()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	repair := po518BeginRepair(t, database, ctx, saved,
		po518NextRevision(t, database, seed), orgID)
	sessionID, err := po518InstallSession(ctx, schema, repair.Tx, repair, po518SessionParams{})
	if err != nil {
		t.Fatalf("install terminal session: %v", err)
	}
	if err := po518InstallItem(ctx, schema, repair.Tx, repair, sessionID,
		repair.Actor.ID, "resolved", "kept_po"); err != nil {
		t.Fatalf("terminal item: %v", err)
	}
	if err := repair.Tx.Commit(ctx); err != nil {
		t.Fatalf("commit repair: %v", err)
	}

	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_orders WHERE id = $1`, schema), saved.ID); err != nil {
		t.Fatalf("delete the whole PO chain: %v", err)
	}
	for _, table := range []string{
		"purchase_order_reconciliation_sessions",
		"purchase_order_reconciliation_items",
		"purchase_order_corrections",
		"purchase_order_source_repairs",
		"purchase_order_source_repair_decisions",
	} {
		var count int
		if err := pool.QueryRow(ctx, fmt.Sprintf(
			`SELECT COUNT(*) FROM %s.%s WHERE %s = $1`, schema, table,
			columnForPO(table)), saved.ID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s still holds %d rows after whole-PO deletion", table, count)
		}
	}
}

func columnForPO(table string) string {
	if table == "purchase_order_source_repairs" || table == "purchase_order_source_repair_decisions" {
		return "correction_id"
	}
	return "purchase_order_id"
}

// TestMigration044LinkedAuditCannotBeDeletedDirectly proves the deferred foreign
// keys are real: while a terminal session still references its repair, deleting
// only the audit row must fail instead of orphaning history.
func TestMigration044LinkedAuditCannotBeDeletedDirectly(t *testing.T) {
	database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
		t, "vera_po_518_audit", "PO-518-AUDIT")
	schema := database.configSchema()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	repaired := po518CommitRepair(t, database, ctx, saved,
		po518RevisionN(t, database, seed, 1), orgID)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s.purchase_order_source_repairs
		WHERE correction_id IN (
			SELECT repair_correction_id FROM %s.purchase_order_reconciliation_sessions
			WHERE purchase_order_id = $1 AND repair_correction_id IS NOT NULL)`,
		schema, schema), repaired.ID); err != nil {
		// An immediate refusal is also correct.
		return
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatalf("deleting the repair audit outlived its linked session")
	}
}

// po518InsertItemRow writes one raw terminal item with caller-chosen shapes, so
// the shape guard can be exercised independently of the composer.
func po518InsertItemRow(ctx context.Context, schema string, q po518Querier,
	repair po518Repair, sessionID int64, key, field, kind string,
	lineKey, sourceRef *string, poValue, sourceValue any, history, status, resolution string) error {
	var lineKeyArg, sourceRefArg, resolutionArg any
	if lineKey != nil {
		lineKeyArg = *lineKey
	}
	if sourceRef != nil {
		sourceRefArg = *sourceRef
	}
	if resolution != "" {
		resolutionArg = resolution
	}
	_, err := q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_items (
			org_id, session_id, purchase_order_id, key, field, kind,
			line_key, source_line_ref, label, po_value, source_value,
			status, resolution, actor_id, history
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'Label',$9,$10,$11,$12,$13,$14)`, schema),
		repair.OrgID, sessionID, repair.POID, key, field, kind,
		lineKeyArg, sourceRefArg, poValue, sourceValue, status, resolutionArg,
		repair.Actor.ID, history)
	return err
}

// TestMigration044ItemBootstrapRequiresActiveCanonicalItems pins the stricter
// bootstrap: only an active generation may be seeded, and every item must match
// the frozen contract shapes (key/field/value/history), not merely the 041
// CHECKs.
func TestMigration044ItemBootstrapRequiresActiveCanonicalItems(t *testing.T) {
	database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
		t, "vera_po_518_shape", "PO-518-SHAPE")
	schema := database.configSchema()
	target := po518RevisionN(t, database, seed, 1)
	validHistory := `[{"Kind":"difference","SourceLineRef":null,"POValue":"Net 30","SourceValue":"Net 60","Status":"resolved","Resolution":"kept_po","ActorID":1,"At":"2026-11-08T00:00:00Z"}]`

	lineKey := func(value string) *string { return &value }
	ref := "1"

	cases := []struct {
		name       string
		mutate     func(tx pgx.Tx, sessionID int64) error
		key        string
		field      string
		kind       string
		lineKey    *string
		sourceRef  *string
		poValue    any
		sourceVal  any
		history    string
		status     string
		resolution string
		wantErr    bool
	}{
		{
			name: "canonical header item is accepted",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`, history: validHistory,
			status: "resolved", resolution: "kept_po",
		},
		{
			name: "inactive generation is refused",
			mutate: func(tx pgx.Tx, sessionID int64) error {
				_, err := tx.Exec(ctx, fmt.Sprintf(
					`UPDATE %s.purchase_order_reconciliation_sessions SET active = FALSE WHERE id = $1`, schema),
					sessionID)
				return err
			},
			key: "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`, history: validHistory,
			status: "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "text quantity is refused",
			key:  "line:id:1:quantity", field: "line:quantity", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: `"3"`, sourceVal: `2`, history: validHistory,
			status: "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "key that disagrees with its field is refused",
			key:  "line:id:1:description", field: "line:quantity", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: `3`, sourceVal: `2`, history: validHistory,
			status: "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "history with a missing member is refused",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`, history: `[{"Kind":"difference"}]`,
			status: "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "unresolved item is refused",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`, history: validHistory,
			status: "unresolved", resolution: "", wantErr: true,
		},
		{
			name: "tax basis with an extra member is refused",
			key:  "link:taxBasis", field: "link:taxBasis", kind: "tax_basis",
			poValue:   `{"TaxZone":"FTZ","TaxType":"Goods","ServiceCategory":"","Extra":"x"}`,
			sourceVal: `{"TaxZone":"FTZ","TaxType":"Goods","ServiceCategory":""}`,
			history:   validHistory,
			status:    "resolved", resolution: "used_quotation", wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repair := po518BeginRepair(t, database, ctx, saved, target, orgID)
			sessionID, err := po518InstallSession(ctx, schema, repair.Tx, repair, po518SessionParams{})
			if err != nil {
				t.Fatalf("install terminal session: %v", err)
			}
			if tc.mutate != nil {
				if err := tc.mutate(repair.Tx, sessionID); err != nil {
					t.Fatalf("mutate session: %v", err)
				}
			}
			err = po518InsertItemRow(ctx, schema, repair.Tx, repair, sessionID,
				tc.key, tc.field, tc.kind, tc.lineKey, tc.sourceRef,
				tc.poValue, tc.sourceVal, tc.history, tc.status, tc.resolution)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("the stricter bootstrap accepted %q", tc.name)
				}
				if !isSQLState(t, err, "23514") {
					t.Fatalf("shape refusal error = %v; want 23514", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("canonical item refused: %v", err)
			}
		})
	}
}

// po518HistoryEvent builds one canonical HistoryEvent JSON document.
func po518HistoryEvent(t *testing.T, kind, status, resolution, poValue, sourceValue string, actor int, at string) string {
	t.Helper()
	resolutionJSON := "null"
	if resolution != "" {
		encoded, err := json.Marshal(resolution)
		if err != nil {
			t.Fatalf("encode resolution: %v", err)
		}
		resolutionJSON = string(encoded)
	}
	return fmt.Sprintf(
		`[{"Kind":%q,"SourceLineRef":null,"POValue":%s,"SourceValue":%s,"Status":%q,"Resolution":%s,"ActorID":%d,"At":%q}]`,
		kind, poValue, sourceValue, status, resolutionJSON, actor, at)
}

// TestMigration044CanonicalShapeRejections is the fail-closed half of the shape
// guard: every near-miss must raise 23514 rather than slip through a NULL.
func TestMigration044CanonicalShapeRejections(t *testing.T) {
	database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
		t, "vera_po_518_shape_reject", "PO-518-REJECT")
	schema := database.configSchema()
	target := po518RevisionN(t, database, seed, 1)
	lineKey := func(value string) *string { return &value }
	ref := "1"
	at := "2026-11-08T00:00:00Z"
	validHeaderHistory := func() string {
		return po518HistoryEvent(t, "difference", "resolved", "kept_po", `"Net 30"`, `"Net 60"`, 1, at)
	}
	validDiscountHistory := func() string {
		return po518HistoryEvent(t, "difference", "resolved", "kept_po", `{"Enabled":true,"Percent":5}`, `{"Enabled":false,"Percent":0}`, 1, at)
	}

	cases := []struct {
		name       string
		key        string
		field      string
		kind       string
		lineKey    *string
		sourceRef  *string
		poValue    any
		sourceVal  any
		history    string
		status     string
		resolution string
		wantErr    bool
	}{
		{
			name: "object-valued payment terms",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `{"PaymentTerms":"Net 30"}`, sourceVal: `"Net 60"`,
			history: validHeaderHistory(), status: "resolved", resolution: "kept_po",
			wantErr: true,
		},
		{
			name: "header key that disagrees with its field",
			key:  "header:incoterms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`,
			history: validHeaderHistory(), status: "resolved", resolution: "kept_po",
			wantErr: true,
		},
		{
			name: "discount Percent as a numeric string",
			key:  "line:id:1:discount", field: "line:discount", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: `{"Enabled":true,"Percent":"5"}`, sourceVal: `{"Enabled":false,"Percent":0}`,
			history: validDiscountHistory(), status: "resolved", resolution: "kept_po",
			wantErr: true,
		},
		{
			name: "discount Percent as null",
			key:  "line:id:1:discount", field: "line:discount", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: `{"Enabled":true,"Percent":null}`, sourceVal: `{"Enabled":false,"Percent":0}`,
			history: validDiscountHistory(), status: "resolved", resolution: "kept_po",
			wantErr: true,
		},
		{
			name: "canonical discount is accepted",
			key:  "line:id:1:discount", field: "line:discount", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: `{"Enabled":true,"Percent":5}`, sourceVal: `{"Enabled":false,"Percent":0}`,
			history: validDiscountHistory(), status: "resolved", resolution: "kept_po",
		},
		{
			name: "SQL NULL purchase-order description value",
			key:  "line:id:1:description", field: "line:description", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: nil, sourceVal: `"Priced widget"`,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po", `"A"`, `"B"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "SQL NULL source description value",
			key:  "line:id:1:description", field: "line:description", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: `"Priced widget"`, sourceVal: nil,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po", `"A"`, `"B"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "SQL NULL purchase-order unit-of-measure value",
			key:  "line:id:1:uom", field: "line:uom", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: nil, sourceVal: `"Pc"`,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po", `"A"`, `"B"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "SQL NULL source unit-of-measure value",
			key:  "line:id:1:uom", field: "line:uom", kind: "difference",
			lineKey: lineKey("id:1"), sourceRef: &ref,
			poValue: `"Pc"`, sourceVal: nil,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po", `"A"`, `"B"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "valid missing-line history with the contract kind is accepted",
			key:  "source:1:match", field: "source:match", kind: "missing_line",
			sourceRef: &ref,
			poValue:   `null`, sourceVal: `"1"`,
			history: po518HistoryEvent(t, "missing_line", "resolved", "acknowledged_missing", `null`, `"1"`, 1, at),
			status:  "resolved", resolution: "acknowledged_missing",
		},
		{
			name: "history Kind uses the old field-id convention",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`,
			history: po518HistoryEvent(t, "header:paymentTerms", "resolved", "kept_po", `"Net 30"`, `"Net 60"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "history Status outside the enum",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`,
			history: po518HistoryEvent(t, "difference", "done", "kept_po", `"Net 30"`, `"Net 60"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "history Resolution outside the enum",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po_lol", `"Net 30"`, `"Net 60"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "history value as an array",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po", `[1,2]`, `"Net 60"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "history value as an untyped object",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po", `{"Whatever":1}`, `"Net 60"`, 1, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "history actor that is not positive",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po", `"Net 30"`, `"Net 60"`, 0, at),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
		{
			name: "history timestamp that is not RFC3339",
			key:  "header:paymentTerms", field: "header:paymentTerms", kind: "difference",
			poValue: `"Net 30"`, sourceVal: `"Net 60"`,
			history: po518HistoryEvent(t, "difference", "resolved", "kept_po", `"Net 30"`, `"Net 60"`, 1, "2026-11-08 00:00:00"),
			status:  "resolved", resolution: "kept_po", wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repair := po518BeginRepair(t, database, ctx, saved, target, orgID)
			sessionID, err := po518InstallSession(ctx, schema, repair.Tx, repair, po518SessionParams{})
			if err != nil {
				t.Fatalf("install terminal session: %v", err)
			}
			err = po518InsertItemRow(ctx, schema, repair.Tx, repair, sessionID,
				tc.key, tc.field, tc.kind, tc.lineKey, tc.sourceRef,
				tc.poValue, tc.sourceVal, tc.history, tc.status, tc.resolution)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("the fail-closed guard accepted %q", tc.name)
				}
				if !isSQLState(t, err, "23514") {
					t.Fatalf("rejection error = %v; want 23514", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("canonical case refused: %v", err)
			}
		})
	}
}

// TestMigration044ShapeFunctionFailsClosedOnSQLNull calls the canonical shape
// function directly with SQL-NULL value columns. The function must return
// exactly FALSE, never SQL NULL: the caller is `IF NOT shape_ok(...)`, and
// `NOT NULL` is NULL, so a NULL answer is an acceptance, not a refusal.
func TestMigration044ShapeFunctionFailsClosedOnSQLNull(t *testing.T) {
	database, ctx, _, _, _, _, _ := po518SavedFixture(
		t, "vera_po_518_shape_null", "PO-518-NULL")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	history := po518HistoryEvent(t, "difference", "resolved", "kept_po", `"A"`, `"B"`, 1, "2026-11-08T00:00:00Z")

	cases := []struct {
		name        string
		field       string
		poValue     any
		sourceValue any
	}{
		{name: "purchase-order value is SQL NULL", field: "line:description",
			poValue: nil, sourceValue: `"A"`},
		{name: "source value is SQL NULL", field: "line:description",
			poValue: `"A"`, sourceValue: nil},
		{name: "both values are SQL NULL", field: "line:uom",
			poValue: nil, sourceValue: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			suffix := strings.TrimPrefix(tc.field, "line:")
			var result *bool
			if err := pool.QueryRow(ctx, fmt.Sprintf(`
				SELECT %s.purchase_order_reconciliation_item_shape_ok(
					$1, $2, 'difference', 'id:1', '1', $3, $4,
					'resolved', 'kept_po', 1, $5::jsonb)`, schema),
				"line:id:1:"+suffix, tc.field, tc.poValue, tc.sourceValue, history).
				Scan(&result); err != nil {
				t.Fatalf("call the shape function: %v", err)
			}
			if result == nil {
				t.Fatalf("the shape function returned SQL NULL for %s, which the caller treats as accepted", tc.name)
			}
			if *result {
				t.Fatalf("the shape function accepted %s", tc.name)
			}
		})
	}
}
