// Command render-theme-pdfs renders one quotation PDF with a chosen brand
// theme for the pixel-delta harness (scripts/pixel-delta.py).
//
// It uses only the stable gpdf API (NewEngine, SetTheme, Render) with a
// four-field Theme, which is unchanged across main, the first-iteration
// commit 70d3089 and the current tree, so the same file compiles and runs on
// all three: checking out an old commit in a worktree and running the same
// command there is how the "before" PDF is produced. (Verified: this file
// builds and renders on main, on 70d3089 and on the branch.)
//
// Reproducing the criterion-4 number from the repository root (run each
// render from the root of its own tree, since the engine loads
// backend/assets relative to the working directory):
//
//	git worktree add /tmp/vera-first 70d3089
//	(cd /tmp/vera-first && go run scripts/render-theme-pdfs.go --out /tmp/first_default.pdf)
//	go run scripts/render-theme-pdfs.go --out /tmp/cur_default.pdf
//	python3 scripts/pixel-delta.py /tmp/first_default.pdf /tmp/cur_default.pdf
//
// Expected: 0 changed pixels on every channel (the current default renders
// byte-identically to the first iteration). An accent swap shows how much of
// the document a rebrand now reaches:
//
//	(cd /tmp/vera-first && go run scripts/render-theme-pdfs.go --out /tmp/first_accent.pdf --accent '#C2410C')
//	go run scripts/render-theme-pdfs.go --out /tmp/cur_accent.pdf --accent '#C2410C'
//	python3 scripts/pixel-delta.py /tmp/first_accent.pdf /tmp/cur_accent.pdf
//
// Usage from the repository root:
//
//	go run scripts/render-theme-pdfs.go --out /tmp/default.pdf
//	go run scripts/render-theme-pdfs.go --out /tmp/accent.pdf --accent '#C2410C'
//	go run scripts/render-theme-pdfs.go --out /tmp/collide.pdf --soft '#F7F2EC'
package main

import (
	"flag"
	"fmt"
	"os"

	"vera-app/backend/render/gpdf"
)

func main() {
	out := flag.String("out", "", "output PDF path (required)")
	deep := flag.String("deep", "", "brand deep hex, empty means default")
	accent := flag.String("accent", "", "brand accent hex, empty means default")
	soft := flag.String("soft", "", "brand soft hex, empty means default")
	ink := flag.String("ink", "", "brand ink hex, empty means default")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "missing --out")
		os.Exit(2)
	}
	eng, err := gpdf.NewEngine(os.DirFS("backend/assets"), "inter-medium.ttf", "inter-bold.ttf")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load engine:", err)
		os.Exit(1)
	}
	eng.SetTheme(gpdf.Theme{Deep: *deep, Accent: *accent, Soft: *soft, Ink: *ink})
	pdf, err := eng.Render(sample())
	if err != nil {
		fmt.Fprintln(os.Stderr, "render:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, pdf, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
}

// sample is a two-item quotation spanning the header, cards, table, totals
// and verification blocks: every palette role paints somewhere in it.
func sample() gpdf.Input {
	return gpdf.Input{
		CompanyName:      "PT. Arcon Penta Persada",
		CompanyAddress:   []string{"Komp. Ruko Legenda Gemilang, Blok D. NO 12, Baloi Permai, Batam"},
		Subject:          "Pekerjaan Instalasi Sistem Sensor Cahaya",
		RecipientName:    "Budi Santoso",
		RecipientCompany: "PT. Infineon Technologies Batam",
		RecipientAddress: "Jl.Boulevard Raya Blok A5 No. 8, Panakkukang, Makassar",
		QuoteNumber:      "1502260002",
		Date:             "02 January 2026",
		ValidUntil:       "02 February 2026",
		Revision:         "00",
		Terms: []string{
			"1. Payment terms agreed for this quotation is 90 days.",
			"2. This quotation is valid for 30 days from the date of issue.",
			"3. Lead time is 60 days.",
			"4. Incoterms: FOB",
		},
		Items: []gpdf.Item{
			{
				No: 1, DescMain: "Supply & Install lighting sensor system (PIR Schneider) at AHU block 3",
				SubItems: []string{
					"6 sensors at Front AHU and 6 sensors at Rear AHU",
					"6-units batten fixtures with guard (2x 1200mm LED tubes)",
				},
				Qty: "1", UoM: "Lot", UnitPrice: "133.083.750", Total: "133.083.750", Disc: "—",
			},
			{
				No: 2, DescMain: "Monthly preventive maintenance for HVAC system",
				Qty: "12", UoM: "Lot", UnitPrice: "9.180.000", Total: "110.160.000", Disc: "—",
			},
		},
		DisplaySubTotal:   "243.243.750",
		DisplayGrandTotal: "243.243.750",
		DiscountPercent:   "0",
		Closing: "We will be happy to supply information that you may need and trust " +
			"that you call on us to fill your order, which will receive our prompt " +
			"and careful attention.",
		VerificationCode: "2e51f59464af4b00",
	}
}
