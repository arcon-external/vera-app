package gpdf

import (
	"testing"

	"vera-app/backend/currency"
)

// TestCurrencyHeadersFitTheirTracks proves a longer or shorter code still
// fits: every catalogue code is three ASCII capitals like IDR, so the header
// width barely moves, but this measures rather than assumes. Each header is
// drawn centred in its track at 10px bold with 10px of cell padding on each
// side (see drawTableHead and thPadPx): a label wider than its track minus
// the padding would bleed into its neighbour.
func TestCurrencyHeadersFitTheirTracks(t *testing.T) {
	e := testEngine(t)
	edges := ColumnEdgesPx()
	for _, code := range currency.Codes() {
		labels := HeadLabels(code)
		if len(labels) != 7 {
			t.Fatalf("HeadLabels(%q) = %d labels; want 7", code, len(labels))
		}
		for _, track := range []int{4, 5, 6} {
			label := labels[track]
			textW := e.Width(label, px(10), true)
			trackW := float64(edges[track+1]-edges[track]) * ptPerPx
			usable := trackW - 2*px(thPadPx)
			t.Logf("%s track %d %q: text %.2fpt, usable %.2fpt", code, track, label, textW, usable)
			if textW > usable {
				t.Errorf("HeadLabels(%q)[%d] = %q (%.2fpt) overflows its %.2fpt track",
					code, track, label, textW, trackW)
			}
		}
	}
}
