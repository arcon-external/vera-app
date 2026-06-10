package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePDF(t *testing.T) {
	ctx := context.Background()
	q := &Quotation{}

	err := Startup(q, ctx)
	if err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	tempPath := filepath.Join(t.TempDir(), "quotation.pdf")
	err = q.save(tempPath)
	if err != nil {
		t.Fatalf("Failed to generate and save PDF: %v", err)
	}

	info, err := os.Stat(tempPath)
	if os.IsNotExist(err) {
		t.Fatal("PDF file was not created")
	}
	if info.Size() == 0 {
		t.Fatal("PDF file is empty")
	}
}

func TestValidateSavePathRejectsNonPDF(t *testing.T) {
	tempDir := t.TempDir()
	_, err := validateSavePath(filepath.Join(tempDir, "quotation.txt"))
	if err == nil {
		t.Fatal("expected non-pdf path to be rejected")
	}
}

func TestValidateSavePathRejectsDirectory(t *testing.T) {
	tempDir := t.TempDir()
	_, err := validateSavePath(tempDir)
	if err == nil {
		t.Fatal("expected directory path to be rejected")
	}
}

func TestValidateSavePathAcceptsPDF(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "quotation.pdf")
	absPath, err := validateSavePath(inputPath)
	if err != nil {
		t.Fatalf("expected valid pdf path, got error: %v", err)
	}

	expectedPath, err := filepath.Abs(inputPath)
	if err != nil {
		t.Fatalf("resolve expected absolute path: %v", err)
	}

	if absPath != expectedPath {
		t.Fatalf("unexpected validated path: got %q want %q", absPath, expectedPath)
	}
}

func TestFormatRupiah(t *testing.T) {
	cases := []struct {
		amount uint64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.000"},
		{50000, "50.000"},
		{1000000, "1.000.000"},
		{115725000, "115.725.000"},
		{999999999, "999.999.999"},
		{1000000000, "1.000.000.000"},
	}

	for _, tc := range cases {
		got := formatRupiah(tc.amount)
		if got != tc.want {
			t.Errorf("formatRupiah(%d) = %q; want %q", tc.amount, got, tc.want)
		}
	}
}

func TestFormatDescription(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "plain single line",
			raw:  "Install lighting system",
			want: `<div class="desc-main">Install lighting system</div>`,
		},
		{
			name: "plain multi-line",
			raw:  "Line one\nLine two",
			want: `<div class="desc-main">Line one Line two</div>`,
		},
		{
			name: "description with bullets",
			raw:  "Main scope\n- sensor 1\n- sensor 2",
			want: `<div class="desc-main">Main scope</div><ul class="desc-subitems"><li>sensor 1</li><li>sensor 2</li></ul>`,
		},
		{
			name: "bullets only",
			raw:  "- item a\n- item b",
			want: `<ul class="desc-subitems"><li>item a</li><li>item b</li></ul>`,
		},
		{
			name: "empty string",
			raw:  "",
			want: "", // html.EscapeString("") = ""
		},
		{
			name: "windows line endings",
			raw:  "Scope\r\n- detail 1\r\n- detail 2",
			want: `<div class="desc-main">Scope</div><ul class="desc-subitems"><li>detail 1</li><li>detail 2</li></ul>`,
		},
		{
			name: "dangling dash treated as text",
			raw:  "- \n- valid item\n-  \n",
			want: `<div class="desc-main">- -</div><ul class="desc-subitems"><li>valid item</li></ul>`,
		},
		{
			name: "blank lines ignored",
			raw:  "\n\nMain text\n\n- bullet\n\n",
			want: `<div class="desc-main">Main text</div><ul class="desc-subitems"><li>bullet</li></ul>`,
		},
		{
			name: "leading dash without space is not a bullet",
			raw:  "Regular text - not a bullet",
			want: `<div class="desc-main">Regular text - not a bullet</div>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(formatDescription(tc.raw))
			if got != tc.want {
				t.Errorf("formatDescription(%q)\n  got: %q\n want: %q", tc.raw, got, tc.want)
			}
		})
	}
}
