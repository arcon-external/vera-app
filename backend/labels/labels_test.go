package labels

import (
	"strings"
	"testing"
)

// The defaults are pinned against the strings the quotation printed before
// this package existed. Each expectation carries the two places that string
// lived -- the HTML template the browser renders and the native renderer's Go
// source -- because those two are what the default must still equal. A default
// that drifts changes what every quotation prints, including the ones already
// archived, so this is the contract and not a convenience.
//
// Provenance, pre-#339:
//
//	docTitle        template-fixed.html:24   draw.go:455
//	recipientHeading template-fixed.html:29  draw.go:492
//	termsHeading    template-fixed.html:196  draw.go:1003
//	metaQuotationNo template-fixed.html:38   layout.go:1005
//	metaIssueDate   template-fixed.html:40   layout.go:1006
//	metaValidUntil  template-fixed.html:42   layout.go:1007
//	metaRevision    template-fixed.html:44   layout.go:1008
//	subject         template-fixed.html:52   draw.go:605
//	columnNo        template-fixed.html:69   layout.go:371
//	columnDescription template-fixed.html:70 layout.go:371
//	columnQty       template-fixed.html:71   layout.go:371
//	columnUoM       template-fixed.html:72   layout.go:371
//	columnUnitPrice template-fixed.html:73   layout.go:371
//	columnAfterDisc template-fixed.html:74   layout.go:371
//	columnTotal     template-fixed.html:75   layout.go:371
//	totalSubTotal   template-fixed.html:206  draw.go:1112
//	totalDiscount   template-fixed.html:214  draw.go:1117
//	totalPpn        template-fixed.html:218  draw.go:1126
//	totalGrandTotal template-fixed.html:222  draw.go:1128
//	closing         template-fixed.html:230  gpdf.go:228 (closingText)
//	verifyScan      template-fixed.html:245  draw.go:1204
//	verifyCode      template-fixed.html:246  draw.go:1207
//	terms*          template-fixed.html:197  gpdf.go:218 (termsLines)
//
// The three price column headers, the discount row and the code line print a
// figure beside the label -- the currency code, the discount percent, the
// short id -- exactly as they did before: the label is the lead-in text and
// the renderer appends the figure. The pin below therefore holds the lead-in,
// and TestTermsSubstituteToTheOldSentences holds the four sentences whole.
var wantDefaults = map[LabelKey]string{
	LabelDocTitle:          "QUOTATION",
	LabelRecipientHeading:  "Quotation to",
	LabelTermsHeading:      "Terms and Conditions",
	LabelMetaQuotationNo:   "Quotation No.",
	LabelMetaIssueDate:     "Issue Date",
	LabelMetaValidUntil:    "Valid Until",
	LabelMetaRevision:      "Revision",
	LabelSubject:           "Subject",
	LabelColumnNo:          "No.",
	LabelColumnDescription: "Description",
	LabelColumnQty:         "Qty.",
	LabelColumnUoM:         "UoM",
	LabelColumnUnitPrice:   "Unit Price",
	LabelColumnAfterDisc:   "After Disc.",
	LabelColumnTotal:       "Total",
	LabelTotalSubTotal:     "Sub Total",
	LabelTotalDiscount:     "Discount",
	LabelTotalPpn:          "PPN 11%",
	LabelTotalGrandTotal:   "Grand Total",
	LabelClosing: "We will be happy to supply information that you may need and trust " +
		"that you call on us to fill your order, which will receive our prompt " +
		"and careful attention.",
	LabelVerifyScan:    "Scan to verify this quotation",
	LabelVerifyCode:    "or enter code",
	LabelTermsPayment:  "1. Payment terms agreed for this quotation is {payment} days.",
	LabelTermsValidity: "2. This quotation is valid for {validity} days.",
	LabelTermsLeadTime: "3. Lead time is {leadtime}.",
	LabelTermsIncoterm: "4. Incoterms: {incoterm}",
}

// TestDefaultLabelsEqualWhatTheTemplatePrinted is the pin: every key's
// built-in default is the string the document carried before labels were
// editable, and the set is exactly the keys declared -- no more, no fewer.
func TestDefaultLabelsEqualWhatTheTemplatePrinted(t *testing.T) {
	d := DefaultLabels()

	if got := len(LabelDefaults()); got != len(wantDefaults) {
		t.Errorf("the label set declares %d keys; this pin expects %d. "+
			"A key was added or removed without updating the pin.", got, len(wantDefaults))
	}

	for _, def := range LabelDefaults() {
		want, ok := wantDefaults[def.Key]
		if !ok {
			t.Errorf("label %q has no pinned default; every key must be pinned "+
				"against what the template printed before #339", def.Key)
			continue
		}
		if got := d.Get(def.Key); got != want {
			t.Errorf("default for %q = %q; want %q (what the template printed)", def.Key, got, want)
		}
		if len(def.Default) > def.MaxLen {
			t.Errorf("label %q default is %d characters but the cap is %d",
				def.Key, len(def.Default), def.MaxLen)
		}
	}

	// The other direction: a pinned key that no longer exists in the set is a
	// pin that has stopped pinning anything.
	for key := range wantDefaults {
		if !Known(key) {
			t.Errorf("the pin carries %q, which the label set no longer declares", key)
		}
	}
}

// TestDefaultLabelsFillEveryField proves DefaultLabels is the whole document
// and not most of it: a field the declaration forgot would print blank, which
// is the bug the "cleared label prints the default" rule exists to prevent.
func TestDefaultLabelsFillEveryField(t *testing.T) {
	d := DefaultLabels()
	for _, def := range LabelDefaults() {
		if d.Get(def.Key) == "" {
			t.Errorf("DefaultLabels().%s is empty; every field must carry its default", def.Key)
		}
	}
	if got := len(LabelDefaults()); got != 26 {
		t.Errorf("the editable set is %d keys (22 labels + 4 terms sentences); want 26", got)
	}
}

// TestTermsSubstituteToTheOldSentences pins the substituted defaults against
// the sentences the template hard-coded at template-fixed.html:197-200 and the
// native renderer composed at gpdf.go:218. The tokens are the contract: the
// figure moves, the sentence does not.
func TestTermsSubstituteToTheOldSentences(t *testing.T) {
	got := DefaultLabels().Terms(TermsValues{
		PaymentTerm: 90,
		Validity:    30,
		LeadTime:    "60 Days",
		Incoterm:    "FOB",
	})
	want := []string{
		"1. Payment terms agreed for this quotation is 90 days.",
		"2. This quotation is valid for 30 days.",
		"3. Lead time is 60 Days.",
		"4. Incoterms: FOB",
	}
	if len(got) != len(want) {
		t.Fatalf("substituted %d terms; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("term %d = %q; want %q", i+1, got[i], want[i])
		}
	}
}

// TestTermsSubstitutionIsSinglePass proves a substituted value cannot inject a
// further placeholder: the replacement is one left-to-right pass, so a lead
// time or incoterm carrying a token of its own prints it literally.
func TestTermsSubstitutionIsSinglePass(t *testing.T) {
	d := DefaultLabels()
	d.TermsLeadTime = "3. Lead time is {leadtime} ({incoterm})."
	got := d.Terms(TermsValues{PaymentTerm: 1, Validity: 1, LeadTime: "{incoterm}", Incoterm: "FOB"})
	if got[2] != "3. Lead time is {incoterm} (FOB)." {
		t.Errorf("term 3 = %q; want the injected token left literal", got[2])
	}
}

// TestResolveOverridesFallsBackToDefaults covers the three ways a label ends
// up unstored: absent, cleared to "", and naming a key the set does not carry.
// All three must resolve to the built-in default, because a stored override
// that could blank a label would print an empty title on a customer document.
func TestResolveOverridesFallsBackToDefaults(t *testing.T) {
	base := DefaultLabels()

	if got := ResolveOverrides(nil); got != base {
		t.Errorf("no overrides resolved to %+v; want the defaults", got)
	}
	if got := ResolveOverrides(map[string]string{"docTitle": "   "}); got != base {
		t.Errorf("a blank override changed the document: %+v", got)
	}
	if got := ResolveOverrides(map[string]string{"retiredLabel": "x"}); got != base {
		t.Errorf("an unknown key changed the document: %+v", got)
	}

	overrides := map[string]string{"docTitle": "PROPOSAL", "closing": "Thank you."}
	want := base
	want.DocTitle = "PROPOSAL"
	want.Closing = "Thank you."
	if got := ResolveOverrides(overrides); got != want {
		t.Errorf("overrides resolved to %+v; want %+v", got, want)
	}
}

// TestWithDefaultsFillsEveryBlankField is the same rule applied to a value
// that arrived half-filled -- a cache that missed some keys, or a server that
// predates one of them. Whitespace-only counts as blank, which is what a
// cleared override looks like on the wire.
func TestWithDefaultsFillsEveryBlankField(t *testing.T) {
	partial := DocumentLabels{DocTitle: "ESTIMATE", Subject: "   "}
	got := partial.WithDefaults()
	if got.DocTitle != "ESTIMATE" {
		t.Errorf("WithDefaults overwrote a set field: docTitle = %q", got.DocTitle)
	}
	if got.Subject != "Subject" {
		t.Errorf("WithDefaults left a whitespace-only field as %q; a blank label would print", got.Subject)
	}
	if got.Closing != DefaultLabels().Closing {
		t.Error("WithDefaults left an empty field empty; a blank label would print")
	}
}

// TestGetSetRoundTripEveryKey proves the key set, the struct fields and the
// two directions between them agree: a field the switch forgot would resolve
// to "" on one side and silently never move on the other.
func TestGetSetRoundTripEveryKey(t *testing.T) {
	var d DocumentLabels
	for _, def := range LabelDefaults() {
		if !d.Set(def.Key, "v-"+string(def.Key)) {
			t.Errorf("Set(%q) refused a known key", def.Key)
			continue
		}
		if got := d.Get(def.Key); got != "v-"+string(def.Key) {
			t.Errorf("Get(%q) = %q after Set; the two switches disagree", def.Key, got)
		}
	}
	if d.Set(LabelKey("nope"), "x") {
		t.Error("Set accepted an unknown key; a caller could invent a field by naming one")
	}
	if d.Get(LabelKey("nope")) != "" {
		t.Error("Get returned a value for an unknown key")
	}
}

// TestValidateOverrideAcceptsTheDefaults proves the rules admit the built-in
// set: a default that its own validation refused would mean the store could
// never store today's document.
func TestValidateOverrideAcceptsTheDefaults(t *testing.T) {
	for _, def := range LabelDefaults() {
		if err := ValidateOverride(def, def.Default); err != nil {
			t.Errorf("ValidateOverride(%q, default) = %v; the default must be storable", def.Key, err)
		}
	}
}

// TestValidateOverrideRefusesBadInput pins each rule and the field it names.
// The plan (Q5/Q7): plain text only, length caps, no markup, no template
// action, no control characters, no unknown placeholder.
func TestValidateOverrideRefusesBadInput(t *testing.T) {
	short, _ := Declaration(LabelSubject)
	terms, _ := Declaration(LabelTermsPayment)

	cases := []struct {
		name string
		def  LabelDefault
		text string
		want string // the field name the error must carry
	}{
		{"a tag", short, "Sub<b>ject</b>", "subject"},
		{"a lone angle bracket", short, "Subject >", "subject"},
		{"a template action", short, "Subject {{.Margin}}", "subject"},
		{"a newline", short, "Subject\nline", "subject"},
		{"a tab", short, "Sub\tject", "subject"},
		{"a delete character", short, "Sub\x7fject", "subject"},
		{"over the cap", short, strings.Repeat("x", short.MaxLen+1), "subject"},
		{"an unknown token", terms, "1. Payment is {amount} days.", "termsPayment"},
		{"an unclosed token", terms, "1. Payment is {payment days.", "termsPayment"},
		{"a stray brace", terms, "1. Payment is {payment} days }", "termsPayment"},
		{"a stray closing brace", terms, "1. Payment is } {payment} days.", "termsPayment"},
		{"a nested brace", terms, "1. Payment is {pay{ment}} days.", "termsPayment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOverride(tc.def, tc.text)
			if err == nil {
				t.Fatalf("ValidateOverride(%q) accepted %q", tc.def.Key, tc.text)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name the field %q", err, tc.want)
			}
		})
	}
}

// TestValidateOverrideAcceptsEveryToken proves the four fixed tokens are all
// storable, together and alone: a sentence may carry none, one, or all four.
func TestValidateOverrideAcceptsEveryToken(t *testing.T) {
	terms, _ := Declaration(LabelTermsPayment)
	for _, text := range []string{
		"Payment is agreed.",
		"{payment} days",
		"{payment} {validity} {leadtime} {incoterm}",
		"Payment {payment}, validity {validity}, lead {leadtime}, term {incoterm}",
	} {
		if err := ValidateOverride(terms, text); err != nil {
			t.Errorf("ValidateOverride refused %q: %v", text, err)
		}
	}
}

// TestCheckOverrides covers the whole-set rules: unknown keys, duplicates, and
// the per-value rules reached through it.
func TestCheckOverrides(t *testing.T) {
	if err := CheckOverrides(nil); err != nil {
		t.Errorf("CheckOverrides(nil) = %v; an empty set is a full clear to defaults", err)
	}
	if err := CheckOverrides([]CompanyLabel{{Key: "docTitle", Value: "PROPOSAL"}}); err != nil {
		t.Errorf("CheckOverrides refused a plain override: %v", err)
	}
	if err := CheckOverrides([]CompanyLabel{{Key: "notALabel", Value: "x"}}); err == nil {
		t.Error("CheckOverrides accepted an unknown key")
	} else if !strings.Contains(err.Error(), "notALabel") {
		t.Errorf("error %q does not name the key", err)
	}
	if err := CheckOverrides([]CompanyLabel{
		{Key: "docTitle", Value: "A"}, {Key: " docTitle ", Value: "B"},
	}); err == nil {
		t.Error("CheckOverrides accepted the same key twice")
	} else if !strings.Contains(err.Error(), "docTitle") {
		t.Errorf("error %q does not name the duplicated key", err)
	}
	if err := CheckOverrides([]CompanyLabel{{Key: "docTitle", Value: "<script>"}}); err == nil {
		t.Error("CheckOverrides accepted markup")
	}
}
