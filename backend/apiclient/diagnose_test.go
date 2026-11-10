package apiclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func diagnoseOptions() DiagnoseOptions {
	return DiagnoseOptions{
		Timeout:       5 * time.Second,
		DialTimeout:   2 * time.Second,
		TLSTimeout:    2 * time.Second,
		HeaderTimeout: 2 * time.Second,
		Requests:      2,
	}
}

func stageNamed(report DiagnoseReport, name string) (DiagnoseStage, bool) {
	for _, stage := range report.Stages {
		if stage.Name == name {
			return stage, true
		}
	}
	return DiagnoseStage{}, false
}

func TestDiagnoseHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	report := Diagnose(context.Background(), server.URL, diagnoseOptions())
	if !report.OK {
		t.Fatalf("diagnosis failed: stage %q: %v", report.ErrorStage, report.Error)
	}
	if report.Status != http.StatusOK {
		t.Errorf("status = %d; want 200", report.Status)
	}
	if report.TotalMS <= 0 {
		t.Error("total duration was not measured")
	}
	if report.RemoteAddr == "" {
		t.Error("remote address was not recorded")
	}
	if _, ok := stageNamed(report, "connect"); !ok {
		t.Error("no connect stage was recorded")
	}
	if _, ok := stageNamed(report, "total"); !ok {
		t.Error("no total stage was recorded")
	}
	if !report.Reused {
		t.Error("the second probe did not reuse the connection")
	}
}

func TestDiagnoseTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	opts := diagnoseOptions()
	opts.TLSConfig = &tls.Config{InsecureSkipVerify: true} // test-only self-signed server
	report := Diagnose(context.Background(), server.URL, opts)
	if !report.OK {
		t.Fatalf("TLS diagnosis failed: stage %q: %v", report.ErrorStage, report.Error)
	}
	if report.TLSMS <= 0 {
		t.Error("TLS duration was not measured")
	}
	if _, ok := stageNamed(report, "tls"); !ok {
		t.Error("no TLS stage was recorded")
	}
}

func TestDiagnoseHeaderTimeoutNamesTheStage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(250 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer server.Close()

	opts := diagnoseOptions()
	opts.HeaderTimeout = 50 * time.Millisecond
	opts.Timeout = time.Second
	report := Diagnose(context.Background(), server.URL, opts)
	if report.OK {
		t.Fatal("diagnosis succeeded despite the header timeout")
	}
	if report.ErrorStage != "header" {
		t.Errorf("error stage = %q; want header", report.ErrorStage)
	}
	if !strings.Contains(strings.ToLower(report.Error), "timeout") {
		t.Errorf("error %q does not name the timeout", report.Error)
	}
}

func TestDiagnoseConnectFailureNamesTheStage(t *testing.T) {
	opts := diagnoseOptions()
	opts.Timeout = 2 * time.Second
	opts.DialTimeout = 500 * time.Millisecond
	report := Diagnose(context.Background(), "http://127.0.0.1:1", opts)
	if report.OK {
		t.Fatal("diagnosis succeeded against a closed port")
	}
	if report.ErrorStage != "connect" {
		t.Errorf("error stage = %q; want connect", report.ErrorStage)
	}
}

func TestDiagnoseDNSFailureNamesTheStage(t *testing.T) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dns unavailable")
		},
	}
	opts := diagnoseOptions()
	opts.Resolver = resolver
	report := Diagnose(context.Background(), "http://vera-diagnose.invalid", opts)
	if report.OK {
		t.Fatal("diagnosis succeeded without DNS")
	}
	if report.ErrorStage != "dns" {
		t.Errorf("error stage = %q; want dns", report.ErrorStage)
	}
}

func TestDiagnoseCanSkipTheReuseProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	opts := diagnoseOptions()
	opts.Requests = 1
	report := Diagnose(context.Background(), server.URL, opts)
	if !report.OK {
		t.Fatalf("diagnosis failed: stage %q: %v", report.ErrorStage, report.Error)
	}
	if report.Reused {
		t.Error("reuse was reported even though the second probe was skipped")
	}
}

func TestDiagnoseJSONShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	report := Diagnose(context.Background(), server.URL, diagnoseOptions())
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	for _, key := range []string{
		"url", "ok", "reused", "dnsMs", "connectMs", "tlsMs", "ttfbMs",
		"totalMs", "stages",
	} {
		if _, ok := document[key]; !ok {
			t.Errorf("report JSON has no %q", key)
		}
	}
	if _, ok := document["errorStage"]; ok {
		t.Error("a successful report carries errorStage")
	}
}

func TestDiagnoseEmptyURL(t *testing.T) {
	report := Diagnose(context.Background(), "", diagnoseOptions())
	if report.OK {
		t.Fatal("an empty URL was diagnosed as reachable")
	}
	if report.ErrorStage != "request" {
		t.Errorf("error stage = %q; want request", report.ErrorStage)
	}
}
