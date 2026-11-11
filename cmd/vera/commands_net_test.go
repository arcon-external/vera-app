package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNetDiagnoseJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	code, stdout, stderr := runCLI(t, "net", "diagnose", "--api", server.URL, "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var report struct {
		OK      bool    `json:"ok"`
		Status  int     `json:"status"`
		TotalMS float64 `json:"totalMs"`
		Stages  []struct {
			Name string `json:"name"`
		} `json:"stages"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout)
	}
	if !report.OK {
		t.Fatalf("report is not OK: %s", stdout)
	}
	if report.Status != http.StatusOK {
		t.Errorf("status = %d; want 200", report.Status)
	}
	if report.TotalMS <= 0 {
		t.Error("total duration was not measured")
	}
	if len(report.Stages) == 0 {
		t.Error("no stages were reported")
	}
}

func TestNetDiagnoseFailureStillExitsZero(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"net", "diagnose",
		"--api", "http://127.0.0.1:1",
		"--timeout", "2s",
		"--json",
	)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var report struct {
		OK         bool   `json:"ok"`
		ErrorStage string `json:"errorStage"`
		Error      string `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout)
	}
	if report.OK {
		t.Fatal("a closed port was diagnosed as reachable")
	}
	if report.ErrorStage != "connect" {
		t.Errorf("error stage = %q; want connect", report.ErrorStage)
	}
	if report.Error == "" {
		t.Error("the failed report carries no error")
	}
}
