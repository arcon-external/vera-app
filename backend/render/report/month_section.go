package report

import (
	"fmt"
	"time"

	"vera-app/backend/poreport"
)

// Issue #641: the selected-month section replaces the legacy bucket-following
// period table in a view export. It prints exactly one month's six metrics from
// the frozen source, then the separate recorded-future and estimated lines.
// Nothing here adds an estimate to an actual, and no figure is recomputed.

func monthHeading(month string) string {
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		return month
	}
	return parsed.Format("January 2006")
}

func (r *reportRenderer) drawSelectedMonthSections() {
	if r.resolved == nil {
		return
	}
	for _, month := range r.resolved.Months {
		r.drawSelectedMonthSection(month)
	}
}

func (r *reportRenderer) drawSelectedMonthSection(month resolvedMonth) {
	var primary *poreport.Metrics
	note := ""
	switch {
	case month.IsCurrent:
		if month.Summary != nil {
			actual := month.Summary.Actual
			primary = &actual
		}
		note = "Actual to date (month-to-date). Actual figures only; future-dated records and estimates are separate lines below and are never added to these figures."
	case month.IsFuture:
		if month.Period != nil && month.Period.PoCount > 0 {
			recorded := month.Period.Metrics
			primary = &recorded
			note = "Recorded evidence in the selected future month. The full-month forecast is a separate alternative below and is never added to it."
		} else {
			note = "No recorded purchase orders in this future month. The full-month forecast is shown separately below as an estimate, never as actual zero."
		}
	default:
		if month.Period != nil {
			recorded := month.Period.Metrics
			primary = &recorded
			note = "Recorded figures for the selected month. A zero row is a loaded month with no purchase orders, not missing data."
		}
	}

	columns := []tableColumn{
		{title: "METRIC", width: 150, align: alignLeft},
		{title: "VALUE", width: contentWidth - 150, align: alignRight},
	}
	values := []string{"-", "-", "-", "-", "-", "-"}
	if primary != nil {
		values = []string{
			formatCount(primary.PoCount),
			r.money(primary.AmountMinor),
			r.money(primary.CostedMinor),
			r.money(primary.CapitalMinor),
			r.signedMoney(primary.MarginMinor),
			formatPct(primary.MarginBps),
		}
	}
	labels := []string{"POs", "PO amount", "Costed amount", "Capital", "PO margin", "PO margin %"}
	rows := make([]tableRow, 0, len(labels))
	for index, label := range labels {
		rows = append(rows, tableRow{cells: []string{label, values[index]}})
	}

	scope := fmt.Sprintf("%s  ·  %s  ·  %s  ·  As of %s  ·  Snapshot %s",
		monthHeading(month.Month),
		categoryFilterLabel(month.Source.Filter.Category),
		month.Source.Filter.Currency,
		month.Source.AsOf,
		generatedLabel(month.Source.GeneratedAt))
	r.drawTable("By period — "+monthHeading(month.Month), scope+"\n"+note, columns, rows, nil)

	if month.IsCurrent && month.Summary != nil {
		if month.Summary.RecordedFuture != nil {
			r.drawEvidenceLine("Future-dated recorded",
				metricsEvidence(r, *month.Summary.RecordedFuture), false)
		}
		r.drawMonthForecast(month)
	} else if month.IsFuture {
		r.drawMonthForecast(month)
	}
}

func metricsEvidence(r *reportRenderer, metrics poreport.Metrics) string {
	return fmt.Sprintf(
		"POs %s  ·  PO amount %s  ·  Costed amount %s  ·  Capital %s  ·  PO margin %s  ·  PO margin %% %s",
		formatCount(metrics.PoCount),
		r.money(metrics.AmountMinor),
		r.money(metrics.CostedMinor),
		r.money(metrics.CapitalMinor),
		r.signedMoney(metrics.MarginMinor),
		formatPct(metrics.MarginBps))
}

func estimateEvidence(r *reportRenderer, remainder poreport.CurrentProjectionRemainder) string {
	return fmt.Sprintf(
		"POs -  ·  PO amount %s (%s – %s)  ·  Costed amount -  ·  Capital %s  ·  PO margin %s  ·  PO margin %% %s  ·  sample %s (%s)",
		r.money(remainder.BaseMinor),
		r.money(remainder.LowMinor),
		r.money(remainder.HighMinor),
		r.money(remainder.CapitalMinor),
		r.signedMoney(remainder.MarginMinor),
		formatPct(remainder.MarginBps),
		formatCount(int64(remainder.SampleSize)),
		projectionSourceLabel(remainder.Source))
}

func (r *reportRenderer) drawMonthForecast(month resolvedMonth) {
	switch {
	case month.IsCurrent:
		if month.Summary == nil {
			return
		}
		projection := month.Summary.Projection
		if projection.Status != poreport.ProjectionStatusAvailable || projection.Remainder == nil {
			r.drawEvidenceLine("Forecast unavailable", projectionReasonLabel(projection.Reason), false)
			return
		}
		r.drawEvidenceLine("Estimated remainder", estimateEvidence(r, *projection.Remainder), true)
	case month.IsFuture:
		currentYear := firstSeven(r.resolved.Overview.AsOf)[:4]
		if month.Month[:4] != currentYear {
			r.drawEvidenceLine("Forecast unavailable",
				"Future months beyond the current year have no current-year forecast. No retired projection is used as a fallback.",
				false)
			return
		}
		current := month.Source.CurrentProjection
		if current.Mode != poreport.ProjectionModeMonth ||
			current.Status != poreport.ProjectionStatusAvailable {
			reason := current.Reason
			if current.Mode != poreport.ProjectionModeMonth {
				reason = poreport.ReasonNotCurrentPeriod
			}
			r.drawEvidenceLine("Forecast unavailable", projectionReasonLabel(reason), false)
			return
		}
		wanted := month.Month + "-01"
		for _, bucket := range current.Buckets {
			if bucket.PeriodStart != wanted {
				continue
			}
			remainder := poreport.CurrentProjectionRemainder{
				LowMinor:     bucket.AmountLowMinor,
				BaseMinor:    bucket.AmountBaseMinor,
				HighMinor:    bucket.AmountHighMinor,
				CapitalMinor: bucket.CapitalMinor,
				MarginMinor:  bucket.MarginMinor,
				MarginBps:    bucket.MarginBps,
				SampleSize:   bucket.SampleSize,
				Source:       bucket.Source,
			}
			r.drawEvidenceLine("Estimated forecast", estimateEvidence(r, remainder), true)
			return
		}
		r.drawEvidenceLine("Forecast unavailable",
			"No projection bucket is supplied for this month.", false)
	}
}

// drawEvidenceLine prints one explicitly labelled line. An estimated line is
// muted and says "estimated" in its label; the label is text, not colour alone.
func (r *reportRenderer) drawEvidenceLine(label, body string, estimated bool) {
	prefix := label
	if estimated {
		prefix = "Estimated (not actual) — " + label
	}
	text := prefix + ": " + body
	color := r.pal.secondary
	if estimated {
		color = r.pal.muted
	}
	lines := r.faces.wrap(text, 7.3, contentWidth-8, false)
	if len(lines) == 0 {
		return
	}
	r.canvas.ensure(float64(len(lines))*10 + 8)
	if estimated {
		surface := r.pal.surface
		r.canvas.rect(marginX, r.canvas.y, contentWidth, float64(len(lines))*10+3, &surface, nil, 0)
	}
	for index, line := range lines {
		r.canvas.text(line, marginX+4, r.canvas.y+8+float64(index)*10, 7.3, false, color)
	}
	r.canvas.y += float64(len(lines))*10 + 8
}
