package store

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Ticket 04 binds the frozen wire shape to the production DTOs. The session,
// item, history, line-identity, discount and tax-basis checks below run
// against the real store types; only the transient feedback/transition
// examples stay test-only, because notices are never persisted.
//
// P2-12: the generated desktop declarations in frontend/wailsjs/go/models.ts
// describe nullable fields as optional non-null values. That generator output
// is not hand-edited; the frontend adapter normalizes null/undefined at the
// transport seam before the pane sees a session.
type reconciliationSessionContract = POReconciliationSession

type reconciliationLineIdentityContract = POReconciliationLineIdentity

type reconciliationItemContract = POReconciliationItem

type reconciliationEventContract = POReconciliationHistoryEvent

type reconciliationDiscountContract = POReconciliationDiscount

type reconciliationTaxBasisContract = POReconciliationTaxBasis

type reconciliationFeedbackContract struct {
	Notice     *string
	Status     *string
	Resolution *string
}

type reconciliationFieldContract struct {
	ID         string
	KeyPattern string
	Label      string
	Mode       string
}

// Independently declared in each consumer. Adding a field on only one side
// fails against the shared catalogue, including non-decision comparisons.
var reconciliationFieldsContract = []reconciliationFieldContract{
	{"header:paymentTerms", "header:paymentTerms", "Payment terms", "commit"},
	{"header:incoterms", "header:incoterms", "Incoterms", "commit"},
	{"line:description", "line:{lineKey}:description", "Line {lineNumber} description", "commit"},
	{"line:uom", "line:{lineKey}:uom", "Line {lineNumber} unit of measure", "commit"},
	{"line:quantity", "line:{lineKey}:quantity", "Line {lineNumber} quantity", "commit"},
	{"line:unitPrice", "line:{lineKey}:unitPrice", "Line {lineNumber} unit price", "commit"},
	{"line:discount", "line:{lineKey}:discount", "Line {lineNumber} discount", "commit"},
	{"line:match", "line:{lineKey}:match", "Line {lineNumber} match", "line-set"},
	{"source:match", "source:{sourceLineRef}:match", "Quotation line {sourceLineRef} missing", "line-set"},
	{"link:taxBasis", "link:taxBasis", "Tax basis", "link"},
	{"derived:totals", "derived:totals", "Totals", "derived"},
}

type reconciliationTransitionContract struct {
	Name            string
	Field           string
	Event           string
	Before          reconciliationFeedbackContract
	After           reconciliationFeedbackContract
	CreatesItem     bool
	WritesPO        bool
	BlocksFinalSave bool
}

type reconciliationComparisonCase struct {
	Name    string
	Source  PurchaseOrderSource
	Entered PurchaseOrderReviewState
	Want    struct {
		Differences          []PurchaseOrderDifference
		ComparableKeys       []string
		Missing              []string
		ExtraLineNumbers     []int
		DuplicateLineNumbers []int
	}
	// Current display drift is explicit, never a normalisation hidden in an
	// assertion. The future wire contract uses minor-unit numbers instead.
	FrontendPriceValues map[string][2]string
}

type reconciliationFixtureContract struct {
	Version int
	Shapes  map[string][]string
	Enums   map[string][]string
	Fields  []reconciliationFieldContract
	Session reconciliationSessionContract
	Values  struct {
		Discount reconciliationDiscountContract
		TaxBasis reconciliationTaxBasisContract
	}
	Transitions   []reconciliationTransitionContract
	IdentityCases []struct {
		Name      string
		BeforeKey string
		AfterKey  string
		BeforeID  *int64
		AfterID   *int64
		Removed   bool
	}
	GateCases []struct {
		Name     string
		Blockers []string
		CanSave  bool
	}
	ComparisonCases []reconciliationComparisonCase
	TotalsCases     []struct {
		Name                   string
		Input                  PurchaseOrderCalcInput
		PrintedGrandTotalMinor int64
		ReasonFields           []string
		Want                   map[string]int64
	}
}

func TestPOReconciliationDerivedTotalsMatchSharedCases(t *testing.T) {
	fixture := reconciliationFixture(t)
	if len(fixture.TotalsCases) == 0 {
		t.Fatal("no derived totals cases")
	}
	for _, tc := range fixture.TotalsCases {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := CalculatePurchaseOrder(tc.Input)
			if err != nil {
				t.Fatal(err)
			}
			actual := map[string]int64{
				"grossMinor": got.Lines[0].GrossMinor, "rowDiscountMinor": got.Lines[0].DiscountMinor,
				"subtotalMinor": got.SubtotalMinor, "discountMinor": got.DiscountMinor,
				"afterDiscountMinor": got.AfterDiscountMinor, "ppnMinor": got.PpnMinor,
				"adjustmentMinor": got.AdjustmentMinor, "grandTotalMinor": got.GrandTotalMinor,
				"printedDifferenceMinor": tc.PrintedGrandTotalMinor - got.GrandTotalMinor,
			}
			if !reflect.DeepEqual(actual, tc.Want) {
				t.Errorf("derived totals = %v; want %v", actual, tc.Want)
			}
		})
	}
}

func reconciliationFixture(t *testing.T) reconciliationFixtureContract {
	t.Helper()
	raw, err := os.ReadFile("../testdata/po_reconciliation_contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture reconciliationFixtureContract
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("strict shared reconciliation fixture: %v", err)
	}
	return fixture
}

func TestPOReconciliationContractCatalogueAndWireKeys(t *testing.T) {
	fixture := reconciliationFixture(t)
	if fixture.Version != 1 {
		t.Fatalf("contract version = %d; want 1", fixture.Version)
	}
	if !reflect.DeepEqual(fixture.Fields, reconciliationFieldsContract) {
		t.Fatalf("Go field catalogue differs from shared contract: got %+v", reconciliationFieldsContract)
	}
	enums := map[string][]string{
		"ItemKind":   {"difference", "missing_line", "extra_line", "duplicate_match", "tax_basis"},
		"ItemStatus": {"unresolved", "resolved"},
		"Resolution": {"kept_po", "used_quotation", "values_now_match", "acknowledged_missing", "acknowledged_extra", "match_corrected", "line_removed"},
		"Notice":     {"empty", "matched"},
	}
	if !reflect.DeepEqual(fixture.Enums, enums) {
		t.Fatal("Go state/kind catalogue differs from shared contract")
	}
	shapes := map[string]any{
		"Session": reconciliationSessionContract{}, "Item": reconciliationItemContract{},
		"HistoryEvent": reconciliationEventContract{}, "LineIdentity": reconciliationLineIdentityContract{},
		"Discount": reconciliationDiscountContract{}, "TaxBasis": reconciliationTaxBasisContract{},
		"Feedback": reconciliationFeedbackContract{}, "Transition": reconciliationTransitionContract{},
	}
	if len(shapes) != len(fixture.Shapes) {
		t.Fatal("shape registry differs from shared contract")
	}
	for name, shape := range shapes {
		t.Run(name, func(t *testing.T) {
			typ := reflect.TypeOf(shape)
			keys := make([]string, typ.NumField())
			for i := range keys {
				field := typ.Field(i)
				if field.Tag.Get("json") != "" {
					t.Fatal("contract uses exact PascalCase keys with explicit nulls, not omitempty or aliases")
				}
				keys[i] = field.Name
			}
			sort.Strings(keys)
			want := append([]string{}, fixture.Shapes[name]...)
			sort.Strings(want)
			if !reflect.DeepEqual(keys, want) {
				t.Fatalf("Go %s keys = %v; shared keys = %v", name, keys, want)
			}
		})
	}
	// Compare the actual round trip with the original specimen. This catches
	// omitted keys at every nesting level, not just unknown decoder fields.
	raw, _ := json.Marshal(fixture.Session)
	var roundTrip map[string]any
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile("../testdata/po_reconciliation_contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var specimen struct{ Session map[string]any }
	if err := json.Unmarshal(original, &specimen); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roundTrip, specimen.Session) || len(fixture.Session.Items) == 0 {
		t.Fatal("session specimen omits or changes a frozen nested wire key")
	}
	for _, item := range fixture.Session.Items {
		var target any
		switch item.Field {
		case "line:discount":
			target = &reconciliationDiscountContract{}
		case "link:taxBasis":
			target = &reconciliationTaxBasisContract{}
		default:
			continue
		}
		for _, value := range []json.RawMessage{item.POValue, item.SourceValue} {
			decoder := json.NewDecoder(bytes.NewReader(value))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(target); err != nil {
				t.Fatalf("%s value shape: %v", item.Key, err)
			}
		}
	}
}

func TestPOReconciliationCurrentComparisonMatchesSharedCases(t *testing.T) {
	fixture := reconciliationFixture(t)
	if len(fixture.ComparisonCases) == 0 {
		t.Fatal("no shared comparison cases")
	}
	for _, tc := range fixture.ComparisonCases {
		t.Run(tc.Name, func(t *testing.T) {
			got := ComparePurchaseOrder(tc.Source, tc.Entered)
			if !reflect.DeepEqual(got.Differences, tc.Want.Differences) {
				t.Errorf("differences = %+v; want %+v", got.Differences, tc.Want.Differences)
			}
			// The current Go comparator lists two header slots twice. This
			// test compares the set; the contract records that legacy defect.
			seen := map[string]bool{}
			keys := []string{}
			for _, key := range got.ComparableKeys {
				if !seen[key] {
					keys = append(keys, key)
					seen[key] = true
				}
			}
			sort.Strings(keys)
			wantKeys := append([]string{}, tc.Want.ComparableKeys...)
			sort.Strings(wantKeys)
			missing := []string{}
			for _, line := range got.Missing {
				missing = append(missing, line.Label())
			}
			if !reflect.DeepEqual(keys, wantKeys) || !reflect.DeepEqual(missing, tc.Want.Missing) ||
				!reflect.DeepEqual(got.ExtraLineNumbers, tc.Want.ExtraLineNumbers) ||
				!reflect.DeepEqual(got.DuplicateLineNumbers, tc.Want.DuplicateLineNumbers) {
				t.Errorf("matching/field set differs: %+v; want %+v", got, tc.Want)
			}
		})
	}
}

func TestPOReconciliationFixtureCoversThePlannedStateMachine(t *testing.T) {
	fixture := reconciliationFixture(t)
	states := map[string]bool{}
	for _, tc := range fixture.Transitions {
		if tc.After.Notice != nil {
			states[*tc.After.Notice] = true
		}
		if tc.After.Status != nil {
			states[*tc.After.Status] = true
		}
		if tc.After.Resolution != nil {
			states[*tc.After.Resolution] = true
		}
		if tc.After.Notice != nil && *tc.After.Notice == "matched" && tc.After.Status != nil {
			t.Fatalf("%s duplicates a review item with a matched notice", tc.Name)
		}
	}
	wantStates := strings.Fields("empty matched unresolved resolved kept_po used_quotation acknowledged_missing acknowledged_extra match_corrected line_removed values_now_match")
	if len(states) != len(wantStates) {
		t.Fatalf("transition state catalogue = %v; want %v", states, wantStates)
	}
	for _, state := range wantStates {
		if !states[state] {
			t.Errorf("no transition example for %s", state)
		}
	}
	for _, tc := range fixture.IdentityCases {
		if tc.BeforeKey != tc.AfterKey {
			t.Errorf("%s re-keys a retained item", tc.Name)
		}
	}
	for _, tc := range fixture.GateCases {
		if tc.CanSave != (len(tc.Blockers) == 0) {
			t.Errorf("%s contradicts the Final Save gate", tc.Name)
		}
	}
}
