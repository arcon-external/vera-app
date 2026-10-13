package backend

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// validLeadTimeItem is one plain line, so these tests carry a quotable
// document without reaching into another test's fixtures.
func validLeadTimeItem() QuotationItemInput {
	return QuotationItemInput{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 1000}
}

// TestFormatLeadTimePinsTheIssueExamples is the composition contract, stated
// with the exact strings the issue asks for: segments in order, zeros skipped,
// ", " between and ", and " before the last, singular/plural per count, and a
// dash when nothing is stated. Anything that prints the lead time reads this
// one function's output, so a divergence here is a divergence on the customer's
// document.
func TestFormatLeadTimePinsTheIssueExamples(t *testing.T) {
	for name, tc := range map[string]struct {
		years, months, weeks, days uint8
		want                       string
	}{
		"all four units": {2, 10, 4, 5, "2 Years, 10 Months, 4 Weeks, and 5 Days"},
		"months only":    {0, 5, 0, 0, "5 Months"},
		"nothing stated": {0, 0, 0, 0, "-"},
		"days only":      {0, 0, 0, 60, "60 Days"},
		"one day":        {0, 0, 0, 1, "1 Day"},
		"one of each":    {1, 1, 1, 1, "1 Year, 1 Month, 1 Week, and 1 Day"},
		"two units":      {0, 3, 0, 7, "3 Months and 7 Days"},
		"two long units": {1, 0, 4, 0, "1 Year and 4 Weeks"},
		"leading zeros":  {0, 0, 2, 9, "2 Weeks and 9 Days"},
		"upper bound":    {255, 255, 255, 255, "255 Years, 255 Months, 255 Weeks, and 255 Days"},
	} {
		t.Run(name, func(t *testing.T) {
			got := FormatLeadTime(tc.years, tc.months, tc.weeks, tc.days)
			if got != tc.want {
				t.Errorf("FormatLeadTime(%d, %d, %d, %d) = %q; want %q",
					tc.years, tc.months, tc.weeks, tc.days, got, tc.want)
			}
		})
	}
}

// TestPrepareComposesTheDisplayLeadTime proves the app layer composes the
// sentence once and both readers of it agree: QuotationData.LeadTime is what
// the template's {{.LeadTime}} prints, and the preview DTO carries the same
// string beside the four figures as entered.
func TestPrepareComposesTheDisplayLeadTime(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if err := q.Prepare(QuotationInput{
		CustomerID:     2,
		Items:          []QuotationItemInput{validLeadTimeItem()},
		Subject:        "Test",
		RecipientName:  "Test",
		Validity:       30,
		PaymentTerm:    30,
		Incoterm:       "EXW",
		LeadTimeYears:  2,
		LeadTimeMonths: 10,
		LeadTimeWeeks:  4,
		LeadTimeDays:   5,
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if want := "2 Years, 10 Months, 4 Weeks, and 5 Days"; q.Data.LeadTime != want {
		t.Errorf("QuotationData.LeadTime = %q; want %q", q.Data.LeadTime, want)
	}

	preview, err := q.GetCustomerPreview()
	if err != nil {
		t.Fatalf("GetCustomerPreview: %v", err)
	}
	if preview.DisplayLeadTime != q.Data.LeadTime {
		t.Errorf("preview lead time = %q; want %q", preview.DisplayLeadTime, q.Data.LeadTime)
	}
	if preview.LeadTimeYears != 2 || preview.LeadTimeMonths != 10 ||
		preview.LeadTimeWeeks != 4 || preview.LeadTimeDays != 5 {
		t.Errorf("preview lead time units = %d/%d/%d/%d; want 2/10/4/5",
			preview.LeadTimeYears, preview.LeadTimeMonths, preview.LeadTimeWeeks, preview.LeadTimeDays)
	}

	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("GetQuotationPreviewHTML: %v", err)
	}
	if want := "<p>3. Lead time is 2 Years, 10 Months, 4 Weeks, and 5 Days.</p>"; !strings.Contains(page, want) {
		t.Errorf("the document's terms line is missing %q:\n%s", want, page)
	}
}

// TestPrepareAcceptsNoLeadTimeAtAll is the other end of the same rule: a
// quotation may state no lead time, which used to be refused ("lead time must
// be greater than zero"). Zero in every unit is not a missing field, and the
// document says so with a dash rather than a commitment to deliver instantly.
func TestPrepareAcceptsNoLeadTimeAtAll(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if err := q.Prepare(QuotationInput{
		CustomerID:    2,
		Items:         []QuotationItemInput{validLeadTimeItem()},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      30,
		PaymentTerm:   30,
		Incoterm:      "EXW",
	}); err != nil {
		t.Fatalf("a quotation with no lead time was refused: %v", err)
	}
	if q.Data.LeadTime != "-" {
		t.Errorf("QuotationData.LeadTime = %q; want %q", q.Data.LeadTime, "-")
	}
	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("GetQuotationPreviewHTML: %v", err)
	}
	if want := "<p>3. Lead time is -.</p>"; !strings.Contains(page, want) {
		t.Errorf("the terms line is missing %q:\n%s", want, page)
	}
}

// TestCanonicalKeepsADayOnlyLeadTimeByteIdentical is the #407 compatibility
// guarantee, in the same shape #274's and #255's are: a revision whose lead
// time is stated in days only -- which is every revision signed before years,
// months and weeks existed -- must produce exactly the bytes it always did.
//
// The golden below is copied verbatim from TestCanonicalFormatIsPinned, which
// predates this change; if these bytes ever differ, do NOT bump
// CanonicalPrefix to "fix" it -- that stops every code already printed from
// verifying. Find what started emitting a line instead.
func TestCanonicalKeepsADayOnlyLeadTimeByteIdentical(t *testing.T) {
	const golden = `VERA1:QUOTE
number: 15020005
revision: 00
recipient_name: Jesicha
recipient_company: PT. Infineon Technologies Batam
recipient_address: Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam
subject: Supply and install scrubber control system
incoterm: EXW
validity_days: 30
payment_term_days: 90
lead_time_days: 60
discount_percent: 5
issuer: PT Arcon Penta Persada
currency: IDR
item_count: 2
item_1: Scrubber control panel|2|Lot|120000|240000
item_2: Commissioning|1|Lot|50000|50000
subtotal: 290000
discount_amount: 14500
ppn_amount: 30305
grand_total: 305805`

	revision := testSignable()
	// The three longer units are the only thing this change added; a stored
	// revision predating them reads back as zero, exactly as a fresh one with
	// no such units does.
	revision.LeadTimeYears = 0
	revision.LeadTimeMonths = 0
	revision.LeadTimeWeeks = 0
	revision.LeadTimeDays = 60

	if got := string(CanonicalRevision(revision)); got != golden {
		t.Errorf("a day-only lead time changed the canonical bytes:\n got %q\nwant %q", got, golden)
	}
}

// TestOldRevisionWithoutStoredUnitsStillVerifies is the other half: a revision
// stored before the columns existed, whose evidence therefore carries only
// lead_time_days, must still verify against a signature made over the old
// bytes. This is the check that turns "the day line is unconditional" from a
// claim into a proof.
func TestOldRevisionWithoutStoredUnitsStillVerifies(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := key.Public().(ed25519.PublicKey)

	// Signed before the three columns existed: only a day figure exists.
	stored := testSignable()
	stored.LeadTimeDays = 14
	signature := SignCanonical(key, CanonicalRevision(stored))

	// Read back from evidence the way GetRevisionEvidenceByShortID returns a
	// pre-025 row: the new columns default to 0.
	reloaded := stored
	reloaded.LeadTimeDays = 14
	reloaded.LeadTimeYears = 0
	reloaded.LeadTimeMonths = 0
	reloaded.LeadTimeWeeks = 0

	if !VerifyCanonical(publicKey, CanonicalRevision(reloaded), signature) {
		t.Fatalf("a revision with no stored units stopped verifying:\n%s", CanonicalRevision(reloaded))
	}

	// And a mutation of the day figure still breaks it, which is what keeps
	// the field signed at all.
	tampered := reloaded
	tampered.LeadTimeDays = 60
	if VerifyCanonical(publicKey, CanonicalRevision(tampered), signature) {
		t.Error("editing the stored lead time still verified: lead_time_days is not covered")
	}
}

// TestCanonicalCoversTheLongerLeadTimeUnits is the positive half: when a
// revision does state its lead time in years, months or weeks, those figures
// print on the customer's document, so they are signed content -- and a
// revision that states none of them emits nothing, exactly like a row with no
// discount.
func TestCanonicalCoversTheLongerLeadTimeUnits(t *testing.T) {
	revision := testSignable()
	revision.LeadTimeDays = 0
	revision.LeadTimeYears = 2
	revision.LeadTimeMonths = 10
	revision.LeadTimeWeeks = 4
	got := string(CanonicalRevision(revision))
	for _, want := range []string{
		"\nlead_time_years: 2\n",
		"\nlead_time_months: 10\n",
		"\nlead_time_weeks: 4\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("canonical bytes are missing %q:\n%s", want, got)
		}
	}

	// A stored revision that has none of them emits none of them, so its
	// bytes are what they always were.
	bare := testSignable()
	bare.LeadTimeYears = 0
	bare.LeadTimeMonths = 0
	bare.LeadTimeWeeks = 0
	for _, unwanted := range []string{"lead_time_years", "lead_time_months", "lead_time_weeks"} {
		if strings.Contains(string(CanonicalRevision(bare)), unwanted) {
			t.Errorf("a revision with no longer units emitted %s:\n%s", unwanted, CanonicalRevision(bare))
		}
	}

	// Each unit's figure is covered: dropping one changes the signed bytes.
	for name, mutate := range map[string]func(r *SignableRevision){
		"years":  func(r *SignableRevision) { r.LeadTimeYears = 0 },
		"months": func(r *SignableRevision) { r.LeadTimeMonths = 0 },
		"weeks":  func(r *SignableRevision) { r.LeadTimeWeeks = 0 },
		"days":   func(r *SignableRevision) { r.LeadTimeDays = 99 },
	} {
		t.Run(name, func(t *testing.T) {
			altered := revision
			mutate(&altered)
			if string(CanonicalRevision(altered)) == got {
				t.Errorf("changing the %s unit left the canonical bytes identical", name)
			}
		})
	}
}

// TestSignableMappersCarryTheLeadTimeUnits pins that both mappers -- the one
// that signs at export and the one that recomputes from stored rows -- carry
// the four units, so a revision signed at export verifies against the row it
// became.
func TestSignableMappersCarryTheLeadTimeUnits(t *testing.T) {
	data := QuotationData{
		QuoteNumber: "1502260099", RevisionID: 0,
		Items:        []QuotationItem{{Description: "Panel", Qty: 1, UoM: "Lot", UnitPrice: 1, Total: 1}},
		LeadTimeDays: 5, LeadTimeWeeks: 4,
	}
	evidence := store.RevisionEvidence{
		QuoteNumber:    "1502260099",
		RevisionNumber: 0,
		Items:          []store.EvidenceItem{{Description: "Panel", Qty: 1, UoM: "Lot", UnitPrice: 1, Total: 1}},
		LeadTimeDays:   5,
		LeadTimeWeeks:  4,
	}

	if got, want := string(CanonicalRevision(SignableFromData(data))),
		string(CanonicalRevision(SignableFromEvidence(evidence))); got != want {
		t.Errorf("mappers disagree over the lead time units:\n--- exported ---\n%s\n--- stored ---\n%s", got, want)
	}
}

// TestRevisionDraftKeepsTheLeadTimeUnitsAsEntered closes the loop from
// prepared data to the stored snapshot: the draft carries the four figures,
// and the display sentence is not what gets stored -- history holds numbers.
func TestRevisionDraftKeepsTheLeadTimeUnitsAsEntered(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	q.Data = QuotationData{
		CustomerID: 2, RecipientCompany: "PT. Test Customer", RecipientAddress: "Test address",
		RevisionID: 1, RecipientName: "Test recipient", Subject: "Test quotation", Incoterm: "EXW",
		Validity: 30, PaymentTerm: 90,
		LeadTimeYears: 1, LeadTimeMonths: 2, LeadTimeWeeks: 3, LeadTimeDays: 4,
		LeadTime: "1 Year, 2 Months, 3 Weeks, and 4 Days",
		Items: []QuotationItem{{
			No: 1, Description: "Test item", Qty: 1, UoM: "Unit",
			OriginalUnitPrice: 100000, OriginalTotal: 100000,
			UnitPrice: 120000, Total: 120000,
		}},
		SubTotal: 120000, GrandTotal: 120000, OriginalSubTotal: 100000,
	}
	q.Data.QuoteNumber = "15020001"

	draft := q.revisionDraft("<html>snapshot</html>", nil, "")
	if draft.LeadTimeYears != 1 || draft.LeadTimeMonths != 2 ||
		draft.LeadTimeWeeks != 3 || draft.LeadTimeDays != 4 {
		t.Errorf("draft lead time = %d/%d/%d/%d; want 1/2/3/4",
			draft.LeadTimeYears, draft.LeadTimeMonths, draft.LeadTimeWeeks, draft.LeadTimeDays)
	}

	// And the same four figures sign the same bytes the revision will.
	signable := SignableFromData(q.Data)
	if signable.LeadTimeYears != 1 || signable.LeadTimeMonths != 2 ||
		signable.LeadTimeWeeks != 3 || signable.LeadTimeDays != 4 {
		t.Errorf("signed lead time = %d/%d/%d/%d; want 1/2/3/4",
			signable.LeadTimeYears, signable.LeadTimeMonths, signable.LeadTimeWeeks, signable.LeadTimeDays)
	}
}

// TestRenderInputCarriesTheComposedSentence pins the seam: the native renderer
// reads the display string the template reads, not the four numbers, so the
// two documents cannot disagree about the lead time (the #377 lesson).
func TestRenderInputCarriesTheComposedSentence(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if err := q.Prepare(QuotationInput{
		CustomerID:     2,
		Items:          []QuotationItemInput{validLeadTimeItem()},
		Subject:        "Test",
		RecipientName:  "Test",
		Validity:       30,
		PaymentTerm:    30,
		Incoterm:       "EXW",
		LeadTimeDays:   5,
		LeadTimeWeeks:  4,
		LeadTimeYears:  2,
		LeadTimeMonths: 10,
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderInput: %v", err)
	}
	const want = "2 Years, 10 Months, 4 Weeks, and 5 Days"
	if in.DisplayLeadTime != want {
		t.Errorf("seam DisplayLeadTime = %q; want %q", in.DisplayLeadTime, want)
	}
	if !strings.Contains(in.HTML, "<p>3. Lead time is "+want+".</p>") {
		t.Errorf("the rendered HTML does not carry the same sentence:\n%s", in.HTML)
	}
}
