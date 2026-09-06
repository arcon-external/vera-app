package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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
		Subject:         "Supply & Installation of Electrical and Mechanical Systems at PT Example Plant",
		RecipientName:   "Budi Santoso",
		DiscountPercent: 5,
		Validity:        30,
		PaymentTerm:     90,
		LeadTime:        60,
		CustomerID:      2,
		Incoterm:        "FOB",
	})

	quoteID, err := generateQuoteNumber(q.Data.CustomerID)
	if err != nil {
		t.Fatalf("generate quote number: %v", err)
	}
	q.Data.QuoteNumber = quoteID

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
		want   string
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

func TestCheckCustomerID(t *testing.T) {
	cases := []struct {
		id   uint8
		want bool
	}{
		{0, false},
		{1, false},
		{2, true},
		{3, true},
		{4, true},
		{5, true},
		{6, true},
		{7, false},
		{255, false},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("ID_%d", tc.id), func(t *testing.T) {
			if got := checkCustomerID(tc.id); got != tc.want {
				t.Errorf("checkCustomerID(%d) = %v; want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestBuildItems(t *testing.T) {
	inputs := []QuotationItemInput{
		{Description: "Item A", Qty: 2, UoM: "Lot", UnitPrice: 10000},
		{Description: "Item B", Qty: 3, UoM: "Unit", UnitPrice: 5000},
	}

	items := buildItems(inputs, 0)
	if len(items) != 2 {
		t.Fatalf("buildItems returned %d items; want 2", len(items))
	}

	if items[0].No != 1 {
		t.Errorf("items[0].No = %d; want 1", items[0].No)
	}
	if items[1].No != 2 {
		t.Errorf("items[1].No = %d; want 2", items[1].No)
	}
	if items[0].Total != 20000 {
		t.Errorf("items[0].Total = %d; want 20000", items[0].Total)
	}
	if items[1].Total != 15000 {
		t.Errorf("items[1].Total = %d; want 15000", items[1].Total)
	}
	if items[0].DisplayUnitPrice != "10.000" {
		t.Errorf("items[0].DisplayUnitPrice = %q; want %q", items[0].DisplayUnitPrice, "10.000")
	}
}

func TestBuildItemsEmpty(t *testing.T) {
	items := buildItems(nil, 0)
	if items == nil {
		t.Fatal("buildItems(nil) returned nil; want empty slice")
	}
	if len(items) != 0 {
		t.Errorf("buildItems(nil) returned %d items; want 0", len(items))
	}
}

func TestBuildDates(t *testing.T) {
	date, validUntil := buildDates(30)

	expectedDate := time.Now().Format("January 02, 2006")
	expectedValid := time.Now().AddDate(0, 0, 30).Format("January 02, 2006")

	if date != expectedDate {
		t.Errorf("date = %q; want %q", date, expectedDate)
	}
	if validUntil != expectedValid {
		t.Errorf("validUntil = %q; want %q", validUntil, expectedValid)
	}
}

func TestBuildPrices(t *testing.T) {
	items := []QuotationItem{
		{Total: 100000},
		{Total: 200000},
	}

	subTotal, discount, grandTotal := buildPrices(items, 10)
	if subTotal != 300000 {
		t.Errorf("subTotal = %d; want 300000", subTotal)
	}
	if discount != 30000 {
		t.Errorf("discount = %d; want 30000", discount)
	}
	if grandTotal != 270000 {
		t.Errorf("grandTotal = %d; want 270000", grandTotal)
	}
}

func TestBuildPricesNoDiscount(t *testing.T) {
	items := []QuotationItem{
		{Total: 50000},
		{Total: 50000},
	}

	subTotal, discount, grandTotal := buildPrices(items, 0)
	if subTotal != 100000 {
		t.Errorf("subTotal = %d; want 100000", subTotal)
	}
	if discount != 0 {
		t.Errorf("discount = %d; want 0", discount)
	}
	if grandTotal != 100000 {
		t.Errorf("grandTotal = %d; want 100000", grandTotal)
	}
}

func TestBuildPricesEmptyItems(t *testing.T) {
	subTotal, discount, grandTotal := buildPrices(nil, 10)
	if subTotal != 0 || discount != 0 || grandTotal != 0 {
		t.Errorf("nil items: got %d, %d, %d; want 0, 0, 0", subTotal, discount, grandTotal)
	}
}

func TestBuildViews(t *testing.T) {
	dSub, dDisc, dGrand := buildViews(100000, 10000, 90000)
	if dSub != "100.000" {
		t.Errorf("display sub total = %q; want %q", dSub, "100.000")
	}
	if dDisc != "10.000" {
		t.Errorf("display discount = %q; want %q", dDisc, "10.000")
	}
	if dGrand != "90.000" {
		t.Errorf("display grand total = %q; want %q", dGrand, "90.000")
	}
}

func TestNewQuotationItem(t *testing.T) {
	item := newQuotationItem("Service installation", 5, "Unit", 10000)

	if item.Description != "Service installation" {
		t.Errorf("Description = %q; want %q", item.Description, "Service installation")
	}
	if item.Qty != 5 {
		t.Errorf("Qty = %d; want 5", item.Qty)
	}
	if item.UoM != "Unit" {
		t.Errorf("UoM = %q; want %q", item.UoM, "Unit")
	}
	if item.UnitPrice != 10000 {
		t.Errorf("UnitPrice = %d; want 10000", item.UnitPrice)
	}
	if item.Total != 50000 {
		t.Errorf("Total = %d; want 50000", item.Total)
	}
	if item.DisplayTotal != "50.000" {
		t.Errorf("DisplayTotal = %q; want %q", item.DisplayTotal, "50.000")
	}
	if item.DisplayDesc == "" {
		t.Error("DisplayDesc is empty")
	}
	if item.No != 0 {
		t.Errorf("No = %d; want 0 (constructor should not set No)", item.No)
	}
}

func TestFormatQuoteNumber(t *testing.T) {
	cases := []struct {
		cusID uint8
		seqID uint16
		want  string
	}{
		{2, 1, "15020001"},
		{3, 5, "15030005"},
		{6, 42, "15060042"},
		{2, 0, "15020000"},
		{6, 1, "15060001"},
		{2, 9999, "15029999"},
		{6, 9999, "15069999"},
		{5, 100, "15050100"},
	}

	for _, tc := range cases {
		got := formatQuoteNumber(tc.cusID, tc.seqID)
		if got != tc.want {
			t.Errorf("formatQuoteNumber(%d, %d) = %q; want %q", tc.cusID, tc.seqID, got, tc.want)
		}
	}
}

func TestPrepareValidation(t *testing.T) {
	validItem := QuotationItemInput{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 1000}

	cases := []struct {
		name  string
		input QuotationInput
	}{
		{
			name:  "missing customer ID",
			input: QuotationInput{},
		},
		{
			name:  "invalid customer ID",
			input: QuotationInput{CustomerID: 255},
		},
		{
			name:  "empty items",
			input: QuotationInput{CustomerID: 2},
		},
		{
			name: "empty item description",
			input: QuotationInput{
				CustomerID: 2,
				Items:      []QuotationItemInput{{Description: "", Qty: 1, UoM: "Lot", UnitPrice: 1000}},
			},
		},
		{
			name: "zero item quantity",
			input: QuotationInput{
				CustomerID: 2,
				Items:      []QuotationItemInput{{Description: "Item", Qty: 0, UoM: "Lot", UnitPrice: 1000}},
			},
		},
		{
			name: "missing item UoM",
			input: QuotationInput{
				CustomerID: 2,
				Items:      []QuotationItemInput{{Description: "Item", Qty: 1, UoM: "", UnitPrice: 1000}},
			},
		},
		{
			name: "empty subject",
			input: QuotationInput{
				CustomerID:    2,
				Items:         []QuotationItemInput{validItem},
				Subject:       "",
				RecipientName: "Test",
				Validity:      30,
				PaymentTerm:   30,
				LeadTime:      30,
			},
		},
		{
			name: "empty recipient name",
			input: QuotationInput{
				CustomerID:    2,
				Items:         []QuotationItemInput{validItem},
				Subject:       "Test",
				RecipientName: "",
				Validity:      30,
				PaymentTerm:   30,
				LeadTime:      30,
			},
		},
		{
			name: "zero validity",
			input: QuotationInput{
				CustomerID:    2,
				Items:         []QuotationItemInput{validItem},
				Subject:       "Test",
				RecipientName: "Test",
				Validity:      0,
				PaymentTerm:   30,
				LeadTime:      30,
			},
		},
		{
			name: "zero payment term",
			input: QuotationInput{
				CustomerID:    2,
				Items:         []QuotationItemInput{validItem},
				Subject:       "Test",
				RecipientName: "Test",
				Validity:      30,
				PaymentTerm:   0,
				LeadTime:      30,
			},
		},
		{
			name: "zero lead time",
			input: QuotationInput{
				CustomerID:    2,
				Items:         []QuotationItemInput{validItem},
				Subject:       "Test",
				RecipientName: "Test",
				Validity:      30,
				PaymentTerm:   30,
				LeadTime:      0,
			},
		},
		{
			name: "discount exceeds 100",
			input: QuotationInput{
				CustomerID:      2,
				Items:           []QuotationItemInput{validItem},
				Subject:         "Test",
				RecipientName:   "Test",
				Validity:        30,
				PaymentTerm:     30,
				LeadTime:        30,
				DiscountPercent: 101,
			},
		},
		{
			name: "margin exceeds 100",
			input: QuotationInput{
				CustomerID:    2,
				Items:         []QuotationItemInput{validItem},
				Subject:       "Test",
				RecipientName: "Test",
				Validity:      30,
				PaymentTerm:   30,
				LeadTime:      30,
				ProfitMargin:  101,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &Quotation{}
			err := q.Prepare(tc.input)
			if err == nil {
				t.Error("expected error but got nil")
			}
			if q.Data.CustomerID != 0 {
				t.Errorf("q.Data.CustomerID = %d; want 0 (should not be set after failure)", q.Data.CustomerID)
			}
		})
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
