package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vera-app/backend/extract"
)

// Issue #477/#478: the text-layer + selective-OCR extractor.
//
// stdin is a PDF; stdout is the extraction contract. Nothing else is read:
// no database DSN, no signing key, no path, no network. The binary is the only
// thing inside the restricted image, and the image pins both this build, the
// Poppler version it drives and the Tesseract version it OCRs with (see
// docker/po-extract/Dockerfile).
//
// Every page is decided on its own: a page with a usable text layer is read
// from that layer and never OCRed; an image-only or near-empty page is rendered
// at a bounded DPI and OCRed. A text header page therefore cannot hide a
// scanned table page.
//
// Exit 0 means stdout is a valid result. Exit 2 means stdout is empty and
// stderr starts with a typed prefix the server maps to a run error:
//
//	page_limit: ...       the document is longer than the page cap
//	ocr_page_limit: ...   too many pages need OCR
//	page_too_large: ...   a page cannot be rendered within the pixel bound
//	timeout: ...          an engine did not finish in time
//	invalid_output: ...   an engine's word map was unusable
//	invalid_pdf: ...      Poppler refused the document
//	invalid_input: ...    the request itself was unusable (empty/oversized)
//	ocr_unavailable: ...  the OCR engine is not installed
//	engine_failed: ...    an engine ran and failed
//
// The server stores the message from that line, so it is written for a staff
// member to read, never as a stack trace.
const (
	// EngineName is the text-layer engine this build drives.
	EngineName = "poppler"
	// OCREngineName is the OCR engine this build drives.
	OCREngineName = "tesseract"
	// Version is the extractor build's own version. It is separate from the
	// Poppler and Tesseract versions so a regression is attributable to one.
	Version = "0.2.0"

	maxInputBytes    = 20 << 20
	maxXMLBytes      = 32 << 20
	maxOCRImageBytes = 64 << 20
	maxOCRTSVBytes   = 8 << 20
	maxStderr        = 4 << 10

	// defaultTextTimeout bounds one pdftotext invocation. The server's own
	// wall-time bound is longer, so a slow Poppler produces the typed timeout
	// here rather than an opaque outer kill.
	defaultTextTimeout = 30 * time.Second
	// defaultOCRTimeout bounds all OCR pages together, and
	// defaultOCRPageTimeout bounds one page's render + OCR pair.
	defaultOCRTimeout     = 30 * time.Second
	defaultOCRPageTimeout = 15 * time.Second
)

func main() {
	if code := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("vextract", flag.ContinueOnError)
	flags.SetOutput(stderr)
	maxPages := flags.Int("max-pages", pageLimitFromEnvironment(), "maximum pages to read")
	pdfToText := flags.String("pdftotext", "pdftotext", "pdftotext executable")
	pdfToPPM := flags.String("pdftoppm", "pdftoppm", "pdftoppm executable")
	tesseract := flags.String("tesseract", "tesseract", "tesseract executable")
	ocrDPI := flags.Int("ocr-dpi", ocrDPIfromEnvironment(), "render resolution for OCR pages")
	ocrLanguages := flags.String("ocr-langs", ocrLanguagesFromEnvironment(), "Tesseract language data")
	maxOCRPages := flags.Int("max-ocr-pages", maxOCRPagesFromEnvironment(), "maximum pages to OCR")
	maxOCRPixels := flags.Int("max-ocr-pixels", extract.DefaultMaxOCRPixels, "maximum pixels per OCR page")
	minConfidence := flags.Float64("min-confidence", extract.DefaultMinConfidence, "OCR confidence floor")
	ocrTimeout := flags.Duration("ocr-timeout", defaultOCRTimeout, "maximum time for all OCR pages")
	ocrPageTimeout := flags.Duration("ocr-page-timeout", defaultOCRPageTimeout, "maximum time for one OCR page")
	textTimeout := flags.Duration("timeout", defaultTextTimeout, "maximum time for one text-engine run")
	showVersion := flags.Bool("version", false, "print the engine versions and exit")
	if err := flags.Parse(args); err != nil {
		writeFailure(stderr, "invalid_input", err)
		return 2
	}

	textVersion := detectEngineVersion(ctx, *pdfToText)
	ocrVersion := detectTesseractVersion(ctx, *tesseract)
	if *showVersion {
		fmt.Fprintf(stdout, "vextract %s (%s %s; OCR %s %s)\n",
			Version, EngineName, textVersion, OCREngineName, ocrVersion)
		return 0
	}
	if *maxPages < 1 || *maxPages > 200 {
		writeFailure(stderr, "invalid_input", fmt.Errorf("page cap %d is outside 1..200", *maxPages))
		return 2
	}
	if *ocrDPI < extract.MinOCRDpi || *ocrDPI > extract.MaxOCRDpi {
		writeFailure(stderr, "invalid_input", fmt.Errorf(
			"ocr-dpi %d is outside %d..%d", *ocrDPI, extract.MinOCRDpi, extract.MaxOCRDpi))
		return 2
	}
	if *maxOCRPages < 1 || *maxOCRPages > 200 {
		writeFailure(stderr, "invalid_input", fmt.Errorf("max-ocr-pages %d is outside 1..200", *maxOCRPages))
		return 2
	}
	if *maxOCRPixels < 1 {
		writeFailure(stderr, "invalid_input", errors.New("max-ocr-pixels must be positive"))
		return 2
	}
	if *minConfidence < 0 || *minConfidence > 100 {
		writeFailure(stderr, "invalid_input", fmt.Errorf("min-confidence %.1f is outside 0..100", *minConfidence))
		return 2
	}
	if strings.TrimSpace(*ocrLanguages) == "" {
		writeFailure(stderr, "invalid_input", errors.New("ocr-langs must name at least one language"))
		return 2
	}

	pdf, err := readBounded(stdin, maxInputBytes)
	if err != nil {
		writeFailure(stderr, "invalid_input", err)
		return 2
	}
	if len(pdf) == 0 {
		writeFailure(stderr, "invalid_input", errors.New("no PDF was provided on standard input"))
		return 2
	}

	started := time.Now()
	pipeline := &extract.Pipeline{
		Text: popplerTextLayer{executable: *pdfToText, timeout: *textTimeout},
		OCR: tesseractEngine{
			renderer:    *pdfToPPM,
			tesseract:   *tesseract,
			languages:   *ocrLanguages,
			version:     ocrVersion,
			pageTimeout: *ocrPageTimeout,
			maxPixels:   *maxOCRPixels,
		},
		Options: extract.PipelineOptions{
			MaxPages:      *maxPages,
			OCRDpi:        *ocrDPI,
			MaxOCRPixels:  *maxOCRPixels,
			MaxOCRPages:   *maxOCRPages,
			MinConfidence: *minConfidence,
			OCRTimeout:    *ocrTimeout,
		},
	}
	result, err := pipeline.Run(ctx, pdf)
	if err != nil {
		writeTypedFailure(stderr, err)
		return 2
	}
	result.Engine = EngineName
	result.EngineVersion = textVersion
	result.DurationMs = time.Since(started).Milliseconds()
	if err := extract.ValidateExtractionOutput(&result, *maxPages); err != nil {
		writeTypedFailure(stderr, err)
		return 2
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		writeFailure(stderr, "invalid_output", fmt.Errorf("write the result: %w", err))
		return 2
	}
	return 0
}

// popplerTextLayer reads the text layer through pdftotext's word map.
type popplerTextLayer struct {
	executable string
	timeout    time.Duration
}

func (layer popplerTextLayer) TextPages(ctx context.Context, pdf []byte, maxPages int) ([]extract.Page, error) {
	xml, err := runPDFToText(ctx, layer.executable, pdf, maxPages+1, layer.timeout)
	if err != nil {
		return nil, err
	}
	return extract.ParseBBoxLayout(xml)
}

// tesseractEngine renders one page and OCRs it. Rendering and OCR are separate
// bounded subprocesses; the page image and the PDF go to the container's
// read-only-root tmpfs and are removed before the call returns.
type tesseractEngine struct {
	renderer    string
	tesseract   string
	languages   string
	version     string
	pageTimeout time.Duration
	maxPixels   int
}

func (engine tesseractEngine) Name() string    { return OCREngineName }
func (engine tesseractEngine) Version() string { return engine.version }

func (engine tesseractEngine) OCRPage(ctx context.Context, pdf []byte, page extract.Page, dpi int) ([]extract.Word, error) {
	timeout := engine.pageTimeout
	if timeout <= 0 {
		timeout = defaultOCRPageTimeout
	}
	pageContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	directory, err := os.MkdirTemp("", "vextract-ocr-")
	if err != nil {
		return nil, &typedFailure{Code: "invalid_input", Err: fmt.Errorf("prepare the OCR workspace: %w", err)}
	}
	defer os.RemoveAll(directory)
	input := filepath.Join(directory, "input.pdf")
	if err := os.WriteFile(input, pdf, 0o600); err != nil {
		return nil, &typedFailure{Code: "invalid_input", Err: fmt.Errorf("stage the PDF for rendering: %w", err)}
	}
	prefix := filepath.Join(directory, "page")
	number := strconv.Itoa(page.Number)
	if _, err := runBounded(pageContext, commandSpec{
		executable:  engine.renderer,
		args:        []string{"-r", strconv.Itoa(dpi), "-f", number, "-l", number, "-png", "-singlefile", input, prefix},
		label:       "the PDF renderer",
		missingCode: "invalid_input",
		failureCode: "invalid_pdf",
		maxOutput:   maxStderr,
	}); err != nil {
		return nil, err
	}
	imagePath := prefix + ".png"
	info, err := os.Stat(imagePath)
	if err != nil {
		return nil, &typedFailure{Code: "invalid_output", Err: errors.New("the PDF renderer produced no page image")}
	}
	if info.Size() > maxOCRImageBytes {
		return nil, &typedFailure{Code: "page_too_large", Err: fmt.Errorf(
			"the rendered page is larger than %d MB", maxOCRImageBytes>>20)}
	}
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, &typedFailure{Code: "invalid_output", Err: fmt.Errorf("read the rendered page: %w", err)}
	}
	config, decodeErr := png.DecodeConfig(file)
	_ = file.Close()
	if decodeErr != nil {
		return nil, &typedFailure{Code: "invalid_output", Err: fmt.Errorf("the rendered page is not a readable image: %w", decodeErr)}
	}
	maxPixels := engine.maxPixels
	if maxPixels <= 0 {
		maxPixels = extract.DefaultMaxOCRPixels
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > int64(maxPixels) {
		return nil, &typedFailure{Code: "page_too_large", Err: fmt.Errorf(
			"the rendered page is %dx%d pixels, above the %d pixel bound", config.Width, config.Height, maxPixels)}
	}
	tsv, err := runBounded(pageContext, commandSpec{
		executable:  engine.tesseract,
		args:        []string{imagePath, "stdout", "-l", engine.languages, "tsv"},
		label:       "the OCR engine",
		missingCode: "ocr_unavailable",
		failureCode: "engine_failed",
		maxOutput:   maxOCRTSVBytes,
	})
	if err != nil {
		return nil, err
	}
	words, err := extract.ParseOCRTSV(tsv, dpi)
	if err != nil {
		return nil, &typedFailure{Code: "invalid_output", Err: err}
	}
	return words, nil
}

// commandSpec describes one bounded child process. The failure code differs by
// tool: a missing OCR engine is a deployment answer, while a Poppler refusal is
// a document answer.
type commandSpec struct {
	executable  string
	args        []string
	label       string
	missingCode string
	failureCode string
	maxOutput   int
}

// runBounded runs a command with a capped stdout/stderr, kills its process
// group on cancellation, and returns typed failures.
func runBounded(ctx context.Context, spec commandSpec) ([]byte, error) {
	command := exec.CommandContext(ctx, spec.executable, spec.args...)
	stdout := &cappedBuffer{limit: spec.maxOutput}
	stderr := &cappedBuffer{limit: maxStderr}
	command.Stdout = stdout
	command.Stderr = stderr
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return nil, &typedFailure{Code: spec.missingCode, Err: fmt.Errorf("%s %q is not installed", spec.label, spec.executable)}
		}
		return nil, &typedFailure{Code: "invalid_input", Err: fmt.Errorf("start %s: %w", spec.label, err)}
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case <-ctx.Done():
		killProcessGroup(command)
		<-done
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &typedFailure{Code: "timeout", Err: fmt.Errorf("%s did not finish in time", spec.label)}
		}
		return nil, &typedFailure{Code: "invalid_input", Err: ctx.Err()}
	case waitErr := <-done:
		if stdout.overflow {
			return nil, &typedFailure{Code: "invalid_output", Err: fmt.Errorf("%s returned more output than this reader accepts", spec.label)}
		}
		if waitErr != nil {
			detail := firstLineOf(stderr.String())
			if detail == "" {
				detail = waitErr.Error()
			}
			return nil, &typedFailure{Code: spec.failureCode, Err: errors.New(detail)}
		}
		return stdout.Bytes(), nil
	}
}

func pageLimitFromEnvironment() int {
	return intFromEnvironment("VERA_EXTRACT_MAX_PAGES", extract.DefaultMaxPages)
}

func ocrDPIfromEnvironment() int {
	return intFromEnvironment("VERA_EXTRACT_OCR_DPI", extract.DefaultOCRDpi)
}

func maxOCRPagesFromEnvironment() int {
	return intFromEnvironment("VERA_EXTRACT_MAX_OCR_PAGES", extract.DefaultMaxOCRPages)
}

func ocrLanguagesFromEnvironment() string {
	if raw := strings.TrimSpace(os.Getenv("VERA_EXTRACT_OCR_LANGS")); raw != "" {
		return raw
	}
	return "eng+ind"
}

func intFromEnvironment(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(reader, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read the PDF: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("the PDF is larger than %d MB", limit>>20)
	}
	return data, nil
}

type typedFailure struct {
	Code string
	Err  error
}

func (failure *typedFailure) Error() string { return failure.Err.Error() }
func (failure *typedFailure) Unwrap() error { return failure.Err }

func runPDFToText(ctx context.Context, executable string, pdf []byte, lastPage int, timeout time.Duration) ([]byte, error) {
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runContext, executable,
		"-bbox-layout", "-enc", "UTF-8", "-l", strconv.Itoa(lastPage), "-", "-")
	command.Stdin = bytes.NewReader(pdf)
	stdout := &cappedBuffer{limit: maxXMLBytes}
	stderr := &cappedBuffer{limit: maxStderr}
	command.Stdout = stdout
	command.Stderr = stderr
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, &typedFailure{Code: "invalid_input", Err: fmt.Errorf("the text engine %q is not installed", executable)}
		}
		return nil, &typedFailure{Code: "invalid_input", Err: fmt.Errorf("start the text engine: %w", err)}
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case <-runContext.Done():
		killProcessGroup(command)
		<-done
		if errors.Is(runContext.Err(), context.DeadlineExceeded) {
			return nil, &typedFailure{Code: "timeout", Err: fmt.Errorf("the text engine did not finish within %s", timeout)}
		}
		return nil, &typedFailure{Code: "invalid_input", Err: runContext.Err()}
	case waitErr := <-done:
		if stdout.overflow {
			return nil, &typedFailure{Code: "invalid_output", Err: errors.New("the text engine returned more output than this reader accepts")}
		}
		if waitErr != nil {
			detail := firstLineOf(stderr.String())
			if detail == "" {
				detail = waitErr.Error()
			}
			return nil, &typedFailure{Code: "invalid_pdf", Err: errors.New(detail)}
		}
		return stdout.Bytes(), nil
	}
}

func detectEngineVersion(ctx context.Context, executable string) string {
	runContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(runContext, executable, "-v")
	var output cappedBuffer
	output.limit = maxStderr
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err == nil {
		if version := versionFromText(output.String()); version != "" {
			return version
		}
	}
	// The image records the installed version at build time, so a failure to
	// query the binary still leaves the result attributable.
	if raw, err := os.ReadFile("/etc/vextract-engine-version"); err == nil {
		if version := versionFromText(string(raw)); version != "" {
			return version
		}
	}
	return "unknown"
}

func detectTesseractVersion(ctx context.Context, executable string) string {
	runContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(runContext, executable, "--version")
	var output cappedBuffer
	output.limit = maxStderr
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err == nil {
		if version := versionFromTesseractText(output.String()); version != "" {
			return version
		}
	}
	if raw, err := os.ReadFile("/etc/vextract-ocr-version"); err == nil {
		if version := versionFromTesseractText(string(raw)); version != "" {
			return version
		}
	}
	return "unavailable"
}

// versionFromTesseractText reads the first line of `tesseract --version`,
// which is "tesseract 5.5.0" with no "version" keyword.
func versionFromTesseractText(text string) string {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.EqualFold(fields[0], "tesseract") {
			return strings.Trim(fields[1], ",;")
		}
		if version := versionFromText(line); version != "" {
			return version
		}
	}
	return ""
}

func versionFromText(text string) string {
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		index := strings.Index(lower, "version ")
		if index < 0 {
			continue
		}
		fields := strings.Fields(line[index+len("version "):])
		if len(fields) > 0 {
			return strings.Trim(fields[0], ",;")
		}
	}
	return ""
}

func firstLineOf(value string) string {
	trimmed := strings.TrimSpace(value)
	if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
		trimmed = trimmed[:newline]
	}
	if len(trimmed) > 300 {
		trimmed = trimmed[:300] + "…"
	}
	return trimmed
}

func writeTypedFailure(stderr io.Writer, err error) {
	var typed *typedFailure
	switch {
	case errors.As(err, &typed):
		writeFailure(stderr, typed.Code, typed.Err)
	case errors.Is(err, extract.ErrOCRLimit):
		writeFailure(stderr, "ocr_page_limit", err)
	case errors.Is(err, extract.ErrPixelLimit):
		writeFailure(stderr, "page_too_large", err)
	case errors.Is(err, extract.ErrOCREngine):
		writeFailure(stderr, "ocr_unavailable", err)
	case errors.Is(err, extract.ErrPageLimit):
		writeFailure(stderr, "page_limit", err)
	case errors.Is(err, extract.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		writeFailure(stderr, "timeout", err)
	case errors.Is(err, extract.ErrInvalidBBoxXML), errors.Is(err, extract.ErrInvalidOCRTSV):
		writeFailure(stderr, "invalid_output", err)
	default:
		writeFailure(stderr, "engine_failed", err)
	}
}

func writeFailure(stderr io.Writer, code string, err error) {
	message := strings.ReplaceAll(err.Error(), "\n", " ")
	fmt.Fprintf(stderr, "%s: %s\n", code, message)
}

// cappedBuffer keeps at most limit bytes and records the overflow. Poppler's
// word map is larger than the JSON result, so this cap is separate from the
// server's output bound.
type cappedBuffer struct {
	buffer   strings.Builder
	limit    int
	overflow bool
}

func (buffer *cappedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.overflow = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = buffer.buffer.Write(data[:remaining])
		buffer.overflow = true
		return len(data), nil
	}
	_, _ = buffer.buffer.Write(data)
	return len(data), nil
}

func (buffer *cappedBuffer) Bytes() []byte  { return []byte(buffer.buffer.String()) }
func (buffer *cappedBuffer) String() string { return buffer.buffer.String() }
