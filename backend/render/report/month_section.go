package report

import (
	"fmt"
	"time"

	"vera-app/backend/poreport"
)

// Issue #641/#642: the selected-month section replaces the legacy
// bucket-following period table in a view export. It prints exactly the chosen
// months' six metrics from their frozen sources, then the separate
// recorded-future and estimated lines. Nothing here adds an estimate to an
// actual, sums months, or recomputes a figure.

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
	switch {
	case r.resolved.Aggregate != nil:
		r.drawAggregateMonths()
	case len(r.resolved.Months) == 0:
		r.drawEmptyMonthSection()
	default:
		r.drawIndividualMonths()
	}
}

// drawEmptyMonthSection prints the explicit empty state. It is not a
// zero-valued financial report: it has no metric rows, no total and no
// estimate, only the section scope and a direction to choose a month.
func (r *reportRenderer) drawEmptyMonthSection() {
	overview := r.resolved.Overview
	scope := fmt.Sprintf("0 months selected  ·  %s  ·  %s  ·  As of %s  ·  Snapshot %s",
		categoryFilterLabel(overview.Filter.Category),
		overview.Filter.Currency,
		overview.AsOf,
		generatedLabel(overview.GeneratedAt))
	r.canvas.ensure(r.titleBlockHeight(scope) + 56)
	r.sectionTitle("By period", scope)
	r.emptyState("No months selected",
		"Choose at least one month to print a period comparison. No figures and no total are shown here.")
}

type monthEvidence struct {
	label     string
	body      string
	estimated bool
}

// monthEvidenceItems returns the separate evidence lines for one selected
// month. A current month adds its recorded future-dated evidence and its
// remainder estimate; a future month adds its full-month estimate or the stable
// unavailable reason. A past month has none.
func (r *reportRenderer) monthEvidenceItems(month resolvedMonth) []monthEvidence {
	var items []monthEvidence
	if month.IsCurrent && month.Summary != nil && month.Summary.RecordedFuture != nil {
		items = append(items, monthEvidence{
			label: "Future-dated recorded",
			body:  metricsEvidence(r, *month.Summary.RecordedFuture),
		})
	}
	if item, ok := r.monthForecastEvidence(month); ok {
		items = append(items, item)
	}
	return items
}

func (r *reportRenderer) monthForecastEvidence(month resolvedMonth) (monthEvidence, bool) {
	switch {
	case month.IsCurrent:
		if month.Summary == nil {
			return monthEvidence{}, false
		}
		projection := month.Summary.Projection
		if projection.Status != poreport.ProjectionStatusAvailable || projection.Remainder == nil {
			return monthEvidence{
				label: "Forecast unavailable",
				body:  projectionReasonLabel(projection.Reason),
			}, true
		}
		return monthEvidence{
			label:     "Estimated remainder",
			body:      estimateEvidence(r, *projection.Remainder),
			estimated: true,
		}, true
	case month.IsFuture:
		currentYear := firstSeven(r.resolved.Overview.AsOf)[:4]
		if month.Month[:4] != currentYear {
			return monthEvidence{
				label: "Forecast unavailable",
				body:  "Future months beyond the current year have no current-year forecast. No retired projection is used as a fallback.",
			}, true
		}
		current := month.Source.CurrentProjection
		if current.Mode != poreport.ProjectionModeMonth ||
			current.Status != poreport.ProjectionStatusAvailable {
			reason := current.Reason
			if current.Mode != poreport.ProjectionModeMonth {
				reason = poreport.ReasonNotCurrentPeriod
			}
			return monthEvidence{
				label: "Forecast unavailable",
				body:  projectionReasonLabel(reason),
			}, true
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
			return monthEvidence{
				label:     "Estimated forecast",
				body:      estimateEvidence(r, remainder),
				estimated: true,
			}, true
		}
		return monthEvidence{
			label: "Forecast unavailable",
			body:  "No projection bucket is supplied for this month.",
		}, true
	default:
		return monthEvidence{}, false
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

// drawEvidenceLine prints one explicitly labelled line outside the comparison
// cards or the aggregate table. An estimated line is muted and says
// "estimated" in its label; the label is text, not colour alone.
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
