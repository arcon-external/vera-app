package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #629's generated desktop surface. Wails generates the report models
// from the domain types themselves, so the JSON tags are the wire contract the
// screen consumes. The method-name guard above proves the call exists; these
// checks pin the shape the frontend compiles against, including the nullable
// percentages: null means "unknown at zero capital", never zero percent.

func TestGeneratedPurchaseOrderReportBindingUsesTheContractTags(t *testing.T) {
	declaration, err := os.ReadFile(filepath.Join("frontend", "wailsjs", "go", "backend", "Quotation.d.ts"))
	if err != nil {
		t.Fatalf("cannot read the Quotation declaration: %v", err)
	}
	want := "GetPurchaseOrderReport(arg1:poreport.Query):Promise<poreport.Report>;"
	if !strings.Contains(string(declaration), want) {
		t.Errorf("Quotation.d.ts does not declare %q; run: wails generate module", want)
	}

	raw, err := os.ReadFile(filepath.Join("frontend", "wailsjs", "go", "models.ts"))
	if err != nil {
		t.Fatalf("cannot read generated models: %v", err)
	}
	models := string(raw)
	if !strings.Contains(models, "export namespace poreport {") {
		t.Fatal("models.ts has no poreport namespace; run: wails generate module")
	}
	for class, fields := range map[string][]string{
		"Query": {"from: string;", "to: string;", "bucket: string;", "category: string;", "currency: string;"},
		"Metrics": {
			"poCount: number;", "amountMinor: number;", "costedMinor: number;",
			"uncostedMinor: number;", "capitalMinor: number;", "marginMinor: number;",
			// Nullable scalar percentages stay optional in the generated model.
			"marginBps?: number;", "coverageBps?: number;",
		},
		"Period":          {"periodStart: string;", "partial: boolean;", "inProgress: boolean;"},
		"Report":          {"generatedAt: string;", "asOf: string;", "filter: Query;", "projection: Projection;"},
		"ProjectedBucket": {"amountLowMinor: number;", "amountBaseMinor: number;", "amountHighMinor: number;", "marginBps?: number;"},
	} {
		block, ok := generatedClassBlock(models, class)
		if !ok {
			t.Errorf("models.ts has no class %s; run: wails generate module", class)
			continue
		}
		for _, field := range fields {
			if !strings.Contains(block, field) {
				t.Errorf("poreport.%s is missing %q; run: wails generate module\n%s", class, field, block)
			}
		}
	}
}

// TestGeneratedPurchaseOrderReportExportUsesTheFrozenContract pins #632's
// binding exactly: the supplied snapshot travels as a string and a successful
// save returns the chosen path, while an empty string means the user
// cancelled. The PDF is rendered from that snapshot, not fetched again.
func TestGeneratedPurchaseOrderReportExportUsesTheFrozenContract(t *testing.T) {
	declaration, err := os.ReadFile(filepath.Join("frontend", "wailsjs", "go", "backend", "Quotation.d.ts"))
	if err != nil {
		t.Fatalf("cannot read the Quotation declaration: %v", err)
	}
	want := "ExportPurchaseOrderReport(arg1:string):Promise<string>;"
	if !strings.Contains(string(declaration), want) {
		t.Errorf("Quotation.d.ts does not declare %q; run: wails generate module", want)
	}

	implementation, err := os.ReadFile(filepath.Join("frontend", "wailsjs", "go", "backend", "Quotation.js"))
	if err != nil {
		t.Fatalf("cannot read the Quotation implementation: %v", err)
	}
	if !strings.Contains(string(implementation), "['ExportPurchaseOrderReport'](arg1)") {
		t.Errorf("Quotation.js does not call ExportPurchaseOrderReport; run: wails generate module")
	}
}
