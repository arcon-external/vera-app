// Package labels owns the customer-visible wording of a quotation document.
//
// Until issue #339 every one of these strings was baked into two places at
// once: the HTML template the browser renders, and the native renderer's Go
// source. The two agreed because a test compared them, and a change to the
// wording had to be made twice, in two languages, with nothing but that test
// keeping them equal. This package is the one declaration both read: the
// template interpolates the effective values and the native renderer prints
// them from the same struct, so a wording change is one edit that reaches
// both renderers by construction.
//
// The defaults are not decoration -- each one is the exact string the template
// printed before this package existed, and labels_test.go pins them against
// those literals. A default that drifts changes what every existing quotation
// prints, so the pin is the contract.
//
// Nothing here reaches the stylesheet. The CSS is built from the brand colours
// alone; a label is plain text validated at the store boundary, never markup,
// so no user text enters a CSS context.
package labels

import (
	"fmt"
	"strconv"
	"strings"
)

// LabelKey identifies one editable customer-visible string. The key is the
// stable name stored in company_labels and served on the wire; the Go field on
// DocumentLabels is what the renderers read.
type LabelKey string

// The editable set, in the order the editor shows it.
const (
	LabelDocTitle          LabelKey = "docTitle"
	LabelRecipientHeading  LabelKey = "recipientHeading"
	LabelTermsHeading      LabelKey = "termsHeading"
	LabelMetaQuotationNo   LabelKey = "metaQuotationNo"
	LabelMetaIssueDate     LabelKey = "metaIssueDate"
	LabelMetaValidUntil    LabelKey = "metaValidUntil"
	LabelMetaRevision      LabelKey = "metaRevision"
	LabelSubject           LabelKey = "subject"
	LabelColumnNo          LabelKey = "columnNo"
	LabelColumnDescription LabelKey = "columnDescription"
	LabelColumnQty         LabelKey = "columnQty"
	LabelColumnUoM         LabelKey = "columnUoM"
	LabelColumnUnitPrice   LabelKey = "columnUnitPrice"
	LabelColumnAfterDisc   LabelKey = "columnAfterDisc"
	LabelColumnTotal       LabelKey = "columnTotal"
	LabelTotalSubTotal     LabelKey = "totalSubTotal"
	LabelTotalDiscount     LabelKey = "totalDiscount"
	LabelTotalPpn          LabelKey = "totalPpn"
	LabelTotalGrandTotal   LabelKey = "totalGrandTotal"
	LabelClosing           LabelKey = "closing"
	LabelVerifyScan        LabelKey = "verifyScan"
	LabelVerifyCode        LabelKey = "verifyCode"
	LabelTermsPayment      LabelKey = "termsPayment"
	LabelTermsValidity     LabelKey = "termsValidity"
	LabelTermsLeadTime     LabelKey = "termsLeadTime"
	LabelTermsIncoterm     LabelKey = "termsIncoterm"
)

// LabelDefault is one key's declaration: the built-in default, the cap an
// override is held to, and whether the value is a terms sentence (which may
// carry the fixed placeholder tokens and is checked against them).
//
// MaxLen is an edit-time ceiling, not a promise that the widest value fits its
// box: the subject strip's label track is 110 CSS px and the item table's
// header cells are narrow, so an override near the cap wraps where a shorter
// one did not. Both renderers wrap the same way, so the document stays one
// document; the cap only stops a paste of a paragraph.
type LabelDefault struct {
	Key     LabelKey
	Default string
	MaxLen  int
	Terms   bool
}

// declarations is the single list every reader derives from: the key set, the
// order the editor shows, the defaults, and the caps. A key added here and
// nowhere else is editable end to end; a key added anywhere else first is a
// key nothing can store.
var declarations = []LabelDefault{
	{Key: LabelDocTitle, Default: "QUOTATION", MaxLen: 24},
	{Key: LabelRecipientHeading, Default: "Quotation to", MaxLen: 60},
	{Key: LabelTermsHeading, Default: "Terms and Conditions", MaxLen: 60},
	{Key: LabelMetaQuotationNo, Default: "Quotation No.", MaxLen: 20},
	{Key: LabelMetaIssueDate, Default: "Issue Date", MaxLen: 20},
	{Key: LabelMetaValidUntil, Default: "Valid Until", MaxLen: 20},
	{Key: LabelMetaRevision, Default: "Revision", MaxLen: 20},
	{Key: LabelSubject, Default: "Subject", MaxLen: 30},
	{Key: LabelColumnNo, Default: "No.", MaxLen: 20},
	{Key: LabelColumnDescription, Default: "Description", MaxLen: 20},
	{Key: LabelColumnQty, Default: "Qty.", MaxLen: 20},
	{Key: LabelColumnUoM, Default: "UoM", MaxLen: 20},
	{Key: LabelColumnUnitPrice, Default: "Unit Price", MaxLen: 20},
	{Key: LabelColumnAfterDisc, Default: "After Disc.", MaxLen: 20},
	{Key: LabelColumnTotal, Default: "Total", MaxLen: 20},
	{Key: LabelTotalSubTotal, Default: "Sub Total", MaxLen: 60},
	{Key: LabelTotalDiscount, Default: "Discount", MaxLen: 60},
	{Key: LabelTotalPpn, Default: "PPN 11%", MaxLen: 60},
	{Key: LabelTotalGrandTotal, Default: "Grand Total", MaxLen: 60},
	{Key: LabelClosing, Default: "We will be happy to supply information that you may need and trust " +
		"that you call on us to fill your order, which will receive our prompt " +
		"and careful attention.", MaxLen: 300},
	{Key: LabelVerifyScan, Default: "Scan to verify this quotation", MaxLen: 60},
	{Key: LabelVerifyCode, Default: "or enter code", MaxLen: 60},
	{Key: LabelTermsPayment, Default: "1. Payment terms agreed for this quotation is {payment} days.",
		MaxLen: 300, Terms: true},
	{Key: LabelTermsValidity, Default: "2. This quotation is valid for {validity} days.",
		MaxLen: 300, Terms: true},
	{Key: LabelTermsLeadTime, Default: "3. Lead time is {leadtime}.",
		MaxLen: 300, Terms: true},
	{Key: LabelTermsIncoterm, Default: "4. Incoterms: {incoterm}",
		MaxLen: 300, Terms: true},
}

// LabelDefaults returns the whole declaration in editor order. The slice is a
// copy: a caller that sorted or appended to it would otherwise reorder the
// editor for everyone.
func LabelDefaults() []LabelDefault {
	out := make([]LabelDefault, len(declarations))
	copy(out, declarations)
	return out
}

// Declaration returns one key's declaration. ok is false for a key the
// document does not carry, which is how a stored override for a retired label
// is recognised rather than guessed at.
func Declaration(key LabelKey) (LabelDefault, bool) {
	for _, def := range declarations {
		if def.Key == key {
			return def, true
		}
	}
	return LabelDefault{}, false
}

// Known reports whether key names an editable label.
func Known(key LabelKey) bool {
	_, ok := Declaration(key)
	return ok
}

// DocumentLabels is the effective wording of one document: every key resolved
// to what prints, an override or the built-in default.
//
// It is a plain value type so it can sit on the render seam beside Theme: both
// renderers read the same struct, and neither can reach a store, a cache or
// each other to get its own copy of a string.
type DocumentLabels struct {
	DocTitle          string
	RecipientHeading  string
	TermsHeading      string
	MetaQuotationNo   string
	MetaIssueDate     string
	MetaValidUntil    string
	MetaRevision      string
	Subject           string
	ColumnNo          string
	ColumnDescription string
	ColumnQty         string
	ColumnUoM         string
	ColumnUnitPrice   string
	ColumnAfterDisc   string
	ColumnTotal       string
	TotalSubTotal     string
	TotalDiscount     string
	TotalPpn          string
	TotalGrandTotal   string
	Closing           string
	VerifyScan        string
	VerifyCode        string
	TermsPayment      string
	TermsValidity     string
	TermsLeadTime     string
	TermsIncoterm     string
}

// DefaultLabels returns the built-in set: what the document printed before any
// label was editable. labels_test.go pins it against the template and the
// native renderer's own pre-#339 literals.
func DefaultLabels() DocumentLabels {
	d := DocumentLabels{}
	for _, def := range declarations {
		d.Set(def.Key, def.Default)
	}
	return d
}

// Get returns the effective value of one key, "" for a key the document does
// not carry.
func (d DocumentLabels) Get(key LabelKey) string {
	switch key {
	case LabelDocTitle:
		return d.DocTitle
	case LabelRecipientHeading:
		return d.RecipientHeading
	case LabelTermsHeading:
		return d.TermsHeading
	case LabelMetaQuotationNo:
		return d.MetaQuotationNo
	case LabelMetaIssueDate:
		return d.MetaIssueDate
	case LabelMetaValidUntil:
		return d.MetaValidUntil
	case LabelMetaRevision:
		return d.MetaRevision
	case LabelSubject:
		return d.Subject
	case LabelColumnNo:
		return d.ColumnNo
	case LabelColumnDescription:
		return d.ColumnDescription
	case LabelColumnQty:
		return d.ColumnQty
	case LabelColumnUoM:
		return d.ColumnUoM
	case LabelColumnUnitPrice:
		return d.ColumnUnitPrice
	case LabelColumnAfterDisc:
		return d.ColumnAfterDisc
	case LabelColumnTotal:
		return d.ColumnTotal
	case LabelTotalSubTotal:
		return d.TotalSubTotal
	case LabelTotalDiscount:
		return d.TotalDiscount
	case LabelTotalPpn:
		return d.TotalPpn
	case LabelTotalGrandTotal:
		return d.TotalGrandTotal
	case LabelClosing:
		return d.Closing
	case LabelVerifyScan:
		return d.VerifyScan
	case LabelVerifyCode:
		return d.VerifyCode
	case LabelTermsPayment:
		return d.TermsPayment
	case LabelTermsValidity:
		return d.TermsValidity
	case LabelTermsLeadTime:
		return d.TermsLeadTime
	case LabelTermsIncoterm:
		return d.TermsIncoterm
	}
	return ""
}

// Set writes one key's value. It reports false for a key the document does not
// carry, so a caller cannot invent a field by naming one.
func (d *DocumentLabels) Set(key LabelKey, value string) bool {
	switch key {
	case LabelDocTitle:
		d.DocTitle = value
	case LabelRecipientHeading:
		d.RecipientHeading = value
	case LabelTermsHeading:
		d.TermsHeading = value
	case LabelMetaQuotationNo:
		d.MetaQuotationNo = value
	case LabelMetaIssueDate:
		d.MetaIssueDate = value
	case LabelMetaValidUntil:
		d.MetaValidUntil = value
	case LabelMetaRevision:
		d.MetaRevision = value
	case LabelSubject:
		d.Subject = value
	case LabelColumnNo:
		d.ColumnNo = value
	case LabelColumnDescription:
		d.ColumnDescription = value
	case LabelColumnQty:
		d.ColumnQty = value
	case LabelColumnUoM:
		d.ColumnUoM = value
	case LabelColumnUnitPrice:
		d.ColumnUnitPrice = value
	case LabelColumnAfterDisc:
		d.ColumnAfterDisc = value
	case LabelColumnTotal:
		d.ColumnTotal = value
	case LabelTotalSubTotal:
		d.TotalSubTotal = value
	case LabelTotalDiscount:
		d.TotalDiscount = value
	case LabelTotalPpn:
		d.TotalPpn = value
	case LabelTotalGrandTotal:
		d.TotalGrandTotal = value
	case LabelClosing:
		d.Closing = value
	case LabelVerifyScan:
		d.VerifyScan = value
	case LabelVerifyCode:
		d.VerifyCode = value
	case LabelTermsPayment:
		d.TermsPayment = value
	case LabelTermsValidity:
		d.TermsValidity = value
	case LabelTermsLeadTime:
		d.TermsLeadTime = value
	case LabelTermsIncoterm:
		d.TermsIncoterm = value
	default:
		return false
	}
	return true
}

// WithDefaults returns d with every blank field replaced by its built-in
// default. Blank means empty or whitespace-only, which is what a cleared
// override looks like on the wire and in a cache: the rule makes an absent, a
// cleared and a half-filled value all resolve the same way, so no path can
// print a label nobody chose.
func (d DocumentLabels) WithDefaults() DocumentLabels {
	out := d
	for _, def := range declarations {
		if strings.TrimSpace(out.Get(def.Key)) == "" {
			out.Set(def.Key, def.Default)
		}
	}
	return out
}

// ResolveOverrides builds the effective set from stored overrides.
//
// A blank or unknown key resolves to the built-in default rather than being
// stored or printed: the table holds overrides only, so a row that has been
// cleared, or that names a label a later release retired, must not be able to
// blank or move anything on the document.
func ResolveOverrides(overrides map[string]string) DocumentLabels {
	d := DefaultLabels()
	for key, value := range overrides {
		k := LabelKey(key)
		value = strings.TrimSpace(value)
		if value == "" || !Known(k) {
			continue
		}
		d.Set(k, value)
	}
	return d
}

// TermsValues are the four figures the terms sentences interpolate. They are
// the same values the template's {{.PaymentTerm}} and friends carried: the
// placeholder tokens exist so a company can reword the sentence without the
// figure moving or disappearing.
type TermsValues struct {
	PaymentTerm uint8
	Validity    uint8
	LeadTime    string
	Incoterm    string
}

// Terms returns the four terms sentences with the fixed placeholder tokens
// substituted.
//
// Substitution is a string replacement over a closed token set, never template
// execution: a sentence is data, and executing it would make the wording a
// program. strings.NewReplacer scans left to right and does not revisit what it
// has written, so a lead time or incoterm carrying a token of its own cannot
// inject one.
//
// The result travels on the render seam beside DisplayLeadTime for the same
// reason that one does: both renderers print the same four strings, so neither
// can reword them on its own.
func (d DocumentLabels) Terms(v TermsValues) []string {
	sub := strings.NewReplacer(
		"{payment}", strconv.Itoa(int(v.PaymentTerm)),
		"{validity}", strconv.Itoa(int(v.Validity)),
		"{leadtime}", v.LeadTime,
		"{incoterm}", v.Incoterm,
	)
	return []string{
		sub.Replace(d.TermsPayment),
		sub.Replace(d.TermsValidity),
		sub.Replace(d.TermsLeadTime),
		sub.Replace(d.TermsIncoterm),
	}
}

// CompanyLabel is one override as it travels: the key that names the label and
// the value the company wants printed instead of the built-in default. An
// empty value means "clear the override", which is why the wire carries it
// rather than a delete request per label.
type CompanyLabel struct {
	Key   string
	Value string
}

// CheckOverrides validates a whole replacement set at the store boundary:
// every key known, no key twice, every value plain text (and every placeholder
// a known token, for the terms sentences). The error names the field, the way
// every other store-boundary validation does, so a refused save says which
// label to fix.
func CheckOverrides(rows []CompanyLabel) error {
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		key := LabelKey(strings.TrimSpace(row.Key))
		def, ok := Declaration(key)
		if !ok {
			return fmt.Errorf("label %q is not an editable quotation label", row.Key)
		}
		if seen[string(key)] {
			return fmt.Errorf("label %q appears twice", row.Key)
		}
		seen[string(key)] = true
		if err := ValidateOverride(def, row.Value); err != nil {
			return err
		}
	}
	return nil
}

// ValidateOverride checks one value against the plain-text rules. The rules
// are what keeps a label from becoming markup:
//
//   - a length cap, so a paste of a paragraph is refused instead of printed;
//   - no "<" or ">", which is what a tag looks like -- the browser escapes
//     them anyway, but a quotation label has no legitimate use for either and
//     refusing them means the native renderer never has to consider them;
//   - no "{{", which is what a template action looks like: the labels are
//     interpolated as data, so an action in a value would only ever print
//     literally, and refusing it keeps the question from being asked;
//   - no control characters, which keeps every label one line -- a newline
//     would wrap in the browser and be a missing glyph in the native
//     renderer, the exact class of divergence this package exists to end.
//
// The terms sentences allow exactly the four fixed placeholder tokens; anything
// else in braces is refused, so a typo'd token is a message rather than a line
// that silently drops its figure.
func ValidateOverride(def LabelDefault, value string) error {
	if len(value) > def.MaxLen {
		return fmt.Errorf("label %q is over %d characters", def.Key, def.MaxLen)
	}
	if strings.ContainsAny(value, "<>") {
		return fmt.Errorf("label %q may not contain \"<\" or \">\": it is printed as plain text", def.Key)
	}
	if strings.Contains(value, "{{") {
		return fmt.Errorf("label %q may not contain \"{{\": it is printed as plain text", def.Key)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("label %q may not contain control characters: it prints on one line", def.Key)
		}
	}
	if def.Terms {
		if err := checkPlaceholders(def.Key, value); err != nil {
			return err
		}
	}
	return nil
}

// placeholderTokens is the closed set a terms sentence may carry.
var placeholderTokens = []string{"{payment}", "{validity}", "{leadtime}", "{incoterm}"}

// checkPlaceholders refuses anything in braces that is not one of the four
// fixed tokens, and any brace that is not part of a token at all. An unbalanced
// brace is refused too: a sentence that opens a placeholder and never closes it
// would print the brace literally, which is a typo rather than a wording.
func checkPlaceholders(key LabelKey, value string) error {
	var kept strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '{' {
			kept.WriteByte(value[i])
			i++
			continue
		}
		end := strings.IndexByte(value[i:], '}')
		if end < 0 {
			return fmt.Errorf("label %q has an unclosed placeholder: use one of %s",
				key, strings.Join(placeholderTokens, ", "))
		}
		token := value[i : i+end+1]
		if !isPlaceholderToken(token) {
			return fmt.Errorf("label %q uses the unknown placeholder %s: use one of %s",
				key, token, strings.Join(placeholderTokens, ", "))
		}
		i += end + 1
	}
	if strings.ContainsAny(kept.String(), "{}") {
		return fmt.Errorf("label %q has a stray \"{\" or \"}\": use one of %s",
			key, strings.Join(placeholderTokens, ", "))
	}
	return nil
}

// isPlaceholderToken reports whether token is one of the four fixed tokens.
func isPlaceholderToken(token string) bool {
	for _, allowed := range placeholderTokens {
		if token == allowed {
			return true
		}
	}
	return false
}
