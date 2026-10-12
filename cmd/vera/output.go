package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
)

// Output has two modes and they are both load-bearing.
//
// Human output is what makes the CLI pleasant to use by hand -- aligned columns,
// money already formatted, nothing to decode.
//
// --json is what makes it usable from a script or a test, and it is why every
// command has one: a test should assert on a field, not on the spacing of a
// table. The JSON is emitted from the same values the table is, so the two
// cannot disagree about what happened.

// emitJSON writes the machine-readable form. Commands call it instead of
// printing when --json is set, and then return.
func (a *app) emitJSON(v any) error {
	if !a.asJSON {
		return nil
	}
	encoder := json.NewEncoder(a.stdout)
	encoder.SetIndent("", "  ")
	// The API's field names are already a contract -- the desktop depends on
	// them -- so the CLI reuses them rather than inventing a second vocabulary
	// for the same values.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

// wantsJSON reports whether the caller asked for JSON, so a command can skip
// building a table it will not print.
func (a *app) wantsJSON() bool { return a.asJSON }

// table prints aligned columns.
func (a *app) table(headers []string, rows [][]string) {
	if a.asJSON || a.quiet {
		return
	}
	w := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	if len(headers) > 0 {
		fmt.Fprintln(w, strings.Join(headers, "\t"))
	}
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// line prints one line, unless --json or --quiet is in force.
func (a *app) line(format string, args ...any) {
	if a.asJSON || a.quiet {
		return
	}
	fmt.Fprintf(a.stdout, format+"\n", args...)
}

// field prints "label  value" aligned across the block of fields a command
// prints, which is the shape most single-object commands want.
func (a *app) field(label, value string) {
	if a.asJSON || a.quiet {
		return
	}
	fmt.Fprintf(a.stdout, "%-14s %s\n", label+":", value)
}

// note prints progress or commentary to stderr, so it never contaminates the
// output a script is capturing.
//
// It survives --json on purpose: stderr is not where the JSON goes, and these
// lines are how a scripted run explains itself in a log afterwards. --quiet is
// the flag for wanting none of it.
func (a *app) note(format string, args ...any) {
	if a.quiet {
		return
	}
	fmt.Fprintf(a.stderr, format+"\n", args...)
}

// warn prints a warning to stderr that --quiet does not silence: it is not
// progress, it is something the caller should know about.
func (a *app) warn(format string, args ...any) {
	fmt.Fprintf(a.stderr, "vera: "+format+"\n", args...)
}

// truncate shortens a long description to a column's worth, marking that it did.
// Descriptions in a quotation are prose; a list view needs them short without
// pretending the short form is the whole thing.
func truncate(text string, limit int) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "…"
}

// writeAll is the one place that writes to stdout from a helper, kept for the
// few cases that need raw output (a PDF's bytes never go here -- see the export
// command, which writes a file).
func (a *app) writeAll(p []byte) error {
	if _, err := a.stdout.Write(p); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
