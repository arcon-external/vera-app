package backend

import "testing"

func TestBuildExportFilename(t *testing.T) {
	cases := []struct {
		name     string
		number   string
		revision uint16
		subject  string
		want     string
	}{
		{"original", "15020003", 0, "Supply Panels", "Arcon_Quotation_15020003_Supply Panels.pdf"},
		{"first revision", "15020003", 1, "Supply Panels", "Arcon_Quotation_15020003_Rev.1_Supply Panels.pdf"},
		{"twelfth revision", "15020003", 12, "Supply Panels", "Arcon_Quotation_15020003_Rev.12_Supply Panels.pdf"},
		{"slashes stripped", "15020003", 0, "A/B\\C:D", "Arcon_Quotation_15020003_A_B_C_D.pdf"},
		{"quotes and stars stripped", "15020003", 0, `Say "hi" *now*?`, "Arcon_Quotation_15020003_Say _hi_ _now__.pdf"},
		{"empty subject falls back", "15020003", 0, "   ", "Arcon_Quotation_15020003_Quotation.pdf"},
		{"long subject capped", "15020003", 0, "123456789012345678901234567890123456789012345678901234567890EXTRA", "Arcon_Quotation_15020003_123456789012345678901234567890123456789012345678901234567890.pdf"},
		{"long subject breaks at a word", "15020003", 0, "Supply & Installation of Electrical and Mechanical Systems and Panels", "Arcon_Quotation_15020003_Supply & Installation of Electrical and Mechanical Systems.pdf"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildExportFilename(tc.number, tc.revision, tc.subject); got != tc.want {
				t.Errorf("BuildExportFilename = %q; want %q", got, tc.want)
			}
		})
	}
}
