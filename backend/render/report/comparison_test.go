package report

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"vera-app/backend/poreport"
)

// Issue #642: the native individual comparison grid and the one-calendar-year
// Aggregate table. The tests read the renderer's own card geometry and the
// PDF text: text alone cannot prove a two-column pair, a full-width odd final
// card or that a card stayed on one page.

func viewRenderer(t *testing.T, view ViewEnvelope) *reportRenderer {
	t.Helper()
	if err := ValidateView(view); err != nil {
		t.Fatalf("validate view: %v", err)
	}
	resolved, err := resolveView(view)
	if err != nil {
		t.Fatalf("resolve view: %v", err)
	}
	loaded := testFonts()
	renderer := &reportRenderer{
		snapshot: resolved.Overview,
		view:     &view,
		resolved: &resolved,
		faces:    loaded,
		pal:      defaultPalette(),
		canvas:   newCanvas(loaded),
	}
	renderer.draw()
	return renderer
}

func individualMonthsView(t *testing.T, months []string) ViewEnvelope {
	t.Helper()
	overview := yearMonthFixture(t)
	return ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0}, Months: months, Layout: ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
}

func cardRows(cards []cardBox) [][]cardBox {
	rows := make([][]cardBox, 0, len(cards))
	for _, card := range cards {
		if len(rows) == 0 || rows[len(rows)-1][0].Row != card.Row {
			rows = append(rows, []cardBox{card})
			continue
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], card)
	}
	return rows
}

// cardSurfaceDrawn proves the bounding box the layout recorded is the box the
// canvas actually drew, so a geometry assertion is not merely a restatement of
// the measuring code.
func cardSurfaceDrawn(r *reportRenderer, box cardBox) bool {
	if box.Page < 0 || box.Page >= len(r.canvas.pages) {
		return false
	}
	for _, prim := range r.canvas.pages[box.Page].prims {
		rect, ok := prim.(rectPrim)
		if !ok || rect.stroke == nil {
			continue
		}
		if math.Abs(rect.x-box.X) < 0.01 && math.Abs(rect.y-box.Y) < 0.01 &&
			math.Abs(rect.w-box.Width) < 0.01 && math.Abs(rect.h-box.Height) < 0.01 {
			return true
		}
	}
	return false
}

func cardTexts(r *reportRenderer, box cardBox) []string {
	if box.Page < 0 || box.Page >= len(r.canvas.pages) {
		return nil
	}
	var texts []string
	for _, prim := range r.canvas.pages[box.Page].prims {
		value, ok := prim.(textPrim)
		if !ok {
			continue
		}
		if value.baseline >= box.Y-0.5 && value.baseline <= box.Y+box.Height+0.5 &&
			value.x >= box.X-0.5 && value.x <= box.X+box.Width+0.5 {
			texts = append(texts, value.text)
		}
	}
	return texts
}

func TestIndividualComparisonCardsUseTwoColumnsAndAFullWidthOddFinal(t *testing.T) {
	half := (contentWidth - cardGap) / 2
	cases := []struct {
		name       string
		months     []string
		wantRowLen []int
	}{
		{name: "three months", months: monthsOfYear(2026)[:3], wantRowLen: []int{2, 1}},
		{name: "four months", months: monthsOfYear(2026)[:4], wantRowLen: []int{2, 2}},
		{name: "five months", months: monthsOfYear(2026)[:5], wantRowLen: []int{2, 2, 1}},
		{name: "twelve months", months: monthsOfYear(2026), wantRowLen: []int{2, 2, 2, 2, 2, 2}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			renderer := viewRenderer(t, individualMonthsView(t, test.months))
			if len(renderer.cards) != len(test.months) {
				t.Fatalf("cards = %d; want %d", len(renderer.cards), len(test.months))
			}
			rows := cardRows(renderer.cards)
			if len(rows) != len(test.wantRowLen) {
				t.Fatalf("rows = %d; want %d", len(rows), len(test.wantRowLen))
			}
			for rowIndex, row := range rows {
				if len(row) != test.wantRowLen[rowIndex] {
					t.Fatalf("row %d has %d cards; want %d", rowIndex, len(row), test.wantRowLen[rowIndex])
				}
				if len(row) == 2 {
					if row[0].FullWidth || row[1].FullWidth {
						t.Errorf("row %d is marked full-width in a pair", rowIndex)
					}
					if math.Abs(row[0].Width-half) > 0.01 || math.Abs(row[1].Width-half) > 0.01 {
						t.Errorf("row %d widths = %.2f/%.2f; want %.2f", rowIndex, row[0].Width, row[1].Width, half)
					}
					if math.Abs(row[0].Y-row[1].Y) > 0.01 || row[0].Page != row[1].Page {
						t.Errorf("row %d pair is not on one baseline/page", rowIndex)
					}
					wantX := row[0].X + row[0].Width + cardGap
					if math.Abs(row[1].X-wantX) > 0.01 {
						t.Errorf("row %d second card x = %.2f; want %.2f", rowIndex, row[1].X, wantX)
					}
				} else {
					if !row[0].FullWidth {
						t.Errorf("row %d odd final card is not marked full-width", rowIndex)
					}
					if math.Abs(row[0].Width-contentWidth) > 0.01 || math.Abs(row[0].X-marginX) > 0.01 {
						t.Errorf("row %d full-width card = x %.2f w %.2f; want x %.2f w %.2f",
							rowIndex, row[0].X, row[0].Width, marginX, contentWidth)
					}
				}
				for _, card := range row {
					if !cardSurfaceDrawn(renderer, card) {
						t.Errorf("%s card surface was not drawn at its recorded box", card.Month)
					}
				}
			}
		})
	}
}

func TestIndividualComparisonCardsStayOnOnePageAndRepeatContext(t *testing.T) {
	renderer := viewRenderer(t, individualMonthsView(t, monthsOfYear(2026)))
	if len(renderer.canvas.pages) < 2 {
		t.Fatalf("twelve cards fit on %d page(s); want several pages to exercise continuation", len(renderer.canvas.pages))
	}
	pagesWithCards := make(map[int]struct{})
	for _, box := range renderer.cards {
		if box.Y < marginTop-0.01 || box.Y+box.Height > contentBot+0.01 {
			t.Errorf("%s card y=%.2f h=%.2f crosses the page content box", box.Month, box.Y, box.Height)
		}
		if !cardSurfaceDrawn(renderer, box) {
			t.Errorf("%s card surface is not on page %d", box.Month, box.Page+1)
		}
		texts := cardTexts(renderer, box)
		if !containsString(texts, monthHeading(box.Month)) {
			t.Errorf("%s card does not hold its own heading on one page", box.Month)
		}
		if !containsString(texts, "PO margin %") {
			t.Errorf("%s card does not hold its final metric row on the same page", box.Month)
		}
		pagesWithCards[box.Page] = struct{}{}
	}
	cardPages := make([]int, 0, len(pagesWithCards))
	for pageIndex := range pagesWithCards {
		cardPages = append(cardPages, pageIndex)
	}
	sort.Ints(cardPages)
	for index, pageIndex := range cardPages {
		text := pageText([]*page{renderer.canvas.pages[pageIndex]})
		if index == 0 {
			requireText(t, text, "By period — 12 selected months")
			continue
		}
		requireText(t, text, "By period — 12 selected months (continued)", "As of 2026-11-19")
	}
	// Every row keeps its pair together; row order is chronological.
	for _, row := range cardRows(renderer.cards) {
		if len(row) == 2 && row[0].Page != row[1].Page {
			t.Errorf("pair %s/%s was split across pages", row[0].Month, row[1].Month)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestIndividualComparisonCardsPrintExactMaximumMoneyWithoutShrinking(t *testing.T) {
	overview := currentYearReport()
	source := annualViewReport(2025, "2026-11-19", nil)
	max := poreport.MaxExactMinor
	values := make([]int64, 3)
	for index := 0; index < 3; index++ {
		values[index] = max - int64(index)
		source.Periods[index].Metrics = poreport.Metrics{
			PoCount: 1, AmountMinor: values[index], CostedMinor: 0,
			UncostedMinor: values[index], CapitalMinor: max, MarginMinor: -max,
		}
	}
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview, source},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{1}, Months: []string{"2025-01", "2025-02", "2025-03"},
			Layout: ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	renderer := viewRenderer(t, view)
	if len(renderer.cards) != 3 {
		t.Fatalf("cards = %d; want 3", len(renderer.cards))
	}
	wanted := []string{}
	for _, value := range values {
		wanted = append(wanted, formatMoney("IDR", value))
	}
	wanted = append(wanted, formatSignedMoney("IDR", -max))
	for index, card := range renderer.cards {
		texts := cardTexts(renderer, card)
		want := formatMoney("IDR", values[index])
		if !containsString(texts, want) {
			t.Errorf("%s card does not print the exact amount %q", card.Month, want)
		}
		for _, prim := range renderer.canvas.pages[card.Page].prims {
			value, ok := prim.(textPrim)
			if !ok || !containsString(wanted, value.text) {
				continue
			}
			if value.size != tableBodySize {
				t.Errorf("%s card sets exact money %q at %.2fpt; want %.2fpt", card.Month, value.text, value.size, tableBodySize)
			}
			if strings.Contains(value.text, "…") {
				t.Errorf("%s card truncates money %q", card.Month, value.text)
			}
		}
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	for _, want := range wanted {
		requireText(t, text, want)
	}
}

func TestAggregateTablePrintsTheServerRecordedTotalNotARowSum(t *testing.T) {
	source := annualViewReport(2025, "2025-12-20", nil)
	var rowSum int64
	for _, period := range source.Periods {
		rowSum += period.Metrics.AmountMinor
	}
	source.Totals = poreport.Metrics{
		PoCount: 37, AmountMinor: rowSum + 777_777,
		CostedMinor: rowSum - 1_000, UncostedMinor: 778_777,
		CapitalMinor: rowSum / 2, MarginMinor: rowSum - 1_000 - rowSum/2,
	}
	view := annualAggregateView(source)
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView(aggregate): %v", err)
	}
	section := viewPeriodSectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	if section == "" {
		t.Fatal("the aggregate PDF has no By period section")
	}
	requireText(t, section,
		"By period — all months in 2025",
		"Recorded total",
		formatMoney("IDR", source.Totals.AmountMinor),
		"not a sum of the printed rows",
		"The current month row is the annual snapshot's recorded row, not an actual-to-date readout.",
		"All categories (all)",
		"As of 2025-12-20",
		"no forecast or estimate is included",
	)
	requireNoText(t, "aggregate total", section, formatMoney("IDR", rowSum))
	requireNoText(t, "past-year aggregate", section, "Estimated (not actual)")
}

func TestAggregateMarksFuturePlaceholdersAndSeparateForecast(t *testing.T) {
	source := currentYearReport()
	forecast := poreport.ProjectedBucket{
		PeriodStart:     "2026-12-01",
		AmountLowMinor:  123_456_000,
		AmountBaseMinor: 123_456_789,
		AmountHighMinor: 123_457_000,
		CapitalMinor:    100_000_000,
		MarginMinor:     23_456_789,
		MarginBps:       fixtureRatio(23_456_789, 100_000_000),
		SampleSize:      11,
		Source:          poreport.SourceTrailingCompleteMonths,
	}
	source.CurrentProjection.Buckets = []poreport.ProjectedBucket{forecast}
	view := annualAggregateView(source)
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView(aggregate): %v", err)
	}
	section := viewPeriodSectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section,
		"Recorded total",
		formatMoney("IDR", source.Totals.AmountMinor),
		"Dec (no recorded POs) - - - - - -",
		"Estimated (not actual) — Estimated forecast (December 2026)",
		fmt.Sprintf("PO amount %s (%s – %s)",
			formatMoney("IDR", forecast.AmountBaseMinor),
			formatMoney("IDR", forecast.AmountLowMinor),
			formatMoney("IDR", forecast.AmountHighMinor)),
	)
	requireNoText(t, "aggregate future placeholders", section,
		"Dec 2026 - - - - - - 0",
		"Estimated forecast (December 2026): POs 1",
	)
}

func TestAggregateDisclosesRecordedFutureEvidence(t *testing.T) {
	source := currentYearReport()
	source.Periods[11].Metrics = poreport.Metrics{
		PoCount: 1, AmountMinor: 7_777_000, CostedMinor: 7_000_000,
		UncostedMinor: 777_000, CapitalMinor: 6_000_000, MarginMinor: 1_000_000,
	}
	source.Totals.PoCount++
	source.Totals.AmountMinor += 7_777_000
	view := annualAggregateView(source)
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView(aggregate): %v", err)
	}
	section := viewPeriodSectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section,
		"Dec (recorded)",
		formatMoney("IDR", 7_777_000),
		"Recorded total",
		"recorded evidence",
	)
}

func TestAggregateFutureYearPrintsUnavailableWithoutZeroActuals(t *testing.T) {
	source := annualViewReport(2027, "2026-11-19", nil)
	for index := range source.Periods {
		source.Periods[index].Metrics = poreport.Metrics{}
	}
	source.Totals = poreport.Metrics{}
	view := annualAggregateView(source)
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView(aggregate): %v", err)
	}
	section := viewPeriodSectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section,
		"By period — all months in 2027",
		"Recorded total",
		"Dec (no recorded POs) - - - - - -",
		"Forecast unavailable for 2027",
		"Future months beyond the current year have no current-year forecast",
	)
	requireNoText(t, "future-year aggregate", section,
		"Estimated (not actual)", "Estimated forecast",
	)
}

// TestAggregateKeepsTheWideTableAndRepeatsItsHeader starts the table near the
// bottom of a page so it must continue, then checks the wide six-metric header
// returns on every page that carries a month row.
func TestAggregateKeepsTheWideTableAndRepeatsItsHeader(t *testing.T) {
	source := currentYearReport()
	view := annualAggregateView(source)
	if err := ValidateView(view); err != nil {
		t.Fatalf("validate aggregate: %v", err)
	}
	resolved, err := resolveView(view)
	if err != nil {
		t.Fatalf("resolve aggregate: %v", err)
	}
	loaded := testFonts()
	renderer := &reportRenderer{
		snapshot: resolved.Overview, view: &view, resolved: &resolved,
		faces: loaded, pal: defaultPalette(), canvas: newCanvas(loaded),
	}
	renderer.canvas.y = contentBot - 220
	renderer.drawAggregateMonths()
	if len(renderer.canvas.pages) < 2 {
		t.Fatalf("aggregate table fit on %d page(s); want a continuation", len(renderer.canvas.pages))
	}
	headerPages := 0
	for _, currentPage := range renderer.canvas.pages {
		text := pageText([]*page{currentPage})
		if !strings.Contains(pdfText(text), "MONTH") {
			continue
		}
		headerPages++
		requireText(t, text, "MONTH", "POs", "AMOUNT", "COSTED", "CAPITAL", "MARGIN", "MARGIN %")
	}
	if headerPages < 2 {
		t.Fatalf("the aggregate header appears on %d page(s); want a repeated header on the continuation", headerPages)
	}
	// The header spans the full content width, not a card-sized column.
	found := false
	for _, currentPage := range renderer.canvas.pages {
		for _, prim := range currentPage.prims {
			value, ok := prim.(textPrim)
			if ok && value.text == "MONTH" {
				if math.Abs(value.x-(marginX+4)) > 0.01 {
					t.Errorf("MONTH header x = %.2f; want %.2f", value.x, marginX+4)
				}
				found = true
			}
		}
	}
	if !found {
		t.Error("aggregate table did not print its MONTH header")
	}
}

// TestIndividualComparisonCardLayoutsRenderPDF proves the same 3/4/5/12-card
// geometry survives a real PDF read: every selected month heading is printed in
// chronological order and the twelve-card grid continues onto later pages.
func TestIndividualComparisonCardLayoutsRenderPDF(t *testing.T) {
	cases := []struct {
		name   string
		months []string
	}{
		{name: "three", months: monthsOfYear(2026)[:3]},
		{name: "four", months: monthsOfYear(2026)[:4]},
		{name: "five", months: monthsOfYear(2026)[:5]},
		{name: "twelve", months: monthsOfYear(2026)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			view := individualMonthsView(t, test.months)
			pdf, err := RenderView(view, os.DirFS("../../assets"))
			if err != nil {
				t.Fatalf("RenderView: %v", err)
			}
			pages := binaryPDFText(t, pdf)
			text := strings.Join(pages, "\n")
			requireText(t, text, "By period — "+fmt.Sprintf("%d selected months", len(test.months)))
			wants := make([]string, 0, len(test.months))
			for _, month := range test.months {
				wants = append(wants, monthHeading(month))
			}
			requireInOrder(t, viewPeriodSectionText(text), wants...)
			if len(test.months) == 12 && len(pages) < 2 {
				t.Fatalf("twelve cards rendered %d page(s); want pagination", len(pages))
			}
		})
	}
}

func TestAggregateDisclosesFutureDatedEvidenceInTheCurrentMonth(t *testing.T) {
	source := currentYearReport()
	// The annual current-month row carries a record dated after the as-of date;
	// the server's actual-to-date readout in CurrentProjection does not.
	source.Periods[10].Metrics.AmountMinor += 7_777_000
	source.Totals.AmountMinor += 7_777_000
	view := annualAggregateView(source)
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView(aggregate): %v", err)
	}
	section := viewPeriodSectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section,
		"Nov (recorded)",
		"Recorded total",
		"Future-dated records are included as recorded evidence, never as a forecast.",
	)
}

// TestAggregateIgnoresTheCurrentMonthOverrideTotals proves the one-year total
// still comes from the annual snapshot, not from a separately referenced
// current-month/day overview.
func TestAggregateIgnoresTheCurrentMonthOverrideTotals(t *testing.T) {
	annual := currentYearReport()
	day := dayMonthFixture(t)
	if annual.Totals.AmountMinor == day.Totals.AmountMinor {
		t.Fatal("fixture error: the annual and day totals are indistinguishable")
	}
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{annual, day},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0, 1}, Months: monthsOfYear(2026),
			Layout: ViewLayoutAggregate, CurrentMonthSource: intPointer(1),
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(annual), Layout: ViewLayoutAggregate},
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView(aggregate): %v", err)
	}
	section := viewPeriodSectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section, "Recorded total", formatMoney("IDR", annual.Totals.AmountMinor))
	requireNoText(t, "aggregate total", section, formatMoney("IDR", day.Totals.AmountMinor))
}
