package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Issue #468: a manual PO draft born from one exact issued Quotation revision.
// These tests drive the real store against a disposable schema: snapshot
// copying, one active record per Quotation under concurrency, replace-lines
// updates, the draft-only guard, and the #465 organization boundary.

// seedPOQuotation writes one issued revision and returns its identity. The
// package line is deliberately a composition so the copy rules are observable:
// a plain line and a priced package whose two parts are the customer-facing
// lines when ShowSubPrices is on.
func seedPOQuotation(t *testing.T, database *Store, customerName, subject string) QuotationRecord {
	t.Helper()
	ctx := context.Background()
	record, err := database.SaveRevision(ctx, RevisionDraft{
		CustomerCode:     2,
		CustomerName:     customerName,
		CustomerAddress:  "Jl. Billing 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          subject,
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		DiscountPercent:  0,
		TaxZone:          "NonFTZ",
		TaxCategory:      "SupplyInstall",
		TaxRateBps:       200,
		Currency:         "USD",
		HTMLSnapshot:     "<html>snapshot</html>",
		Items: []RevisionItem{
			{
				ItemNo:                1,
				Description:           "Plain widget",
				Qty:                   3,
				UoM:                   "Pc",
				UnitPrice:             500,
				MarginEnabled:         true,
				MarginPercent:         20,
				MarginAmount:          300,
				OriginalTotal:         1500,
				MarginalizedUnitPrice: 600,
				MarginalizedTotal:     1800,
				ListUnitPrice:         650,
				DiscountEnabled:       true,
				DiscountPercent:       5,
			},
			{
				ItemNo:                2,
				Description:           "Installed package",
				Qty:                   1,
				UoM:                   "Lot",
				UnitPrice:             1000,
				MarginEnabled:         false,
				MarginPercent:         0,
				MarginAmount:          0,
				OriginalTotal:         1000,
				MarginalizedUnitPrice: 1250,
				MarginalizedTotal:     1250,
				ListUnitPrice:         1250,
				ShowSubPrices:         true,
				SubItems: []RevisionSubItem{
					{
						SubNo:         1,
						Description:   "Mounting",
						Qty:           2,
						UoM:           "Unit",
						UnitCost:      300,
						UnitPrice:     400,
						MarginAmount:  100,
						Total:         800,
						ListUnitPrice: 420,
					},
					{
						SubNo:         2,
						Description:   "Commissioning",
						Qty:           1,
						UoM:           "Lot",
						UnitCost:      200,
						UnitPrice:     450,
						MarginAmount:  250,
						Total:         450,
						ListUnitPrice: 450,
					},
				},
			},
		},
		Totals: RevisionTotals{
			OriginalSubtotal:          2500,
			OriginalGrand:             2500,
			MarginalizedSubtotal:      3050,
			MarginalizedGrand:         3050,
			MarginalizedGrandAfterTax: 3111,
			CustomerGrandTotal:        3111,
		},
	})
	if err != nil {
		t.Fatalf("seed quotation %q: %v", subject, err)
	}
	return record
}

// poTestUser creates the actor a draft records.
func poTestUser(t *testing.T, database *Store) AuthUser {
	t.Helper()
	user, err := database.CreateUser(context.Background(),
		fmt.Sprintf("po-%d@example.com", time.Now().UnixNano()), "PO Staff", "hash", "staff")
	if err != nil {
		t.Fatalf("create PO user: %v", err)
	}
	return user
}

// poContext attaches the seeded organization exactly as the API does.
func poContext(t *testing.T, database *Store) context.Context {
	t.Helper()
	ctx := context.Background()
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	return ContextWithOrg(ctx, org.ID)
}

func dateOnly(value time.Time) string { return value.Format("2006-01-02") }

func TestPurchaseOrderDraftCreatesFromTheExactIssuedRevision(t *testing.T) {
	database := openTestSchema(t, "vera_po_create")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Supply package")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	today := dateOnly(companyToday(time.Now()))
	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create PO draft: %v", err)
	}

	if draft.State != PurchaseOrderStateDraft {
		t.Errorf("state = %q; want draft", draft.State)
	}
	if draft.QuotationID != quotation.ID || draft.SourceRevision != quotation.RevisionNumber {
		t.Errorf("source = quotation %d revision %d; want %d/%d",
			draft.QuotationID, draft.SourceRevision, quotation.ID, quotation.RevisionNumber)
	}
	if draft.QuoteNumber != quotation.QuoteNumber {
		t.Errorf("quote number = %q; want %q", draft.QuoteNumber, quotation.QuoteNumber)
	}
	// Customer identity comes from the selected revision, not the live
	// customer directory (story 30).
	if draft.CustomerName != quotation.CustomerName {
		t.Errorf("customer = %q; want %q", draft.CustomerName, quotation.CustomerName)
	}
	if draft.BillingAddress != quotation.RecipientAddress {
		t.Errorf("billing address = %q; want revision address %q",
			draft.BillingAddress, quotation.RecipientAddress)
	}
	if draft.DeliveryAddress != quotation.RecipientAddress {
		t.Errorf("delivery address = %q; want the billing default %q",
			draft.DeliveryAddress, quotation.RecipientAddress)
	}
	// Commercial basis is inherited, not recomputed.
	if draft.Currency != "USD" || draft.TaxZone != "NonFTZ" || draft.TaxCategory != "SupplyInstall" {
		t.Errorf("basis = %s/%s/%s; want USD/NonFTZ/SupplyInstall",
			draft.Currency, draft.TaxZone, draft.TaxCategory)
	}
	// The Quotation revision stores a day count only, so its wording seeds the
	// PO as that count in words. From here the field is free text.
	if draft.PaymentTerms != "45 days" || draft.Incoterms != "CIF" {
		t.Errorf("terms = %q/%q; want %q/CIF", draft.PaymentTerms, draft.Incoterms, "45 days")
	}
	if draft.PONumber != "" || draft.PODate != nil || draft.RequestedDeliveryDate != nil {
		t.Errorf("optional customer fields were invented: number %q date %v delivery %v",
			draft.PONumber, draft.PODate, draft.RequestedDeliveryDate)
	}
	if draft.CreatedBy != user.ID || draft.UpdatedBy != user.ID {
		t.Errorf("actor = %d/%d; want %d", draft.CreatedBy, draft.UpdatedBy, user.ID)
	}
	if got, want := dateOnly(draft.ReceivedDate), today; got != want {
		t.Errorf("received date = %s; want today %s", got, want)
	}

	// The customer-facing lines: the plain item keeps its printed list price,
	// and the package is represented by its priced parts because that is what
	// the customer document shows (ShowSubPrices).
	if len(draft.Items) != 3 {
		t.Fatalf("lines = %d; want 3 (%v)", len(draft.Items), draft.Items)
	}
	first := draft.Items[0]
	if first.Description != "Plain widget" || first.QuantityScaled != 30000 ||
		first.UoM != "Pc" || first.UnitPriceMinor != 650 {
		t.Errorf("plain line = %+v; want widget qty 3.0000 at 650", first)
	}
	if first.QuotationItemNo == nil || *first.QuotationItemNo != 1 || first.QuotationSubNo != nil {
		t.Errorf("plain line reference = %v/%v; want item 1", first.QuotationItemNo, first.QuotationSubNo)
	}
	second := draft.Items[1]
	if second.Description != "Mounting" || second.QuantityScaled != 20000 ||
		second.UnitPriceMinor != 420 || second.QuotationItemNo == nil ||
		*second.QuotationItemNo != 2 || second.QuotationSubNo == nil || *second.QuotationSubNo != 1 {
		t.Errorf("package line 1 = %+v; want item 2.1 at 420", second)
	}
	third := draft.Items[2]
	if third.Description != "Commissioning" || third.QuantityScaled != 10000 ||
		third.UnitPriceMinor != 450 || third.QuotationSubNo == nil || *third.QuotationSubNo != 2 {
		t.Errorf("package line 2 = %+v; want item 2.2 at 450", third)
	}
	for i, item := range draft.Items {
		if item.LineNumber != i+1 {
			t.Errorf("line %d number = %d; want %d", i, item.LineNumber, i+1)
		}
	}

	// Reopen by id returns the same snapshot.
	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen PO draft: %v", err)
	}
	if reopened.ID != draft.ID || reopened.CustomerName != draft.CustomerName ||
		len(reopened.Items) != len(draft.Items) {
		t.Errorf("reopened = %+v; want the stored draft", reopened)
	}
}

func TestPurchaseOrderDraftDefaultsReceivedDateAndAcceptsAnOverride(t *testing.T) {
	database := openTestSchema(t, "vera_po_received")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Received date")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	past := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	override, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		ReceivedDate:   &past,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create with received override: %v", err)
	}
	if got := dateOnly(override.ReceivedDate); got != "2026-01-15" {
		t.Errorf("received date = %s; want 2026-01-15", got)
	}
}

func TestPurchaseOrderDraftOneActiveRecordPerQuotationUnderConcurrency(t *testing.T) {
	database := openTestSchema(t, "vera_po_unique")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Concurrent")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	const attempts = 8
	var wait sync.WaitGroup
	results := make(chan error, attempts)
	wait.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wait.Done()
			_, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
				QuotationID:    quotation.ID,
				RevisionNumber: quotation.RevisionNumber,
				CreatedBy:      user.ID,
			})
			results <- err
		}()
	}
	wait.Wait()
	close(results)

	created, exists, other := 0, 0, 0
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrPurchaseOrderExists):
			exists++
		default:
			other++
			t.Errorf("unexpected concurrent create error: %v", err)
		}
	}
	if created != 1 || exists != attempts-1 || other != 0 {
		t.Errorf("concurrent creates = %d created / %d refused / %d other; want 1/%d/0",
			created, exists, other, attempts-1)
	}
}

func TestPurchaseOrderDraftRejectsAnUnknownSource(t *testing.T) {
	database := openTestSchema(t, "vera_po_source")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Source")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	if _, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber + 1,
		CreatedBy:      user.ID,
	}); !errors.Is(err, ErrPurchaseOrderSourceNotFound) {
		t.Errorf("missing revision error = %v; want ErrPurchaseOrderSourceNotFound", err)
	}
	if _, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID + 9999,
		RevisionNumber: 0,
		CreatedBy:      user.ID,
	}); !errors.Is(err, ErrPurchaseOrderSourceNotFound) {
		t.Errorf("missing quotation error = %v; want ErrPurchaseOrderSourceNotFound", err)
	}
}

func TestPurchaseOrderDraftUpdateReplacesHeaderAndLines(t *testing.T) {
	database := openTestSchema(t, "vera_po_update")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Update")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	poDate := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	delivery := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	received := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	itemNo, subNo := 2, 1
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID:                    draft.ID,
		BillingAddress:        "Jl. Billing edited",
		DeliveryAddress:       "Jl. Delivery",
		ShipToCode:            "SHIP-1",
		ContactEmail:          "buyer@example.com",
		ContactPhone:          "+62 21 555",
		PONumber:              "PO-9001",
		PODate:                &poDate,
		ReceivedDate:          &received,
		RequestedDeliveryDate: &delivery,
		PaymentTerms:          "50% advance",
		Incoterms:             "FOB",
		UpdatedBy:             user.ID,
		Items: []PurchaseOrderItem{
			{
				LineNumber:      1,
				Description:     "Edited mounting",
				QuotationItemNo: &itemNo,
				QuotationSubNo:  &subNo,
				QuantityScaled:  25000,
				UoM:             "Unit",
				UnitPriceMinor:  425,
			},
			{
				LineNumber:     2,
				Description:    "Extra freight",
				QuantityScaled: 10000,
				UoM:            "Lot",
				UnitPriceMinor: 75,
			},
		},
	})
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if updated.BillingAddress != "Jl. Billing edited" || updated.DeliveryAddress != "Jl. Delivery" ||
		updated.ShipToCode != "SHIP-1" || updated.ContactEmail != "buyer@example.com" ||
		updated.ContactPhone != "+62 21 555" || updated.PONumber != "PO-9001" ||
		updated.PaymentTerms != "50% advance" || updated.Incoterms != "FOB" {
		t.Errorf("header did not persist: %+v", updated)
	}
	if dateOnly(*updated.PODate) != "2026-10-01" || dateOnly(updated.ReceivedDate) != "2026-10-02" ||
		dateOnly(*updated.RequestedDeliveryDate) != "2026-11-20" {
		t.Errorf("dates = %v / %v / %v", updated.PODate, updated.ReceivedDate, updated.RequestedDeliveryDate)
	}
	if updated.UpdatedBy != user.ID {
		t.Errorf("updated by = %d; want %d", updated.UpdatedBy, user.ID)
	}
	if len(updated.Items) != 2 {
		t.Fatalf("lines = %d; want 2", len(updated.Items))
	}
	if updated.Items[0].Description != "Edited mounting" ||
		updated.Items[0].QuotationSubNo == nil || *updated.Items[0].QuotationSubNo != 1 ||
		updated.Items[1].QuotationItemNo != nil {
		t.Errorf("replaced lines = %+v; want the edited pair and an unmapped extra", updated.Items)
	}

	// The quotation snapshot itself is untouched: a PO edit never rewrites
	// the issued revision.
	reloaded, err := database.GetQuotation(ctx, quotation.ID, &quotation.RevisionNumber)
	if err != nil {
		t.Fatalf("reload quotation: %v", err)
	}
	if reloaded.Items[0].Description != "Plain widget" ||
		reloaded.CustomerName != "PT. Buyer" {
		t.Errorf("quotation changed by a PO edit: %+v", reloaded)
	}
}

func TestPurchaseOrderDraftUpdateRefusesUnmappedOrInvalidLines(t *testing.T) {
	database := openTestSchema(t, "vera_po_badline")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Bad line")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	wrongItem := 99
	_, err = updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "30 days after delivery",
		Items: []PurchaseOrderItem{{
			LineNumber: 1, Description: "Bogus", QuotationItemNo: &wrongItem,
			QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 1,
		}},
	})
	if !errors.Is(err, ErrPurchaseOrderBadReference) {
		t.Errorf("unknown item reference error = %v; want ErrPurchaseOrderBadReference", err)
	}

	_, err = updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "30 days after delivery",
		Items: []PurchaseOrderItem{{
			LineNumber: 1, Description: "Zero", QuantityScaled: 0,
			UoM: "Pc", UnitPriceMinor: 1,
		}},
	})
	if err == nil {
		t.Error("a non-positive quantity was stored; want refusal")
	}
}

func TestPurchaseOrderDraftUpdateRefusesANonDraft(t *testing.T) {
	database := openTestSchema(t, "vera_po_notdraft")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "State")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	// A saved row must carry its final identity (migration 032); this test is
	// about the draft-only write guard, so it seeds that identity directly.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_orders
		 SET state = 'saved', po_number = 'PO-LEGACY', po_number_key = 'po-legacy',
		     saved_at = NOW(), saved_by = $2
		 WHERE id = $1`, database.configSchema()),
		draft.ID, user.ID); err != nil {
		t.Fatalf("mark saved: %v", err)
	}

	_, err = updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "30 days after delivery",
	})
	if !errors.Is(err, ErrPurchaseOrderNotDraft) {
		t.Errorf("update saved error = %v; want ErrPurchaseOrderNotDraft", err)
	}
}

func TestPurchaseOrdersAreOrganizationScoped(t *testing.T) {
	database := openTestSchema(t, "vera_po_org")
	poQuotation := seedPOQuotation(t, database, "PT. Buyer", "Scoped")
	user := poTestUser(t, database)
	ctxA := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	// A second tenant, inserted only to prove no query crosses the boundary.
	var orgB string
	if err := pool.QueryRow(ctxA, fmt.Sprintf(
		`INSERT INTO %s.organizations (name) VALUES ('Other Org') RETURNING id::text`,
		database.configSchema())).Scan(&orgB); err != nil {
		t.Fatalf("insert second org: %v", err)
	}
	ctxB := ContextWithOrg(context.Background(), orgB)

	draftA, err := database.CreatePurchaseOrderDraft(ctxA, PurchaseOrderCreate{
		QuotationID:    poQuotation.ID,
		RevisionNumber: poQuotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create in org A: %v", err)
	}
	if _, err := database.GetPurchaseOrder(ctxB, draftA.ID); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("org B read org A's PO: %v; want ErrPurchaseOrderNotFound", err)
	}
	if _, err := updateDraft(t, database, ctxB, PurchaseOrderUpdate{
		ID: draftA.ID, UpdatedBy: user.ID, PaymentTerms: "50% advance",
	}); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("org B updated org A's PO: %v; want ErrPurchaseOrderNotFound", err)
	}

	// The selector occupancy query is scoped the same way: org B sees the
	// Quotation but not org A's active PO.
	rows, _, err := database.ListPOSourceQuotations(ctxB, 1, "Scoped", false, 0)
	if err != nil {
		t.Fatalf("list sources for org B: %v", err)
	}
	for _, row := range rows {
		if row.ID == poQuotation.ID && row.ActivePO != nil {
			t.Errorf("org B saw org A's active PO on quotation %d: %+v", row.ID, row.ActivePO)
		}
	}
}

func TestPurchaseOrderMigrationDeclaresOwnershipAndOneActiveRecord(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.version == 30 {
			sql = migration.sql
		}
	}
	if sql == "" {
		t.Fatal("migration 030 is missing")
	}
	for _, expected := range []string{
		`"vera_test".purchase_orders`,
		`"vera_test".purchase_order_items`,
		"org_id UUID NOT NULL REFERENCES",
		"quotation_id BIGINT NOT NULL REFERENCES",
		"source_revision SMALLINT NOT NULL",
		"state TEXT NOT NULL",
		"received_date DATE NOT NULL",
		"payment_terms TEXT NOT NULL",
		"char_length(payment_terms) <= 500",
		"quantity_scaled BIGINT NOT NULL",
		"unit_price_minor BIGINT NOT NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS purchase_orders_active_quotation_idx",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("migration 030 missing %q", expected)
		}
	}
	// The Judge's representational finding: a day count cannot hold commercial
	// wording. A reintroduced SMALLINT column fails here.
	if strings.Contains(sql, "payment_terms_days") {
		t.Error("migration 030 still declares payment_terms_days; terms are free text")
	}
}

func TestPurchaseOrderDraftRoundTripsCommercialPaymentWording(t *testing.T) {
	database := openTestSchema(t, "vera_po_terms")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Payment wording")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if draft.PaymentTerms != "45 days" {
		t.Fatalf("seeded terms = %q; want the quotation's own %q", draft.PaymentTerms, "45 days")
	}

	// The two wordings the Judge named. Both are real commercial terms and
	// neither is a day count, so both must survive an update and a reopen
	// byte for byte.
	for _, wording := range []string{"50% advance", "30 days after delivery"} {
		updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
			ID:           draft.ID,
			UpdatedBy:    user.ID,
			PaymentTerms: wording,
			Items:        draft.Items,
		})
		if err != nil {
			t.Fatalf("update to %q: %v", wording, err)
		}
		if updated.PaymentTerms != wording {
			t.Errorf("updated terms = %q; want %q", updated.PaymentTerms, wording)
		}
		reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reopen after %q: %v", wording, err)
		}
		if reopened.PaymentTerms != wording {
			t.Errorf("reopened terms = %q; want %q", reopened.PaymentTerms, wording)
		}
	}

	// An over-long paste is refused by the store before the schema's CHECK
	// would, so the editor gets a named field rather than a database error.
	_, err = updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID:           draft.ID,
		UpdatedBy:    user.ID,
		PaymentTerms: strings.Repeat("x", maxPaymentTermsLength+1),
		Items:        draft.Items,
	})
	if !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("over-long terms error = %v; want ErrPurchaseOrderInvalid", err)
	}

	// An empty draft term is allowed: a draft may omit what is not agreed yet.
	cleared, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "   ", Items: draft.Items,
	})
	if err != nil {
		t.Fatalf("clear terms: %v", err)
	}
	if cleared.PaymentTerms != "" {
		t.Errorf("cleared terms = %q; want empty", cleared.PaymentTerms)
	}
}

// TestPurchaseOrderPaymentTermsBoundaryCountsCharacters pins the one length
// contract the editor, this store and the schema's char_length CHECK share:
// Unicode characters, not UTF-8 bytes and not UTF-16 units.
//
// The red-first probe is 200 ideographs: 200 characters but 600 UTF-8 bytes.
// A byte counter refuses a value the schema accepts, which is the defect the
// Judge found; 500 ideographs are 1500 bytes and must still fit.
func TestPurchaseOrderPaymentTermsBoundaryCountsCharacters(t *testing.T) {
	database := openTestSchema(t, "vera_po_terms_unicode")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Unicode terms")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	update := func(terms string) (PurchaseOrder, error) {
		return updateDraft(t, database, ctx, PurchaseOrderUpdate{
			ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: terms, Items: draft.Items,
		})
	}

	// 200 characters, 600 bytes. The old len() guard failed here.
	shortMultiByte := strings.Repeat("界", 200)
	if _, err := update(shortMultiByte); err != nil {
		t.Fatalf("200 ideographs are 200 characters (600 bytes): %v", err)
	}

	// Exactly 500 characters, 1500 bytes: the boundary the schema's
	// char_length accepts.
	atLimit := strings.Repeat("界", maxPaymentTermsLength)
	if got := utf8.RuneCountInString(atLimit); got != maxPaymentTermsLength {
		t.Fatalf("fixture has %d characters, want %d", got, maxPaymentTermsLength)
	}
	updated, err := update(atLimit)
	if err != nil {
		t.Fatalf("500 ideographs are 500 characters (1500 bytes): %v", err)
	}
	if updated.PaymentTerms != atLimit {
		t.Errorf("stored terms changed: got %d characters, want %d",
			utf8.RuneCountInString(updated.PaymentTerms), maxPaymentTermsLength)
	}
	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen at the boundary: %v", err)
	}
	if reopened.PaymentTerms != atLimit {
		t.Errorf("reopened terms = %d characters; want %d",
			utf8.RuneCountInString(reopened.PaymentTerms), maxPaymentTermsLength)
	}

	// 501 characters is one over, whatever its byte length.
	if _, err := update(atLimit + "界"); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("501-character terms error = %v; want ErrPurchaseOrderInvalid", err)
	}

	// Mixed ASCII and multi-byte text at exactly 500 characters. "PO 界 #" is
	// six characters, so 83 repeats plus "AB" is 500, not 500 code units.
	mixed := strings.Repeat("PO 界 #", 83) + "AB"
	if got := utf8.RuneCountInString(mixed); got != maxPaymentTermsLength {
		t.Fatalf("mixed fixture has %d characters, want %d", got, maxPaymentTermsLength)
	}
	updated, err = update(mixed)
	if err != nil {
		t.Fatalf("mixed 500-character terms: %v", err)
	}
	if updated.PaymentTerms != mixed {
		t.Errorf("mixed terms changed on the way through: got %q", updated.PaymentTerms)
	}

	// The schema itself must agree. 500 ideographs above already prove its
	// char_length accepted 1500 bytes; this proves 501 characters is refused
	// even when the store's guard is bypassed.
	_, err = database.pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_orders SET payment_terms = $1 WHERE id = $2`,
		database.configSchema(),
	), strings.Repeat("界", maxPaymentTermsLength+1), draft.ID)
	if err == nil {
		t.Fatal("schema accepted 501 characters; char_length boundary is wrong")
	}
}

// Issue #470: the explained calculation. A draft stores the inputs it was
// given and the exact figures those inputs produce, so reopening it shows the
// same arithmetic even if the calculator is later corrected.

// seedPOQuotationTax is seedPOQuotation with an explicit customer-facing tax
// basis, so PPN (Non-FTZ goods) and no-PPN (FTZ, services) are both reachable.
func seedPOQuotationTax(t *testing.T, database *Store, zone, category string, taxRateBps int16) QuotationRecord {
	t.Helper()
	ctx := context.Background()
	record, err := database.SaveRevision(ctx, RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "PT. Buyer",
		CustomerAddress:  "Jl. Billing 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          "Taxed supply",
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		TaxZone:          zone,
		TaxCategory:      category,
		TaxRateBps:       taxRateBps,
		Currency:         "USD",
		HTMLSnapshot:     "<html>snapshot</html>",
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "Taxed widget",
			Qty:                   1,
			UoM:                   "Pc",
			UnitPrice:             1000,
			MarginalizedUnitPrice: 1000,
			MarginalizedTotal:     1000,
			ListUnitPrice:         1000,
			OriginalTotal:         1000,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:     1000,
			OriginalGrand:        1000,
			MarginalizedSubtotal: 1000,
			MarginalizedGrand:    1000,
			PpnAmount:            0,
			CustomerGrandTotal:   1000,
		},
	})
	if err != nil {
		t.Fatalf("seed taxed quotation: %v", err)
	}
	return record
}

// poCalculationLines reads back only the persisted calculation, so an
// assertion names the money it expects rather than the whole item struct.
func poCalculationLines(items []PurchaseOrderItem) [][3]int64 {
	out := make([][3]int64, 0, len(items))
	for _, item := range items {
		out = append(out, [3]int64{item.GrossMinor, item.DiscountMinor, item.NetMinor})
	}
	return out
}

func TestPurchaseOrderDraftStoresTheExplainedCalculation(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_persist")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Calculated")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Creation copies customer-facing prices, so the whole order is its lines.
	// The seeded first line carries a 5% row discount, and #471 copies it too:
	// starting from the list price alone would raise the PO above the printed
	// row. 1950 less its 97 discount, plus 840 plus 450 = 3143.
	if got := poCalculationLines(draft.Items); !reflect.DeepEqual(got, [][3]int64{
		{1950, 97, 1853}, {840, 0, 840}, {450, 0, 450},
	}) {
		t.Errorf("created lines = %v; want gross/discount/net 1950/97/1853, 840, 450", got)
	}
	if draft.SubtotalMinor != 3143 || draft.DiscountMinor != 0 ||
		draft.PpnBps != 0 || draft.PpnMinor != 0 ||
		draft.AdjustmentMinor != 0 || draft.AdjustmentReason != "" ||
		draft.GrandTotalMinor != 3143 {
		t.Errorf("created totals = %+v; want subtotal/grand 3143 and no PPN", draft)
	}
	// The source document's printed figures are evidence, copied once. The
	// seeded revision printed 3050 after its own discount basis, 0 PPN and a
	// customer grand of 3111 -- none of which the PO calculation overwrites.
	if draft.PrintedSubtotalMinor != 3050 || draft.PrintedDiscountMinor != 0 ||
		draft.PrintedPpnMinor != 0 || draft.PrintedGrandTotalMinor != 3111 {
		t.Errorf("printed snapshot = %d/%d/%d/%d; want 3050/0/0/3111",
			draft.PrintedSubtotalMinor, draft.PrintedDiscountMinor,
			draft.PrintedPpnMinor, draft.PrintedGrandTotalMinor)
	}

	// Now enter a row discount, a whole-PO discount and a signed adjustment.
	itemNo1, itemNo2 := 1, 2
	subNo1, subNo2 := 1, 2
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID:               draft.ID,
		UpdatedBy:        user.ID,
		PaymentTerms:     "45 days",
		DiscountPercent:  10,
		AdjustmentMinor:  -26,
		AdjustmentReason: "Match the printed total",
		Items: []PurchaseOrderItem{
			{
				QuotationItemNo: &itemNo1, QuantityScaled: 25000, UoM: "Pc",
				Description: "Plain widget", UnitPriceMinor: 650,
				DiscountEnabled: true, DiscountPercent: 10,
			},
			{
				QuotationItemNo: &itemNo2, QuotationSubNo: &subNo1, QuantityScaled: 20000,
				UoM: "Unit", Description: "Mounting", UnitPriceMinor: 420,
			},
			{
				QuotationItemNo: &itemNo2, QuotationSubNo: &subNo2, QuantityScaled: 10000,
				UoM: "Lot", Description: "Commissioning", UnitPriceMinor: 450,
				DiscountEnabled: true, DiscountPercent: 50,
			},
		},
	})
	if err != nil {
		t.Fatalf("update with calculation: %v", err)
	}

	// 2.5 x 650 = 1625, 10% is 162.5 and truncates to 162 -> net 1463.
	// 2 x 420 = 840. 1 x 450 = 450, 50% = 225 -> net 225.
	// Subtotal 2528; 10% whole discount is 252.8 -> 252; after 2276.
	// SupplyInstall carries no PPN, so the adjustment is the only signed part.
	wantLines := [][3]int64{{1625, 162, 1463}, {840, 0, 840}, {450, 225, 225}}
	if got := poCalculationLines(updated.Items); !reflect.DeepEqual(got, wantLines) {
		t.Errorf("updated lines = %v; want %v", got, wantLines)
	}
	if updated.SubtotalMinor != 2528 || updated.DiscountMinor != 252 ||
		updated.PpnBps != 0 || updated.PpnMinor != 0 ||
		updated.AdjustmentMinor != -26 || updated.AdjustmentReason != "Match the printed total" ||
		updated.GrandTotalMinor != 2250 {
		t.Errorf("updated totals = %+v; want 2528/252/0/0/-26/2250", updated)
	}
	// The printed source figures did not move with the PO's own numbers.
	if updated.PrintedSubtotalMinor != 3050 || updated.PrintedGrandTotalMinor != 3111 {
		t.Errorf("printed snapshot moved: %d/%d; want 3050/3111",
			updated.PrintedSubtotalMinor, updated.PrintedGrandTotalMinor)
	}

	// Reopening returns the stored figures, not a recomputation: a later
	// calculator change must not rewrite an entered PO.
	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !reflect.DeepEqual(poCalculationLines(reopened.Items), wantLines) ||
		reopened.SubtotalMinor != 2528 || reopened.DiscountMinor != 252 ||
		reopened.AdjustmentMinor != -26 || reopened.GrandTotalMinor != 2250 {
		t.Errorf("reopened calculation = %+v; want the stored figures", reopened)
	}
}

func TestPurchaseOrderPPNIsStoredForNonFTZGoodsOnly(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_ppn")
	quotation := seedPOQuotationTax(t, database, "NonFTZ", "SupplyOnly", 1100)
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 1000 goods, 11% PPN on top: the customer pays 1110 and the tax is a
	// separate printed figure, never part of the discounted subtotal.
	if draft.PpnBps != 1100 || draft.PpnMinor != 110 ||
		draft.SubtotalMinor != 1000 || draft.GrandTotalMinor != 1110 {
		t.Errorf("PPN totals = %+v; want 1100 bps / 110 / subtotal 1000 / grand 1110", draft)
	}

	// A whole discount is taken before PPN: 1000 - 10% = 900, PPN 99, 999.
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID:              draft.ID,
		UpdatedBy:       user.ID,
		PaymentTerms:    "45 days",
		DiscountPercent: 10,
		Items:           draft.Items,
	})
	if err != nil {
		t.Fatalf("update discount: %v", err)
	}
	if updated.DiscountMinor != 100 || updated.PpnMinor != 99 ||
		updated.GrandTotalMinor != 999 {
		t.Errorf("discounted PPN totals = %+v; want discount 100 / PPN 99 / grand 999", updated)
	}
}

func TestPurchaseOrderUpdateRefusesUnexplainedAdjustmentsAndInvalidTotals(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_guard")
	quotation := seedPOQuotationTax(t, database, "NonFTZ", "SupplyOnly", 1100)
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	update := func(patch PurchaseOrderUpdate) (PurchaseOrder, error) {
		patch.ID = draft.ID
		patch.UpdatedBy = user.ID
		patch.PaymentTerms = "45 days"
		patch.Items = draft.Items
		return updateDraft(t, database, ctx, patch)
	}

	if _, err := update(PurchaseOrderUpdate{AdjustmentMinor: -1}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("unexplained adjustment error = %v; want ErrPurchaseOrderInvalid", err)
	}
	if _, err := update(PurchaseOrderUpdate{
		AdjustmentMinor: -1111, AdjustmentReason: "Too far",
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("negative grand total error = %v; want ErrPurchaseOrderInvalid", err)
	}
	if _, err := update(PurchaseOrderUpdate{DiscountPercent: 101}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("discount above 100 error = %v; want ErrPurchaseOrderInvalid", err)
	}
	if _, err := update(PurchaseOrderUpdate{DiscountPercent: -1}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("negative discount error = %v; want ErrPurchaseOrderInvalid", err)
	}

	// Nothing refused above may have been persisted.
	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.DiscountMinor != 0 || reopened.AdjustmentMinor != 0 ||
		reopened.GrandTotalMinor != 1110 {
		t.Errorf("a refused update changed the stored PO: %+v", reopened)
	}
}

func TestPurchaseOrderUpdateRefusesOverflowWithoutWritingHalfACalculation(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_overflow")
	quotation := seedPOQuotationTax(t, database, "FTZ", "", 0)
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "45 days",
		Items: []PurchaseOrderItem{{
			Description: "Impossible", QuantityScaled: math.MaxInt64,
			UoM: "Pc", UnitPriceMinor: math.MaxInt64,
		}},
	})
	if !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("overflow error = %v; want ErrPurchaseOrderInvalid", err)
	}
	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if len(reopened.Items) != 1 || reopened.Items[0].Description != "Taxed widget" ||
		reopened.SubtotalMinor != 1000 || reopened.GrandTotalMinor != 1000 {
		t.Errorf("an overflowing update persisted part of itself: %+v", reopened)
	}
}

// TestPurchaseOrderUpdateRefusesTheJudgeCounterexample is the transport-range
// regression: quantity 3 at 9_007_199_254_740_991 is exact in Go
// (27_021_597_764_222_973) but the same figure cannot survive a JS number. The
// update is refused before anything is written, rather than stored with a value
// the editor would read back one unit lower.
func TestPurchaseOrderUpdateRefusesTheJudgeCounterexample(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_exact_range")
	quotation := seedPOQuotationTax(t, database, "FTZ", "", 0)
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for _, testCase := range []struct {
		name  string
		item  PurchaseOrderItem
		stage string
	}{
		{
			name: "gross above the bound",
			item: PurchaseOrderItem{
				Description: "Precise", QuantityScaled: 30000, UoM: "Pc",
				UnitPriceMinor: MaxExactMinor,
			},
			stage: "line 1 gross",
		},
		{
			name: "unit price above the bound",
			item: PurchaseOrderItem{
				Description: "Precise", QuantityScaled: 10000, UoM: "Pc",
				UnitPriceMinor: MaxExactMinor + 1,
			},
			stage: "line 1 unit price",
		},
		{
			name: "scaled quantity above the bound",
			item: PurchaseOrderItem{
				Description: "Precise", QuantityScaled: MaxExactMinor + 1, UoM: "Pc",
				UnitPriceMinor: 1,
			},
			stage: "line 1 quantity",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
				ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "45 days",
				Items: []PurchaseOrderItem{testCase.item},
			})
			if !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Fatalf("error = %v; want ErrPurchaseOrderInvalid", err)
			}
			if !strings.Contains(err.Error(), testCase.stage) ||
				!strings.Contains(err.Error(), "exceeds the supported amount range") {
				t.Errorf("error = %v; want %q named as out of range", err, testCase.stage)
			}
		})
	}

	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if len(reopened.Items) != 1 || reopened.Items[0].Description != "Taxed widget" ||
		reopened.SubtotalMinor != 1000 {
		t.Errorf("a refused exact-range update changed the PO: %+v", reopened)
	}
}

// TestPurchaseOrderDraftRefusesASourceRevisionWithoutPrintedTotals is the
// missing-evidence guard: a revision with no totals row must never become a PO
// whose printed grand total is a fabricated zero. Creation fails with a named
// source-integrity error and writes nothing.
func TestPurchaseOrderDraftRefusesASourceRevisionWithoutPrintedTotals(t *testing.T) {
	database := openTestSchema(t, "vera_po_source_totals")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "No printed totals")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s.quotation_revision_totals
		WHERE revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE quotation_id = $1 AND revision_number = $2
		)`, schema, schema), quotation.ID, quotation.RevisionNumber); err != nil {
		t.Fatalf("delete source totals: %v", err)
	}

	_, err = database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if !errors.Is(err, ErrPurchaseOrderSourceIncomplete) {
		t.Fatalf("error = %v; want ErrPurchaseOrderSourceIncomplete", err)
	}
	if errors.Is(err, ErrPurchaseOrderSourceNotFound) {
		t.Error("a revision with no totals was reported as a missing revision")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "total") {
		t.Errorf("error = %v; want it to name the missing totals", err)
	}

	var count int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.purchase_orders WHERE quotation_id = $1`, schema),
		quotation.ID).Scan(&count); err != nil {
		t.Fatalf("count purchase orders: %v", err)
	}
	if count != 0 {
		t.Errorf("purchase orders = %d; want none created from incomplete evidence", count)
	}
}

// TestPurchaseOrderDraftRefusesSourceAmountsBeyondExactRange makes the source
// path obey the same bound as an entered line: printed totals and source prices
// that cannot round-trip through a JS number are refused at creation.
func TestPurchaseOrderDraftRefusesSourceAmountsBeyondExactRange(t *testing.T) {
	database := openTestSchema(t, "vera_po_source_range")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Printed too large")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_revision_totals
		SET customer_grand_total = $1
		WHERE revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE quotation_id = $2 AND revision_number = $3
		)`, schema, schema),
		MaxExactMinor+1, quotation.ID, quotation.RevisionNumber); err != nil {
		t.Fatalf("inflate printed total: %v", err)
	}

	_, err = database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if !errors.Is(err, ErrPurchaseOrderSourceOutOfRange) {
		t.Fatalf("printed total error = %v; want ErrPurchaseOrderSourceOutOfRange", err)
	}

	// Restore the totals and push a source unit price past the bound instead.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_revision_totals
		SET customer_grand_total = 3111
		WHERE revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE quotation_id = $1 AND revision_number = $2
		)`, schema, schema), quotation.ID, quotation.RevisionNumber); err != nil {
		t.Fatalf("restore printed total: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_revision_items
		SET list_unit_price = $1
		WHERE revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE quotation_id = $2 AND revision_number = $3
		) AND item_no = 1`, schema, schema),
		MaxExactMinor+1, quotation.ID, quotation.RevisionNumber); err != nil {
		t.Fatalf("inflate source unit price: %v", err)
	}
	_, err = database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if !errors.Is(err, ErrPurchaseOrderSourceOutOfRange) {
		t.Fatalf("source price error = %v; want ErrPurchaseOrderSourceOutOfRange", err)
	}

	// Restore the price and push the source quantity past the scaled bound.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_revision_items
		SET list_unit_price = 650
		WHERE revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE quotation_id = $1 AND revision_number = $2
		) AND item_no = 1`, schema, schema), quotation.ID, quotation.RevisionNumber); err != nil {
		t.Fatalf("restore source price: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_revision_items
		SET qty = $1
		WHERE revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE quotation_id = $2 AND revision_number = $3
		) AND item_no = 1`, schema, schema),
		900719925475, quotation.ID, quotation.RevisionNumber); err != nil {
		t.Fatalf("inflate source quantity: %v", err)
	}
	_, err = database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if !errors.Is(err, ErrPurchaseOrderSourceOutOfRange) {
		t.Fatalf("source quantity error = %v; want ErrPurchaseOrderSourceOutOfRange", err)
	}

	var count int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.purchase_orders WHERE quotation_id = $1`, schema),
		quotation.ID).Scan(&count); err != nil {
		t.Fatalf("count purchase orders: %v", err)
	}
	if count != 0 {
		t.Errorf("purchase orders = %d; want none created from out-of-range evidence", count)
	}
}

// TestPurchaseOrderMigration031EnforcesExactRangesAndTrimmedReasons proves the
// schema backstops independently of the Go guards: a hand-written statement
// cannot store an amount above 2^53-1, an untrimmed whitespace-only reason, or
// a reason longer than 200 characters.
func TestPurchaseOrderMigration031EnforcesExactRangesAndTrimmedReasons(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_schema_bounds")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Schema bounds")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_items
		SET gross_minor = $1, net_minor = $1, discount_minor = 0
		WHERE purchase_order_id = $2 AND line_number = 1`, schema),
		MaxExactMinor+1, draft.ID)
	requirePGConstraint(t, err, "purchase_order_items_exact_range_check")

	_, err = pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET printed_grand_total_minor = $1
		WHERE id = $2`, schema), MaxExactMinor+1, draft.ID)
	requirePGConstraint(t, err, "purchase_orders_exact_range_check")

	// Every code point the shared fixture calls blank must be refused as the
	// whole reason, and every code point it calls content must be storable.
	// The grand formula is kept consistent so the reason check is the one that
	// fails. The fixture is read by the Go calculator and the editor tests too.
	whitespace := poWhitespaceFixture(t)
	for _, point := range whitespace.Blank {
		_, err = pool.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_orders
			SET adjustment_minor = 1,
			    adjustment_reason = $1,
			    grand_total_minor = subtotal_minor - discount_minor + ppn_minor + 1
			WHERE id = $2`, schema), string(rune(point.CodePoint)), draft.ID)
		if err == nil {
			t.Errorf("%s (%U) only was stored as an explained adjustment",
				point.Name, point.CodePoint)
			continue
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" ||
			pgErr.ConstraintName != "purchase_orders_adjustment_explained_check" {
			t.Errorf("%s (%U): error = %v; want the explained-adjustment CHECK",
				point.Name, point.CodePoint, err)
		}
	}
	for _, point := range whitespace.Content {
		_, err = pool.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_orders
			SET adjustment_minor = 1,
			    adjustment_reason = $1,
			    grand_total_minor = subtotal_minor - discount_minor + ppn_minor + 1
			WHERE id = $2`, schema), string(rune(point.CodePoint)), draft.ID)
		if err != nil {
			t.Errorf("%s (%U) is content, but the schema refused it: %v",
				point.Name, point.CodePoint, err)
		}
	}
	// Put the row back to an unexplained zero adjustment before the length
	// check below, so only that CHECK can fail.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET adjustment_minor = 0, adjustment_reason = '',
		    grand_total_minor = subtotal_minor - discount_minor + ppn_minor
		WHERE id = $1`, schema), draft.ID); err != nil {
		t.Fatalf("reset the adjustment: %v", err)
	}

	// The bound is Unicode characters, the same unit Go's rune count and the
	// editor's code-point count use.
	_, err = pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET adjustment_minor = 1,
		    adjustment_reason = $1,
		    grand_total_minor = subtotal_minor - discount_minor + ppn_minor + 1
		WHERE id = $2`, schema), strings.Repeat("界", MaxAdjustmentReasonLength+1), draft.ID)
	if err == nil {
		t.Fatal("a 201-character reason was stored")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("over-long reason error = %v; want a CHECK violation", err)
	}
}

func requirePGConstraint(t *testing.T, err error, constraint string) {
	t.Helper()
	if err == nil {
		t.Fatalf("statement succeeded; want constraint %s to fail", constraint)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error = %v; want a Postgres constraint violation (%s)", err, constraint)
	}
	if pgErr.Code != "23514" || pgErr.ConstraintName != constraint {
		t.Errorf("violation = %s/%s; want 23514/%s",
			pgErr.Code, pgErr.ConstraintName, constraint)
	}
}

// TestPurchaseOrderMigration031RefusesAnEmptyReasonForANonzeroAdjustment pins
// the empty string at the real constraint. The blank class is negated, and a
// negated "+" matches the empty string: the explained-adjustment CHECK then
// passed for a nonzero adjustment with no reason at all, while Go's
// strings.TrimSpace and the editor both refused the same input (issue #470,
// wave 4). The class must be "*" so "no reason" is blank, not content.
func TestPurchaseOrderMigration031RefusesAnEmptyReasonForANonzeroAdjustment(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_empty_reason")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Empty reason")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if draft.SubtotalMinor <= 1 {
		t.Fatalf("draft subtotal = %d; want a positive base for the adjustment", draft.SubtotalMinor)
	}

	// writeReason is the hand-written path the CHECK exists for: the Go store
	// would have normalised the reason, so only direct SQL can reach it.
	writeReason := func(amount int64, reason string) error {
		t.Helper()
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_orders
			SET adjustment_minor = $1,
			    adjustment_reason = $2,
			    grand_total_minor = subtotal_minor - discount_minor + ppn_minor + $1
			WHERE id = $3`, schema), amount, reason, draft.ID)
		return err
	}

	// The regression: nonzero with no reason at all.
	requirePGConstraint(t, writeReason(1, ""), "purchase_orders_adjustment_explained_check")
	requirePGConstraint(t, writeReason(-1, ""), "purchase_orders_adjustment_explained_check")

	// Whitespace-only was already refused; keep it beside the empty case so a
	// future rewrite of the class cannot trade one for the other.
	requirePGConstraint(t, writeReason(1, "\t"), "purchase_orders_adjustment_explained_check")
	requirePGConstraint(t, writeReason(1, "\u00a0"), "purchase_orders_adjustment_explained_check")

	// A real reason is what the column is for.
	if err := writeReason(1, "Match the printed total"); err != nil {
		t.Fatalf("a real reason was refused: %v", err)
	}

	// A zero adjustment carries no reason, and that stays legal: it is how
	// every ordinary draft is stored.
	if err := writeReason(0, ""); err != nil {
		t.Fatalf("a zero adjustment with no reason was refused: %v", err)
	}
}

func TestPurchaseOrderLinesMatchNestedQuotationSubItems(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_nested")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Nested")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	itemNo, subNo := 2, 2
	// Item 2.2 is a line of the frozen revision; 2.3 is not.
	accepted, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "45 days",
		Items: []PurchaseOrderItem{{
			Description: "Commissioning", QuotationItemNo: &itemNo, QuotationSubNo: &subNo,
			QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 450,
		}},
	})
	if err != nil {
		t.Fatalf("item 2.2 rejected: %v", err)
	}
	if len(accepted.Items) != 1 || *accepted.Items[0].QuotationSubNo != 2 {
		t.Errorf("accepted line = %+v; want item 2.2", accepted.Items)
	}
	if accepted.SubtotalMinor != 450 || accepted.GrandTotalMinor != 450 {
		t.Errorf("nested line totals = %d/%d; want 450/450",
			accepted.SubtotalMinor, accepted.GrandTotalMinor)
	}

	missing := 3
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "45 days",
		Items: []PurchaseOrderItem{{
			Description: "Missing", QuotationItemNo: &itemNo, QuotationSubNo: &missing,
			QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 450,
		}},
	}); !errors.Is(err, ErrPurchaseOrderBadReference) {
		t.Errorf("item 2.3 error = %v; want ErrPurchaseOrderBadReference", err)
	}
}

func TestPurchaseOrderMigration031DeclaresTheCalculation(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.version == 31 {
			sql = migration.sql
		}
	}
	if sql == "" {
		t.Fatal("migration 031 is missing")
	}
	for _, expected := range []string{
		`"vera_test".purchase_orders`,
		`"vera_test".purchase_order_items`,
		"discount_percent SMALLINT NOT NULL DEFAULT 0",
		"subtotal_minor BIGINT NOT NULL",
		"discount_minor BIGINT NOT NULL",
		"ppn_bps SMALLINT NOT NULL",
		"ppn_minor BIGINT NOT NULL",
		"adjustment_minor BIGINT NOT NULL",
		"adjustment_reason TEXT NOT NULL",
		"char_length(adjustment_reason) <= 200",
		"grand_total_minor BIGINT NOT NULL",
		"printed_subtotal_minor BIGINT NOT NULL",
		"printed_discount_minor BIGINT NOT NULL",
		"printed_ppn_minor BIGINT NOT NULL",
		"printed_grand_total_minor BIGINT NOT NULL",
		"quantity_scaled::numeric * unit_price_minor / 10000",
		"purchase_order_items_exact_range_check",
		"purchase_orders_exact_range_check",
		"9007199254740991",
		"adjustment_reason !~",
		"purchase_orders_grand_formula_check",
		"purchase_orders_adjustment_explained_check",
		"purchase_order_items_net_formula_check",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("migration 031 missing %q", expected)
		}
	}
}

// TestPurchaseOrderMigration031BackfillsAPreCalculationDraft exercises the
// embedded backfill against a row shaped like a #468 draft: entered lines, no
// discounts, no adjustment, no stored calculation. The integration branch is
// unreleased, so no production row can be pre-031 -- but the migration must
// still leave a disposable schema's draft correct rather than reading zero.
//
// It runs the migration's own UPDATE statements (not a copy) after zeroing the
// calculated columns, so a change to the embedded SQL is what this test reads.
func TestPurchaseOrderMigration031BackfillsAPreCalculationDraft(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_backfill")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Backfill")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	// Reshape the lines to two rounding boundaries the backfill must match the
	// Go calculator on: 0.5000 x 1 = 0.5 -> 1 (half away from zero), and
	// 0.3333 x 300 = 99.99 -> 100.
	lines := []struct {
		qty   int64
		price int64
	}{
		{5000, 1},
		{3333, 300},
	}
	for index, line := range lines {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_order_items
			SET quantity_scaled = $1, unit_price_minor = $2,
			    gross_minor = 0, discount_minor = 0, net_minor = 0
			WHERE purchase_order_id = $3 AND line_number = $4`, schema),
			line.qty, line.price, draft.ID, index+1); err != nil {
			t.Fatalf("reshape line %d: %v", index+1, err)
		}
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s.purchase_order_items
		WHERE purchase_order_id = $1 AND line_number > $2`, schema),
		draft.ID, len(lines)); err != nil {
		t.Fatalf("drop extra lines: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET subtotal_minor = 0, discount_minor = 0, ppn_bps = 0, ppn_minor = 0,
		    adjustment_minor = 0, adjustment_reason = '', grand_total_minor = 0,
		    printed_subtotal_minor = 0, printed_discount_minor = 0,
		    printed_ppn_minor = 0, printed_grand_total_minor = 0
		WHERE id = $1`, schema), draft.ID); err != nil {
		t.Fatalf("zero the calculation: %v", err)
	}

	// loadMigrations quotes the schema itself, so hand it the bare name.
	migrations, err := loadMigrations("vera_po_calc_backfill")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var migrationSQL string
	for _, migration := range migrations {
		if migration.version == 31 {
			migrationSQL = migration.sql
		}
	}
	start := strings.Index(migrationSQL, "-- Drafts created before this migration")
	end := strings.Index(migrationSQL, "-- The two totals invariants")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("cannot locate the backfill block in migration 031")
	}
	// The backfill runs before 031 adds its total invariants, so the
	// simulation removes them first and re-adds the migration's own block
	// afterwards -- otherwise a legitimate intermediate state (subtotal set,
	// grand still zero) would be refused by a constraint that did not exist
	// when the backfill was written.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		ALTER TABLE %s.purchase_orders
			DROP CONSTRAINT purchase_orders_grand_formula_check,
			DROP CONSTRAINT purchase_orders_discount_bound_check,
			DROP CONSTRAINT purchase_orders_adjustment_explained_check;
		ALTER TABLE %s.purchase_order_items
			DROP CONSTRAINT purchase_order_items_net_formula_check,
			DROP CONSTRAINT purchase_order_items_discount_bound_check;`,
		schema, schema)); err != nil {
		t.Fatalf("drop the calculation constraints: %v", err)
	}
	if _, err := pool.Exec(ctx, migrationSQL[start:end]); err != nil {
		t.Fatalf("run migration 031 backfill: %v", err)
	}
	if _, err := pool.Exec(ctx, migrationSQL[end:]); err != nil {
		t.Fatalf("re-add the calculation constraints: %v", err)
	}

	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen after backfill: %v", err)
	}
	if got := poCalculationLines(reopened.Items); !reflect.DeepEqual(got, [][3]int64{
		{1, 0, 1}, {100, 0, 100},
	}) {
		t.Errorf("backfilled lines = %v; want 1/1 and 100/100", got)
	}
	// FTZ goods in the seed carry no PPN; the printed figures come back from
	// the source revision the draft was created from.
	if reopened.SubtotalMinor != 101 || reopened.PpnBps != 0 ||
		reopened.PpnMinor != 0 || reopened.GrandTotalMinor != 101 {
		t.Errorf("backfilled totals = %+v; want subtotal/grand 101 and no PPN", reopened)
	}
	if reopened.PrintedSubtotalMinor != 3050 || reopened.PrintedGrandTotalMinor != 3111 {
		t.Errorf("backfilled printed totals = %d/%d; want 3050/3111",
			reopened.PrintedSubtotalMinor, reopened.PrintedGrandTotalMinor)
	}
}

// TestPurchaseOrderMigration031RefusesToFabricatePrintedTotals builds a real
// pre-031 schema, writes a draft shaped exactly like a #468 record, removes the
// source revision's totals row, and runs the real migrator. 031 must fail and
// roll back -- no printed column may exist to read back as zero -- then succeed
// once the evidence is restored.
func TestPurchaseOrderMigration031RefusesToFabricatePrintedTotals(t *testing.T) {
	dsn := testDSN(t)
	schema := "vera_po_calc_backfill_missing"
	pool := migrateThrough(t, dsn, schema, 30)
	database := &Store{pool: pool, config: Config{DSN: dsn, Schema: schema}}
	t.Cleanup(database.Close)

	quotation := seedPOQuotation(t, database, "PT. Buyer", "Missing printed evidence")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	qualified := quoteIdentifier(schema)

	var customerID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT customer_id FROM %s.quotations WHERE id = $1`, qualified),
		quotation.ID).Scan(&customerID); err != nil {
		t.Fatalf("read the quotation customer: %v", err)
	}
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}

	var draftID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, customer_id, customer_name,
			received_date, currency, tax_zone, tax_category, created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, CURRENT_DATE, 'USD', 'NonFTZ', 'SupplyInstall', $6, $6)
		RETURNING id`, qualified),
		org.ID, quotation.ID, quotation.RevisionNumber, customerID,
		quotation.CustomerName, user.ID).Scan(&draftID); err != nil {
		t.Fatalf("insert the pre-031 draft: %v", err)
	}
	for lineNumber, line := range []struct {
		description string
		qty         int64
		price       int64
	}{
		{"Half unit", 5000, 1},
		{"Rounding boundary", 3333, 300},
	} {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_items (
				purchase_order_id, line_number, description, quantity_scaled, uom, unit_price_minor
			) VALUES ($1, $2, $3, $4, 'Pc', $5)`, qualified),
			draftID, lineNumber+1, line.description, line.qty, line.price); err != nil {
			t.Fatalf("insert pre-031 line %d: %v", lineNumber+1, err)
		}
	}

	var revisionID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM %s.quotation_revisions
		WHERE quotation_id = $1 AND revision_number = $2`, qualified),
		quotation.ID, quotation.RevisionNumber).Scan(&revisionID); err != nil {
		t.Fatalf("read the source revision: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.quotation_revision_totals WHERE revision_id = $1`, qualified),
		revisionID); err != nil {
		t.Fatalf("remove the source totals: %v", err)
	}

	err = database.Migrate(ctx)
	if err == nil {
		t.Fatal("migration 031 fabricated printed totals from a missing source")
	}
	if !strings.Contains(err.Error(), "031") ||
		!strings.Contains(err.Error(), "no source revision totals") {
		t.Errorf("migration error = %v; want it to name 031 and the missing source totals", err)
	}
	var recorded bool
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM %s.schema_migrations WHERE version = 31)`, qualified)).
		Scan(&recorded); err != nil {
		t.Fatalf("check migration bookkeeping: %v", err)
	}
	if recorded {
		t.Error("migration 031 was recorded despite failing")
	}
	var printedColumn bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = 'purchase_orders'
			  AND column_name = 'printed_subtotal_minor'
		)`, schema).Scan(&printedColumn); err != nil {
		t.Fatalf("check the rolled-back columns: %v", err)
	}
	if printedColumn {
		t.Error("031 left its printed columns behind after rolling back; a zero could be read as evidence")
	}

	// Restore the evidence and migrate again: the same draft must backfill to
	// the figures the source revision actually printed, not to zero.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_revision_totals (
			revision_id, original_subtotal, original_discount, original_grand_total,
			marginalized_subtotal, marginalized_discount, marginalized_grand_total,
			tax_amount, marginalized_grand_after_tax, profit_amount, nett_profit,
			ppn_amount, customer_grand_total
		) VALUES ($1, 0, 0, 0, 3050, 0, 3050, 0, 3111, 0, 0, 0, 3111)`, qualified),
		revisionID); err != nil {
		t.Fatalf("restore the source totals: %v", err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate after restoring the evidence: %v", err)
	}
	reopened, err := database.GetPurchaseOrder(ctx, draftID)
	if err != nil {
		t.Fatalf("reopen after backfill: %v", err)
	}
	if reopened.SubtotalMinor != 101 || reopened.GrandTotalMinor != 101 {
		t.Errorf("backfilled totals = %d/%d; want 101/101",
			reopened.SubtotalMinor, reopened.GrandTotalMinor)
	}
	if reopened.PrintedSubtotalMinor != 3050 ||
		reopened.PrintedDiscountMinor != 0 ||
		reopened.PrintedPpnMinor != 0 ||
		reopened.PrintedGrandTotalMinor != 3111 {
		t.Errorf("backfilled printed totals = %d/%d/%d/%d; want 3050/0/0/3111",
			reopened.PrintedSubtotalMinor, reopened.PrintedDiscountMinor,
			reopened.PrintedPpnMinor, reopened.PrintedGrandTotalMinor)
	}
}

// migrateThrough applies the embedded migrations up to and including version
// `through`, recording them in a fresh schema exactly as Migrate does. It lets
// a test hold a schema at the shape a draft was written against before the next
// migration existed.
func migrateThrough(t *testing.T, dsn, schema string, through int) *pgxpool.Pool {
	t.Helper()
	resetTestSchema(t, dsn, schema)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for migration setup: %v", err)
	}
	qualified := quoteIdentifier(schema)
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+qualified); err != nil {
		pool.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, qualified)); err != nil {
		pool.Close()
		t.Fatalf("create schema_migrations: %v", err)
	}
	migrations, err := loadMigrations(schema)
	if err != nil {
		pool.Close()
		t.Fatalf("load migrations: %v", err)
	}
	applied := 0
	for _, migration := range migrations {
		if migration.version > through {
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			pool.Close()
			t.Fatalf("begin migration %d: %v", migration.version, err)
		}
		if _, err := tx.Exec(ctx, migration.sql); err != nil {
			_ = tx.Rollback(ctx)
			pool.Close()
			t.Fatalf("apply migration %d (%s): %v", migration.version, migration.name, err)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s.schema_migrations (version, name) VALUES ($1, $2)`, qualified),
			migration.version, migration.name); err != nil {
			_ = tx.Rollback(ctx)
			pool.Close()
			t.Fatalf("record migration %d: %v", migration.version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			pool.Close()
			t.Fatalf("commit migration %d: %v", migration.version, err)
		}
		applied++
	}
	if applied != through {
		pool.Close()
		t.Fatalf("applied %d migrations through version %d", applied, through)
	}
	return pool
}

// seedPOQuotationModernTax writes a revision under the current tax model:
// tax_type and service_category carry the basis, and the legacy tax_category is
// empty, exactly as a quotation prepared by today's editor stores it.
func seedPOQuotationModernTax(t *testing.T, database *Store, zone, taxType, serviceCategory string) QuotationRecord {
	t.Helper()
	record, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "PT. Buyer",
		CustomerAddress:  "Jl. Billing 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          "Modern tax basis",
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		TaxZone:          zone,
		TaxType:          taxType,
		ServiceCategory:  serviceCategory,
		TaxRateBps:       200,
		Currency:         "USD",
		HTMLSnapshot:     "<html>snapshot</html>",
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "Modern widget",
			Qty:                   1,
			UoM:                   "Pc",
			UnitPrice:             1000,
			MarginalizedUnitPrice: 1000,
			MarginalizedTotal:     1000,
			ListUnitPrice:         1000,
			OriginalTotal:         1000,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:     1000,
			OriginalGrand:        1000,
			MarginalizedSubtotal: 1000,
			MarginalizedGrand:    1000,
			CustomerGrandTotal:   1000,
		},
	})
	if err != nil {
		t.Fatalf("seed modern quotation: %v", err)
	}
	return record
}

// TestPurchaseOrderPPNResolvesTheCurrentTaxModel is the regression guard for a
// quotation issued after the Goods/Services split: its legacy tax_category is
// empty, so a resolver that read only the legacy field refused to create the PO
// at all. The rate is stored once, and an update reuses it rather than
// re-resolving against a later tax model.
func TestPurchaseOrderPPNResolvesTheCurrentTaxModel(t *testing.T) {
	database := openTestSchema(t, "vera_po_calc_modern_tax")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	goods := seedPOQuotationModernTax(t, database, "NonFTZ", "Goods", "")
	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    goods.ID,
		RevisionNumber: goods.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create goods PO from the current tax model: %v", err)
	}
	if draft.PpnBps != 1100 || draft.PpnMinor != 110 ||
		draft.GrandTotalMinor != 1110 {
		t.Errorf("goods PPN = %+v; want 1100 bps / 110 / grand 1110", draft)
	}

	// An update keeps the stored rate: it is a snapshot of the source's basis,
	// not a fresh resolution.
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PaymentTerms: "45 days",
		DiscountPercent: 10, Items: draft.Items,
	})
	if err != nil {
		t.Fatalf("update modern goods PO: %v", err)
	}
	if updated.PpnBps != 1100 || updated.PpnMinor != 99 ||
		updated.GrandTotalMinor != 999 {
		t.Errorf("updated goods PPN = %+v; want 1100 bps / 99 / grand 999", updated)
	}

	services := seedPOQuotationModernTax(t, database, "NonFTZ", "Services", "NonConstruction")
	servicePO, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    services.ID,
		RevisionNumber: services.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create services PO from the current tax model: %v", err)
	}
	if servicePO.PpnBps != 0 || servicePO.PpnMinor != 0 ||
		servicePO.GrandTotalMinor != 1000 {
		t.Errorf("services PPN = %+v; want no PPN and grand 1000", servicePO)
	}
}
