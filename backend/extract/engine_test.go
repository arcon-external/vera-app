package extract

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"vera-app/backend/apicontract"
)

// Issue #477: the extractor boundary.
//
// The server runs a separate executable with the PDF on stdin and the result
// contract on stdout. These tests replace the executable with tiny shell
// commands, so the boundary is exercised without Poppler and without Docker:
// the input is the exact PDF bytes, a timeout kills the process, an oversized
// or malformed answer is refused, and a missing binary fails clearly.

func validEngineOutput() apicontract.ExtractionOutput {
	return apicontract.ExtractionOutput{
		SchemaVersion: apicontract.ExtractionSchemaVersion,
		Engine:        "stub",
		EngineVersion: "1.0",
		PageCount:     1,
		PagesWithText: 1,
		Pages: []apicontract.ExtractionPageInfo{
			{Number: 1, Width: 595, Height: 842},
		},
		Fields: []apicontract.ExtractionField{
			{Key: apicontract.ExtractionFieldPONumber, ExtractionCell: apicontract.ExtractionCell{
				Value: "4500123456", Raw: "4500123456", Status: apicontract.ExtractionStatusExtracted,
				Page: pointerTo(1), BBox: &apicontract.ExtractionBBox{X: 10, Y: 20, Width: 50, Height: 10},
			}},
			{Key: apicontract.ExtractionFieldPODate, ExtractionCell: apicontract.ExtractionCell{
				Status: apicontract.ExtractionStatusAbsent,
			}},
			{Key: apicontract.ExtractionFieldRequestedDeliveryDate, ExtractionCell: apicontract.ExtractionCell{
				Status: apicontract.ExtractionStatusAbsent,
			}},
			{Key: apicontract.ExtractionFieldCurrency, ExtractionCell: apicontract.ExtractionCell{
				Status: apicontract.ExtractionStatusAbsent,
			}},
			{Key: apicontract.ExtractionFieldPaymentTerms, ExtractionCell: apicontract.ExtractionCell{
				Status: apicontract.ExtractionStatusNeedsReview, Value: "30 days", Raw: "30 days",
			}},
			{Key: apicontract.ExtractionFieldIncoterms, ExtractionCell: apicontract.ExtractionCell{
				Status: apicontract.ExtractionStatusAbsent,
			}},
		},
		Rows: []apicontract.ExtractionRow{
			{
				LineNumber:  apicontract.ExtractionCell{Value: "1", Status: apicontract.ExtractionStatusExtracted},
				Description: apicontract.ExtractionCell{Value: "Panel", Status: apicontract.ExtractionStatusExtracted},
				Quantity:    apicontract.ExtractionCell{Value: "2", Status: apicontract.ExtractionStatusExtracted},
				UoM:         apicontract.ExtractionCell{Value: "Pc", Status: apicontract.ExtractionStatusExtracted},
				UnitPrice:   apicontract.ExtractionCell{Value: "1250.00", Status: apicontract.ExtractionStatusExtracted},
				Amount:      apicontract.ExtractionCell{Value: "2500.00", Status: apicontract.ExtractionStatusExtracted},
				Status:      apicontract.ExtractionStatusExtracted,
				Page:        1,
			},
		},
	}
}

func engineJSON(t *testing.T, output apicontract.ExtractionOutput) string {
	t.Helper()
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal stub output: %v", err)
	}
	return string(encoded)
}

func commandEngine(command ...string) *CommandEngine {
	return &CommandEngine{
		Command:        command,
		Timeout:        2 * time.Second,
		MaxOutputBytes: 1 << 20,
		MaxPages:       40,
	}
}

func TestCommandEngineSendsThePDFOnStdinAndReturnsTheContract(t *testing.T) {
	pdf := []byte("%PDF-1.7 stub bytes")
	output := engineJSON(t, validEngineOutput())
	// The command proves stdin arrived by requiring the PDF header before it
	// writes the answer; an empty stdin fails with a different exit code.
	engine := commandEngine("sh", "-c",
		`input=$(cat); case "$input" in %PDF-*) printf '%s' "$STUB_OUTPUT";; *) exit 9;; esac`)
	engine.Environment = []string{"STUB_OUTPUT=" + output}
	result, err := engine.Extract(context.Background(), pdf)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if result.Engine != "stub" || result.PageCount != 1 || len(result.Fields) != 6 || len(result.Rows) != 1 {
		t.Errorf("result = %+v", result)
	}
}

func TestCommandEngineRefusesMalformedOversizedAndFailedAnswers(t *testing.T) {
	pdf := []byte("%PDF-1.7 stub bytes")
	cases := []struct {
		name    string
		command []string
		maxOut  int
		want    error
	}{
		{"not json", []string{"sh", "-c", `printf 'not json'`}, 1 << 20, ErrInvalidOutput},
		{"wrong schema", []string{"sh", "-c", `printf '{"schemaVersion":99}'`}, 1 << 20, ErrInvalidOutput},
		{"oversized", []string{"sh", "-c", `head -c 200 /dev/zero | tr '\0' 'x'`}, 64, ErrOutputTooLarge},
		{"failed", []string{"sh", "-c", `printf 'pdftotext: broken' >&2; exit 3`}, 1 << 20, ErrEngineFailed},
		{"missing binary", []string{"vera-extractor-that-does-not-exist"}, 1 << 20, ErrExtractorUnavailable},
		{"page limit", []string{"sh", "-c", `cat <<'JSON'
{"schemaVersion":1,"engine":"stub","engineVersion":"1","pageCount":41,"pagesWithText":41,"fields":[],"rows":[],"abstentions":{}}
JSON`}, 1 << 20, ErrPageLimit},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := commandEngine(testCase.command...)
			engine.MaxOutputBytes = testCase.maxOut
			_, err := engine.Extract(context.Background(), pdf)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v; want %v", err, testCase.want)
			}
		})
	}
}

func TestCommandEngineKillsASlowExtractor(t *testing.T) {
	engine := commandEngine("sh", "-c", `sleep 10`)
	engine.Timeout = 80 * time.Millisecond
	started := time.Now()
	_, err := engine.Extract(context.Background(), []byte("%PDF-1.7 stub"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v; want timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("timeout took %v; the child process was not killed promptly", elapsed)
	}
}

func TestCommandEngineRefusesAnOversizedInputBeforeStarting(t *testing.T) {
	engine := commandEngine("sh", "-c", `printf '{"schemaVersion":1}'`)
	engine.MaxInputBytes = 8
	_, err := engine.Extract(context.Background(), []byte(strings.Repeat("x", 9)))
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("error = %v; want input too large", err)
	}
}

func TestValidateExtractionOutputRecomputesAbstentionsAndRejectsBadShapes(t *testing.T) {
	output := validEngineOutput()
	output.Abstentions = apicontract.ExtractionAbstentions{Fields: 99, Rows: 99}
	if err := ValidateExtractionOutput(&output, 40); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if output.Abstentions.Fields != 5 || output.Abstentions.Rows != 0 {
		t.Errorf("abstentions = %+v; want 5 fields/0 rows recomputed", output.Abstentions)
	}

	bad := validEngineOutput()
	bad.Fields = bad.Fields[:5]
	if err := ValidateExtractionOutput(&bad, 40); !errors.Is(err, ErrInvalidOutput) {
		t.Errorf("missing field error = %v; want invalid output", err)
	}

	badStatus := validEngineOutput()
	badStatus.Fields[0].Status = "maybe"
	if err := ValidateExtractionOutput(&badStatus, 40); !errors.Is(err, ErrInvalidOutput) {
		t.Errorf("bad status error = %v; want invalid output", err)
	}

	badPages := validEngineOutput()
	badPages.Pages = nil
	if err := ValidateExtractionOutput(&badPages, 40); !errors.Is(err, ErrInvalidOutput) {
		t.Errorf("bad pages error = %v; want invalid output", err)
	}
}

func TestCommandEnginePreflightNamesAnUnavailableProgram(t *testing.T) {
	missing := commandEngine("vera-extractor-that-does-not-exist")
	if err := missing.Available(context.Background()); !errors.Is(err, ErrExtractorUnavailable) {
		t.Errorf("preflight of a missing program = %v; want ErrExtractorUnavailable", err)
	}
	empty := commandEngine()
	if err := empty.Available(context.Background()); !errors.Is(err, ErrExtractorUnavailable) {
		t.Errorf("preflight with no command = %v; want ErrExtractorUnavailable", err)
	}
	if err := commandEngine("sh", "-c", "true").Available(context.Background()); err != nil {
		t.Errorf("preflight of an available program = %v; want nil", err)
	}
}

func TestSplitExtractorCommand(t *testing.T) {
	if got := SplitCommand("  vextract  "); len(got) != 1 || got[0] != "vextract" {
		t.Errorf("SplitCommand = %v; want [vextract]", got)
	}
	if got := SplitCommand(""); len(got) != 0 {
		t.Errorf("SplitCommand(empty) = %v; want none", got)
	}
}

func TestCommandEngineDoesNotLeakServerSecrets(t *testing.T) {
	t.Setenv("VERA_POSTGRES_DSN", "postgres://user:secret@example/db")
	t.Setenv("VERA_SIGNING_KEY", "not-a-real-key")
	output := engineJSON(t, validEngineOutput())
	engine := commandEngine("sh", "-c", `if env | grep -Eq '^VERA_(POSTGRES_DSN|SIGNING_KEY)='; then exit 7; fi
if [ -z "$VERA_EXTRACT_MAX_PAGES" ]; then exit 8; fi
cat >/dev/null
printf '%s' "$STUB_OUTPUT"`)
	engine.Environment = []string{"STUB_OUTPUT=" + output}
	if _, err := engine.Extract(context.Background(), []byte("%PDF-1.7 stub")); err != nil {
		t.Fatalf("extract: %v (exit 7 means a server secret crossed the boundary; exit 8 means the page cap was missing)", err)
	}
}
