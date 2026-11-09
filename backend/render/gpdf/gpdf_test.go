package gpdf

// The seam between the quotation view and this engine's own Input.

import (
	"testing"

	"vera-app/backend/render/quotation"
)

// TestEngineInputCarriesTheNote pins the mapping the lazy renderer performs:
// the note the application put on the render seam reaches the engine's Input.
// Without this, the browser would print the note and the native PDF would not
// -- an HTML-only change that no engine test would notice.
func TestEngineInputCarriesTheNote(t *testing.T) {
	const note = "Delivery is ex-works.\nInstallation by the customer."
	in := engineInput(quotation.Input{Note: note})
	if in.Note != note {
		t.Errorf("engine note = %q; want %q", in.Note, note)
	}
	if got := engineInput(quotation.Input{}).Note; got != "" {
		t.Errorf("engine note = %q for a quotation with no note; want empty", got)
	}
}
