package report

import (
	"fmt"
	"strings"

	"vera-app/backend/poreport"
)

func (r *reportRenderer) drawProjectionDetails() {
	r.sectionTitle("Projection detail",
		"The monthly and yearly forecasts are independent. Each states its method and the complete-month sample it was read from; "+
			"an unavailable forecast states why rather than inventing a trend.")
	r.drawProjectionDetail("Monthly projection  ·  next 12 months", r.snapshot.Projection.Monthly, poreport.BucketMonth)
	r.drawProjectionDetail("Yearly projection  ·  next 3 years", r.snapshot.Projection.Yearly, poreport.BucketYear)
}

func (r *reportRenderer) drawProjectionDetail(label string, series poreport.ProjectionSeries, bucket string) {
	c := r.canvas
	pal := r.pal
	c.ensure(40)
	c.text(label, marginX, c.y+10, 9, true, pal.emphasis)
	status := projectionStatusLabel(series.Status)
	statusColor := pal.muted
	if series.Status == poreport.ProjectionStatusAvailable {
		statusColor = pal.accent
	}
	c.textRight(status, marginX+contentWidth, c.y+10, 7, true, statusColor)
	c.y += 18

	if series.Status == poreport.ProjectionStatusAvailable {
		r.drawProjectionTable(series, bucket)
	} else {
		r.reasonBox(series.Reason)
	}

	r.paragraph("Method", series.Method)
	r.paragraph("Basis", r.projectionBasisText(series))
	if series.Status == poreport.ProjectionStatusAvailable {
		r.projectionSources(series.Buckets)
	}
	c.y += 8
}

func (r *reportRenderer) reasonBox(reason string) {
	c := r.canvas
	lines := r.faces.wrap(projectionReasonLabel(reason), 7.4, contentWidth-24, false)
	height := float64(len(lines))*10 + 18
	c.ensure(height + 4)
	c.rect(marginX, c.y, contentWidth, height, &r.pal.surface, &r.pal.edge, 0.5)
	c.text("REASON  "+reason, marginX+10, c.y+13, 6.4, true, r.pal.muted)
	baseline := c.y + 25
	for _, line := range lines {
		c.text(line, marginX+10, baseline, 7.4, false, r.pal.secondary)
		baseline += 10
	}
	c.y += height + 8
}

func (r *reportRenderer) paragraph(label, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	c := r.canvas
	prefix := label + ": "
	prefixWidth := r.faces.width(prefix, 7.3, true)
	if prefixWidth > 70 {
		prefixWidth = 70
	}
	lines := r.faces.wrap(text, 7.3, contentWidth-prefixWidth, false)
	if len(lines) == 0 {
		return
	}
	c.ensure(float64(len(lines))*10 + 6)
	c.text(prefix, marginX, c.y+8, 7.3, true, r.pal.emphasis)
	for _, line := range lines {
		c.text(line, marginX+prefixWidth, c.y+8, 7.3, false, r.pal.secondary)
		c.y += 10
	}
	c.y += 3
}

func (r *reportRenderer) projectionBasisText(series poreport.ProjectionSeries) string {
	basis := series.Basis
	asOf := basis.AsOf
	if asOf == "" {
		asOf = "not stated"
	}
	if basis.HistoryFrom == "" || basis.HistoryTo == "" {
		return fmt.Sprintf("No complete history window is available; as of %s.", asOf)
	}
	return fmt.Sprintf("History %s to %s (exclusive)  ·  %d complete months  ·  %d purchase orders  ·  as of %s.",
		basis.HistoryFrom, basis.HistoryTo, basis.CompleteMonths, basis.PoCount, asOf)
}

func (r *reportRenderer) projectionSources(buckets []poreport.ProjectedBucket) {
	seen := map[string]bool{}
	var sources []string
	for _, bucket := range buckets {
		if bucket.Source == "" || seen[bucket.Source] {
			continue
		}
		seen[bucket.Source] = true
		sources = append(sources, bucket.Source)
	}
	if len(sources) == 0 {
		return
	}
	r.paragraph("Bucket sources", strings.Join(projectionSourceLabels(sources), "; "))
}

func projectionSourceLabels(sources []string) []string {
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		out = append(out, projectionSourceLabel(source)+" ["+source+"]")
	}
	return out
}
