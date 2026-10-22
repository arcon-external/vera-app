package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

// Issue #471: comparing an entered Purchase Order with the exact Quotation
// revision it was born from, and deciding what the final save may record.
//
// The comparison is deliberately narrow and customer-facing: descriptions,
// quantities, units, list prices, row discounts, payment terms and Incoterms.
// Supplier cost, margin, PPH22 and profit never appear here (parent story 42).
//
// Matching is by the stored item/sub reference, never by line position. A
// customer may reorder or flatten the printed table, so position says nothing
// about which quotation line a PO line follows. Every line keeps its customer
// line number untouched while its reference is corrected.
//
// A value is "different" only when the commercial value differs: surrounding
// text whitespace is ignored (the parent decision), quantities compare as the
// exact scaled integers, and money compares in minor units. A discount is one
// value (enabled + percent), so 0% disabled and 5% enabled are different while
// 0% disabled and 0% enabled are not.

// The two explicit choices a difference review can record. There is no third
// "unset" choice: a difference without one of these blocks the final save.
const (
	PurchaseOrderChoicePO        = "po"
	PurchaseOrderChoiceQuotation = "quotation"
)

// PurchaseOrderSourceLine is one customer-facing line of the frozen source
// revision. For a package whose parts print with prices (show_sub_prices), the
// parts are the lines, because that is what the customer document shows; every
// other line is itself.
type PurchaseOrderSourceLine struct {
	QuotationItemNo int
	QuotationSubNo  *int
	Description     string
	UoM             string
	QuantityScaled  int64
	UnitPriceMinor  int64
	DiscountEnabled bool
	DiscountPercent int16
}

// Label renders the source line as the customer document numbers it: "3" for
// an item, "3.1" for a priced part.
func (line PurchaseOrderSourceLine) Label() string {
	if line.QuotationSubNo == nil {
		return strconv.Itoa(line.QuotationItemNo)
	}
	return fmt.Sprintf("%d.%d", line.QuotationItemNo, *line.QuotationSubNo)
}

// PurchaseOrderSource is the exact source revision's customer-facing values.
// Printed totals are the document evidence the review shows beside the entered
// order; they are copied from the revision, never recalculated, and never a
// target the saved total must equal.
type PurchaseOrderSource struct {
	QuoteNumber            string
	CustomerName           string
	Currency               string
	PaymentTerms           string
	Incoterms              string
	BillingAddress         string
	TaxZone                string
	TaxCategory            string
	TaxType                string
	ServiceCategory        string
	PrintedSubtotalMinor   int64
	PrintedDiscountMinor   int64
	PrintedPpnMinor        int64
	PrintedGrandTotalMinor int64
	Lines                  []PurchaseOrderSourceLine
	CustomerID             int64
	QuotationID            int64
	RevisionID             int64
	RevisionNumber         int16
	DiscountPercent        int16
}

// PurchaseOrderReviewState is the entered side of the comparison: the header
// wording plus the lines, with their references and values. It is built from
// the values a final save would store, so the review and the saved row cannot
// disagree.
type PurchaseOrderReviewState struct {
	PaymentTerms    string
	Incoterms       string
	DiscountPercent int16
	Lines           []PurchaseOrderItem
}

// PurchaseOrderDifference is one named value the review must decide. Key is
// stable and is what a decision records; Label is what staff read.
type PurchaseOrderDifference struct {
	Key         string
	Label       string
	POValue     string
	SourceValue string
}

// PurchaseOrderComparison is the full review surface. Differences are the
// differing values; Missing and Extra are the lines that do not pair at all;
// DuplicateLineNumbers are PO lines that claim a source line another line
// already claimed. ComparableKeys is every slot a decision may name, including
// values that already agree, so a "Use Quotation" decision for an applied value
// is still recognised as real.
type PurchaseOrderComparison struct {
	Missing              []PurchaseOrderSourceLine
	ExtraLineNumbers     []int
	DuplicateLineNumbers []int
	Differences          []PurchaseOrderDifference
	ComparableKeys       []string
}

type purchaseOrderPair struct {
	sourceIndex int
	line        PurchaseOrderItem
}

// ComparePurchaseOrder matches the entered lines to the source by reference and
// names every difference. It is pure: the same inputs always produce the same
// report, and the store and its tests both read this one implementation.
func ComparePurchaseOrder(source PurchaseOrderSource, entered PurchaseOrderReviewState) PurchaseOrderComparison {
	byRef := make(map[string]int, len(source.Lines))
	for index, line := range source.Lines {
		byRef[line.Label()] = index
	}
	claimedBy := make(map[int]int, len(source.Lines))
	comparison := PurchaseOrderComparison{
		Missing:              make([]PurchaseOrderSourceLine, 0),
		ExtraLineNumbers:     make([]int, 0),
		DuplicateLineNumbers: make([]int, 0),
		Differences:          make([]PurchaseOrderDifference, 0),
		ComparableKeys:       make([]string, 0, len(entered.Lines)*5+2),
	}
	comparison.ComparableKeys = append(comparison.ComparableKeys,
		"header:paymentTerms", "header:incoterms")

	pairs := make([]purchaseOrderPair, 0, len(entered.Lines))
	for _, line := range entered.Lines {
		if line.QuotationItemNo == nil {
			comparison.ExtraLineNumbers = append(comparison.ExtraLineNumbers, line.LineNumber)
			continue
		}
		label := strconv.Itoa(*line.QuotationItemNo)
		if line.QuotationSubNo != nil {
			label = fmt.Sprintf("%s.%d", label, *line.QuotationSubNo)
		}
		index, ok := byRef[label]
		if !ok {
			// An unknown reference is not a match. The store refuses it before
			// a save; here it is also an unpaired line so a reviewer sees it.
			comparison.ExtraLineNumbers = append(comparison.ExtraLineNumbers, line.LineNumber)
			continue
		}
		if firstLine, taken := claimedBy[index]; taken {
			comparison.DuplicateLineNumbers = append(comparison.DuplicateLineNumbers, line.LineNumber)
			_ = firstLine
			continue
		}
		claimedBy[index] = line.LineNumber
		pairs = append(pairs, purchaseOrderPair{sourceIndex: index, line: line})
	}
	for index, line := range source.Lines {
		if _, claimed := claimedBy[index]; !claimed {
			comparison.Missing = append(comparison.Missing, line)
		}
	}

	comparison.addTextDifference("header:paymentTerms", "Payment terms",
		entered.PaymentTerms, source.PaymentTerms)
	comparison.addTextDifference("header:incoterms", "Incoterms",
		entered.Incoterms, source.Incoterms)
	comparison.addValueDifference("header:discountPercent", "Whole-PO discount",
		fmt.Sprintf("%d%%", entered.DiscountPercent), fmt.Sprintf("%d%%", source.DiscountPercent))

	for _, pair := range pairs {
		sourceLine := source.Lines[pair.sourceIndex]
		prefix := fmt.Sprintf("line:%d", pair.line.LineNumber)
		lineName := fmt.Sprintf("Line %d", pair.line.LineNumber)
		comparison.addTextDifference(prefix+":description", lineName+" description",
			pair.line.Description, sourceLine.Description)
		comparison.addTextDifference(prefix+":uom", lineName+" unit of measure",
			pair.line.UoM, sourceLine.UoM)
		comparison.addValueDifference(prefix+":quantity", lineName+" quantity",
			formatReviewQuantity(pair.line.QuantityScaled), formatReviewQuantity(sourceLine.QuantityScaled))
		comparison.addValueDifference(prefix+":unitPrice", lineName+" unit price",
			strconv.FormatInt(pair.line.UnitPriceMinor, 10), strconv.FormatInt(sourceLine.UnitPriceMinor, 10))
		comparison.addValueDifference(prefix+":discount", lineName+" discount",
			formatReviewDiscount(pair.line.DiscountEnabled, pair.line.DiscountPercent),
			formatReviewDiscount(sourceLine.DiscountEnabled, sourceLine.DiscountPercent))
	}

	sort.Ints(comparison.ExtraLineNumbers)
	sort.Ints(comparison.DuplicateLineNumbers)
	return comparison
}

// addTextDifference records a text slot and compares it with surrounding
// whitespace ignored. Internal whitespace is real content: "30 days  after"
// is not "30 days after".
func (comparison *PurchaseOrderComparison) addTextDifference(key, label, entered, source string) {
	comparison.ComparableKeys = append(comparison.ComparableKeys, key)
	if strings.TrimSpace(entered) == strings.TrimSpace(source) {
		return
	}
	comparison.Differences = append(comparison.Differences, PurchaseOrderDifference{
		Key:         key,
		Label:       label,
		POValue:     strings.TrimSpace(entered),
		SourceValue: strings.TrimSpace(source),
	})
}

func (comparison *PurchaseOrderComparison) addValueDifference(key, label, entered, source string) {
	comparison.ComparableKeys = append(comparison.ComparableKeys, key)
	if entered == source {
		return
	}
	comparison.Differences = append(comparison.Differences, PurchaseOrderDifference{
		Key:         key,
		Label:       label,
		POValue:     entered,
		SourceValue: source,
	})
}

// ValidatePurchaseOrderDecisions holds the final-save rule: every difference
// has exactly one recorded choice, a "Use Quotation" choice cannot accompany a
// value that still differs, and every missing or extra line is explicitly
// acknowledged. Missing or extra lines that are not named are refused with the
// line numbers in the message, so the caller can show them.
func ValidatePurchaseOrderDecisions(
	comparison PurchaseOrderComparison,
	decisions []PurchaseOrderDecision,
	acknowledgedMissing []string,
	acknowledgedExtra []int,
) error {
	if len(comparison.DuplicateLineNumbers) > 0 {
		return fmt.Errorf(
			"%w: purchase order lines %s match the same quotation line",
			ErrPurchaseOrderUnresolvedDifference, joinInts(comparison.DuplicateLineNumbers))
	}

	known := make(map[string]bool, len(comparison.ComparableKeys))
	for _, key := range comparison.ComparableKeys {
		known[key] = true
	}
	chosen := make(map[string]string, len(decisions))
	for _, decision := range decisions {
		if !known[decision.Key] {
			return fmt.Errorf("%w: %q is not a review value", ErrPurchaseOrderUnresolvedDifference, decision.Key)
		}
		if _, duplicate := chosen[decision.Key]; duplicate {
			return fmt.Errorf("%w: %q was decided twice", ErrPurchaseOrderUnresolvedDifference, decision.Key)
		}
		if decision.Choice != PurchaseOrderChoicePO && decision.Choice != PurchaseOrderChoiceQuotation {
			return fmt.Errorf("%w: %q has no valid choice", ErrPurchaseOrderUnresolvedDifference, decision.Key)
		}
		chosen[decision.Key] = decision.Choice
	}

	problems := make([]string, 0, len(comparison.Differences))
	for _, difference := range comparison.Differences {
		choice, decided := chosen[difference.Key]
		if !decided {
			problems = append(problems, fmt.Sprintf("%q still differs and has no choice", difference.Key))
			continue
		}
		if choice == PurchaseOrderChoiceQuotation {
			problems = append(problems,
				fmt.Sprintf("%q chose the quotation but the saved value still differs", difference.Key))
		}
	}

	missingWanted := make(map[string]bool, len(comparison.Missing))
	for _, line := range comparison.Missing {
		missingWanted[line.Label()] = true
	}
	missingGiven := make(map[string]bool, len(acknowledgedMissing))
	for _, label := range acknowledgedMissing {
		trimmed := strings.TrimSpace(label)
		if !missingWanted[trimmed] {
			return fmt.Errorf("%w: quotation line %q is not missing", ErrPurchaseOrderUnresolvedDifference, label)
		}
		missingGiven[trimmed] = true
	}
	for _, line := range comparison.Missing {
		if !missingGiven[line.Label()] {
			problems = append(problems,
				fmt.Sprintf("quotation line %s is not on the purchase order and was not acknowledged", line.Label()))
		}
	}

	extraWanted := make(map[int]bool, len(comparison.ExtraLineNumbers))
	for _, lineNumber := range comparison.ExtraLineNumbers {
		extraWanted[lineNumber] = true
	}
	extraGiven := make(map[int]bool, len(acknowledgedExtra))
	for _, lineNumber := range acknowledgedExtra {
		if !extraWanted[lineNumber] {
			return fmt.Errorf("%w: line %d is not an extra line", ErrPurchaseOrderUnresolvedDifference, lineNumber)
		}
		extraGiven[lineNumber] = true
	}
	for _, lineNumber := range comparison.ExtraLineNumbers {
		if !extraGiven[lineNumber] {
			problems = append(problems,
				fmt.Sprintf("line %d has no quotation match and was not acknowledged", lineNumber))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrPurchaseOrderUnresolvedDifference, strings.Join(problems, "; "))
	}
	return nil
}

// formatReviewQuantity renders the exact scaled quantity as the shortest
// decimal with at most four places, the same shape the editor's quantity field
// holds.
func formatReviewQuantity(scaled int64) string {
	if scaled <= 0 {
		return "0"
	}
	whole := scaled / QuantityScale
	fraction := strconv.FormatInt(scaled%QuantityScale, 10)
	for len(fraction) < 4 {
		fraction = "0" + fraction
	}
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "" {
		return strconv.FormatInt(whole, 10)
	}
	return strconv.FormatInt(whole, 10) + "." + fraction
}

// formatReviewDiscount shows a row discount as one value. A disabled discount
// is "None" whatever percentage sits beside it, because the row prints no
// discount then.
func formatReviewDiscount(enabled bool, percent int16) string {
	if !enabled {
		return "None"
	}
	return fmt.Sprintf("%d%%", percent)
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.Itoa(value)
	}
	return strings.Join(parts, ", ")
}

// errInvalidDecision is kept unexported: callers classify with errors.Is on
// ErrPurchaseOrderUnresolvedDifference, never on message text.

// GetPurchaseOrderReview opens one record beside the exact source revision it
// is checked against, inside the caller's organization. The source is read
// fresh from the frozen revision; a record whose stored currency or customer
// disagrees with that revision is refused rather than compared as if the two
// sides were the same money (parent story 37).
func (s *Store) GetPurchaseOrderReview(ctx context.Context, id int64) (PurchaseOrderReview, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	schema := s.configSchema()
	order, err := getPurchaseOrder(ctx, pool, schema, org.ID, id)
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	// #496: a never-linked or formerly-linked draft has no frozen revision to
	// compare against. The server refuses the review by name so the UI can
	// show its Link guidance without a broken source read.
	if order.SourceState != PurchaseOrderSourceStateLive {
		return PurchaseOrderReview{}, ErrPurchaseOrderSourceMissing
	}
	source, err := loadPurchaseOrderSourceDetail(ctx, pool, schema, order.QuotationID, order.SourceRevision)
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	if err := checkPurchaseOrderSourceMatches(order, source); err != nil {
		return PurchaseOrderReview{}, err
	}
	documentState, err := purchaseOrderDocumentState(ctx, pool, schema, order.ID)
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	return PurchaseOrderReview{
		Order:         order,
		Source:        source,
		DocumentState: documentState,
	}, nil
}

// purchaseOrderDocumentState answers which document branch a record is in:
// "attached" when one active customer PO PDF exists, "none" otherwise. The
// final save asks for the no-PDF confirmation only in the "none" branch.
func purchaseOrderDocumentState(ctx context.Context, q purchaseOrderQuerier, schema string, purchaseOrderID int64) (string, error) {
	var attached bool
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %s.purchase_order_documents
			WHERE purchase_order_id = $1 AND active
		)`, schema), purchaseOrderID).Scan(&attached)
	if err != nil {
		return "", fmt.Errorf("read purchase order %d document state: %w", purchaseOrderID, err)
	}
	if attached {
		return PurchaseOrderDocumentAttached, nil
	}
	return PurchaseOrderDocumentNone, nil
}

// checkPurchaseOrderSourceMatches is the one place the currency rule is
// structural: the PO is denominated in the source revision's currency and has
// no editable currency, so a disagreement means the link is broken. Amounts
// are never converted; the comparison simply refuses to happen.
func checkPurchaseOrderSourceMatches(order PurchaseOrder, source PurchaseOrderSource) error {
	if order.Currency != source.Currency {
		return fmt.Errorf("%w: record is %q, revision is %q",
			ErrPurchaseOrderSourceCurrencyMismatch, order.Currency, source.Currency)
	}
	if order.CustomerID != source.CustomerID {
		return fmt.Errorf("%w: customer %d is not the revision's customer %d",
			ErrPurchaseOrderSourceMismatch, order.CustomerID, source.CustomerID)
	}
	return nil
}

// FinalizePurchaseOrder is the one-way draft -> saved transition. It re-reads
// the source, recomputes the calculation from the submitted values, requires
// the difference review to be explicitly decided, and asks for the no-PDF
// confirmation while no attachment exists. Only purchase_orders and
// purchase_order_items are written; the Quotation revision is never touched.
//
// The saved grand total belongs to the purchase order, not to the quotation.
// The explicit per-field choices explain how the order differs from its
// source; missing and extra lines are acknowledged rather than cancelled. The
// source's printed total stays available as document evidence, and the signed
// adjustment is optional: it reconciles what the customer's PO document
// actually printed, and is never required to reverse an accepted difference
// (parent stories 41, 45-48 and 51).
func (s *Store) FinalizePurchaseOrder(ctx context.Context, input PurchaseOrderFinalize) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.ID < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	input.PONumber = strings.TrimSpace(input.PONumber)
	input.PaymentTerms = strings.TrimSpace(input.PaymentTerms)
	input.Incoterms = strings.TrimSpace(input.Incoterms)
	if utf8.RuneCountInString(input.PaymentTerms) > maxPaymentTermsLength {
		return PurchaseOrder{}, fmt.Errorf("%w: payment terms must be at most %d characters",
			ErrPurchaseOrderInvalid, maxPaymentTermsLength)
	}

	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrder{}, err
	}
	schema := s.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("begin final purchase order transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currency, customerName string
	var quotationID, customerID int64
	var sourceRevision, ppnBps int16
	var storedReceived time.Time
	lock, err := lockPurchaseOrderWrite(ctx, tx, schema, org.ID, input.ID,
		input.ExpectedVersion, PurchaseOrderStateDraft)
	if err != nil {
		return PurchaseOrder{}, err
	}
	// #496: a never-linked or formerly-linked draft cannot be finalized. The
	// refusal happens here, at the server boundary, before any confirmation
	// number or saved state is written.
	if err := requirePurchaseOrderSource(lock); err != nil {
		return PurchaseOrder{}, err
	}
	quotationID = lock.QuotationID
	sourceRevision = lock.SourceRevision
	customerID = lock.CustomerID
	customerName = lock.CustomerName
	currency = lock.Currency
	ppnBps = lock.PpnBps
	storedReceived = lock.ReceivedDate

	source, err := loadPurchaseOrderSourceDetail(ctx, tx, schema, quotationID, sourceRevision)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := checkPurchaseOrderSourceMatches(PurchaseOrder{
		CustomerID: customerID,
		Currency:   currency,
	}, source); err != nil {
		return PurchaseOrder{}, err
	}

	// The final-save minimum: source revision (loaded above), a customer PO
	// number, a received date, the customer snapshot, the currency, and at
	// least one valid item. A draft may be missing any of them. The item and PO
	// number rules are shared with a saved-record correction, so the two can
	// never drift.
	received := normalizeDate(input.ReceivedDate)
	if received.IsZero() {
		received = normalizeDate(&storedReceived)
	}
	if received.IsZero() {
		return PurchaseOrder{}, fmt.Errorf("%w: a received date is required", ErrPurchaseOrderIncomplete)
	}
	if customerID < 1 || strings.TrimSpace(customerName) == "" {
		return PurchaseOrder{}, fmt.Errorf("%w: the customer is missing", ErrPurchaseOrderIncomplete)
	}
	if strings.TrimSpace(currency) == "" {
		return PurchaseOrder{}, fmt.Errorf("%w: the currency is missing", ErrPurchaseOrderIncomplete)
	}
	for index := range input.Items {
		input.Items[index].LineNumber = index + 1
	}
	if err := validateSavedPurchaseOrderUpdateInput(input.PurchaseOrderUpdate); err != nil {
		return PurchaseOrder{}, err
	}
	if err := validatePurchaseOrderReferences(ctx, tx, schema, quotationID, sourceRevision, input.Items); err != nil {
		return PurchaseOrder{}, err
	}
	lines, calculation, err := calculatePurchaseOrderLines(
		input.Items, input.DiscountPercent, input.AdjustmentMinor,
		input.AdjustmentReason, ppnBps)
	if err != nil {
		return PurchaseOrder{}, err
	}

	comparison := ComparePurchaseOrder(source, PurchaseOrderReviewState{
		PaymentTerms:    input.PaymentTerms,
		Incoterms:       input.Incoterms,
		DiscountPercent: input.DiscountPercent,
		Lines:           lines,
	})
	if err := ValidatePurchaseOrderDecisions(
		comparison, input.Decisions, input.AcknowledgedMissing, input.AcknowledgedExtra); err != nil {
		return PurchaseOrder{}, err
	}

	documentState, err := purchaseOrderDocumentState(ctx, tx, schema, input.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	// The confirmation is only for a save that genuinely has no PDF. An
	// attached save leaves the absence fields null even if the client sent the
	// checkbox, so a stray flag cannot record a missing document that exists.
	recordAbsence := documentState == PurchaseOrderDocumentNone
	if recordAbsence && !input.DocumentConfirmed {
		return PurchaseOrder{}, ErrPurchaseOrderPDFConfirmationRequired
	}

	// A final save that replaced a draft the caller was warned about keeps the
	// replaced draft, including its attachment reference, before the transition.
	if input.Overwrite {
		stored, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
		if err != nil {
			return PurchaseOrder{}, err
		}
		if _, err := insertPurchaseOrderCorrection(ctx, tx, schema, stored, org.ID,
			PurchaseOrderCorrectionKindDraftOverwrite, PurchaseOrderDraftOverwriteReason,
			input.UpdatedBy); err != nil {
			return PurchaseOrder{}, err
		}
	}

	numberKey := PurchaseOrderNumberKey(input.PONumber)
	tag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET
			state = $3,
			po_number = $4,
			po_number_key = $5,
			billing_address = $6,
			delivery_address = $7,
			ship_to_code = $8,
			contact_email = $9,
			contact_phone = $10,
			po_date = $11,
			received_date = $12,
			requested_delivery_date = $13,
			payment_terms = $14,
			incoterms = $15,
			discount_percent = $16,
			subtotal_minor = $17,
			discount_minor = $18,
			ppn_bps = $19,
			ppn_minor = $20,
			adjustment_minor = $21,
			adjustment_reason = $22,
			grand_total_minor = $23,
			saved_at = NOW(),
			saved_by = $24,
			pdf_absent_confirmed_at = CASE WHEN $25::boolean THEN NOW() ELSE NULL::timestamptz END,
			pdf_absent_confirmed_by = CASE WHEN $25::boolean THEN $26::bigint ELSE NULL::bigint END,
			updated_by = $24,
			updated_at = NOW()
		WHERE id = $1 AND org_id = $2 AND state = 'draft'`, schema),
		input.ID, org.ID, PurchaseOrderStateSaved, input.PONumber, numberKey,
		input.BillingAddress, input.DeliveryAddress, input.ShipToCode,
		input.ContactEmail, input.ContactPhone, dateOrNil(input.PODate), received,
		dateOrNil(input.RequestedDeliveryDate), input.PaymentTerms, input.Incoterms,
		input.DiscountPercent, calculation.SubtotalMinor, calculation.DiscountMinor,
		calculation.PpnBps, calculation.PpnMinor, calculation.AdjustmentMinor,
		calculation.AdjustmentReason, calculation.GrandTotalMinor, input.UpdatedBy,
		recordAbsence, input.UpdatedBy,
	)
	if err != nil {
		if isSavedPurchaseOrderNumberConflict(err) {
			return PurchaseOrder{}, ErrPurchaseOrderNumberTaken
		}
		return PurchaseOrder{}, fmt.Errorf("save purchase order %d: %w", input.ID, err)
	}
	// The row lock makes a zero-row update impossible; checking anyway means a
	// future change to the predicate cannot silently leave a draft draft while
	// its lines are replaced.
	if tag.RowsAffected() != 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotDraft
	}
	if err := replacePurchaseOrderItems(ctx, tx, schema, input.ID, lines); err != nil {
		return PurchaseOrder{}, err
	}

	// #474: when staff ask for a confirmation number, allocate it in this same
	// transaction, after the record is saved. A later refusal or commit failure
	// rolls the counter back with the save, so a failed attempt leaves no gap.
	if input.AssignConfirmationNumber {
		if err := allocatePurchaseOrderConfirmationTx(
			ctx, tx, schema, org.ID, input.ID, input.UpdatedBy, time.Now()); err != nil {
			return PurchaseOrder{}, err
		}
	}

	saved, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, fmt.Errorf("commit purchase order final save: %w", err)
	}
	return saved, nil
}

// PurchaseOrderNumberKey is the stored uniqueness key: the trimmed number,
// lower-cased. The same policy is documented in migration 032.
func PurchaseOrderNumberKey(number string) string {
	return strings.ToLower(strings.TrimSpace(number))
}

func isSavedPurchaseOrderNumberConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == "purchase_orders_saved_number_idx"
}
