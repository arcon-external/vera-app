package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The archive read (#518 B1): every generation, in one snapshot, with the
// recorded audit version kept separate from the record's current version.

// po518CommitRepair runs one complete repair-shaped transaction against a saved
// record and returns the reloaded, resulting record. It is the fixture the
// history read is asserted against; the repair feature itself is B2.
func po518CommitRepair(t *testing.T, database *Store, ctx context.Context,
	saved PurchaseOrder, target QuotationRecord, orgID string) PurchaseOrder {
	t.Helper()
	schema := database.configSchema()
	repair := po518BeginRepair(t, database, ctx, saved, target, orgID)
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
	reloaded, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reload repaired record: %v", err)
	}
	return reloaded
}

func TestListPOReconciliationSessionsCurrentVersionAndGeneration(t *testing.T) {
	database, ctx, saved, _, _, _, _ := po518SavedFixture(
		t, "vera_po_518_read_one", "PO-518-READ-1")

	history, err := database.ListPOReconciliationSessions(ctx, saved.ID)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if history.PurchaseOrderID != saved.ID {
		t.Errorf("history PO = %d; want %d", history.PurchaseOrderID, saved.ID)
	}
	if history.POVersion != saved.Version {
		t.Errorf("outer POVersion = %d; want the stored %d", history.POVersion, saved.Version)
	}
	if history.Sessions == nil {
		t.Fatalf("Sessions is nil; want an empty slice for a record with no session")
	}
	if len(history.Sessions) != 1 {
		t.Fatalf("generations = %d; want the one finalize session", len(history.Sessions))
	}
	record := history.Sessions[0]
	if record.Generation != 1 {
		t.Errorf("generation = %d; want 1", record.Generation)
	}
	if !record.SourceAvailable {
		t.Errorf("SourceAvailable = false; want true for a live source")
	}
	if record.RepairCorrectionID != nil {
		t.Errorf("RepairCorrectionID = %d; want null for a create/link session", *record.RepairCorrectionID)
	}
	if !record.Session.Active || !record.Session.ReadOnly {
		t.Errorf("session active/readonly = %v/%v; want true/true", record.Session.Active, record.Session.ReadOnly)
	}
	if record.Session.POVersion != saved.Version {
		t.Errorf("stored session version = %d; want %d", record.Session.POVersion, saved.Version)
	}
	if record.Session.SourceQuotationID != saved.QuotationID ||
		record.Session.SourceRevision != saved.SourceRevision {
		t.Errorf("session source = %d/%d; want %d/%d",
			record.Session.SourceQuotationID, record.Session.SourceRevision,
			saved.QuotationID, saved.SourceRevision)
	}
	if record.Session.Items == nil {
		t.Errorf("session items are nil; want an empty slice")
	}
}

func TestListPOReconciliationSessionsEveryGeneration(t *testing.T) {
	database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
		t, "vera_po_518_read_all", "PO-518-READ-ALL")

	first := po518CommitRepair(t, database, ctx, saved,
		po518RevisionN(t, database, seed, 1), orgID)
	second := po518CommitRepair(t, database, ctx, first,
		po518RevisionN(t, database, seed, 2), orgID)

	history, err := database.ListPOReconciliationSessions(ctx, second.ID)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(history.Sessions) != 3 {
		t.Fatalf("generations = %d; want 3 (create + two repairs)", len(history.Sessions))
	}
	if history.POVersion != second.Version {
		t.Errorf("outer POVersion = %d; want the current %d", history.POVersion, second.Version)
	}
	for index, record := range history.Sessions {
		wantGeneration := 3 - index
		if record.Generation != wantGeneration {
			t.Errorf("position %d generation = %d; want %d", index, record.Generation, wantGeneration)
		}
		if record.SourceAvailable != true {
			t.Errorf("generation %d SourceAvailable = false; want true", record.Generation)
		}
	}
	// Exactly one active generation, and it is the newest.
	active := 0
	for _, record := range history.Sessions {
		if record.Session.Active {
			active++
			if record.Generation != 3 {
				t.Errorf("active generation = %d; want 3", record.Generation)
			}
		}
		if !record.Session.ReadOnly {
			t.Errorf("generation %d is not read-only", record.Generation)
		}
	}
	if active != 1 {
		t.Errorf("active generations = %d; want 1", active)
	}
	// The create generation never carries a repair link; both repairs do.
	if history.Sessions[2].RepairCorrectionID != nil {
		t.Errorf("the create generation carries a repair link")
	}
	for _, record := range history.Sessions[:2] {
		if record.RepairCorrectionID == nil {
			t.Errorf("generation %d has no repair link", record.Generation)
		}
	}
	// The nested version is the recorded audit version, so the oldest is
	// expected to disagree with the current record.
	if history.Sessions[2].Session.POVersion == history.POVersion {
		t.Errorf("the create generation's stored version equals the current one; the read rewrote history")
	}
	// Old generations keep their own source pair, not the current one.
	if history.Sessions[2].Session.SourceRevision != saved.SourceRevision {
		t.Errorf("old generation revision = %d; want the recorded %d",
			history.Sessions[2].Session.SourceRevision, saved.SourceRevision)
	}
}

func TestListPOReconciliationSessionsSourceClearedStaysReadable(t *testing.T) {
	database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
		t, "vera_po_518_read_source", "PO-518-READ-SRC")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	repaired := po518CommitRepair(t, database, ctx, saved,
		po518RevisionN(t, database, seed, 1), orgID)

	// Source deletion is FK maintenance: the pointer clears, the immutable pair
	// and every item stay.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_reconciliation_sessions
		SET source_revision_id = NULL
		WHERE purchase_order_id = $1`, database.configSchema()), repaired.ID); err != nil {
		t.Fatalf("clear source pointer: %v", err)
	}

	history, err := database.ListPOReconciliationSessions(ctx, repaired.ID)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(history.Sessions) != 2 {
		t.Fatalf("generations = %d; want 2", len(history.Sessions))
	}
	for _, record := range history.Sessions {
		if record.SourceAvailable {
			t.Errorf("generation %d SourceAvailable = true; want false after source deletion", record.Generation)
		}
		if record.Session.SourceQuotationID == 0 {
			t.Errorf("generation %d lost its source pair", record.Generation)
		}
	}
	if len(history.Sessions[0].Session.Items) == 0 {
		t.Errorf("the repaired generation lost its retained items")
	}
}

func TestListPOReconciliationSessionsLegacyAndTenant(t *testing.T) {
	database, ctx, saved, _, staff, _, _ := po518SavedFixture(
		t, "vera_po_518_read_legacy", "PO-518-READ-LEG")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	// A pre-041 record: saved, with no reconciliation session at all. The draft
	// session is removed while the record is still a draft (the one state the
	// guard allows), then the row is moved to saved directly.
	draft := createFinalizeDraft(t, database, ctx, staff, seedFinalizeQuotation(
		t, database, 2, "PT. Buyer", "B1 legacy"))
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_reconciliation_sessions WHERE purchase_order_id = $1`, schema),
		draft.ID); err != nil {
		t.Fatalf("remove draft session: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET state = 'saved', po_number = $2, po_number_key = $3,
		    saved_at = NOW(), saved_by = $4
		WHERE id = $1`, schema), draft.ID, "PO-518-LEGACY", "po-518-legacy", staff.ID); err != nil {
		t.Fatalf("mark legacy record saved: %v", err)
	}

	legacy, err := database.ListPOReconciliationSessions(ctx, draft.ID)
	if err != nil {
		t.Fatalf("legacy history: %v", err)
	}
	if legacy.Sessions == nil {
		t.Errorf("legacy Sessions is nil; want []")
	}
	if len(legacy.Sessions) != 0 {
		t.Errorf("legacy generations = %d; want 0", len(legacy.Sessions))
	}
	if legacy.POVersion == 0 {
		t.Errorf("legacy POVersion = 0; want the record version")
	}

	if _, err := database.ListPOReconciliationSessions(ctx, 999999); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("unknown record error = %v; want ErrPurchaseOrderNotFound", err)
	}
	if _, err := database.ListPOReconciliationSessions(ctx, 0); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("invalid id error = %v; want ErrPurchaseOrderNotFound", err)
	}

	// The tenant boundary is the context identity, not a parameter.
	var otherOrgID string
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"INSERT INTO %s.organizations (name) VALUES ('Other Org') RETURNING id::text", schema)).
		Scan(&otherOrgID); err != nil {
		t.Fatalf("seed other organization: %v", err)
	}
	otherCtx := ContextWithOrg(context.Background(), otherOrgID)
	if _, err := database.ListPOReconciliationSessions(otherCtx, saved.ID); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("foreign history = %v; want ErrPurchaseOrderNotFound", err)
	}
}

// po518ReadWithCommit runs one production history read while a repair commits
// exactly between its version read and its generation read. The test-only hook
// makes the interleaving deterministic; no timing is relied on.
func po518ReadWithCommit(t *testing.T, database *Store, ctx context.Context,
	saved PurchaseOrder, admin AuthUser, target QuotationRecord, orgID string,
) (POReconciliationSessions, error) {
	t.Helper()
	input := repairInput(t, saved, admin, target, "controlled snapshot repair")

	paused := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	testOnlyAfterVersionRead = func(int64) {
		once.Do(func() { close(paused) })
		<-release
	}
	defer func() { testOnlyAfterVersionRead = nil }()

	repairErr := make(chan error, 1)
	repairContext := ContextWithOrg(context.Background(), orgID)
	go func() {
		<-paused
		_, err := database.RepairPurchaseOrderSource(repairContext, input)
		repairErr <- err
		close(release)
	}()

	history, err := database.ListPOReconciliationSessions(ctx, saved.ID)
	if repairErrValue := <-repairErr; repairErrValue != nil {
		t.Fatalf("controlled repair: %v", repairErrValue)
	}
	return history, err
}

// po518SnapshotViolations applies the single-snapshot invariant to one envelope.
// The paused read must satisfy it; a weakened isolation must break it.
func po518SnapshotViolations(history POReconciliationSessions, initialVersion int64) []string {
	var violations []string
	want := int(history.POVersion-initialVersion) + 1
	if len(history.Sessions) != want {
		violations = append(violations, fmt.Sprintf(
			"version %d carries %d generations; want %d", history.POVersion, len(history.Sessions), want))
	}
	for _, generation := range history.Sessions {
		if generation.Session.Active && generation.Session.POVersion != history.POVersion {
			violations = append(violations, fmt.Sprintf(
				"outer version %d carries an active generation recorded at %d",
				history.POVersion, generation.Session.POVersion))
		}
	}
	return violations
}

// TestListPOReconciliationSessionsSnapshotIsDeterministic proves the production
// reader's single snapshot with a controlled commit between its two statements,
// and proves the assertion fails when that isolation is weakened.
func TestListPOReconciliationSessionsSnapshotIsDeterministic(t *testing.T) {
	t.Run("repeatable read keeps the pre-commit snapshot", func(t *testing.T) {
		database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
			t, "vera_po_518_snapshot_ok", "PO-518-SNAP-OK")
		admin := poRepairAdmin(t, database)
		target := po518RevisionN(t, database, seed, 1)
		initialVersion := saved.Version

		history, err := po518ReadWithCommit(t, database, ctx, saved, admin, target, orgID)
		if err != nil {
			t.Fatalf("history read: %v", err)
		}
		if history.POVersion != initialVersion {
			t.Errorf("paused read version = %d; want the pre-commit %d", history.POVersion, initialVersion)
		}
		if len(history.Sessions) != 1 {
			t.Errorf("paused read generations = %d; want the pre-commit 1", len(history.Sessions))
		}
		if violations := po518SnapshotViolations(history, initialVersion); len(violations) > 0 {
			t.Errorf("the paused read violated the single-snapshot invariant: %v", violations)
		}

		// The commit did happen: a fresh read sees the installed generation.
		after, err := database.ListPOReconciliationSessions(ctx, saved.ID)
		if err != nil {
			t.Fatalf("fresh read: %v", err)
		}
		if after.POVersion != initialVersion+1 || len(after.Sessions) != 2 {
			t.Errorf("post-commit read = version %d with %d generations; want %d with 2",
				after.POVersion, len(after.Sessions), initialVersion+1)
		}
	})

	t.Run("read committed is detected as mixed", func(t *testing.T) {
		database, ctx, saved, seed, _, orgID, _ := po518SavedFixture(
			t, "vera_po_518_snapshot_weak", "PO-518-SNAP-WEAK")
		admin := poRepairAdmin(t, database)
		target := po518RevisionN(t, database, seed, 1)
		initialVersion := saved.Version

		previous := reconciliationHistoryIsolation
		reconciliationHistoryIsolation = pgx.ReadCommitted
		defer func() { reconciliationHistoryIsolation = previous }()

		history, err := po518ReadWithCommit(t, database, ctx, saved, admin, target, orgID)
		if err != nil {
			t.Fatalf("history read: %v", err)
		}
		if history.POVersion != initialVersion {
			t.Fatalf("weakened read version = %d; want the pre-commit %d", history.POVersion, initialVersion)
		}
		// The same assertion the production default must satisfy has to fail
		// here. If it still holds, the test cannot detect a weakened reader.
		violations := po518SnapshotViolations(history, initialVersion)
		if len(violations) == 0 {
			t.Fatalf("read committed produced a consistent snapshot; the assertion is not isolation-sensitive")
		}
		t.Logf("read committed mixed the snapshot as expected: %v", violations)
	})
}
