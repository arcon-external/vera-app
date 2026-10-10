package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"

	"vera-app/backend"
)

// quotationsCommands are the commands the CLI exists for.
//
// They come in three kinds and the difference matters:
//
//   - validate and render work on the local machine, using the application's own
//     code, and touch the server only to read the client directory and the
//     company profile. They are how you check a document without filing it.
//   - preview asks the server what it would produce, which is authoritative.
//   - export does the whole thing: number, signed revision, PDF, attach.
func quotationsCommands() *command {
	return commandGroup("quotations", "Quotations: check, preview, render, export, and the archive.",
		quotationsValidateCommand(),
		quotationsPreviewCommand(),
		quotationsRenderCommand(),
		quotationsExportCommand(),
		quotationsNextNumberCommand(),
		quotationsListCommand(),
		quotationsShowCommand(),
		quotationsRevisionsCommand(),
		quotationsAttachCommand(),
		quotationsRemoveCommand(),
	)
}

// loadInputDoc reads the --input file every quotation command shares.
func (a *app) loadInputDoc(input string) (*InputDoc, error) {
	if input == "" {
		return nil, usagef(a.cmd, "--input is required (a JSON file, or - for stdin); see 'vera schema'")
	}
	doc, err := LoadInputDoc(input)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// prepare renders the input into a quotation the local renderer can use.
func (a *app) prepare(input string) (*backend.Quotation, *InputDoc, error) {
	doc, err := a.loadInputDoc(input)
	if err != nil {
		return nil, nil, err
	}
	quotation, err := a.localQuotation()
	if err != nil {
		return nil, nil, err
	}
	payload, err := doc.toQuotationInput()
	if err != nil {
		return nil, nil, err
	}
	if err := quotation.Prepare(payload); err != nil {
		// The server's validation messages are the good ones -- they name the
		// line and the field -- so they are passed through unchanged.
		return nil, nil, err
	}
	return quotation, doc, nil
}

func quotationsValidateCommand() *command {
	var input string
	return &command{
		name:    "validate",
		summary: "Price an input file locally and print the totals, without filing anything.",
		usage:   "vera quotations validate --input quotation.json",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&input, "input", "", "the quotation JSON file, or - for stdin")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			quotation, doc, err := a.prepare(input)
			if err != nil {
				return err
			}
			internal, err := quotation.GetInternalBreakdown()
			if err != nil {
				return err
			}
			customer, err := quotation.GetCustomerPreview()
			if err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{
				"customerCode": doc.CustomerCode,
				"customer":     customer,
				"internal":     internal,
			}); err != nil {
				return err
			}
			a.line("The input prices cleanly.")
			a.blank()
			a.printInternalTotals(internal)
			return nil
		},
	}
}

// printInternalTotals is the internal arithmetic, in the order it happens.
//
// It is the one place a CLI user sees the difference between what the customer
// pays and what the company keeps, so the labels say which is which -- PPN is
// collected for the state and is never profit.
func (a *app) printInternalTotals(internal backend.InternalBreakdown) {
	rows := [][]string{
		{"Cost subtotal", internal.DisplayOriginalSubtotal},
		{"Customer subtotal", internal.DisplayMarginalizedSubtotal},
		{"Row discounts", internal.DisplayMarginalizedDiscount},
		{"Whole-quotation discount", internal.DisplayOriginalDiscount},
		{"Grand total (customer pays)", internal.DisplayGrand},
	}
	if internal.DisplayPpn != "" {
		rows = append(rows, []string{"PPN 11% (collected for the state)", internal.DisplayPpn})
	}
	if internal.DisplayTax != "" {
		rows = append(rows, []string{"PPH22 (internal, never printed)", internal.DisplayTax})
	}
	rows = append(rows,
		[]string{"Nett after tax", internal.DisplayMarginalizedAfterTax},
		[]string{"Nett profit", internal.DisplayProfit},
	)
	a.table(nil, rows)
}

func quotationsPreviewCommand() *command {
	var input string
	return &command{
		name:    "preview",
		summary: "Ask the server to price the input and show both views it would produce.",
		usage:   "vera quotations preview --input quotation.json",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&input, "input", "", "the quotation JSON file, or - for stdin")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			doc, err := a.loadInputDoc(input)
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			// The server's own Prepare: this is what its renderer would see,
			// which is the point of previewing rather than validating.
			var response struct {
				Customer backend.CustomerPreview   `json:"customer"`
				Internal backend.InternalBreakdown `json:"internal"`
			}
			input, err := doc.toQuotationInput()
			if err != nil {
				return err
			}
			if err := client.Post("/v1/quotations/prepare", input, &response); err != nil {
				return err
			}
			if err := a.emitJSON(response); err != nil {
				return err
			}
			a.line("Customer view")
			a.table(nil, [][]string{
				{"Subtotal", response.Customer.DisplaySubTotal},
				{"Discount", response.Customer.DisplayDiscountAmount},
				{"Grand total", response.Customer.DisplayGrandTotal},
			})
			a.blank()
			a.line("Internal view")
			a.printInternalTotals(response.Internal)
			return nil
		},
	}
}

func quotationsRenderCommand() *command {
	var input, out string
	return &command{
		name:    "render",
		summary: "Render the PDF locally, filing nothing and using no quote number.",
		usage:   "vera quotations render --input quotation.json --out quote.pdf",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&input, "input", "", "the quotation JSON file, or - for stdin")
			fs.StringVar(&out, "out", "", "where to write the PDF (required)")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if out == "" {
				return usagef(a.cmd, "--out is required; naming the file is the caller's job here")
			}
			quotation, _, err := a.prepare(input)
			if err != nil {
				return err
			}
			pdf, err := quotation.RenderPDF()
			if err != nil {
				return fmt.Errorf("render the quotation: %w", err)
			}
			if err := writeFile(out, pdf); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"out": out, "bytes": len(pdf), "filed": false}); err != nil {
				return err
			}
			a.line("Wrote %s (%d bytes). Nothing was filed; use 'export' to file it.", out, len(pdf))
			return nil
		},
	}
}

// quotationsExportCommand is the whole flow in one command.
func quotationsExportCommand() *command {
	var input, out string
	var draftID int64
	return &command{
		name:    "export",
		summary: "Export: allocate the quote number, store a signed revision, render the PDF, and attach it.",
		usage:   "vera quotations export --input quotation.json --out quote.pdf [--draft N]",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&input, "input", "", "the quotation JSON file, or - for stdin")
			fs.StringVar(&out, "out", "", "where to write the PDF (required)")
			fs.Int64Var(&draftID, "draft", 0, "also consume this draft, which the server deletes only after the revision is stored")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if out == "" {
				return usagef(a.cmd, "--out is required; naming the file is the caller's job here")
			}
			quotation, doc, err := a.prepare(input)
			if err != nil {
				return err
			}
			if draftID > 0 {
				if err := quotation.SetDraftContext(draftID); err != nil {
					return err
				}
			}
			// ExportTo is the desktop's own export, minus the save dialog: the
			// same request, the same local render, the same attach. Sharing it
			// is what keeps a scripted document identical to a clicked one.
			if err := quotation.ExportTo(out); err != nil {
				return err
			}
			verifyURL := quotation.VerifyURL()
			shortID := shortIDFromVerifyURL(verifyURL)
			if err := a.emitJSON(map[string]any{
				"out":          out,
				"quoteNumber":  quotation.Data.QuoteNumber,
				"revision":     quotation.Data.RevisionID,
				"verifyUrl":    verifyURL,
				"shortId":      shortID,
				"grandTotal":   quotation.Data.DisplayGrandTotal,
				"customerCode": doc.CustomerCode,
			}); err != nil {
				return err
			}
			a.field("Quotation", quotation.Data.QuoteNumber)
			a.field("Revision", fmt.Sprintf("%02d", quotation.Data.RevisionID))
			a.field("Grand total", quotation.Data.DisplayGrandTotal)
			a.field("PDF", out)
			if shortID != "" {
				a.field("Verify", verifyURL)
				a.note("Check it with: vera verify %s", shortID)
			}
			return nil
		},
	}
}

// shortIDFromVerifyURL pulls the code out of the printed verification address,
// which is the same value a customer reads off the document.
func shortIDFromVerifyURL(verifyURL string) string {
	for i := len(verifyURL) - 1; i >= 0; i-- {
		if verifyURL[i] == '/' {
			return verifyURL[i+1:]
		}
	}
	return verifyURL
}

func quotationsNextNumberCommand() *command {
	var customer int
	return &command{
		name:    "next-number",
		summary: "Preview the next quote number for a client, without allocating it.",
		usage:   "vera quotations next-number --client 2",
		flags: func(fs *flag.FlagSet) {
			fs.IntVar(&customer, "client", 0, "the client code (required)")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if customer < 1 || customer > 99 {
				return usagef(a.cmd, "--client must be a client code between 1 and 99")
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var response struct {
				Number string `json:"number"`
			}
			if err := client.Get(fmt.Sprintf("/v1/quotations/next-sequence?customerId=%d", customer), &response); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"customerCode": customer, "quoteNumber": response.Number}); err != nil {
				return err
			}
			a.line("%s", response.Number)
			a.note("Peeking does not consume it: the next export still gets this number.")
			return nil
		},
	}
}

// QuotationSummary is one archive row.
type QuotationSummary struct {
	ID             int64  `json:"id"`
	QuoteNumber    string `json:"quoteNumber"`
	CustomerName   string `json:"customerName"`
	Subject        string `json:"subject"`
	DisplayGrand   string `json:"displayGrand"`
	GrandTotal     int64  `json:"grandTotal"`
	RevisionNumber int16  `json:"revision"`
	CreatedAt      string `json:"createdAt"`
}

func quotationsListCommand() *command {
	var limit int
	var customer string
	return &command{
		name:    "list",
		summary: "List filed quotations, newest first.",
		usage:   "vera quotations list [--limit 20] [--customer N]",
		flags: func(fs *flag.FlagSet) {
			fs.IntVar(&limit, "limit", 20, "how many to show (0 for all)")
			fs.StringVar(&customer, "customer", "", "only quotations for this client name or quote-number prefix")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var response struct {
				Quotations []QuotationSummary `json:"quotations"`
			}
			if err := client.Get("/v1/quotations", &response); err != nil {
				return err
			}
			rows := response.Quotations
			if customer != "" {
				rows = filterQuotations(rows, customer)
			}
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			if err := a.emitJSON(map[string]any{"quotations": rows, "count": len(rows)}); err != nil {
				return err
			}
			if len(rows) == 0 {
				a.line("No quotations.")
				return nil
			}
			table := make([][]string, 0, len(rows))
			for _, row := range rows {
				table = append(table, []string{
					fmt.Sprintf("%d", row.ID),
					row.QuoteNumber,
					fmt.Sprintf("%02d", row.RevisionNumber),
					row.CustomerName,
					row.DisplayGrand,
					truncate(row.Subject, 40),
				})
			}
			a.table([]string{"ID", "NUMBER", "REV", "CLIENT", "GRAND TOTAL", "SUBJECT"}, table)
			return nil
		},
	}
}

func filterQuotations(rows []QuotationSummary, needle string) []QuotationSummary {
	var out []QuotationSummary
	for _, row := range rows {
		if containsFold(row.CustomerName, needle) || containsFold(row.QuoteNumber, needle) || containsFold(row.Subject, needle) {
			out = append(out, row)
		}
	}
	return out
}

func quotationsShowCommand() *command {
	var revision int
	var asHTML bool
	return &command{
		name:    "show",
		summary: "Show one filed quotation, or one of its revisions.",
		usage:   "vera quotations show <id> [--revision N] [--html]",
		flags: func(fs *flag.FlagSet) {
			fs.IntVar(&revision, "revision", -1, "a specific revision (default: the latest)")
			fs.BoolVar(&asHTML, "html", false, "print the stored customer document as HTML")
		},
		run: func(a *app) error {
			if err := a.requireArgs(1, "a quotation id (see 'vera quotations list')"); err != nil {
				return err
			}
			id, err := a.parseID("quotation id", a.args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/v1/quotations/%d", id)
			if revision >= 0 {
				path += fmt.Sprintf("?revision=%d", revision)
			}
			var detail map[string]any
			if err := client.Get(path, &detail); err != nil {
				return err
			}
			if asHTML {
				// The stored snapshot is the document as issued: printing it
				// verbatim is how you see exactly what the customer saw.
				html, _ := detail["htmlSnapshot"].(string)
				if html == "" {
					return fmt.Errorf("this revision has no stored document")
				}
				return a.writeAll([]byte(html))
			}
			if err := a.emitJSON(detail); err != nil {
				return err
			}
			a.field("Number", stringField(detail, "quoteNumber"))
			a.field("Revision", stringField(detail, "revision"))
			a.field("Customer", stringField(detail, "customerName"))
			a.field("Subject", stringField(detail, "subject"))
			a.field("Grand total", stringField(detail, "displayGrand"))
			a.field("Issued", stringField(detail, "createdAt"))
			return nil
		},
	}
}

func quotationsRevisionsCommand() *command {
	return &command{
		name:    "revisions",
		summary: "List the revisions of one quotation.",
		usage:   "vera quotations revisions <id>",
		run: func(a *app) error {
			if err := a.requireArgs(1, "a quotation id"); err != nil {
				return err
			}
			id, err := a.parseID("quotation id", a.args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var response struct {
				Revisions []int16 `json:"revisions"`
			}
			if err := client.Get(fmt.Sprintf("/v1/quotations/%d/revisions", id), &response); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"id": id, "revisions": response.Revisions}); err != nil {
				return err
			}
			if len(response.Revisions) == 0 {
				a.line("No revisions.")
				return nil
			}
			rows := make([][]string, 0, len(response.Revisions))
			for _, rev := range response.Revisions {
				rows = append(rows, []string{fmt.Sprintf("%02d", rev)})
			}
			a.table([]string{"REVISION"}, rows)
			a.note("Each revision is immutable: an edit adds the next one rather than changing this.")
			return nil
		},
	}
}

func quotationsAttachCommand() *command {
	var pdf string
	return &command{
		name:    "attach",
		summary: "Attach a PDF to an existing revision (when the file was rendered elsewhere).",
		usage:   "vera quotations attach <id> <revision> --pdf quote.pdf",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&pdf, "pdf", "", "the PDF file to attach (required)")
		},
		run: func(a *app) error {
			if err := a.requireArgs(2, "a quotation id and a revision number"); err != nil {
				return err
			}
			id, err := a.parseID("quotation id", a.args[0])
			if err != nil {
				return err
			}
			revision, err := a.parseID("revision", a.args[1])
			if err != nil {
				return err
			}
			if pdf == "" {
				return usagef(a.cmd, "--pdf is required")
			}
			data, err := os.ReadFile(pdf)
			if err != nil {
				return fmt.Errorf("read %s: %w", pdf, err)
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var document struct {
				HasPDF bool `json:"hasPdf"`
			}
			if err := client.Put(
				fmt.Sprintf("/v1/quotations/%d/revisions/%d/document", id, revision),
				map[string]string{
					"pdfBase64": base64Std(data),
					"pdfPath":   pdf,
				}, &document); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"id": id, "revision": revision, "attached": true, "bytes": len(data)}); err != nil {
				return err
			}
			a.line("Attached %s to revision %02d of quotation %d.", pdf, revision, id)
			return nil
		},
	}
}

func quotationsRemoveCommand() *command {
	return &command{
		name:    "rm",
		summary: "Delete a quotation and every revision of it (admin).",
		usage:   "vera quotations rm <id>",
		run: func(a *app) error {
			if err := a.requireArgs(1, "a quotation id"); err != nil {
				return err
			}
			id, err := a.parseID("quotation id", a.args[0])
			if err != nil {
				return err
			}
			if err := a.confirm("Delete quotation %d and all its revisions? This cannot be undone.", id); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.Delete(fmt.Sprintf("/v1/quotations/%d", id), nil); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"removed": id}); err != nil {
				return err
			}
			a.line("Deleted quotation %d.", id)
			return nil
		},
	}
}

// --- small helpers the commands above share ---------------------------------

func (a *app) blank() {
	if a.asJSON || a.quiet {
		return
	}
	fmt.Fprintln(a.stdout)
}

// containsFold is a case-insensitive substring test for --customer filtering,
// which people type in whatever case the client name happens to be written.
func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// base64Std encodes the PDF for the attach request, which carries it in JSON.
func base64Std(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func writeFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func stringField(values map[string]any, key string) string {
	if v, ok := values[key]; ok {
		switch typed := v.(type) {
		case string:
			return typed
		case float64:
			return fmt.Sprintf("%.0f", typed)
		}
	}
	return ""
}
