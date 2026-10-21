package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// Issue #475: repairing a saved Purchase Order's source link.
//
// These tests drive the real store against a disposable schema: the admin gate,
// the confirmation-number boundary (same customer keeps it, another customer
// only before assignment), all #471 difference rules repeated against the new
// source, the occupied-target race, the version CAS, the old/new source and
// selected choices in history, and the rule that a missing record or target is
// never recreated.

// poRepairAdmin creates the actor a repair requires. Repairs are admin-only at
// the route and re-checked here against the stored role, so this is a real admin
// row rather than a flag a caller could set.
func poRepairAdmin(t *testing.T, database *Store) AuthUser {
	t.Helper()
	user, err := database.CreateUser(context.Background(),
		fmt.Sprintf("po-repair-admin-%d@example.com", time.Now().UnixNano()),
		"PO Repair Admin", "hash", "admin")
	if err != nil {
		t.Fatalf("create PO repair admin: %v", err)
	}
	return user
}

// customerIDByCode resolves one seeded client, so a cross-customer assertion
// names the real row rather than a name string.
func customerIDByCode(t *testing.T, database *Store, code int) int64 {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var id int64
	if err := pool.QueryRow(context.Background(), fmt.Sprintf(
		`SELECT id FROM %s.customers WHERE customer_code = $1`, database.configSchema()),
		code).Scan(&id); err != nil {
		t.Fatalf("customer code %d: %v", code, err)
	}
	return id
}

// repairQuotationOptions parameterise a seeded issued revision. The defaults
// seedRepairQuotation uses are the FTZ goods basis: no PPN, so the calculated
// total equals the printed subtotal.
type repairQuotationOptions struct {
	CustomerCode    int
	CustomerName    string
	Subject         string
	Address         string
	Currency        string
	TaxZone         string
	TaxType         string
	UnitPrice       int64
	PaymentTermDays int16
}

// seedRepairQuotationWith writes one issued revision with the chosen values,
// so a repair to it produces a reviewable difference.
func seedRepairQuotationWith(t *testing.T, database *Store, opts repairQuotationOptions) QuotationRecord {
	t.Helper()
	subtotal := opts.UnitPrice * 2
	ppn := int64(0)
	if opts.TaxZone == "NonFTZ" && opts.TaxType == "Goods" {
		ppn = subtotal * 11 / 100
	}
	record, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:    int16(opts.CustomerCode),
		CustomerName:    opts.CustomerName,
		CustomerAddress: opts.Address,
		RecipientName:   "Budi Santoso",
		// The revision's recipient address is what a PO copies as its billing
		// address, so a cross-customer repair is observable through it.
		RecipientAddress: opts.Address,
		Subject:          opts.Subject,
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  opts.PaymentTermDays,
		LeadTimeDays:     10,
		DiscountPercent:  0,
		TaxZone:          opts.TaxZone,
		TaxType:          opts.TaxType,
		TaxRateBps:       0,
		Currency:         opts.Currency,
		HTMLSnapshot:     "<html>repair snapshot</html>",
		Items: []RevisionItem{
			{
				ItemNo:                1,
				Description:           "Priced widget",
				Qty:                   2,
				UoM:                   "Pc",
				UnitPrice:             opts.UnitPrice,
				MarginPercent:         0,
				MarginAmount:          0,
				OriginalTotal:         subtotal,
				MarginalizedUnitPrice: opts.UnitPrice,
				MarginalizedTotal:     subtotal,
				ListUnitPrice:         opts.UnitPrice,
			},
			{
				ItemNo:                3,
				Description:           "Zero-value line",
				Qty:                   1,
				UoM:                   "Lot",
				UnitPrice:             0,
				MarginPercent:         0,
				MarginAmount:          0,
				OriginalTotal:         0,
				MarginalizedUnitPrice: 0,
				MarginalizedTotal:     0,
				ListUnitPrice:         0,
			},
		},
		Totals: RevisionTotals{
			OriginalSubtotal:          subtotal,
			OriginalGrand:             subtotal,
			MarginalizedSubtotal:      subtotal,
			MarginalizedGrand:         subtotal,
			PpnAmount:                 ppn,
			MarginalizedGrandAfterTax: subtotal + ppn,
			CustomerGrandTotal:        subtotal + ppn,
		},
	})
	if err != nil {
		t.Fatalf("seed repair quotation %q: %v", opts.Subject, err)
	}
	return record
}

// seedRepairQuotation is the default FTZ-USD revision the repair tests use.
func seedRepairQuotation(
	t *testing.T,
	database *Store,
	customerCode int,
	customerName, subject, address string,
	unitPrice int64,
	paymentTermDays int16,
) QuotationRecord {
	t.Helper()
	return seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode:    customerCode,
		CustomerName:    customerName,
		Subject:         subject,
		Address:         address,
		Currency:        "USD",
		TaxZone:         "FTZ",
		TaxType:         "Goods",
		UnitPrice:       unitPrice,
		PaymentTermDays: paymentTermDays,
	})
}

// repairInput is the payload a repair submits: the record's current values as
// the starting point, plus the chosen target and the reason. Tests change only
// what they exercise.
func repairInput(
	t *testing.T,
	order PurchaseOrder,
	actor AuthUser,
	target QuotationRecord,
	reason string,
) PurchaseOrderSourceRepairInput {
	t.Helper()
	items := make([]PurchaseOrderItem, len(order.Items))
	for index, item := range order.Items {
		item.LineNumber = index + 1
		items[index] = item
	}
	return PurchaseOrderSourceRepairInput{
		PurchaseOrderUpdate: PurchaseOrderUpdate{
			ID:                    order.ID,
			BillingAddress:        order.BillingAddress,
			DeliveryAddress:       order.DeliveryAddress,
			ShipToCode:            order.ShipToCode,
			ContactEmail:          order.ContactEmail,
			ContactPhone:          order.ContactPhone,
			PONumber:              order.PONumber,
			PODate:                order.PODate,
			ReceivedDate:          &order.ReceivedDate,
			RequestedDeliveryDate: order.RequestedDeliveryDate,
			PaymentTerms:          order.PaymentTerms,
			Incoterms:             order.Incoterms,
			UpdatedBy:             actor.ID,
			Items:                 items,
			DiscountPercent:       order.DiscountPercent,
			AdjustmentMinor:       order.AdjustmentMinor,
			AdjustmentReason:      order.AdjustmentReason,
			ExpectedVersion:       order.Version,
		},
		Reason:         reason,
		QuotationID:    target.ID,
		RevisionNumber: target.RevisionNumber,
	}
}

func TestRepairPurchaseOrderSourceRequiresAnAdministrator(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_admin")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-ADMIN")

	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, staff, target, "move to the right quotation")); !errors.Is(err, ErrPurchaseOrderRepairForbidden) {
		t.Fatalf("staff repair error = %v; want ErrPurchaseOrderRepairForbidden", err)
	}
	unchanged, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("read after refused repair: %v", err)
	}
	if unchanged.QuotationID != saved.QuotationID || unchanged.SourceRevision != saved.SourceRevision {
		t.Errorf("a refused repair moved the source: %d/%d -> %d/%d",
			saved.QuotationID, saved.SourceRevision, unchanged.QuotationID, unchanged.SourceRevision)
	}
	if unchanged.Version != saved.Version {
		t.Errorf("a refused repair bumped the version: %d -> %d", saved.Version, unchanged.Version)
	}
	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history after refused repair: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("a refused repair wrote %d history rows; want 0", len(history))
	}

	repaired, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, target, "move to the right quotation"))
	if err != nil {
		t.Fatalf("admin repair: %v", err)
	}
	if repaired.QuotationID != target.ID || repaired.SourceRevision != target.RevisionNumber {
		t.Errorf("source = %d/%d; want %d/%d",
			repaired.QuotationID, repaired.SourceRevision, target.ID, target.RevisionNumber)
	}
	if repaired.Version != saved.Version+1 {
		t.Errorf("repaired version = %d; want %d", repaired.Version, saved.Version+1)
	}
}

func TestRepairPurchaseOrderSourceKeepsAConfirmationNumberAndRecordsTheMove(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_confirmation")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	// Same customer, same composition: the repair is a pure link change, so no
	// difference choice is needed.
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-KEEP")

	assigned, err := database.AssignPurchaseOrderConfirmationNumber(ctx, saved.ID, admin.ID)
	if err != nil {
		t.Fatalf("assign confirmation: %v", err)
	}
	if assigned.ConfirmationNumber == nil {
		t.Fatal("confirmation number was not assigned")
	}

	repaired, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, assigned, admin, target, "the order followed the wrong revision"))
	if err != nil {
		t.Fatalf("same-customer repair: %v", err)
	}
	if repaired.ConfirmationNumber == nil || *repaired.ConfirmationNumber != *assigned.ConfirmationNumber {
		t.Errorf("confirmation number = %v; want the assigned %v",
			repaired.ConfirmationNumber, assigned.ConfirmationNumber)
	}
	if repaired.CustomerID != assigned.CustomerID {
		t.Errorf("same-customer repair moved customer %d -> %d", assigned.CustomerID, repaired.CustomerID)
	}
	if repaired.QuotationID != target.ID || repaired.SourceRevision != target.RevisionNumber {
		t.Errorf("source = %d/%d; want %d/%d",
			repaired.QuotationID, repaired.SourceRevision, target.ID, target.RevisionNumber)
	}
	if repaired.QuoteNumber != target.QuoteNumber {
		t.Errorf("quote number = %q; want the target's %q", repaired.QuoteNumber, target.QuoteNumber)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history rows = %d; want 1 repair snapshot", len(history))
	}
	entry := history[0]
	if entry.Kind != PurchaseOrderCorrectionKindSourceRepair {
		t.Errorf("history kind = %q; want %q", entry.Kind, PurchaseOrderCorrectionKindSourceRepair)
	}
	if entry.Repair == nil {
		t.Fatal("the repair snapshot has no old/new source")
	}
	if entry.Repair.PreviousQuotationID != original.ID ||
		entry.Repair.PreviousSourceRevision != original.RevisionNumber ||
		entry.Repair.PreviousQuoteNumber != original.QuoteNumber {
		t.Errorf("previous source = %d/%d/%q; want %d/%d/%q",
			entry.Repair.PreviousQuotationID, entry.Repair.PreviousSourceRevision,
			entry.Repair.PreviousQuoteNumber, original.ID, original.RevisionNumber, original.QuoteNumber)
	}
	if entry.Repair.NewQuotationID != target.ID ||
		entry.Repair.NewSourceRevision != target.RevisionNumber ||
		entry.Repair.NewQuoteNumber != target.QuoteNumber {
		t.Errorf("new source = %d/%d/%q; want %d/%d/%q",
			entry.Repair.NewQuotationID, entry.Repair.NewSourceRevision,
			entry.Repair.NewQuoteNumber, target.ID, target.RevisionNumber, target.QuoteNumber)
	}
	// The snapshot remains the pre-repair record: old quote number, old source.
	if entry.QuoteNumber != original.QuoteNumber || entry.SourceRevision != original.RevisionNumber {
		t.Errorf("snapshot says %q revision %d; want the old %q revision %d",
			entry.QuoteNumber, entry.SourceRevision, original.QuoteNumber, original.RevisionNumber)
	}
}

func TestRepairPurchaseOrderSourceMovesCustomerOnlyBeforeAssignment(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_cross_customer")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	other := seedRepairQuotation(t, database, 4, "PT. Other", "Other source", "Jl. Billing B", 100, 45)
	// A second free revision of the same other customer, because the first
	// successful move occupies the first one.
	otherAgain := seedRepairQuotation(t, database, 4, "PT. Other", "Other source again", "Jl. Billing B", 100, 45)

	// Before assignment: the record may move to another customer.
	before := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-CROSS-1")
	moved, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, before, admin, other, "the PO belongs to the other legal entity"))
	if err != nil {
		t.Fatalf("cross-customer repair before assignment: %v", err)
	}
	if moved.CustomerID != customerIDByCode(t, database, 4) {
		t.Fatalf("customer = %d; want the target's %d", moved.CustomerID, customerIDByCode(t, database, 4))
	}
	if moved.CustomerName != "PT. Other" {
		t.Errorf("customer name = %q; want PT. Other", moved.CustomerName)
	}
	// The old customer's billing address must not survive on the moved record.
	if moved.BillingAddress != "Jl. Billing B" {
		t.Errorf("billing address = %q; want the new source's Jl. Billing B", moved.BillingAddress)
	}
	if moved.ConfirmationNumber != nil {
		t.Errorf("an unassigned record gained confirmation number %v", *moved.ConfirmationNumber)
	}

	// After assignment: the number carries the old customer's code, so the
	// move is refused and nothing is written.
	assignedRecord := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-CROSS-2")
	assigned, err := database.AssignPurchaseOrderConfirmationNumber(ctx, assignedRecord.ID, admin.ID)
	if err != nil {
		t.Fatalf("assign confirmation: %v", err)
	}
	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, assigned, admin, otherAgain, "move to the other customer")); !errors.Is(err, ErrPurchaseOrderConfirmationCustomerLocked) {
		t.Fatalf("cross-customer repair after assignment error = %v; want ErrPurchaseOrderConfirmationCustomerLocked", err)
	}
	unchanged, err := database.GetPurchaseOrder(ctx, assigned.ID)
	if err != nil {
		t.Fatalf("read after refused cross-customer repair: %v", err)
	}
	if unchanged.CustomerID != assigned.CustomerID || unchanged.QuotationID != assigned.QuotationID {
		t.Errorf("refused cross-customer repair changed the record: customer %d -> %d, quotation %d -> %d",
			assigned.CustomerID, unchanged.CustomerID, assigned.QuotationID, unchanged.QuotationID)
	}
	if unchanged.Version != assigned.Version {
		t.Errorf("refused cross-customer repair bumped version %d -> %d", assigned.Version, unchanged.Version)
	}
}

func TestRepairPurchaseOrderSourceRefusesAnOccupiedTarget(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_occupied")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Target", "Jl. Billing A", 100, 45)
	saved := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-OCCUPIED")
	// A draft is an active PO for the one-active-record rule, so it occupies
	// the target quotation even before it is saved.
	createFinalizeDraft(t, database, ctx, staff, target)

	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, target, "move to an occupied quotation")); !errors.Is(err, ErrPurchaseOrderTargetOccupied) {
		t.Fatalf("occupied target error = %v; want ErrPurchaseOrderTargetOccupied", err)
	}
	unchanged, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("read after refused repair: %v", err)
	}
	if unchanged.QuotationID != original.ID || unchanged.Version != saved.Version {
		t.Errorf("refused occupied-target repair wrote the record: source %d, version %d",
			unchanged.QuotationID, unchanged.Version)
	}
}

// TestRepairPurchaseOrderSourceLosesTheRaceForAnOccupiedTarget proves the
// unique quotation index, not just the read-check, is what a concurrent
// creator meets. An uncommitted insert into the target quotation is invisible
// to the repair's pre-check; the repair's UPDATE then waits on that index
// entry, and the commit turns the wait into the named refusal.
func TestRepairPurchaseOrderSourceLosesTheRaceForAnOccupiedTarget(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_race")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Target", "Jl. Billing A", 100, 45)
	saved := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-RACE")
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocking insert: %v", err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	if _, err := blocker.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id,
			customer_name, received_date, currency, tax_zone, created_by,
			updated_by
		) VALUES ($1,$2,$3,'draft',$4,$5,$6,$7,$8,$9,$9)`, schema),
		org.ID, target.ID, target.RevisionNumber, customerIDByCode(t, database, 2),
		"PT. Buyer", time.Now(), "USD", "FTZ", staff.ID,
	); err != nil {
		t.Fatalf("uncommitted occupying insert: %v", err)
	}

	input := repairInput(t, saved, admin, target, "move while another record is being created")
	done := make(chan error, 1)
	go func() {
		_, repairErr := database.RepairPurchaseOrderSource(ctx, input)
		done <- repairErr
	}()

	waitDeadline := time.Now().Add(10 * time.Second)
	waited := false
	for time.Now().Before(waitDeadline) {
		var waiting bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database()
				  AND pid <> pg_backend_pid()
				  AND wait_event_type = 'Lock'
				  AND query ILIKE '%UPDATE%purchase_orders%'
			)`).Scan(&waiting); err != nil {
			t.Fatalf("poll for the repair wait: %v", err)
		}
		if waiting {
			waited = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waited {
		_ = blocker.Rollback(ctx)
		t.Fatal("the repair never waited on the target quotation index; the race was not exercised")
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("commit the occupying insert: %v", err)
	}
	if err := <-done; !errors.Is(err, ErrPurchaseOrderTargetOccupied) {
		t.Fatalf("race repair error = %v; want ErrPurchaseOrderTargetOccupied", err)
	}
	unchanged, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("read after lost race: %v", err)
	}
	if unchanged.QuotationID != original.ID {
		t.Errorf("lost race moved the source to %d; want it left on %d", unchanged.QuotationID, original.ID)
	}
}

func TestRepairPurchaseOrderSourceRequiresTheLoadedVersionAndAnExistingTarget(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_versions")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Target", "Jl. Billing A", 100, 45)
	saved := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-VERSION")

	stale := repairInput(t, saved, admin, target, "stale editor")
	stale.ExpectedVersion = saved.Version - 1
	var conflict *PurchaseOrderVersionConflictError
	if _, err := database.RepairPurchaseOrderSource(ctx, stale); !errors.As(err, &conflict) {
		t.Fatalf("stale repair error = %v; want a version conflict", err)
	}
	missingVersion := repairInput(t, saved, admin, target, "no version")
	missingVersion.ExpectedVersion = 0
	if _, err := database.RepairPurchaseOrderSource(ctx, missingVersion); !errors.Is(err, ErrPurchaseOrderVersionRequired) {
		t.Fatalf("missing version error = %v; want ErrPurchaseOrderVersionRequired", err)
	}

	missingQuotation := repairInput(t, saved, admin, target, "gone quotation")
	missingQuotation.QuotationID = 9_999_999
	if _, err := database.RepairPurchaseOrderSource(ctx, missingQuotation); !errors.Is(err, ErrPurchaseOrderSourceNotFound) {
		t.Fatalf("missing quotation error = %v; want ErrPurchaseOrderSourceNotFound", err)
	}
	missingRevision := repairInput(t, saved, admin, target, "gone revision")
	missingRevision.RevisionNumber = 7
	if _, err := database.RepairPurchaseOrderSource(ctx, missingRevision); !errors.Is(err, ErrPurchaseOrderSourceNotFound) {
		t.Fatalf("missing revision error = %v; want ErrPurchaseOrderSourceNotFound", err)
	}

	// Deleted after the review was opened: refused as not found, never
	// recreated under the same identity or source.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_orders WHERE id = $1`, database.configSchema()),
		saved.ID); err != nil {
		t.Fatalf("delete the record: %v", err)
	}
	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, target, "record deleted")); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Fatalf("deleted record error = %v; want ErrPurchaseOrderNotFound", err)
	}
	var count int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.purchase_orders WHERE id = $1`, database.configSchema()),
		saved.ID).Scan(&count); err != nil {
		t.Fatalf("count recreated rows: %v", err)
	}
	if count != 0 {
		t.Errorf("a repair recreated the deleted purchase order (%d rows)", count)
	}
}

func TestRepairPurchaseOrderSourceRecordsTheSelectedDifferences(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_differences")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Target", "Jl. Billing A", 150, 30)
	saved := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-DIFF")

	undecided := repairInput(t, saved, admin, target, "right revision")
	if _, err := database.RepairPurchaseOrderSource(ctx, undecided); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("undecided repair error = %v; want ErrPurchaseOrderUnresolvedDifference", err)
	}
	if after, err := database.GetPurchaseOrder(ctx, saved.ID); err != nil || after.Version != saved.Version {
		t.Fatalf("undecided repair wrote the record: version %d, err %v", after.Version, err)
	}

	applied := repairInput(t, saved, admin, target, "the order followed the right revision")
	// The review's "Use Quotation" choices are applied to the working copy by
	// the caller before the save, exactly as a final save does.
	for index := range applied.Items {
		if applied.Items[index].QuotationItemNo != nil && *applied.Items[index].QuotationItemNo == 1 {
			applied.Items[index].UnitPriceMinor = 150
		}
	}
	applied.PaymentTerms = "30 days"
	applied.Decisions = []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoiceQuotation},
		{Key: "line:1:unitPrice", Choice: PurchaseOrderChoiceQuotation},
	}
	repaired, err := database.RepairPurchaseOrderSource(ctx, applied)
	if err != nil {
		t.Fatalf("repair with decisions: %v", err)
	}
	if repaired.PaymentTerms != "30 days" {
		t.Errorf("payment terms = %q; want the quotation's 30 days", repaired.PaymentTerms)
	}
	if repaired.Items[0].UnitPriceMinor != 150 || repaired.GrandTotalMinor != 300 {
		t.Errorf("line 1 = %d minor, grand = %d; want 150 and 300",
			repaired.Items[0].UnitPriceMinor, repaired.GrandTotalMinor)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 || history[0].Repair == nil {
		t.Fatalf("history = %d rows, repair %v; want one repair snapshot", len(history), history)
	}
	chosen := map[string]string{}
	for _, decision := range history[0].Repair.Decisions {
		chosen[decision.Key] = decision.Choice
	}
	if chosen["header:paymentTerms"] != PurchaseOrderChoiceQuotation ||
		chosen["line:1:unitPrice"] != PurchaseOrderChoiceQuotation {
		t.Errorf("recorded decisions = %v; want both quotation choices", chosen)
	}
	// The snapshot keeps the old values, not a recomputation of the new ones.
	if history[0].PaymentTerms != "45 days" {
		t.Errorf("snapshot payment terms = %q; want the old 45 days", history[0].PaymentTerms)
	}
}

func TestRepairPurchaseOrderSourceRefreshesTheSourceDerivedValues(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_tax")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	nonFTZ := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Non-FTZ target",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "NonFTZ",
		TaxType: "Goods", UnitPrice: 100, PaymentTermDays: 45,
	})
	euro := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Euro target",
		Address: "Jl. Billing A", Currency: "EUR", TaxZone: "FTZ",
		TaxType: "Goods", UnitPrice: 100, PaymentTermDays: 45,
	})
	saved := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-TAX")

	repaired, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, nonFTZ, "the goods are non-FTZ"))
	if err != nil {
		t.Fatalf("tax-basis repair: %v", err)
	}
	if repaired.TaxZone != "NonFTZ" || repaired.PpnBps != 1100 {
		t.Errorf("tax basis = %q/%d bps; want NonFTZ/1100", repaired.TaxZone, repaired.PpnBps)
	}
	if repaired.PpnMinor != 22 {
		t.Errorf("PPN = %d minor; want 22 (11%% of 200)", repaired.PpnMinor)
	}
	if repaired.PrintedPpnMinor != 22 || repaired.PrintedGrandTotalMinor != 222 {
		t.Errorf("printed totals = ppn %d, grand %d; want 22 and 222",
			repaired.PrintedPpnMinor, repaired.PrintedGrandTotalMinor)
	}

	euroRecord := savedPurchaseOrder(t, database, ctx, staff, original, "PO-475-EUR")
	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, euroRecord, admin, euro, "currency change")); !errors.Is(err, ErrPurchaseOrderSourceCurrencyMismatch) {
		t.Fatalf("currency mismatch error = %v; want ErrPurchaseOrderSourceCurrencyMismatch", err)
	}
}

func TestListPOSourceQuotationsRepairModeFreesOnlyTheRepairedRecord(t *testing.T) {
	database := openTestSchema(t, "vera_po_475_selector")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	first := seedRepairQuotation(t, database, 2, "PT. Buyer", "First", "Jl. Billing A", 100, 45)
	second := seedRepairQuotation(t, database, 4, "PT. Other", "Second", "Jl. Billing B", 100, 45)
	saved := savedPurchaseOrder(t, database, ctx, staff, first, "PO-475-SELECTOR")
	createFinalizeDraft(t, database, ctx, staff, second)

	rows, _, err := database.ListPOSourceQuotations(ctx, 1, "", false, saved.ID)
	if err != nil {
		t.Fatalf("repair-mode selector: %v", err)
	}
	byID := map[int64]POSourceQuotation{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if row, ok := byID[first.ID]; !ok || row.ActivePO != nil {
		t.Errorf("the repaired record's quotation = %v; want it selectable with no active PO", row.ActivePO)
	}
	if row, ok := byID[second.ID]; !ok || row.ActivePO == nil {
		t.Errorf("another occupied quotation = %v; want it still disabled", row.ActivePO)
	}

	// Normal entry keeps the repaired record's quotation occupied.
	rows, _, err = database.ListPOSourceQuotations(ctx, 1, "", false, 0)
	if err != nil {
		t.Fatalf("normal selector: %v", err)
	}
	for _, row := range rows {
		if row.ID == first.ID && row.ActivePO == nil {
			t.Error("normal PO entry made an occupied quotation selectable")
		}
	}

	if _, _, err := database.ListPOSourceQuotations(ctx, 1, "", false, 9_999_999); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Fatalf("unknown excluded record error = %v; want ErrPurchaseOrderNotFound", err)
	}
}

// Issue #481: repairing a saved record whose source Quotation was permanently
// deleted. The record is not a new draft and nothing about it may be erased:
// its own values, its customer PO number and an assigned confirmation number
// are the starting point, the former link stays readable until the repair
// commits, and the history records what the link moved from.
func TestRepairSourceMissingPurchaseOrderLinksTheSurvivorAndKeepsItsIdentity(t *testing.T) {
	database := openTestSchema(t, "vera_po_481_missing_repair")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Survives", "PO-481-SURVIVES")
	admin := deletionUser(t, database, "admin")

	assigned, err := database.AssignPurchaseOrderConfirmationNumber(ctx, saved.ID, admin.ID)
	if err != nil {
		t.Fatalf("assign confirmation: %v", err)
	}
	deleteQuotationRecordWithFreshPreview(t, database, admin, ctx, quotation)

	missing, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("read the source-missing record: %v", err)
	}
	if !missing.SourceMissing {
		t.Fatal("the surviving record is not marked source missing")
	}
	if missing.QuotationID != quotation.ID || missing.QuoteNumber != quotation.QuoteNumber {
		t.Errorf("former link = %d/%q; want %d/%q",
			missing.QuotationID, missing.QuoteNumber, quotation.ID, quotation.QuoteNumber)
	}
	if missing.ConfirmationNumber == nil || *missing.ConfirmationNumber != *assigned.ConfirmationNumber {
		t.Errorf("confirmation number = %v; want the assigned %v",
			missing.ConfirmationNumber, assigned.ConfirmationNumber)
	}
	if missing.PONumber != saved.PONumber || missing.DeliveryAddress != saved.DeliveryAddress {
		t.Errorf("readable values drifted: %+v", missing)
	}

	// The archive finds it by the former number and by the independent
	// source-missing predicate while it is waiting for a new source.
	searched, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		Search: quotation.QuoteNumber, Page: 1,
	})
	if err != nil {
		t.Fatalf("search by former quote number: %v", err)
	}
	if len(searched.Rows) != 1 || searched.Rows[0].ID != saved.ID {
		t.Errorf("search by former number = %+v; want the survivor", searched.Rows)
	}
	only, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		SourceMissing: PurchaseOrderSourceMissingOnly, Page: 1,
	})
	if err != nil {
		t.Fatalf("sourceMissing=only: %v", err)
	}
	if len(only.Rows) != 1 || only.Rows[0].ID != saved.ID || !only.Rows[0].SourceMissing {
		t.Errorf("sourceMissing=only = %+v; want the survivor", only.Rows)
	}
	excluded, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		SourceMissing: PurchaseOrderSourceMissingExclude, Page: 1,
	})
	if err != nil {
		t.Fatalf("sourceMissing=exclude: %v", err)
	}
	if len(excluded.Rows) != 0 {
		t.Errorf("sourceMissing=exclude = %+v; want none", excluded.Rows)
	}

	target := seedRepairQuotation(t, database, 2, "PT. Survives", "Replacement source", "Jl. Billing 1", 100, 45)
	review, err := database.ReviewPurchaseOrderSource(ctx, missing.ID, target.ID, target.RevisionNumber)
	if err != nil {
		t.Fatalf("review the source-missing record: %v", err)
	}
	if !review.Order.SourceMissing {
		t.Error("the review lost the missing-source state")
	}
	if review.Source.QuotationID != target.ID || review.Order.QuoteNumber != quotation.QuoteNumber {
		t.Errorf("review source = %d; order quote = %q; want %d and the old %q",
			review.Source.QuotationID, review.Order.QuoteNumber, target.ID, quotation.QuoteNumber)
	}

	repaired, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, missing, admin, target, "the source quotation was deleted"))
	if err != nil {
		t.Fatalf("repair the source-missing record: %v", err)
	}
	if repaired.SourceMissing {
		t.Error("the repaired record still claims a missing source")
	}
	if repaired.QuotationID != target.ID || repaired.SourceRevision != target.RevisionNumber {
		t.Errorf("repaired source = %d/%d; want %d/%d",
			repaired.QuotationID, repaired.SourceRevision, target.ID, target.RevisionNumber)
	}
	if repaired.QuoteNumber != target.QuoteNumber {
		t.Errorf("repaired quote number = %q; want %q", repaired.QuoteNumber, target.QuoteNumber)
	}
	if repaired.ConfirmationNumber == nil || *repaired.ConfirmationNumber != *assigned.ConfirmationNumber {
		t.Errorf("confirmation number = %v; want the assigned %v",
			repaired.ConfirmationNumber, assigned.ConfirmationNumber)
	}
	if repaired.PONumber != saved.PONumber || repaired.DeliveryAddress != saved.DeliveryAddress ||
		repaired.GrandTotalMinor != saved.GrandTotalMinor {
		t.Errorf("repair did not start from the record's own values: %+v", repaired)
	}
	if repaired.Version <= missing.Version {
		t.Errorf("version = %d; want it forward from %d", repaired.Version, missing.Version)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 || history[0].Repair == nil {
		t.Fatalf("history = %+v; want one source repair", history)
	}
	if history[0].Repair.PreviousQuotationID != quotation.ID ||
		history[0].Repair.PreviousQuoteNumber != quotation.QuoteNumber ||
		history[0].Repair.NewQuotationID != target.ID ||
		history[0].Repair.NewQuoteNumber != target.QuoteNumber {
		t.Errorf("repair history = %+v; want former %d/%q -> %d/%q",
			history[0].Repair, quotation.ID, quotation.QuoteNumber, target.ID, target.QuoteNumber)
	}

	// The former reference belongs to the missing state only: the repaired row
	// is a live link again, and the archive follows the new source.
	var formerCleared bool
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT former_quotation_id IS NULL AND former_quotation_uuid IS NULL
		       AND former_quote_number IS NULL AND former_source_revision IS NULL
		FROM %s.purchase_orders WHERE id = $1`, database.configSchema()),
		saved.ID).Scan(&formerCleared); err != nil {
		t.Fatalf("read former source columns: %v", err)
	}
	if !formerCleared {
		t.Error("the repaired record kept a former-source reference")
	}
	only, err = database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		SourceMissing: PurchaseOrderSourceMissingOnly, Page: 1,
	})
	if err != nil {
		t.Fatalf("sourceMissing=only after repair: %v", err)
	}
	if len(only.Rows) != 0 {
		t.Errorf("sourceMissing=only after repair = %+v; want none", only.Rows)
	}
	byNewNumber, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		Search: target.QuoteNumber, Page: 1,
	})
	if err != nil {
		t.Fatalf("search by the new quote number: %v", err)
	}
	if len(byNewNumber.Rows) != 1 || byNewNumber.Rows[0].ID != saved.ID {
		t.Errorf("search by the new number = %+v; want the repaired record", byNewNumber.Rows)
	}
}

// The #475 eligibility rules are repeated for a source-missing record: an
// occupied target, a cross-customer move after assignment and a stale editor
// are all refused without writing anything or erasing the missing state.
func TestRepairSourceMissingPurchaseOrderRepeatsTheSavedEligibilityRules(t *testing.T) {
	database := openTestSchema(t, "vera_po_481_missing_eligibility")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Survives", "PO-481-ELIGIBLE")
	admin := deletionUser(t, database, "admin")
	staff := poTestUser(t, database)

	assigned, err := database.AssignPurchaseOrderConfirmationNumber(ctx, saved.ID, admin.ID)
	if err != nil {
		t.Fatalf("assign confirmation: %v", err)
	}
	deleteQuotationRecordWithFreshPreview(t, database, admin, ctx, quotation)
	missing, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("read the source-missing record: %v", err)
	}

	// An occupied target is refused; the live PO that occupies it is untouched.
	occupied := seedRepairQuotation(t, database, 2, "PT. Survives", "Occupied target", "Jl. Billing 1", 100, 45)
	savedPurchaseOrder(t, database, ctx, staff, occupied, "PO-481-OCCUPIED")
	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, missing, admin, occupied, "move to an occupied source")); !errors.Is(err, ErrPurchaseOrderTargetOccupied) {
		t.Fatalf("occupied target error = %v; want ErrPurchaseOrderTargetOccupied", err)
	}

	// A different customer is refused after the number exists.
	other := seedRepairQuotation(t, database, 4, "PT. Other", "Other customer", "Jl. Billing B", 100, 45)
	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, missing, admin, other, "move to another customer")); !errors.Is(err, ErrPurchaseOrderConfirmationCustomerLocked) {
		t.Fatalf("cross-customer error = %v; want ErrPurchaseOrderConfirmationCustomerLocked", err)
	}

	// A stale editor is a named conflict, never an overwrite.
	stale := repairInput(t, missing, admin, seedRepairQuotation(t, database, 2, "PT. Survives", "Fresh target", "Jl. Billing 1", 100, 45), "stale editor")
	stale.ExpectedVersion = missing.Version - 1
	var conflict *PurchaseOrderVersionConflictError
	if _, err := database.RepairPurchaseOrderSource(ctx, stale); !errors.As(err, &conflict) {
		t.Fatalf("stale repair error = %v; want a version conflict", err)
	}

	// Every refusal left the record exactly where it was: still source missing,
	// with the assigned number and the same version.
	after, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("read after refused repairs: %v", err)
	}
	if !after.SourceMissing || after.QuotationID != quotation.ID {
		t.Errorf("refusals changed the source: %+v", after)
	}
	if after.ConfirmationNumber == nil || *after.ConfirmationNumber != *assigned.ConfirmationNumber {
		t.Errorf("refusals changed the confirmation number: %v", after.ConfirmationNumber)
	}
	if after.Version != missing.Version {
		t.Errorf("refusals moved version %d -> %d", missing.Version, after.Version)
	}
}
