package gpdf

// The document the layout tests measure against.
//
// It is a trimmed transcription of a real quotation — the first six of thirteen
// items, which is the whole of page one — taken from the browser's own rendering
// so that every measurement taken of it is a measurement of what the browser
// would produce. The customer name, address and terms are the ones that
// quotation carried.

import (
	"vera-app/backend/labels"
)

func sampleInput() Input {
	item := func(no int, desc string, subs []string, qty, uom, price string) Item {
		return Item{
			No:        no,
			DescMain:  desc,
			SubItems:  subs,
			Qty:       qty,
			UoM:       uom,
			UnitPrice: price,
			Total:     price,
		}
	}
	return Input{
		QuoteNumber:      "1502260002",
		Date:             "02 January 2026",
		ValidUntil:       "02 February 2026",
		CompanyName:      "PT. Arcon Penta Persada",
		CompanyAddress:   []string{"Komp. Ruko Legenda Gemilang, Blok D. NO 12, Baloi Permai, Batam"},
		RecipientName:    "PT. Sandhi MCS",
		RecipientCompany: "Ruko Panakkukang",
		RecipientAddress: "Jl.Boulevard Raya Blok A5 No. 8, Panakkukang, Makassar",
		Subject:          "Pekerjaan Instalasi Sistem Sensor Cahaya",
		// The PPN row is decided by the numeric, the value the template's
		// {{if .PpnAmount}} tests (issue #377). The fixture carries one so
		// every measurement taken of it keeps the three-row totals card a
		// real quotation with PPN prints.
		PpnAmount:         76592203,
		DiscountPercent:   "0",
		DisplaySubTotal:   "0",
		DisplayDiscount:   "0",
		DisplayPpn:        "76.592.203",
		DisplayGrandTotal: "0",
		// The effective wording. Before #339 this fixture carried its labels
		// only implicitly -- the engine baked them -- so every measurement here
		// was of the default document without saying so. It says so now, and
		// the labels are the same defaults the pin in backend/labels holds.
		Labels: labels.DefaultLabels(),
		Terms: []string{
			"1. Payment terms agreed for this quotation is 90 days.",
			"2. This quotation is valid for 30 days from the date of issue.",
			"3. Prices are exclusive of VAT.",
		},
		Closing: "We will be happy to supply information that you may need and trust " +
			"that you call on us to fill your order, which will receive our " +
			"prompt and careful attention.",
		VerificationCaption: "PT. Arcon Penta Persada",
		VerificationCode:    "2e51f59464af4b00",
		Items: []Item{
			item(1,
				"Supply & Install lighting sensor system (PIR Schneider) at AHU block 3, Mezzanine Front & Rear with following scopes:",
				[]string{
					"6 sensors at Front AHU and 6 sensors at Rear AHU",
					"6-units batten fixtures with guard (2x 1200mm LED tubes)",
					"Rewiring with Auto-Manual selector and lighting switch",
				},
				"1", "Lot", "133.083.750"),
			item(2,
				"Supply & Install lighting sensor system (PIR Schneider) 14 sensors at CDA block 4.3",
				nil, "1", "Lot", "68.965.500"),
			item(3,
				"Supply & Install lighting sensor system (PIR Schneider) 5 sensors at office block 9",
				nil, "1", "Lot", "25.495.500"),
			item(4,
				"Supply and install aircond automation system at Mesjid Batam 1:",
				[]string{
					"Metal enclosure panel with timers",
					"Zelio Schneider programmable controller",
					"Wiring and commissioning",
				},
				"1", "Lot", "38.136.000"),
			item(5,
				"Supply & install exhaust fan system at CDA block 2:",
				[]string{
					"4 units axial exhaust fan 18 1.5HP",
					"Galvanized ducting complete with damper and grille",
					"Electrical wiring and control panel",
				},
				"2", "Lot", "63.250.000"),
			item(6,
				"Monthly preventive maintenance for HVAC system at main office building",
				nil, "12", "Lot", "110.160.000"),
		},
	}
}
