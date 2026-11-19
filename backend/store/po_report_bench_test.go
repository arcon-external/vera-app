package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/poreport"
)

// Issue #629's measured target: the report at 10,000 saved purchase orders.
// This benchmark runs only against the documented disposable DSN and recreates
// a throwaway schema first; it never touches a shared or production database.
// The numbers it prints are measurements, not promises: the approved 120-period
// cap is a product decision and is not reduced to make a target easier.
//
//	go test ./backend/store -run '^$' -bench BenchmarkPurchaseOrderReport10000 \
//	  -benchtime=10x -count=1 \
//	  -timeout=20m
func BenchmarkPurchaseOrderReport10000(b *testing.B) {
	benchmarkPurchaseOrderReport(b, 10000)
}

// BenchmarkPurchaseOrderReportScale runs the same measurement at the three
// sizes the report has to hold: a small office, a busy month, and the 10k
// archive the ticket names. Each sub-benchmark seeds its own throwaway schema
// and reports per-iteration p50/p95 alongside the mean, so a regression in the
// tail is visible rather than hidden by an average.
//
//	go test ./backend/store -run '^$' -bench BenchmarkPurchaseOrderReportScale \
//	  -benchtime=20x -count=5 -benchmem -timeout=30m
func BenchmarkPurchaseOrderReportScale(b *testing.B) {
	for _, total := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("%d", total), func(b *testing.B) {
			benchmarkPurchaseOrderReport(b, total)
		})
	}
}

func benchmarkPurchaseOrderReport(b *testing.B, total int) {
	b.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		b.Skip("set VERA_POSTGRES_TEST_DSN to run the report benchmark")
	}
	ctx := context.Background()
	const schema = "vera_po_report_bench_629"

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		b.Fatalf("connect for benchmark schema reset: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`); err != nil {
		pool.Close()
		b.Fatalf("reset benchmark schema: %v", err)
	}
	pool.Close()

	database, err := Open(ctx, Config{DSN: dsn, Schema: schema})
	if err != nil {
		b.Fatalf("open benchmark store: %v", err)
	}
	b.Cleanup(database.Close)
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		b.Fatalf("default organization: %v", err)
	}
	ctx = ContextWithOrg(ctx, org.ID)
	user, err := database.CreateUser(ctx, fmt.Sprintf("report-bench-%d@example.com", time.Now().UnixNano()),
		"Report Benchmark", "hash", "staff")
	if err != nil {
		b.Fatalf("benchmark user: %v", err)
	}
	seedReportBenchmark(b, database, org.ID, user.ID, total)

	query := poreport.Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}
	now := time.Date(2026, 1, 15, 5, 0, 0, 0, time.UTC)
	historyTo := reportHistoryCutoff(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), companyToday(now))

	b.ResetTimer()
	var snapshot PurchaseOrderReportSnapshot
	durations := make([]time.Duration, 0, b.N)
	for iteration := 0; iteration < b.N; iteration++ {
		started := time.Now()
		snapshot, err = database.ReportPurchaseOrders(ctx, query, now)
		durations = append(durations, time.Since(started))
		if err != nil {
			b.Fatalf("report at %d POs: %v", total, err)
		}
	}
	b.StopTimer()
	slices.Sort(durations)
	b.ReportMetric(float64(reportQuantile(durations, 0.50).Microseconds())/1000, "p50_ms")
	b.ReportMetric(float64(reportQuantile(durations, 0.95).Microseconds())/1000, "p95_ms")

	// The payload is measured on the same shape the screen receives, including
	// the two projections. It is reported, not enforced: the approved range is
	// not shortened to fit a byte target.
	report := poreport.Report{
		GeneratedAt:    now.UTC().Format(time.RFC3339),
		AsOf:           snapshot.AsOf.Format(time.DateOnly),
		Filter:         query,
		Company:        poreport.Company{Name: "PT Arcon Penta Persada", Address: "Batam"},
		Population:     poreport.Population{AvailableCurrencies: snapshot.AvailableCurrencies, HasUnassigned: snapshot.HasUnassigned, SourceMissingPoCount: snapshot.SourceMissingPoCount},
		Totals:         snapshot.Totals,
		Periods:        snapshot.Periods,
		Categories:     snapshot.Categories,
		CategorySeries: snapshot.CategorySeries,
		Projection:     poreport.Project(snapshot.History, query, snapshot.AsOf, snapshot.Totals.PoCount),
		Notes:          []poreport.Note{},
	}
	payload, err := json.Marshal(report)
	if err != nil {
		b.Fatalf("marshal benchmark report: %v", err)
	}
	b.ReportMetric(float64(len(payload)), "payload_bytes")
	b.Logf("%d-PO report: %d periods, %d categories, %d history months, %d PO count",
		total, len(snapshot.Periods), len(snapshot.Categories), len(snapshot.History), snapshot.Totals.PoCount)

	// The approved 120-period cap is measured rather than assumed: the same
	// population over a ten-year month axis, including 108 zero buckets.
	wideQuery := poreport.Query{
		From: "2016-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}
	wide, err := database.ReportPurchaseOrders(ctx, wideQuery, now)
	if err != nil {
		b.Fatalf("120-period report at %d POs: %v", total, err)
	}
	wideReport := report
	wideReport.Filter = wideQuery
	wideReport.AsOf = wide.AsOf.Format(time.DateOnly)
	wideReport.Population = poreport.Population{
		AvailableCurrencies:  wide.AvailableCurrencies,
		HasUnassigned:        wide.HasUnassigned,
		SourceMissingPoCount: wide.SourceMissingPoCount,
	}
	wideReport.Totals = wide.Totals
	wideReport.Periods = wide.Periods
	wideReport.Categories = wide.Categories
	wideReport.CategorySeries = wide.CategorySeries
	wideReport.Projection = poreport.Project(wide.History, wideQuery, wide.AsOf, wide.Totals.PoCount)
	widePayload, err := json.Marshal(wideReport)
	if err != nil {
		b.Fatalf("marshal 120-period report: %v", err)
	}
	b.ReportMetric(float64(len(widePayload)), "payload_120_bytes")
	b.Logf("120-period cap payload: %d bytes across %d periods", len(widePayload), len(wide.Periods))

	// One EXPLAIN ANALYZE of the exact production statement, for the index and
	// plan evidence the ticket requires. It runs after the timed loop; the
	// smaller sizes get the same measurement because their plan choice is the
	// question (seq scan versus the report index), not just their latency.
	explainBenchmarkReport(b, database, org.ID, query, historyTo)
}

// reportQuantile is the nearest-rank percentile of an already sorted sample:
// the smallest duration at or above the requested rank. With b.N samples it
// makes no interpolation assumptions, so p95 of 20 runs is the 19th value.
func reportQuantile(sorted []time.Duration, quantile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(float64(len(sorted)) * quantile)
	if float64(rank) < float64(len(sorted))*quantile {
		rank++
	}
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func seedReportBenchmark(b *testing.B, database *Store, orgID string, userID int64, total int) {
	b.Helper()
	ctx := context.Background()
	pool, err := database.poolOrError()
	if err != nil {
		b.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	var customerID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM %s.customers WHERE customer_code = 2`, schema)).Scan(&customerID); err != nil {
		b.Fatalf("seed benchmark customer: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %[1]s.quotations (quote_number, customer_id)
		SELECT '99' || lpad(i::text, 8, '0'), $1
		FROM generate_series(1, %[2]d) AS i`, schema, total), customerID); err != nil {
		b.Fatalf("seed benchmark quotations: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %[1]s.quotation_revisions (
			quotation_id, revision_number, recipient_name, recipient_address, subject,
			incoterm, validity_days, payment_term_days, lead_time_days,
			discount_percent, tax_zone, tax_category, tax_rate_bps, tax_type,
			service_category, short_id, currency, issuer_name, recipient_company, note
		)
		SELECT q.id, 0, 'Benchmark Recipient', 'Benchmark Address', 'Benchmark subject',
		       'CIF', 30, 30, 10, 0, 'FTZ', '', 0, 'Goods', '',
		       'bench-' || q.id, 'IDR', '', '', ''
		FROM %[1]s.quotations q`, schema)); err != nil {
		b.Fatalf("seed benchmark revisions: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %[1]s.quotation_revision_items (
			revision_id, item_no, description, qty, uom, unit_price,
			margin_enabled, margin_percent, margin_amount, original_total,
			marginalized_unit_price, marginalized_total, list_unit_price
		)
		SELECT r.id, 1, 'Benchmark line', 1, 'Pc', 100,
		       FALSE, 0, 0, 100, 200, 200, 200
		FROM %[1]s.quotation_revisions r`, schema)); err != nil {
		b.Fatalf("seed benchmark revision items: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %[1]s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id, customer_name,
			received_date, currency, tax_zone, tax_type, service_category,
			subtotal_minor, discount_minor, discount_percent, ppn_bps, ppn_minor,
			adjustment_minor, adjustment_reason, grand_total_minor,
			printed_subtotal_minor, printed_discount_minor, printed_ppn_minor,
			printed_grand_total_minor, po_number, po_number_key, saved_at, saved_by,
			created_by, updated_by, business_category
		)
		SELECT $1, q.id, 0, 'saved', $2, 'PT. Benchmark Buyer',
		       DATE '2025-01-01' + ((q.id - 1) %% 365)::int,
		       'IDR', 'FTZ', 'Goods', '',
		       200, 0, 0, 0, 0, 0, '', 200, 200, 0, 0, 200,
		       'BENCH-PO-' || q.id, 'bench-po-' || q.id, NOW(), $3, $3, $3,
		       CASE (q.id - 1) %% 4
		           WHEN 0 THEN 'FM'
		           WHEN 1 THEN 'FAB'
		           WHEN 2 THEN 'SUPPLY_GEN'
		           ELSE 'OTHERS'
		       END
		FROM %[1]s.quotations q
		WHERE q.quote_number LIKE '99%%'`, schema), orgID, customerID, userID); err != nil {
		b.Fatalf("seed benchmark purchase orders: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %[1]s.purchase_order_items (
			purchase_order_id, line_number, description, quotation_item_no,
			quantity_scaled, uom, unit_price_minor, gross_minor, discount_minor, net_minor
		)
		SELECT po.id, 1, 'Benchmark line', 1, 10000, 'Pc', 200, 200, 0, 200
		FROM %[1]s.purchase_orders po`, schema)); err != nil {
		b.Fatalf("seed benchmark purchase order items: %v", err)
	}

	var orders int
	if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s.purchase_orders`, schema)).Scan(&orders); err != nil {
		b.Fatalf("count benchmark POs: %v", err)
	}
	if orders != total {
		b.Fatalf("benchmark seeded %d purchase orders; want %d", orders, total)
	}
	// Production tables carry autovacuum statistics; a freshly inserted fixture
	// does not. Analyze so the measured plan is the plan a real database gets.
	for _, table := range []string{
		"purchase_orders", "purchase_order_items", "quotation_revisions",
		"quotation_revision_items", "quotation_revision_sub_items",
	} {
		if _, err := pool.Exec(ctx, fmt.Sprintf("ANALYZE %s.%s", schema, table)); err != nil {
			b.Fatalf("analyze benchmark table %s: %v", table, err)
		}
	}
}

func explainBenchmarkReport(b *testing.B, database *Store, orgID string, query poreport.Query, historyTo time.Time) {
	b.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		b.Fatalf("pool: %v", err)
	}
	rows, err := pool.Query(context.Background(),
		"EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) "+purchaseOrderReportSQL(database.configSchema()),
		orgID, query.Currency, query.From, query.To, query.Category,
		historyTo.Format(time.DateOnly), query.Bucket)
	if err != nil {
		b.Fatalf("explain report: %v", err)
	}
	defer rows.Close()
	b.Log("EXPLAIN (ANALYZE, BUFFERS) for the seeded report population:")
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			b.Fatalf("scan explain line: %v", err)
		}
		b.Log(line)
	}
	if err := rows.Err(); err != nil {
		b.Fatalf("read explain plan: %v", err)
	}
}
