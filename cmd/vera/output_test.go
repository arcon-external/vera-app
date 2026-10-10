package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestMoneyGroupsThousandsWithDots(t *testing.T) {
	// The documents print rupiah this way, so a table that grouped differently
	// would read as a different number to anyone scanning it.
	cases := map[int64]string{
		0:             "0",
		1:             "1",
		999:           "999",
		1000:          "1.000",
		15000:         "15.000",
		1234567:       "1.234.567",
		58881000:      "58.881.000",
		1000000000000: "1.000.000.000.000",
		-2500000:      "-2.500.000",
	}
	for amount, want := range cases {
		if got := money(amount); got != want {
			t.Errorf("money(%d) = %q; want %q", amount, got, want)
		}
	}
}

func TestTruncateCollapsesWhitespaceAndMarksTheCut(t *testing.T) {
	// Descriptions are prose that arrives with newlines and runs of spaces from
	// a paste; a table cell needs one line.
	if got := truncate("Control   panel\nreplacement", 40); got != "Control panel replacement" {
		t.Errorf("truncate = %q; want the whitespace collapsed", got)
	}
	got := truncate(strings.Repeat("x", 100), 10)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncate = %q; want it to mark the cut", got)
	}
	if len([]rune(got)) != 10 {
		t.Errorf("truncate = %q (%d runes); want 10", got, len([]rune(got)))
	}
	// A description of exactly the limit is not a truncation.
	if got := truncate("exactly ten", 11); got != "exactly ten" {
		t.Errorf("truncate = %q; want it left alone", got)
	}
}

func TestTableAlignsColumns(t *testing.T) {
	var out bytes.Buffer
	a := newApp(&out, &bytes.Buffer{})
	a.table([]string{"ID", "NAME"}, [][]string{{"1", "Short"}, {"22", "A much longer name"}})
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("table printed %d lines; want 3 (%q)", len(lines), out.String())
	}
	// Every row must put NAME in the same column, which is the whole point of
	// a table over a list of sentences.
	column := strings.Index(lines[0], "NAME")
	for i, line := range lines[1:] {
		if strings.Index(line, strings.Fields(line)[1]) != column {
			t.Errorf("row %d puts its second column at %d; want %d (%q)",
				i, strings.Index(line, strings.Fields(line)[1]), column, line)
		}
	}
}

func TestJSONModeSilencesTheTable(t *testing.T) {
	var out bytes.Buffer
	a := newApp(&out, &bytes.Buffer{})
	a.asJSON = true
	a.table([]string{"ID"}, [][]string{{"1"}})
	a.line("human text")
	a.field("Label", "value")
	if out.Len() != 0 {
		t.Errorf("--json still printed human output: %q", out.String())
	}
}

func TestNotesGoToStderrSoTheyCannotCorruptJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	a := newApp(&stdout, &stderr)
	a.asJSON = true
	a.note("working…")
	if stdout.Len() != 0 {
		t.Errorf("a note reached stdout, where a script is reading JSON: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "working") {
		t.Errorf("stderr = %q; want the note", stderr.String())
	}
}

func TestWarningsSurviveQuiet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	a := newApp(&stdout, &stderr)
	a.quiet = true
	a.warn("something you should know")
	// --quiet silences progress, not problems: a warning that vanishes is a
	// warning nobody reads.
	if !strings.Contains(stderr.String(), "something you should know") {
		t.Errorf("stderr = %q; --quiet swallowed a warning", stderr.String())
	}
}

func TestSessionFileIsOwnerOnly(t *testing.T) {
	isolate(t)
	if err := SaveSession(&Session{API: "https://example.test", Token: "t", Email: "a@b.test"}); err != nil {
		t.Fatal(err)
	}
	path, err := sessionPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// A bearer token is a credential: anyone else on the machine must not be
	// able to read it.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session file mode = %o; want 600", perm)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	isolate(t)
	want := &Session{API: "https://example.test", Token: "tok", Email: "a@b.test", Role: "admin"}
	if err := SaveSession(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("LoadSession returned nil after a save")
	}
	if got.Token != want.Token || got.Email != want.Email || got.API != want.API || got.Role != want.Role {
		t.Errorf("round trip lost data: got %+v, want %+v", got, want)
	}
}

func TestLoadSessionWithoutAFileIsNotAnError(t *testing.T) {
	isolate(t)
	got, err := LoadSession()
	if err != nil {
		t.Fatalf("a missing session file was reported as an error: %v", err)
	}
	if got != nil {
		t.Errorf("got %+v; want nil", got)
	}
}

func TestCorruptSessionFileSaysSo(t *testing.T) {
	dir := isolate(t)
	seed := dir + "/vera"
	if err := os.MkdirAll(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seed+"/session.json", []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSession()
	if err == nil {
		t.Fatal("a corrupt session file was accepted")
	}
	// Silently treating this as "signed out" would send someone hunting for a
	// password problem that is really a permissions or corruption problem.
	if !strings.Contains(err.Error(), "session.json") {
		t.Errorf("error = %q; want it to name the file", err)
	}
}

func TestClearSessionIsIdempotent(t *testing.T) {
	isolate(t)
	if err := ClearSession(); err != nil {
		t.Fatalf("clearing a session that does not exist failed: %v", err)
	}
	if err := SaveSession(&Session{Token: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearSession(); err != nil {
		t.Fatal(err)
	}
	if err := ClearSession(); err != nil {
		t.Fatalf("clearing twice failed: %v", err)
	}
	if session, _ := LoadSession(); session != nil {
		t.Errorf("the session survived a clear: %+v", session)
	}
}
