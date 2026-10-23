// Command veraseed fills a developer's confirmed local Postgres database with
// the fictional Quotation and Purchase Order set from issue #500.
//
// It is not part of veraserver: the deployed API never exposes it, and it has
// no HTTP route. It reads the server-owned VERA_POSTGRES_DSN (or the PG*
// fallback), refuses anything but one explicitly confirmed local database, and
// only then migrates and writes. See docs/local-seed.md.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"vera-app/backend/localseed"
	"vera-app/backend/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("veraseed", flag.ContinueOnError)
	flags.SetOutput(stderr)
	target := flags.String("target", "", "the resolved local database name, confirmed explicitly (required)")
	actorEmail := flags.String("actor-email", "", "an existing local administrator's email; defaults to the first active admin")
	jsonOutput := flags.Bool("json", false, "print the report as JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintln(stderr, "veraseed: unexpected arguments:", flags.Args())
		return 2
	}

	config, err := store.LoadConfigFromEnvironment()
	if err != nil {
		fmt.Fprintln(stderr, "veraseed:", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	report, runErr := localseed.Run(ctx, localseed.Options{
		Config:     config,
		Target:     *target,
		ActorEmail: *actorEmail,
	})
	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, "veraseed: print the report:", err)
			return 1
		}
	} else if report.Target != "" {
		printReport(stdout, report)
	}
	if runErr != nil {
		fmt.Fprintln(stderr, "veraseed:", runErr)
		return 1
	}
	return 0
}

func printReport(w io.Writer, report localseed.Report) {
	fmt.Fprintf(w, "Local mock seed: %s\n", report.Target)
	fmt.Fprintf(w, "Mode: %s; manifest v%d; actor %s\n", report.Mode, report.ManifestVersion, report.ActorEmail)
	fmt.Fprintf(w, "Live Quotations: %d/%d (%d occupied, %d available)\n",
		report.LiveQuotations, report.ExpectedQuotations, report.Occupied, report.Available)
	fmt.Fprintf(w, "Purchase Orders: %d/%d\n", report.PurchaseOrders, report.ExpectedPurchaseOrders)
	fmt.Fprintf(w, "Synthetic sources created and deleted: %d\n", report.SyntheticSourcesDeleted)
	for _, fixture := range report.Fixtures {
		fmt.Fprintf(w, "  %-16s %-8s %-40s %s\n", fixture.Kind, fixture.State, fixture.Key, fixture.Detail)
	}
	if len(report.Notes) > 0 {
		fmt.Fprintln(w, "Notes:")
		for _, note := range report.Notes {
			fmt.Fprintf(w, "  - %s\n", note)
		}
	}
}
