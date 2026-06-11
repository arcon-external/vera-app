package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePDF(t *testing.T) {
	ctx := context.Background()
	q := &Quotation{}

	err := Startup(q, ctx)
	if err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	q.Prepare(QuotationInput{
		Items: []QuotationItemInput{
			{
				Description: "Supply & Install lighting sensor system (PIR Schneider) at AHU block 3, Mezzanine Front & Rear with following scopes:\n- 6 sensors at Front AHU and 6 sensors at Rear AHU\n- 6-units batten fixtures with guard (2x 1200mm LED tubes)\n- Rewiring with Auto-Manual selector and lighting switch",
				Qty:         1,
				UoM:         "Lot",
				UnitPrice:   115725000,
			},
			{
				Description: "Supply & Install lighting sensor system (PIR Schneider) 14 sensors at CDA block 4.3",
				Qty:         1,
				UoM:         "Lot",
				UnitPrice:   59970000,
			},
			{
				Description: "Supply & Install lighting sensor system (PIR Schneider) 5 sensors at office block 9",
				Qty:         1,
				UoM:         "Lot",
				UnitPrice:   22170000,
			},
			{
				Description: "Supply and install aircond automation system at Mesjid Batam 1:\n- Metal enclosure panel with timers\n- Zelio Schneider programmable controller\n- Wiring and commissioning",
				Qty:         1,
				UoM:         "Lot",
				UnitPrice:   34050000,
			},
			{
				Description: "Supply & install exhaust fan system at CDA block 2:\n- 4 units axial exhaust fan 18\" 1.5HP\n- Galvanized ducting complete with damper and grille\n- Electrical wiring and control panel",
				Qty:         2,
				UoM:         "Lot",
				UnitPrice:   28750000,
			},
			{
				Description: "Monthly preventive maintenance for HVAC system at main office building",
				Qty:         12,
				UoM:         "Month",
				UnitPrice:   8500000,
			},
			{
				Description: "Supply and delivery of LED batten light fixture 2x18W complete with:\n- Emergency battery backup\n- Surface mounting bracket\n- 3-year manufacturer warranty",
				Qty:         48,
				UoM:         "Unit",
				UnitPrice:   475000,
			},
			{
				Description: "Electrical installation and rewiring for production line extension:\n- Main distribution board upgrade (200A)\n- Cable tray and trunking installation\n- Power point and lighting circuit wiring\n- Testing and certification",
				Qty:         1,
				UoM:         "Lot",
				UnitPrice:   78500000,
			},
			{
				Description: "Supply, fabrication, and installation of cable ladder system:\n- Hot-dip galvanized steel, 600mm width\n- Wall brackets and support arms\n- Expansion joints and coupler plates",
				Qty:         85,
				UoM:         "Meter",
				UnitPrice:   425000,
			},
			{
				Description: "Testing and commissioning services for entire electrical installation scope",
				Qty:         1,
				UoM:         "Lot",
				UnitPrice:   15300000,
			},
		},
		Subject:          "Supply & Installation of Electrical and Mechanical Systems at PT Example Plant",
		RecipientName:    "Budi Santoso",
		RecipientCompany: "PT Example Industrial",
		RecipientAddress: "Kawasan Industri Batamindo, Jl. Gaharu Lot 42, Kel. Muka Kuning, Kota Batam 29433",
		DiscountPercent:  5,
		Validity:         30,
		PaymentTerm:      90,
		LeadTime:         60,
	})

	pdfBuf, err := q.build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	if len(pdfBuf) == 0 {
		t.Fatal("PDF buffer is empty")
	}

	inspectDir := "bin"
	if err := os.MkdirAll(inspectDir, 0o755); err != nil {
		t.Fatalf("create inspect dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inspectDir, "quotation.pdf"), pdfBuf, 0o644); err != nil {
		t.Fatalf("write inspect pdf: %v", err)
	}
}

func TestValidateSavePathRejectsNonPDF(t *testing.T) {
	tempDir := t.TempDir()
	_, err := validateSavePath(filepath.Join(tempDir, "quotation.txt"))
	if err == nil {
		t.Fatal("expected non-pdf path to be rejected")
	}
}

func TestValidateSavePathRejectsDirectory(t *testing.T) {
	tempDir := t.TempDir()
	_, err := validateSavePath(tempDir)
	if err == nil {
		t.Fatal("expected directory path to be rejected")
	}
}

func TestValidateSavePathAcceptsPDF(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "quotation.pdf")
	absPath, err := validateSavePath(inputPath)
	if err != nil {
		t.Fatalf("expected valid pdf path, got error: %v", err)
	}

	expectedPath, err := filepath.Abs(inputPath)
	if err != nil {
		t.Fatalf("resolve expected absolute path: %v", err)
	}

	if absPath != expectedPath {
		t.Fatalf("unexpected validated path: got %q want %q", absPath, expectedPath)
	}
}

func TestFormatRupiah(t *testing.T) {
	cases := []struct {
		amount uint64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.000"},
		{50000, "50.000"},
		{1000000, "1.000.000"},
		{115725000, "115.725.000"},
		{999999999, "999.999.999"},
		{1000000000, "1.000.000.000"},
	}

	for _, tc := range cases {
		got := formatRupiah(tc.amount)
		if got != tc.want {
			t.Errorf("formatRupiah(%d) = %q; want %q", tc.amount, got, tc.want)
		}
	}
}

func TestFormatDescription(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "plain single line",
			raw:  "Install lighting system",
			want: `<div class="desc-main">Install lighting system</div>`,
		},
		{
			name: "plain multi-line",
			raw:  "Line one\nLine two",
			want: `<div class="desc-main">Line one Line two</div>`,
		},
		{
			name: "description with bullets",
			raw:  "Main scope\n- sensor 1\n- sensor 2",
			want: `<div class="desc-main">Main scope</div><ul class="desc-subitems"><li>sensor 1</li><li>sensor 2</li></ul>`,
		},
		{
			name: "bullets only",
			raw:  "- item a\n- item b",
			want: `<ul class="desc-subitems"><li>item a</li><li>item b</li></ul>`,
		},
		{
			name: "empty string",
			raw:  "",
			want: "", // html.EscapeString("") = ""
		},
		{
			name: "windows line endings",
			raw:  "Scope\r\n- detail 1\r\n- detail 2",
			want: `<div class="desc-main">Scope</div><ul class="desc-subitems"><li>detail 1</li><li>detail 2</li></ul>`,
		},
		{
			name: "dangling dash treated as text",
			raw:  "- \n- valid item\n-  \n",
			want: `<div class="desc-main">- -</div><ul class="desc-subitems"><li>valid item</li></ul>`,
		},
		{
			name: "blank lines ignored",
			raw:  "\n\nMain text\n\n- bullet\n\n",
			want: `<div class="desc-main">Main text</div><ul class="desc-subitems"><li>bullet</li></ul>`,
		},
		{
			name: "leading dash without space is not a bullet",
			raw:  "Regular text - not a bullet",
			want: `<div class="desc-main">Regular text - not a bullet</div>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(formatDescription(tc.raw))
			if got != tc.want {
				t.Errorf("formatDescription(%q)\n  got: %q\n want: %q", tc.raw, got, tc.want)
			}
		})
	}
}
