package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"vera-app/backend/apiclient"
)

// netCommands are the diagnostics that answer "can this machine reach the
// server, and if not, which step failed?" They are deliberately separate from
// the account commands: a diagnosis needs no session.
func netCommands() []*command {
	return []*command{
		commandGroup("net", "Network diagnostics for the API connection.",
			netDiagnoseCommand(),
		),
	}
}

func netDiagnoseCommand() *command {
	var requests int
	return &command{
		name:    "diagnose",
		summary: "Measure the API connection stage by stage.",
		usage:   "vera net diagnose [--requests N]",
		flags: func(fs *flag.FlagSet) {
			fs.IntVar(&requests, "requests", 2, "probe requests; the second checks connection reuse")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if requests < 1 {
				return usagef(a.cmd, "--requests must be at least 1")
			}

			base := a.apiBase()
			if err := checkTransport(base, a.insecure); err != nil {
				return err
			}
			timeout := a.timeout
			if timeout <= 0 {
				timeout = 15 * time.Second
			}

			report := apiclient.Diagnose(context.Background(), base, apiclient.DiagnoseOptions{
				Timeout:  timeout,
				Requests: requests,
			})
			if a.wantsJSON() {
				return a.emitJSON(report)
			}

			a.field("API", report.URL)
			rows := make([][]string, 0, len(report.Stages))
			for _, stage := range report.Stages {
				detail := strings.TrimSpace(stage.Detail)
				if stage.Error != "" {
					if detail != "" {
						detail += " "
					}
					detail += "(" + stage.Error + ")"
				}
				rows = append(rows, []string{stage.Name, fmt.Sprintf("%.1f ms", stage.DurationMS), detail})
			}
			a.table([]string{"STAGE", "TIME", "DETAIL"}, rows)

			if report.Status != 0 {
				a.field("Status", fmt.Sprintf("%d %s", report.Status, report.HTTPVersion))
			}
			if report.RemoteAddr != "" {
				a.field("Remote", report.RemoteAddr)
			}
			if requests > 1 {
				switch {
				case report.Reused:
					a.field("Reuse", fmt.Sprintf("reused in %.1f ms", report.ReuseMS))
				case report.ReuseError != "":
					a.field("Reuse", "failed: "+report.ReuseError)
				default:
					a.field("Reuse", "not reused")
				}
			}

			a.line("")
			if !report.OK {
				a.line("FAILED at %s: %s", report.ErrorStage, report.Error)
				return nil
			}
			if report.Status >= 500 {
				a.line("REACHED, but the server returned %d", report.Status)
				return nil
			}
			a.line("OK: reached the API in %.1f ms", report.TotalMS)
			return nil
		},
	}
}
