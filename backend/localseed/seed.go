package localseed

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"vera-app/backend/store"
)

// Run modes. "created" means this invocation wrote the fixture set; "existing"
// means a previous run marker was found and nothing was created.
const (
	ModeCreated  = "created"
	ModeExisting = "existing"
)

// Fixture states reported for one manifest entry.
const (
	FixturePresent = "present"
	FixtureChanged = "changed"
	FixtureMissing = "missing"
	FixtureDeleted = "deleted"
)

const (
	markerTable  = "local_seed_fixtures"
	markerRunKey = "run"
)

var markerVersion = strconv.Itoa(ManifestVersion)

// Options is one seed invocation. Config is the already-resolved Postgres
// configuration (VERA_POSTGRES_DSN / VERA_POSTGRES_SCHEMA or the PG* fallback);
// Target is the operator's explicit confirmation of the database name.
type Options struct {
	Config     store.Config
	Target     string
	ActorEmail string
}

// FixtureReport is one manifest entry's state.
type FixtureReport struct {
	Key      string
	Kind     string
	Label    string
	State    string
	Detail   string
	EntityID int64
}

// Report is what one seed run observed or created. It contains no DSN, user or
// password: only the host, database and schema, and the fictional fixtures.
type Report struct {
	Target                  string
	Mode                    string
	ManifestVersion         int
	ActorEmail              string
	ExpectedQuotations      int
	ExpectedPurchaseOrders  int
	LiveQuotations          int
	PurchaseOrders          int
	Occupied                int
	Available               int
	SyntheticSourcesDeleted int
	Fixtures                []FixtureReport
	Notes                   []string
}

// Run validates the target, then either creates the manifest or reports the
// existing fixture set. It never deletes a fixture and never rebuilds a
// partial set: the run marker is written before the first fixture, so a crash
// produces a reportable incomplete state rather than a duplicate one on the
// next run.
func Run(ctx context.Context, opts Options) (Report, error) {
	manifest := MockManifest()
	if err := manifest.Validate(); err != nil {
		return Report{}, fmt.Errorf("local seed manifest: %w", err)
	}

	target, err := ValidateTarget(opts.Config, opts.Target)
	if err != nil {
		return Report{}, err
	}
	parsed, err := pgx.ParseConfig(opts.Config.DSN)
	if err != nil {
		return Report{}, fmt.Errorf("local seed target: cannot parse the resolved Postgres configuration; refusing before any migration or write")
	}
	if err := verifyConnectedTarget(ctx, parsed, target.Database); err != nil {
		return Report{}, err
	}

	// Only after both the resolved-target check and the connected-database
	// check may anything migrate or write.
	database, err := store.Open(ctx, opts.Config)
	if err != nil {
		return Report{}, fmt.Errorf("local seed: open the confirmed local store: %w", err)
	}
	defer database.Close()

	// The seeded organization is the migration's single row. Purchase Order
	// writes and the admin deletion lifecycle are tenant-owned and require it on
	// the context; quotations are not, but one context serves the whole run.
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("local seed: resolve the local organization: %w", err)
	}
	ctx = store.ContextWithOrg(ctx, org.ID)

	conn, err := pgx.ConnectConfig(ctx, parsed)
	if err != nil {
		return Report{}, fmt.Errorf("local seed: connect to the confirmed local database %q: %s",
			target.Database, sanitizeConnectionError(err, parsed.Password))
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := ensureMarkerTable(ctx, conn, target.Schema); err != nil {
		return Report{}, err
	}

	actor, err := pickActor(ctx, database, opts.ActorEmail)
	if err != nil {
		return Report{}, err
	}

	existing, markers, err := readMarkers(ctx, conn, target.Schema)
	if err != nil {
		return Report{}, err
	}
	if existing {
		report, err := inspect(ctx, database, target, actor, markers, ModeExisting)
		report.Notes = append(report.Notes,
			"An earlier seed run marker was found. No fixture was created, overwritten or deleted.")
		return report, err
	}

	// A preflight read before the first fixture write: client codes are
	// immutable, and SaveRevision's customer upsert would otherwise rename an
	// existing real client that happens to occupy a mock code.
	if err := checkCustomerCodes(ctx, database, manifest); err != nil {
		return Report{}, err
	}

	tag, err := conn.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (fixture_key, kind, entity_id, entity_uuid, fingerprint, state)
		VALUES ($1, 'run', 0, '', $2, 'started')
		ON CONFLICT (fixture_key) DO NOTHING`, markerIdentifier(target.Schema)),
		markerRunKey, markerVersion)
	if err != nil {
		return Report{}, fmt.Errorf("local seed: record the run marker: %w", err)
	}
	if tag.RowsAffected() != 1 {
		// Another process won the race; report its state rather than creating a
		// second set.
		_, markers, readErr := readMarkers(ctx, conn, target.Schema)
		if readErr != nil {
			return Report{}, readErr
		}
		report, inspectErr := inspect(ctx, database, target, actor, markers, ModeExisting)
		report.Notes = append(report.Notes,
			"Another seed run recorded the run marker first. No fixture was created by this invocation.")
		return report, inspectErr
	}

	report, createErr := createFixtures(ctx, database, conn, target, actor, manifest)
	if createErr != nil {
		_, markers, readErr := readMarkers(ctx, conn, target.Schema)
		if readErr != nil {
			return Report{}, fmt.Errorf("%w (and the partial-state report failed: %v)", createErr, readErr)
		}
		partial, inspectErr := inspect(ctx, database, target, actor, markers, ModeCreated)
		if inspectErr != nil {
			return partial, fmt.Errorf("%w (and the partial-state report failed: %v)", createErr, inspectErr)
		}
		return partial, createErr
	}
	return report, nil
}

func ensureMarkerTable(ctx context.Context, conn *pgx.Conn, schema string) error {
	if _, err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			fixture_key TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			entity_id BIGINT NOT NULL DEFAULT 0,
			entity_uuid TEXT NOT NULL DEFAULT '',
			fingerprint TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, markerIdentifier(schema))); err != nil {
		return fmt.Errorf("local seed: create the local marker table in schema %s: %w", schema, err)
	}
	return nil
}

func markerIdentifier(schema string) string {
	return pgx.Identifier{schema, markerTable}.Sanitize()
}

// checkCustomerCodes keeps the fictional fixtures from renaming a client that
// already occupies one of their codes. The check is a read; a refusal here
// happens before the run marker and before any fixture write.
func checkCustomerCodes(ctx context.Context, database *store.Store, manifest Manifest) error {
	clients, err := database.ListClients(ctx)
	if err != nil {
		return fmt.Errorf("local seed: read the local client directory: %w", err)
	}
	names := make(map[int16]string, len(clients))
	for _, client := range clients {
		names[client.Code] = client.Name
	}
	specs := append(append([]QuotationSpec{}, manifest.Quotations...), manifest.SyntheticSources...)
	for _, spec := range specs {
		if existing, taken := names[spec.CustomerCode]; taken && existing != spec.CustomerName {
			return fmt.Errorf(
				"local seed: client code %d already belongs to %q, not the fictional %q; refusing to rename an existing client",
				spec.CustomerCode, existing, spec.CustomerName)
		}
	}
	return nil
}

type marker struct {
	Key         string
	Kind        string
	State       string
	UUID        string
	Fingerprint string
	ID          int64
}

// readMarkers reports whether any marker exists and returns all of them. Any
// marker at all makes the run "existing": there is deliberately no partial
// rebuild path, because a rebuild is exactly how a seed duplicates fixtures.
func readMarkers(ctx context.Context, conn *pgx.Conn, schema string) (bool, []marker, error) {
	rows, err := conn.Query(ctx, fmt.Sprintf(`
		SELECT fixture_key, kind, entity_id, entity_uuid, fingerprint, state
		FROM %s ORDER BY fixture_key`, markerIdentifier(schema)))
	if err != nil {
		return false, nil, fmt.Errorf("local seed: read the local marker table: %w", err)
	}
	defer rows.Close()
	markers := make([]marker, 0, 16)
	for rows.Next() {
		var item marker
		if err := rows.Scan(&item.Key, &item.Kind, &item.ID, &item.UUID, &item.Fingerprint, &item.State); err != nil {
			return false, nil, fmt.Errorf("local seed: scan the local marker table: %w", err)
		}
		markers = append(markers, item)
	}
	if err := rows.Err(); err != nil {
		return false, nil, fmt.Errorf("local seed: iterate the local marker table: %w", err)
	}
	return len(markers) > 0, markers, nil
}

func recordMarker(ctx context.Context, conn *pgx.Conn, schema, key, kind string, id int64, uuid, fingerprint, state string) error {
	tag, err := conn.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (fixture_key, kind, entity_id, entity_uuid, fingerprint, state)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (fixture_key) DO UPDATE SET
			entity_id = EXCLUDED.entity_id,
			entity_uuid = EXCLUDED.entity_uuid,
			fingerprint = EXCLUDED.fingerprint,
			state = EXCLUDED.state,
			updated_at = NOW()`, markerIdentifier(schema)),
		key, kind, id, uuid, fingerprint, state)
	if err != nil {
		return fmt.Errorf("local seed: record the fixture marker %q: %w", key, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("local seed: fixture marker %q was not recorded", key)
	}
	return nil
}

func pickActor(ctx context.Context, database *store.Store, requestedEmail string) (store.AuthUser, error) {
	users, err := database.ListUsers(ctx)
	if err != nil {
		return store.AuthUser{}, fmt.Errorf("local seed: read the local users: %w", err)
	}
	requested := strings.TrimSpace(requestedEmail)
	if requested != "" {
		for _, user := range users {
			if strings.EqualFold(user.Email, requested) {
				if user.Role != "admin" {
					return store.AuthUser{}, fmt.Errorf("local seed: %q is not an administrator; the seed deletes its two synthetic sources through the admin deletion lifecycle", requested)
				}
				if !user.IsActive {
					return store.AuthUser{}, fmt.Errorf("local seed: %q is inactive", requested)
				}
				return user, nil
			}
		}
		return store.AuthUser{}, fmt.Errorf("local seed: no local user has the email %q", requested)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	for _, user := range users {
		if user.Role == "admin" && user.IsActive {
			return user, nil
		}
	}
	return store.AuthUser{}, fmt.Errorf("local seed: no active administrator exists; start the local API once (scripts/dev-api.sh) so it bootstraps the first admin, or pass --actor-email for an existing local admin")
}

// inspect reads every marker and reports its state. It is also the fresh-run
// report: the callers create markers as they go, so the same code describes
// both a completed run and an interrupted one.
func inspect(
	ctx context.Context,
	database *store.Store,
	target Target,
	actor store.AuthUser,
	markers []marker,
	mode string,
) (Report, error) {
	manifest := MockManifest()
	byKey := make(map[string]marker, len(markers))
	for _, item := range markers {
		byKey[item.Key] = item
	}

	report := Report{
		Target:                 target.String(),
		Mode:                   mode,
		ManifestVersion:        ManifestVersion,
		ActorEmail:             actor.Email,
		ExpectedQuotations:     len(manifest.Quotations),
		ExpectedPurchaseOrders: len(manifest.PurchaseOrders),
		Notes: []string{
			"Every fixture is fictional and marked [MOCK]; customer names, subjects, PO numbers and PDFs say so.",
			"Seeded quotation revisions are intentionally unsigned. They are manual-testing fixtures, not issued documents, and the public verification portal is not part of this set.",
			"Customer PO documents and quotation PDFs are synthetic text/scanned PDFs; no extraction accuracy is claimed.",
			"Re-running preserves existing fixtures, never overwrites them and never deletes records.",
		},
	}
	for _, item := range markers {
		if item.Key == markerRunKey && item.Fingerprint != "" && item.Fingerprint != markerVersion {
			report.Notes = append(report.Notes, fmt.Sprintf(
				"The markers were written by manifest v%s; this command is v%s. Existing fixtures were left untouched.",
				item.Fingerprint, markerVersion))
		}
		if item.Key == markerRunKey && item.State == "started" {
			report.Notes = append(report.Notes,
				"The previous seed run did not record completion; fixtures may be partial. Nothing was rebuilt.")
		}
	}
	occupied := make(map[int64]bool)
	quotationIDs := make(map[int64]bool)

	for _, quotation := range manifest.Quotations {
		item, ok := byKey[quotation.Key]
		entry := FixtureReport{Key: quotation.Key, Kind: "quotation", Label: quotation.Subject, State: FixtureMissing}
		if !ok {
			entry.Detail = "no marker; this run did not create it"
			report.Fixtures = append(report.Fixtures, entry)
			continue
		}
		entry.EntityID = item.ID
		record, err := database.GetQuotation(ctx, item.ID, nil)
		if err != nil {
			if isNotFound(err) {
				entry.Detail = "the marked Quotation no longer exists"
			} else {
				return report, fmt.Errorf("local seed: inspect quotation %q: %w", quotation.Key, err)
			}
			report.Fixtures = append(report.Fixtures, entry)
			continue
		}
		quotationIDs[record.ID] = true
		revisions, err := database.ListRevisionNumbers(ctx, record.ID)
		if err != nil {
			return report, fmt.Errorf("local seed: inspect quotation %q revisions: %w", quotation.Key, err)
		}
		fingerprint := quotationFingerprint(record.QuoteNumber, revisions)
		switch {
		case item.UUID != "" && item.UUID != record.UUID:
			entry.State = FixtureChanged
			entry.Detail = "the row identity changed since seeding"
		case item.Fingerprint != "" && item.Fingerprint != fingerprint:
			entry.State = FixtureChanged
			entry.Detail = "edited since seeding; its values were left alone"
		default:
			entry.State = FixturePresent
			entry.Detail = record.QuoteNumber
		}
		report.Fixtures = append(report.Fixtures, entry)
	}

	for _, source := range manifest.SyntheticSources {
		item, ok := byKey[source.Key]
		entry := FixtureReport{Key: source.Key, Kind: "synthetic_source", Label: source.Subject, State: FixtureMissing}
		if !ok {
			entry.Detail = "no marker; the synthetic source was not created"
		} else if item.State == FixtureDeleted {
			entry.State = FixtureDeleted
			entry.Detail = "created and deleted through the admin deletion lifecycle, as intended"
			report.SyntheticSourcesDeleted++
		} else {
			entry.State = FixturePresent
			entry.EntityID = item.ID
			entry.Detail = "still present; the run did not finish its deletion step"
			quotationIDs[item.ID] = true
		}
		report.Fixtures = append(report.Fixtures, entry)
	}

	for _, orderSpec := range manifest.PurchaseOrders {
		item, ok := byKey[orderSpec.Key]
		entry := FixtureReport{Key: orderSpec.Key, Kind: "purchase_order", Label: orderSpec.Label, State: FixtureMissing}
		if !ok {
			entry.Detail = "no marker; this run did not create it"
			report.Fixtures = append(report.Fixtures, entry)
			continue
		}
		entry.EntityID = item.ID
		order, err := database.GetPurchaseOrder(ctx, item.ID)
		if err != nil {
			if isNotFound(err) {
				entry.Detail = "the marked Purchase Order no longer exists"
			} else {
				return report, fmt.Errorf("local seed: inspect purchase order %q: %w", orderSpec.Key, err)
			}
			report.Fixtures = append(report.Fixtures, entry)
			continue
		}
		if order.SourceState == store.PurchaseOrderSourceStateLive {
			occupied[order.QuotationID] = true
		}
		fingerprint := purchaseOrderFingerprint(order)
		switch {
		case item.UUID != "" && item.UUID != order.UUID:
			entry.State = FixtureChanged
			entry.Detail = "the row identity changed since seeding"
		case item.Fingerprint != "" && item.Fingerprint != fingerprint:
			entry.State = FixtureChanged
			entry.Detail = fmt.Sprintf("edited since seeding (version %d); its values were left alone", order.Version)
		default:
			entry.State = FixturePresent
			entry.Detail = fmt.Sprintf("%s, source %s, version %d", order.State, order.SourceState, order.Version)
		}
		report.Fixtures = append(report.Fixtures, entry)
	}

	for _, entry := range report.Fixtures {
		if entry.Kind == "quotation" && (entry.State == FixturePresent || entry.State == FixtureChanged) {
			report.LiveQuotations++
		}
		if entry.Kind == "purchase_order" && (entry.State == FixturePresent || entry.State == FixtureChanged) {
			report.PurchaseOrders++
		}
	}
	for id := range occupied {
		if quotationIDs[id] {
			report.Occupied++
		}
	}
	report.Available = report.LiveQuotations - report.Occupied
	if report.Available < 0 {
		report.Available = 0
	}
	return report, nil
}

func quotationFingerprint(quoteNumber string, revisions []int16) string {
	return quoteNumber + "|" + fmt.Sprintf("%d", len(revisions))
}

func purchaseOrderFingerprint(order store.PurchaseOrder) string {
	return fmt.Sprintf("%s|%s|%d", order.State, order.SourceState, order.Version)
}

func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) ||
		errors.Is(err, store.ErrPurchaseOrderNotFound) ||
		errors.Is(err, store.ErrPurchaseOrderDocumentNotFound)
}

// createFixtures writes the manifest in dependency order and records a marker
// as each fixture lands. The run marker already exists, so a failure here is
// reportable and re-runnable without duplicate records.
func createFixtures(
	ctx context.Context,
	database *store.Store,
	conn *pgx.Conn,
	target Target,
	actor store.AuthUser,
	manifest Manifest,
) (Report, error) {
	live := make(map[string]createdQuotation, len(manifest.Quotations))
	for _, spec := range manifest.Quotations {
		created, err := createQuotation(ctx, database, spec)
		if err != nil {
			return Report{}, fmt.Errorf("local seed: create quotation %q: %w", spec.Key, err)
		}
		live[spec.Key] = created
		if err := recordQuotationMarker(ctx, conn, target.Schema, spec, created); err != nil {
			return Report{}, err
		}
	}

	synthetic := make(map[string]createdQuotation, len(manifest.SyntheticSources))
	for _, spec := range manifest.SyntheticSources {
		created, err := createQuotation(ctx, database, spec)
		if err != nil {
			return Report{}, fmt.Errorf("local seed: create synthetic source %q: %w", spec.Key, err)
		}
		synthetic[spec.Key] = created
		if err := recordMarker(ctx, conn, target.Schema, spec.Key, "synthetic_source",
			created.Record.ID, created.Record.UUID, "", "created"); err != nil {
			return Report{}, err
		}
	}

	orders := make(map[string]store.PurchaseOrder, len(manifest.PurchaseOrders))
	for _, spec := range manifest.PurchaseOrders {
		var source *createdQuotation
		if spec.SourceKey != "" {
			if created, ok := live[spec.SourceKey]; ok {
				copied := created
				source = &copied
			} else if created, ok := synthetic[spec.SourceKey]; ok {
				copied := created
				source = &copied
			} else {
				return Report{}, fmt.Errorf("local seed: purchase order %q names an unknown source %q", spec.Key, spec.SourceKey)
			}
		}
		order, err := createPurchaseOrder(ctx, database, actor, spec, source)
		if err != nil {
			return Report{}, fmt.Errorf("local seed: create purchase order %q: %w", spec.Key, err)
		}
		orders[spec.Key] = order
		if err := recordMarker(ctx, conn, target.Schema, spec.Key, "purchase_order",
			order.ID, order.UUID, purchaseOrderFingerprint(order), FixturePresent); err != nil {
			return Report{}, err
		}
	}

	// The former-source cases exist only after their quotations are deleted
	// through the ordinary admin deletion lifecycle. Do this last, so every PO
	// that needs the source has already been created and reviewed.
	for _, spec := range manifest.SyntheticSources {
		created := synthetic[spec.Key]
		if err := deleteSyntheticSource(ctx, database, actor, created); err != nil {
			return Report{}, fmt.Errorf("local seed: delete synthetic source %q: %w", spec.Key, err)
		}
		if err := recordMarker(ctx, conn, target.Schema, spec.Key, "synthetic_source",
			created.Record.ID, created.Record.UUID, "", FixtureDeleted); err != nil {
			return Report{}, err
		}
	}

	// Releasing a deleted source bumps the surviving PO's version through the
	// ordinary update trigger. Record the final fingerprint now, or every rerun
	// would report the seed's own deletion as a developer edit.
	for _, spec := range manifest.PurchaseOrders {
		if spec.SourceState != sourceFormer {
			continue
		}
		order, err := database.GetPurchaseOrder(ctx, orders[spec.Key].ID)
		if err != nil {
			return Report{}, fmt.Errorf("local seed: re-read former-source purchase order %q: %w", spec.Key, err)
		}
		if err := recordMarker(ctx, conn, target.Schema, spec.Key, "purchase_order",
			order.ID, order.UUID, purchaseOrderFingerprint(order), FixturePresent); err != nil {
			return Report{}, err
		}
	}

	if _, err := conn.Exec(ctx, fmt.Sprintf(`
		UPDATE %s SET state = 'complete', updated_at = NOW()
		WHERE fixture_key = $1`, markerIdentifier(target.Schema)), markerRunKey); err != nil {
		return Report{}, fmt.Errorf("local seed: mark the run complete: %w", err)
	}

	_, markers, err := readMarkers(ctx, conn, target.Schema)
	if err != nil {
		return Report{}, err
	}
	report, err := inspect(ctx, database, target, actor, markers, ModeCreated)
	if err != nil {
		return report, err
	}
	report.Notes = append(report.Notes,
		"The two synthetic source Quotations were created, linked to their purchase orders and deleted through the admin deletion lifecycle; they are not part of the six live fixtures.")
	return report, nil
}

func recordQuotationMarker(ctx context.Context, conn *pgx.Conn, schema string, spec QuotationSpec, created createdQuotation) error {
	return recordMarker(ctx, conn, schema, spec.Key, "quotation",
		created.Record.ID, created.Record.UUID,
		quotationFingerprint(created.QuoteNumber, created.RevisionNumbers()), FixturePresent)
}
