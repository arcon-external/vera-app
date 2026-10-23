package localseed

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/store"
)

func integrationDSN(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run the local seed integration tests")
	}
	return dsn
}

func freshSeedSchema(t *testing.T, schema string) store.Config {
	t.Helper()
	dsn := integrationDSN(t)
	dropSchema(t, dsn, schema)
	return store.Config{DSN: dsn, Schema: schema}
}

func dropSchema(t *testing.T, dsn, schema string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect for schema reset: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
		t.Fatalf("drop schema %s: %v", schema, err)
	}
}

func schemaExists(t *testing.T, dsn, schema string) bool {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to check the schema: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var exists bool
	if err := conn.QueryRow(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", schema).Scan(&exists); err != nil {
		t.Fatalf("check schema %s: %v", schema, err)
	}
	return exists
}

func databaseName(t *testing.T, dsn string) string {
	t.Helper()
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse the test DSN: %v", err)
	}
	return parsed.Database
}

func seedAdmin(t *testing.T, config store.Config) store.AuthUser {
	t.Helper()
	database, err := store.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("open the seed schema: %v", err)
	}
	defer database.Close()
	admin, err := database.CreateUser(context.Background(),
		"seed-admin@example.invalid", "Seed Admin", "not-a-real-password-hash", "admin")
	if err != nil {
		t.Fatalf("create the seed admin: %v", err)
	}
	return admin
}

func seedContext(t *testing.T, database *store.Store) context.Context {
	t.Helper()
	org, err := database.DefaultOrganization(context.Background())
	if err != nil {
		t.Fatalf("resolve the seeded organization: %v", err)
	}
	return store.ContextWithOrg(context.Background(), org.ID)
}

func runSeed(t *testing.T, config store.Config) (Report, error) {
	t.Helper()
	return Run(context.Background(), Options{
		Config: config,
		Target: databaseName(t, config.DSN),
	})
}

func fixtureByKey(t *testing.T, report Report) map[string]FixtureReport {
	t.Helper()
	byKey := make(map[string]FixtureReport, len(report.Fixtures))
	for _, fixture := range report.Fixtures {
		byKey[fixture.Key] = fixture
	}
	return byKey
}

func TestSeedFreshLocalDatabaseHasTheApprovedFixtureMatrix(t *testing.T) {
	config := freshSeedSchema(t, "vera_seed_fresh")
	admin := seedAdmin(t, config)

	report, err := runSeed(t, config)
	if err != nil {
		t.Fatalf("seed a fresh schema: %v", err)
	}
	if report.Mode != ModeCreated {
		t.Fatalf("mode = %q; want %q", report.Mode, ModeCreated)
	}
	if report.LiveQuotations != 6 || report.ExpectedQuotations != 6 {
		t.Fatalf("live quotations = %d/%d; want 6/6", report.LiveQuotations, report.ExpectedQuotations)
	}
	if report.PurchaseOrders != 8 || report.ExpectedPurchaseOrders != 8 {
		t.Fatalf("purchase orders = %d/%d; want 8/8", report.PurchaseOrders, report.ExpectedPurchaseOrders)
	}
	if report.Occupied != 5 || report.Available != 1 {
		t.Fatalf("occupied/available = %d/%d; want 5/1", report.Occupied, report.Available)
	}
	if report.SyntheticSourcesDeleted != 2 {
		t.Fatalf("synthetic sources deleted = %d; want 2", report.SyntheticSourcesDeleted)
	}
	for _, fixture := range report.Fixtures {
		wantState := FixturePresent
		if fixture.Kind == "synthetic_source" {
			wantState = FixtureDeleted
		}
		if fixture.State != wantState {
			t.Errorf("fresh fixture %q (%s) = %q; want %q: %s",
				fixture.Key, fixture.Kind, fixture.State, wantState, fixture.Detail)
		}
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal the report: %v", err)
	}
	for _, forbidden := range []string{"postgres://", "not-a-real-password-hash", "sslmode"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("the report leaks %q", forbidden)
		}
	}

	database, err := store.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("reopen the seeded schema: %v", err)
	}
	defer database.Close()
	ctx := seedContext(t, database)

	quotations, err := database.ListQuotations(ctx)
	if err != nil {
		t.Fatalf("list quotations: %v", err)
	}
	if len(quotations) != 6 {
		t.Fatalf("stored quotations = %d; want exactly 6 live fixtures", len(quotations))
	}
	for _, quotation := range quotations {
		if !strings.Contains(quotation.Subject, Fictional) || !strings.Contains(quotation.CustomerName, "(MOCK)") {
			t.Errorf("stored quotation %q is not clearly fictional: subject %q, customer %q",
				quotation.QuoteNumber, quotation.Subject, quotation.CustomerName)
		}
	}

	all, err := database.ListPurchaseOrders(ctx, store.PurchaseOrderListQuery{
		PageSize: store.PurchaseOrderListMaxPageSize,
	})
	if err != nil {
		t.Fatalf("list purchase orders: %v", err)
	}
	if all.Total != 8 {
		t.Fatalf("stored purchase orders = %d; want exactly 8", all.Total)
	}
	only, err := database.ListPurchaseOrders(ctx, store.PurchaseOrderListQuery{
		PageSize:      store.PurchaseOrderListMaxPageSize,
		SourceMissing: store.PurchaseOrderSourceMissingOnly,
	})
	if err != nil {
		t.Fatalf("list source-missing purchase orders: %v", err)
	}
	if only.Total != 3 {
		t.Fatalf("source-missing rows = %d; want 3 (never-linked, former draft, former finalized)", only.Total)
	}
	exclude, err := database.ListPurchaseOrders(ctx, store.PurchaseOrderListQuery{
		PageSize:      store.PurchaseOrderListMaxPageSize,
		SourceMissing: store.PurchaseOrderSourceMissingExclude,
	})
	if err != nil {
		t.Fatalf("list live-source purchase orders: %v", err)
	}
	if exclude.Total != 5 {
		t.Fatalf("live-source rows = %d; want 5", exclude.Total)
	}
	states := map[string]bool{}
	for _, row := range only.Rows {
		states[row.SourceState] = true
	}
	if !states[store.PurchaseOrderSourceStateNever] || !states[store.PurchaseOrderSourceStateFormer] {
		t.Fatalf("source-missing rows do not show both never and former: %+v", only.Rows)
	}

	fixtures := fixtureByKey(t, report)
	quotationIDs := make(map[string]int64)
	for _, key := range []string{
		"ftz-goods-finalized-pdf",
		"nonftz-goods-finalized-differences",
		"services-nonconstruction-incomplete-draft",
		"services-construction-draft-differences",
		"nonftz-subitems-no-pdf-correction",
		"ftz-goods-available",
	} {
		quotationIDs[key] = fixtures[key].EntityID
	}

	revisionCounts := map[string]int{
		"ftz-goods-finalized-pdf":                   2,
		"nonftz-goods-finalized-differences":        1,
		"services-nonconstruction-incomplete-draft": 1,
		"services-construction-draft-differences":   1,
		"nonftz-subitems-no-pdf-correction":         2,
		"ftz-goods-available":                       2,
	}
	for key, want := range revisionCounts {
		revisions, err := database.ListRevisionNumbers(ctx, quotationIDs[key])
		if err != nil {
			t.Fatalf("list revisions for %q: %v", key, err)
		}
		if len(revisions) != want {
			t.Errorf("%s revisions = %d; want %d", key, len(revisions), want)
		}
		for _, revision := range revisions {
			record, err := database.GetQuotation(ctx, quotationIDs[key], &revision)
			if err != nil {
				t.Fatalf("read %q revision %d: %v", key, revision, err)
			}
			if len(record.PDFData) == 0 {
				t.Errorf("%s revision %d has no synthetic PDF", key, revision)
				continue
			}
			if err := store.ValidatePurchaseOrderPDF(record.PDFData); err != nil {
				t.Errorf("%s revision %d PDF is invalid: %v", key, revision, err)
			}
			if !strings.Contains(string(record.PDFData), "FICTIONAL MOCK QUOTATION") {
				t.Errorf("%s revision %d PDF is not clearly fictional", key, revision)
			}
		}
	}

	type orderExpectation struct {
		key          string
		state        string
		sourceState  string
		document     string
		confirmation bool
		correction   bool
	}
	expectations := []orderExpectation{
		{"po-linked-incomplete-draft", store.PurchaseOrderStateDraft, store.PurchaseOrderSourceStateLive, documentNone, false, false},
		{"po-linked-draft-differences", store.PurchaseOrderStateDraft, store.PurchaseOrderSourceStateLive, documentNone, false, false},
		{"po-finalized-pdf", store.PurchaseOrderStateSaved, store.PurchaseOrderSourceStateLive, documentText, false, false},
		{"po-finalized-differences-adjustment", store.PurchaseOrderStateSaved, store.PurchaseOrderSourceStateLive, documentNone, false, false},
		{"po-finalized-no-pdf-confirmation-correction", store.PurchaseOrderStateSaved, store.PurchaseOrderSourceStateLive, documentNone, true, true},
		{"po-never-linked-mixed-pdf", store.PurchaseOrderStateDraft, store.PurchaseOrderSourceStateNever, documentMixed, false, false},
		{"po-former-linked-draft", store.PurchaseOrderStateDraft, store.PurchaseOrderSourceStateFormer, documentNone, false, false},
		{"po-former-linked-finalized", store.PurchaseOrderStateSaved, store.PurchaseOrderSourceStateFormer, documentText, false, false},
	}
	for _, want := range expectations {
		order, err := database.GetPurchaseOrder(ctx, fixtures[want.key].EntityID)
		if err != nil {
			t.Fatalf("read %q: %v", want.key, err)
		}
		if order.State != want.state || order.SourceState != want.sourceState {
			t.Errorf("%s = state %q source %q; want %q/%q",
				want.key, order.State, order.SourceState, want.state, want.sourceState)
		}
		if want.sourceState != store.PurchaseOrderSourceStateLive {
			if !order.SourceMissing {
				t.Errorf("%s does not report Source missing", want.key)
			}
		}
		if want.key == "po-never-linked-mixed-pdf" {
			if !order.SourceMissing || order.PrintedTotalsKnown {
				t.Errorf("never-linked draft reports sourceMissing=%v printedTotalsKnown=%v; want true/false",
					order.SourceMissing, order.PrintedTotalsKnown)
			}
		}
		document, documentErr := database.GetActivePurchaseOrderDocument(ctx, order.ID)
		switch want.document {
		case documentNone:
			if documentErr == nil {
				t.Errorf("%s unexpectedly has a customer PDF version %d", want.key, document.Version)
			} else if !errors.Is(documentErr, store.ErrPurchaseOrderDocumentNotFound) {
				t.Errorf("%s document read failed: %v", want.key, documentErr)
			}
		case documentText, documentMixed:
			if documentErr != nil {
				t.Fatalf("%s has no customer PDF: %v", want.key, documentErr)
			}
			if err := store.ValidatePurchaseOrderPDF(document.Data); err != nil {
				t.Errorf("%s customer PDF is invalid: %v", want.key, err)
			}
			if !strings.HasPrefix(document.OriginalFilename, "mock-") {
				t.Errorf("%s filename %q is not marked as a mock document", want.key, document.OriginalFilename)
			}
			if !strings.Contains(string(document.Data), "FICTIONAL MOCK CUSTOMER PURCHASE ORDER") {
				t.Errorf("%s customer PDF is not clearly fictional", want.key)
			}
			if want.document == documentMixed && !strings.Contains(string(document.Data), "/Subtype /Image") {
				t.Errorf("%s mixed PDF has no image-only page", want.key)
			}
		}
		if want.confirmation && order.ConfirmationNumber == nil {
			t.Errorf("%s has no confirmation number", want.key)
		}
		if !want.confirmation && order.ConfirmationNumber != nil {
			t.Errorf("%s unexpectedly has confirmation number %q", want.key, *order.ConfirmationNumber)
		}
		if want.correction {
			history, err := database.GetPurchaseOrderHistory(ctx, order.ID)
			if err != nil {
				t.Fatalf("%s history: %v", want.key, err)
			}
			if len(history) == 0 {
				t.Errorf("%s has no correction history", want.key)
			}
		}
	}

	adjusted, err := database.GetPurchaseOrder(ctx,
		fixtures["po-finalized-differences-adjustment"].EntityID)
	if err != nil {
		t.Fatalf("read the adjusted PO: %v", err)
	}
	if adjusted.AdjustmentMinor <= 0 || strings.TrimSpace(adjusted.AdjustmentReason) == "" {
		t.Errorf("adjusted PO = %d/%q; want a positive adjustment and a reason",
			adjusted.AdjustmentMinor, adjusted.AdjustmentReason)
	}

	// The former finalized record must be genuinely ready for the admin source
	// repair: a read-only review against the available Quotation must pass.
	availableID := quotationIDs["ftz-goods-available"]
	available, err := database.GetQuotation(ctx, availableID, nil)
	if err != nil {
		t.Fatalf("read the available quotation: %v", err)
	}
	former, err := database.GetPurchaseOrder(ctx,
		fixtures["po-former-linked-finalized"].EntityID)
	if err != nil {
		t.Fatalf("read the former finalized PO: %v", err)
	}
	if _, err := database.ReviewPurchaseOrderSource(ctx, former.ID, availableID, available.RevisionNumber); err != nil {
		t.Fatalf("the former finalized PO is not ready for source repair against the available quotation: %v", err)
	}

	audits, err := database.ListDeletionAudits(ctx, 10)
	if err != nil {
		t.Fatalf("list deletion audits: %v", err)
	}
	deletedSources := 0
	for _, audit := range audits {
		if audit.TargetType == store.DeletionTargetQuotation && strings.Contains(audit.Reason, "Synthetic mock seed") {
			deletedSources++
		}
	}
	if deletedSources != 2 {
		t.Fatalf("synthetic-source deletion audits = %d; want 2", deletedSources)
	}
	if admin.Role != "admin" {
		t.Fatalf("the seed admin role = %q; want admin", admin.Role)
	}
}

func TestSeedRerunIsIdempotentAndPreservesEdits(t *testing.T) {
	config := freshSeedSchema(t, "vera_seed_rerun")
	admin := seedAdmin(t, config)

	first, err := runSeed(t, config)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	fixtures := fixtureByKey(t, first)
	poID := fixtures["po-linked-incomplete-draft"].EntityID
	quotationID := fixtures["ftz-goods-finalized-pdf"].EntityID

	database, err := store.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("open the seeded schema: %v", err)
	}
	ctx := seedContext(t, database)

	order, err := database.GetPurchaseOrder(ctx, poID)
	if err != nil {
		t.Fatalf("read the draft to edit: %v", err)
	}
	update := baseUpdate(order, admin.ID)
	update.DeliveryAddress = "Edited by the developer (MOCK EDIT)"
	if _, err := database.UpdatePurchaseOrderDraft(ctx, update); err != nil {
		t.Fatalf("edit the draft: %v", err)
	}

	// A new revision is a legitimate user edit of a seeded quotation. The
	// rerun must leave it in place and report the changed revision set.
	record, err := database.GetQuotation(ctx, quotationID, nil)
	if err != nil {
		t.Fatalf("read the quotation to add a revision: %v", err)
	}
	added, err := database.SaveRevision(ctx, store.RevisionDraft{
		QuoteNumber:      record.QuoteNumber,
		QuotationUUID:    record.UUID,
		RevisionNumber:   record.RevisionNumber + 1,
		CustomerCode:     record.CustomerCode,
		CustomerName:     record.CustomerName,
		CustomerAddress:  record.CustomerAddress,
		RecipientName:    record.RecipientName,
		RecipientAddress: record.RecipientAddress,
		Subject:          record.Subject + " - developer edit",
		Incoterm:         record.Incoterm,
		Currency:         record.Currency,
		TaxZone:          record.TaxZone,
		TaxCategory:      record.TaxCategory,
		TaxType:          record.TaxType,
		ServiceCategory:  record.ServiceCategory,
		TaxRateBps:       record.TaxRateBps,
		DiscountPercent:  record.DiscountPercent,
		PaymentTermDays:  record.PaymentTermDays,
		ValidityDays:     record.ValidityDays,
		LeadTimeDays:     record.LeadTimeDays,
		LeadTimeWeeks:    record.LeadTimeWeeks,
		LeadTimeMonths:   record.LeadTimeMonths,
		LeadTimeYears:    record.LeadTimeYears,
		Items:            record.Items,
		Totals:           record.Totals,
		PDFData:          record.PDFData,
		HTMLSnapshot:     record.HTMLSnapshot,
	})
	if err != nil {
		t.Fatalf("add a revision: %v", err)
	}
	if added.RevisionNumber != record.RevisionNumber+1 {
		t.Fatalf("added revision = %d; want %d", added.RevisionNumber, record.RevisionNumber+1)
	}
	database.Close()

	second, err := runSeed(t, config)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if second.Mode != ModeExisting {
		t.Fatalf("mode = %q; want %q", second.Mode, ModeExisting)
	}
	if second.LiveQuotations != 6 || second.PurchaseOrders != 8 || second.Occupied != 5 || second.Available != 1 {
		t.Fatalf("second report = %d quotations, %d POs, %d occupied, %d available; want 6, 8, 5, 1",
			second.LiveQuotations, second.PurchaseOrders, second.Occupied, second.Available)
	}

	database, err = store.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("reopen after the second seed: %v", err)
	}
	defer database.Close()
	ctx = seedContext(t, database)
	quotations, err := database.ListQuotations(ctx)
	if err != nil {
		t.Fatalf("list quotations after rerun: %v", err)
	}
	if len(quotations) != 6 {
		t.Fatalf("quotations after rerun = %d; want 6 (no duplicates)", len(quotations))
	}
	all, err := database.ListPurchaseOrders(ctx, store.PurchaseOrderListQuery{
		PageSize: store.PurchaseOrderListMaxPageSize,
	})
	if err != nil {
		t.Fatalf("list purchase orders after rerun: %v", err)
	}
	if all.Total != 8 {
		t.Fatalf("purchase orders after rerun = %d; want 8 (no duplicates)", all.Total)
	}
	revisions, err := database.ListRevisionNumbers(ctx, quotationID)
	if err != nil {
		t.Fatalf("list the edited quotation revisions: %v", err)
	}
	if len(revisions) != 3 {
		t.Fatalf("edited quotation revisions = %d; want 3 (the seed's 2 plus the developer's 1)", len(revisions))
	}
	edited, err := database.GetPurchaseOrder(ctx, poID)
	if err != nil {
		t.Fatalf("read the edited draft after rerun: %v", err)
	}
	if edited.DeliveryAddress != "Edited by the developer (MOCK EDIT)" {
		t.Fatalf("the rerun overwrote the edited delivery address: %q", edited.DeliveryAddress)
	}
	secondFixtures := fixtureByKey(t, second)
	if secondFixtures["po-linked-incomplete-draft"].State != FixtureChanged {
		t.Errorf("the edited draft is reported as %q; want %q",
			secondFixtures["po-linked-incomplete-draft"].State, FixtureChanged)
	}
	if !strings.Contains(secondFixtures["po-linked-incomplete-draft"].Detail, "edited") {
		t.Errorf("the edited draft detail is %q; want it to name the edit",
			secondFixtures["po-linked-incomplete-draft"].Detail)
	}
	if secondFixtures["ftz-goods-finalized-pdf"].State != FixtureChanged {
		t.Errorf("the quotation with a new revision is reported as %q; want %q",
			secondFixtures["ftz-goods-finalized-pdf"].State, FixtureChanged)
	}
}

// TestSeedRefusesUnsafeTargetsBeforeAnyWrite is the end-to-end proof of the
// safety rule: a refused target leaves no schema, so no migration and no
// fixture marker can have run.
func TestSeedRerunReportsMissingFixturesWithoutRebuilding(t *testing.T) {
	config := freshSeedSchema(t, "vera_seed_missing")
	admin := seedAdmin(t, config)
	first, err := runSeed(t, config)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	removedID := fixtureByKey(t, first)["po-finalized-pdf"].EntityID

	database, err := store.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("open the seeded schema: %v", err)
	}
	ctx := seedContext(t, database)
	preview, err := database.PreviewDeletion(ctx, store.DeletionPreviewInput{
		TargetType: store.DeletionTargetPurchaseOrder,
		TargetID:   removedID,
		ActorID:    admin.ID,
	})
	if err != nil {
		t.Fatalf("preview removing one fixture: %v", err)
	}
	if _, err := database.ConfirmDeletion(ctx, store.DeletionConfirmInput{
		TargetType:          store.DeletionTargetPurchaseOrder,
		TargetID:            removedID,
		Scope:               store.DeletionScopeRecord,
		Reason:              "Developer removed one mock fixture before re-running the seed.",
		TypedNumber:         preview.DocumentNumber,
		ExpectedFingerprint: preview.Fingerprint,
		ActorID:             admin.ID,
	}); err != nil {
		t.Fatalf("remove one fixture: %v", err)
	}
	database.Close()

	second, err := runSeed(t, config)
	if err != nil {
		t.Fatalf("second seed after a missing fixture: %v", err)
	}
	if second.Mode != ModeExisting {
		t.Fatalf("mode = %q; want existing", second.Mode)
	}
	missing := fixtureByKey(t, second)["po-finalized-pdf"]
	if missing.State != FixtureMissing {
		t.Fatalf("removed fixture state = %q; want missing", missing.State)
	}
	if second.PurchaseOrders != 7 {
		t.Fatalf("reported purchase orders = %d; want 7", second.PurchaseOrders)
	}
	if second.Occupied != 4 || second.Available != 2 {
		t.Fatalf("occupied/available = %d/%d; want 4/2 after the removal", second.Occupied, second.Available)
	}

	database, err = store.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("reopen after the missing-fixture rerun: %v", err)
	}
	defer database.Close()
	all, err := database.ListPurchaseOrders(seedContext(t, database), store.PurchaseOrderListQuery{
		PageSize: store.PurchaseOrderListMaxPageSize,
	})
	if err != nil {
		t.Fatalf("list purchase orders: %v", err)
	}
	if all.Total != 7 {
		t.Fatalf("the rerun rebuilt the removed fixture: %d purchase orders, want 7", all.Total)
	}
}

func TestSeedRefusesToRenameAnExistingClient(t *testing.T) {
	config := freshSeedSchema(t, "vera_seed_client_collision")
	seedAdmin(t, config)
	database, err := store.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("open the seed schema: %v", err)
	}
	if _, err := database.CreateClient(context.Background(), 90, "PT Klien Nyata (MOCK-CODE COLLISION)", "Jalan Nyata No. 1"); err != nil {
		t.Fatalf("create the occupying client: %v", err)
	}
	database.Close()

	if _, err := runSeed(t, config); err == nil {
		t.Fatal("the seed renamed an existing client that occupies a mock code")
	} else if !strings.Contains(err.Error(), "client code 90") {
		t.Fatalf("refusal %q does not name the occupied client code", err)
	}

	database, err = store.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("reopen after the refusal: %v", err)
	}
	defer database.Close()
	quotations, err := database.ListQuotations(context.Background())
	if err != nil {
		t.Fatalf("list quotations after the refusal: %v", err)
	}
	if len(quotations) != 0 {
		t.Fatalf("the refused seed wrote %d quotations", len(quotations))
	}
	client, err := database.GetCustomerByCode(context.Background(), 90)
	if err != nil {
		t.Fatalf("read the occupying client: %v", err)
	}
	if client.Name != "PT Klien Nyata (MOCK-CODE COLLISION)" {
		t.Fatalf("the refused seed renamed the client to %q", client.Name)
	}
}

func TestSeedRefusesUnsafeTargetsBeforeAnyWrite(t *testing.T) {
	dsn := integrationDSN(t)
	schema := "vera_seed_refused"
	dropSchema(t, dsn, schema)
	confirmed := databaseName(t, dsn)

	remote := "postgres://vera:fake-password@remote.invalid/" + confirmed + "?sslmode=disable"
	_, err := Run(context.Background(), Options{
		Config: store.Config{DSN: remote, Schema: schema},
		Target: confirmed,
	})
	if err == nil {
		t.Fatal("the seed accepted a remote database")
	}
	if schemaExists(t, dsn, schema) {
		t.Fatal("the refused remote run created the schema")
	}

	_, err = Run(context.Background(), Options{
		Config: store.Config{DSN: dsn, Schema: schema},
		Target: "vera_production_lookalike",
	})
	if err == nil {
		t.Fatal("the seed accepted an unconfirmed database name")
	}
	if schemaExists(t, dsn, schema) {
		t.Fatal("the mismatched confirmation created the schema")
	}

	_, err = Run(context.Background(), Options{
		Config: store.Config{DSN: dsn, Schema: schema},
		Target: "",
	})
	if err == nil {
		t.Fatal("the seed accepted an empty confirmation")
	}
	if schemaExists(t, dsn, schema) {
		t.Fatal("the empty confirmation created the schema")
	}
}
