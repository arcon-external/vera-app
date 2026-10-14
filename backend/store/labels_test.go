package store

// Guards for the company's label overrides (issue #339).
//
// The store is the only writer of company_labels, and the rules that matter are
// the ones a database cannot enforce on its own: an override that names no real
// label, a value that is not plain text, and the fallback that makes a cleared
// label print the built-in default. These need a real database because the
// property under test is the round trip.

import (
	"context"
	"strings"
	"testing"

	"vera-app/backend/labels"
)

func TestCompanyLabelsRoundTrip(t *testing.T) {
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_company_labels")

	// A fresh schema seeds nothing: the table holds overrides only, and the
	// defaults stay in Go. The effective set is therefore the built-in one.
	fresh, err := database.GetCompanyLabels(ctx)
	if err != nil {
		t.Fatalf("get labels on a fresh schema: %v", err)
	}
	if fresh != labels.DefaultLabels() {
		t.Errorf("a schema with no overrides resolved to %+v; want the defaults", fresh)
	}

	updated, err := database.UpdateCompanyLabels(ctx, []labels.CompanyLabel{
		{Key: "docTitle", Value: "PROPOSAL"},
		{Key: "closing", Value: "Thank you."},
		{Key: "termsPayment", Value: "1. Payment is {payment} days."},
	})
	if err != nil {
		t.Fatalf("update labels: %v", err)
	}
	if updated.DocTitle != "PROPOSAL" || updated.Closing != "Thank you." {
		t.Errorf("updated = %+v; want the stored overrides applied", updated)
	}
	if updated.TermsPayment != "1. Payment is {payment} days." {
		t.Errorf("terms payment = %q; want the stored override", updated.TermsPayment)
	}
	// Everything else stays the built-in default, not blank.
	if updated.Subject != labels.DefaultLabels().Subject {
		t.Errorf("an unrelated label moved: subject = %q", updated.Subject)
	}

	stored, err := database.GetCompanyLabels(ctx)
	if err != nil {
		t.Fatalf("get stored labels: %v", err)
	}
	if stored != updated {
		t.Errorf("stored = %+v; want %+v (the write did not persist)", stored, updated)
	}

	// A replacement is a replacement: the labels left out go back to the
	// built-in default rather than surviving from the previous write.
	replaced, err := database.UpdateCompanyLabels(ctx, []labels.CompanyLabel{
		{Key: "subject", Value: "Scope"},
	})
	if err != nil {
		t.Fatalf("replace labels: %v", err)
	}
	if replaced.DocTitle != labels.DefaultLabels().DocTitle {
		t.Errorf("a full replace kept an earlier override: docTitle = %q", replaced.DocTitle)
	}
	if replaced.Subject != "Scope" {
		t.Errorf("subject = %q; want Scope", replaced.Subject)
	}

	// An empty value clears that label, which is the wire contract for
	// "restore the built-in".
	cleared, err := database.UpdateCompanyLabels(ctx, []labels.CompanyLabel{
		{Key: "subject", Value: ""},
	})
	if err != nil {
		t.Fatalf("clear labels: %v", err)
	}
	if cleared != labels.DefaultLabels() {
		t.Errorf("after clearing, the effective set is %+v; want the defaults", cleared)
	}

	// A completely empty set is a full clear, not an error.
	emptied, err := database.UpdateCompanyLabels(ctx, nil)
	if err != nil {
		t.Fatalf("empty the label table: %v", err)
	}
	if emptied != labels.DefaultLabels() {
		t.Errorf("an empty set resolved to %+v; want the defaults", emptied)
	}
}

func TestUpdateCompanyLabelsRefusesBadInput(t *testing.T) {
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_company_labels_refuse")

	for name, rows := range map[string][]labels.CompanyLabel{
		"unknown key":     {{Key: "notALabel", Value: "x"}},
		"duplicate key":   {{Key: "docTitle", Value: "A"}, {Key: " docTitle ", Value: "B"}},
		"markup":          {{Key: "docTitle", Value: "<b>QUOTATION</b>"}},
		"template action": {{Key: "subject", Value: "Subject {{.Margin}}"}},
		"newline":         {{Key: "subject", Value: "Sub\nject"}},
		"over the cap":    {{Key: "docTitle", Value: strings.Repeat("X", 25)}},
		"bad token":       {{Key: "termsPayment", Value: "1. Payment is {amount} days."}},
	} {
		_, err := database.UpdateCompanyLabels(ctx, rows)
		if err == nil {
			t.Errorf("%s accepted; want a validation error", name)
			continue
		}
		// Every error names the field it objects to, the way the store's
		// other validations do.
		if !strings.Contains(err.Error(), "label") {
			t.Errorf("%s error = %q; want it to name the label", name, err)
		}
	}

	// A refused write must leave the table untouched: the set is replaced whole
	// or not at all.
	after, err := database.GetCompanyLabels(ctx)
	if err != nil {
		t.Fatalf("get labels after refused writes: %v", err)
	}
	if after != labels.DefaultLabels() {
		t.Errorf("a refused write changed the table: %+v", after)
	}
}
