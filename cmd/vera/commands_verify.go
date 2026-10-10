package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// The public verification portal is the authority on whether a document is
// genuine, so `vera verify` asks it rather than re-implementing the check. That
// way the answer is exactly what a customer clicking the QR code would see.
//
// The page is HTML, and the verdict rides in one class name on one element:
//
//	<div class="verdict verdict-valid">
//
// That class is what the stylesheet keys off, so it is a real contract rather
// than incidental markup.
//
// The anchor on `class="verdict ..."` is load-bearing. The stylesheet defines
// every one of these names -- `.verdict-valid`, `.verdict-mismatch` and the rest
// -- so a pattern that merely looked for `verdict-<word>` anywhere in the page
// would match a CSS rule and report whatever class happened to be styled first.
// That is how a missing document gets reported as valid, which is the one
// failure this command must never have.
var verdictMarker = regexp.MustCompile(`class="verdict\s+verdict-(valid|superseded|mismatch|unavailable|notfound)\b`)

// parseVerdict reads the verdict out of a rendered page.
//
// It insists on exactly one match. Zero means the page is not the shape this
// parser knows; more than one means it cannot tell which is the real verdict.
// Both are reported as unknown, and unknown is never treated as valid.
func parseVerdict(page string) (string, error) {
	matches := verdictMarker.FindAllStringSubmatch(page, -1)
	switch len(matches) {
	case 1:
		return matches[0][1], nil
	case 0:
		return "", fmt.Errorf("the portal's page does not carry a verdict this tool recognises")
	default:
		return "", fmt.Errorf("the portal's page carries %d verdicts, so none can be trusted", len(matches))
	}
}

// verifyCommand checks a printed document's code against the public portal.
//
// No session: this is the same unauthenticated check the customer performs, and
// it working without credentials is the point of it.
func verifyCommand() *command {
	var number, amount string
	return &command{
		name:    "verify",
		summary: "Check a printed document's verification code, as a customer would.",
		usage:   "vera verify <short-id> [--number <quote-number>] [--amount <grand-total>]",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&number, "number", "", "also assert the quotation number printed on the document")
			fs.StringVar(&amount, "amount", "", "also assert the grand total printed on the document")
		},
		run: func(a *app) error {
			if err := a.requireArgs(1, "a verification code (the id printed beside the QR code)"); err != nil {
				return err
			}
			shortID := strings.TrimSpace(a.args[0])

			// No session on purpose: this is the same check a customer
			// performs, and it working without credentials is the point.
			client, err := a.anonClient()
			if err != nil {
				return err
			}

			// The portal takes a form post and answers with the page it would
			// have rendered. Sending it as a form keeps the CLI on the same
			// path a browser takes, including the rate limiter.
			form := map[string]string{"id": shortID}
			if number != "" {
				form["number"] = number
			}
			if amount != "" {
				form["amount"] = amount
			}
			body, err := client.PostForm("/verify", form)
			if err != nil {
				return err
			}

			verdict, err := parseVerdict(body)
			if err != nil {
				// Never guess. An unrecognised page is reported as unknown
				// rather than as valid, because the one outcome that must never
				// happen is a false all-clear.
				return fmt.Errorf("%w; check it by hand at %s/v/%s", err, a.apiBase(), shortID)
			}

			if err := a.emitJSON(map[string]any{
				"id":      shortID,
				"verdict": verdict,
				"valid":   verdict == "valid",
			}); err != nil {
				return err
			}

			// A verdict other than "valid" is a real answer, not a crash, and
			// it is reported through the exit code so a script can tell
			// "superseded" from "forged" without reading English. The message
			// is the whole human output for those cases -- printing the bare
			// word as well would say the same thing twice.
			switch verdict {
			case "valid":
				a.line("valid")
				return nil
			case "notfound":
				return &exitCoder{code: exitNotFound, message: fmt.Sprintf("no document matches %s", shortID)}
			case "mismatch":
				return &exitCoder{code: exitValidation, message: fmt.Sprintf("%s does not match the record: the printed figures or the code have been altered", shortID)}
			case "superseded":
				return &exitCoder{code: exitValidation, message: fmt.Sprintf("%s is genuine but a later revision replaced it", shortID)}
			default:
				return &exitCoder{code: exitServer, message: "the verification portal could not answer right now; try again shortly"}
			}
		},
	}
}

// exitCoder is a failure that already knows its exit code, for the cases where
// the code carries information the message cannot: `vera verify` reports
// superseded, forged and unknown separately, and all three arrive as a 200.
type exitCoder struct {
	code    int
	message string
}

func (e *exitCoder) Error() string { return e.message }

// readInputFile reads an input JSON document from a path, or from stdin when the
// path is "-".
//
// stdin matters more than it looks: it is what lets a test or another tool pipe
// a quotation straight in without the CLI having to own a temporary file, and it
// is the natural way to generate a payload with a script.
func readInputFile(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 8<<20))
		if err != nil {
			return nil, fmt.Errorf("read input from stdin: %w", err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("nothing on stdin")
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}
