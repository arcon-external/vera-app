package report

import (
	"fmt"
	"strings"

	"vera-app/backend/poreport"
)

// Issue #643: the selected-category section of a versioned export view. It is
// the category analogue of month_section.go and deliberately shares the card
// grid in comparison.go. Every figure is a supplied category row or the
// category source's own server totals; nothing here sums rows, forecasts, or
// re-orders the source.

var categoryMetricLabels = []string{
	"POs", "PO amount", "Costed amount", "Capital", "PO margin", "PO margin %",
}

func (r *reportRenderer) drawSelectedCategorySection() {
	if r.resolved == nil || r.view == nil {
		return
	}
	if len(r.view.Categories.Codes) == 0 {
		r.drawEmptyCategorySection()
		return
	}
	if r.view.Categories.Layout == ViewLayoutAggregate {
		r.drawAggregateCategories()
		return
	}
	r.drawIndividualCategories()
}

// categoryRangeText is the inherited date range the category section prints,
// in the same half-open spelling as the report header.
func (r *reportRenderer) categoryRangeText(source poreport.Report) string {
	return source.Filter.From + " → " + source.Filter.To + " (exclusive)"
}

// categoryScopeText is the scope every category card and the aggregate table
// inherit. It names the category section's own source, never the Performance
// filter that may have restricted the overview.
func (r *reportRenderer) categoryScopeText(source poreport.Report) string {
	return fmt.Sprintf("%s  ·  %s  ·  As of %s  ·  Snapshot %s",
		r.categoryRangeText(source),
		source.Filter.Currency,
		source.AsOf,
		generatedLabel(source.GeneratedAt))
}

// categorySourceCaption names what the aggregate rows are. A category=all
// source prints the supplied row count; a #641 compatibility source that is
// restricted to the overview's own category prints that filter honestly.
func (r *reportRenderer) categorySourceCaption(source poreport.Report) string {
	if source.Filter.Category == poreport.CategoryAll {
		return fmt.Sprintf("All categories (%d)", len(source.Categories))
	}
	return categoryFilterLabel(source.Filter.Category)
}

// categoryHeadings maps each supplied row to its printed heading. An empty
// supplied label falls back to the workbook name; a label shared by more than
// one row gains its stable code only where that is what tells the rows apart.
func categoryHeadings(rows []poreport.Category) map[string]string {
	bases := make(map[string]string, len(rows))
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		base := strings.TrimSpace(row.Label)
		if base == "" {
			base = categoryLabel(row.BusinessCategory)
		}
		bases[row.BusinessCategory] = base
		counts[base]++
	}
	headings := make(map[string]string, len(bases))
	for code, base := range bases {
		if counts[base] > 1 && base != code {
			headings[code] = base + " (" + code + ")"
			continue
		}
		headings[code] = base
	}
	return headings
}

// drawEmptyCategorySection prints the explicit empty selection. It is not a
// zero-valued comparison: it has no metric row, no total and no estimate, only
// the inherited scope and a direction to choose a category.
func (r *reportRenderer) drawEmptyCategorySection() {
	source := r.resolved.Categories
	scope := "0 categories selected  ·  " + r.categoryScopeText(source)
	r.canvas.ensure(r.titleBlockHeight(scope) + 56)
	r.sectionTitle("By business category", scope)
	r.emptyState("No categories selected",
		"Choose at least one business category to print a category comparison. No figures and no total are shown here.")
}

// planCategoryCard lays out one category's six actual metrics. There is no
// evidence block: a category row is supplied actual evidence, never a forecast.
// The only disclosure is the uncosted amount, which is never counted as profit.
func (r *reportRenderer) planCategoryCard(item resolvedCategory, width float64, heading string) comparisonCardPlan {
	inner := width - 2*cardPad
	plan := comparisonCardPlan{Key: item.Code, Heading: heading}
	plan.Scope = r.faces.wrapCell(r.categoryScopeText(r.resolved.Categories), cardScopeSize, inner, false)

	values := []string{
		formatCount(item.Metrics.PoCount),
		r.money(item.Metrics.AmountMinor),
		r.money(item.Metrics.CostedMinor),
		r.money(item.Metrics.CapitalMinor),
		r.signedMoney(item.Metrics.MarginMinor),
		formatPct(item.Metrics.MarginBps),
	}
	plan.Metrics = make([]cardMetricPlan, 0, len(categoryMetricLabels))
	for index, label := range categoryMetricLabels {
		lines := r.faces.wrapCell(values[index], cardValueSize, inner-cardLabelWidth, false)
		if len(lines) == 0 {
			lines = []string{""}
		}
		plan.Metrics = append(plan.Metrics, cardMetricPlan{
			Label:      label,
			ValueLines: lines,
			Height:     float64(len(lines))*cardLineHeight + 2.4,
		})
	}
	if item.Metrics.UncostedMinor > 0 {
		plan.Note = r.faces.wrapCell(fmt.Sprintf(
			"Uncosted %s is not counted as profit; PO margin covers only the costed amount.",
			r.money(item.Metrics.UncostedMinor)), cardNoteSize, inner, false)
	}
	plan.Height = plan.measuredHeight()
	return plan
}

func (r *reportRenderer) drawIndividualCategories() {
	source := r.resolved.Categories
	items := r.resolved.ResolvedCategories
	if len(items) == 0 {
		r.drawEmptyCategorySection()
		return
	}
	headings := categoryHeadings(source.Categories)
	heading := func(code string) string {
		if value, ok := headings[code]; ok {
			return value
		}
		return categoryLabel(code)
	}

	if len(items) == 1 {
		plan := r.planCategoryCard(items[0], contentWidth,
			"By business category — "+heading(items[0].Code))
		r.canvas.ensure(plan.Height)
		top := r.canvas.y
		r.drawComparisonCard(plan, marginX, top, contentWidth, plan.Height, 0, true, cardSectionCategory)
		r.canvas.y = top + plan.Height + cardGap
		return
	}

	title := fmt.Sprintf("By business category — %d selected categories", len(items))
	if len(items) == len(source.Categories) {
		title = "By business category — All categories (individual)"
	}
	half := (contentWidth - cardGap) / 2
	plans := make([]comparisonCardPlan, len(items))
	for index, item := range items {
		plans[index] = r.planCategoryCard(item, half, heading(item.Code))
	}
	r.drawComparisonCardGrid(title, r.categoryScopeText(source), cardSectionCategory, plans,
		func(index int, width float64) comparisonCardPlan {
			return r.planCategoryCard(items[index], width, heading(items[index].Code))
		})
}

// drawAggregateCategories prints the combined wide table over every supplied
// row. Its one total is the category source's server total, labelled Total; no
// row is summed and no forecast enters the table.
func (r *reportRenderer) drawAggregateCategories() {
	source := r.resolved.Categories
	headings := categoryHeadings(source.Categories)
	columns := actualTableColumns("CATEGORY")
	rows := make([]tableRow, 0, len(source.Categories))
	for index, category := range source.Categories {
		var fill *rgb
		if index%2 == 1 {
			surface := r.pal.surface
			fill = &surface
		}
		label := headings[category.BusinessCategory]
		if label == "" {
			label = categoryLabel(category.BusinessCategory)
		}
		rows = append(rows, tableRow{fill: fill, cells: []string{
			label,
			formatCount(category.PoCount),
			r.money(category.AmountMinor),
			r.money(category.CostedMinor),
			r.money(category.CapitalMinor),
			r.signedMoney(category.MarginMinor),
			formatPct(category.MarginBps),
		}})
	}
	total := r.totalRowFor(source, "Total")
	note := fmt.Sprintf(
		"%s  ·  %s. The Total row is this snapshot's server total, not a sum of the printed rows; no forecast or estimate is included.",
		r.categorySourceCaption(source), r.categoryScopeText(source))
	// Keep the table title, its note, the header and several rows together.
	r.canvas.ensure(170)
	r.drawTable("By business category", note, columns, rows, &total)
}
