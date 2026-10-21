package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"vera-app/backend/extract"
)

// Issue #477: the text-layer extractor.
//
// stdin is a PDF; stdout is the extraction contract. Nothing else is read:
// no database DSN, no signing key, no path, no network. The binary is the only
// thing inside the restricted image, and the image pins both this build and
// the Poppler version it drives (see docker/po-extract/Dockerfile).
//
// Exit 0 means stdout is a valid result. Exit 2 means stdout is empty and
// stderr starts with a typed prefix the server maps to a run error:
//
//	page_limit: ...      the document is longer than the page cap
//	timeout: ...         the text engine did not finish in time
//	invalid_output: ...  the engine's word map was unusable
//	invalid_pdf: ...     Poppler refused the document
//	invalid_input: ...   the request itself was unusable (empty/oversized)
//
// The server stores the message from that line, so it is written for a staff
// member to read, never as a stack trace.
const (
	// EngineName is the text-layer engine this build drives.
	EngineName = "poppler"
	// Version is the extractor build's own version. It is separate from the
	// Poppler version so a regression is attributable to one or the other.
	Version = "0.1.0"

	maxInputBytes = 20 << 20
	maxXMLBytes   = 32 << 20
	maxStderr     = 4 << 10

	// defaultTextTimeout bounds one pdftotext invocation. The server's own
	// wall-time bound is longer, so a slow Poppler produces the typed timeout
	// here rather than an opaque outer kill.
	defaultTextTimeout = 30 * time.Second
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
	timeout := flags.Duration("timeout", defaultTextTimeout, "maximum time for one text-engine run")
	showVersion := flags.Bool("version", false, "print the engine version and exit")
	if err := flags.Parse(args); err != nil {
		writeFailure(stderr, "invalid_input", err)
		return 2
	}

	engineVersion := detectEngineVersion(ctx, *pdfToText)
	if *showVersion {
		fmt.Fprintf(stdout, "vextract %s (%s %s)\n", Version, EngineName, engineVersion)
		return 0
	}
	if *maxPages < 1 || *maxPages > 200 {
		writeFailure(stderr, "invalid_input", fmt.Errorf("page cap %d is outside 1..200", *maxPages))
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
	xml, err := runPDFToText(ctx, *pdfToText, pdf, *maxPages+1, *timeout)
	if err != nil {
		var typed *typedFailure
		if errors.As(err, &typed) {
			writeFailure(stderr, typed.Code, typed.Err)
		} else {
			writeFailure(stderr, "invalid_pdf", err)
		}
		return 2
	}
	pages, err := extract.ParseBBoxLayout(xml)
	if err != nil {
		writeFailure(stderr, "invalid_output", err)
		return 2
	}
	if len(pages) > *maxPages {
		writeFailure(stderr, "page_limit",
			fmt.Errorf("the PDF has more than %d pages; read it and enter the order manually", *maxPages))
		return 2
	}
	result, err := extract.Propose(pages, extract.Options{MaxPages: *maxPages})
	if err != nil {
		if errors.Is(err, extract.ErrPageLimit) {
			writeFailure(stderr, "page_limit", err)
		} else {
			writeFailure(stderr, "invalid_output", err)
		}
		return 2
	}
	result.Engine = EngineName
	result.EngineVersion = engineVersion
	result.DurationMs = time.Since(started).Milliseconds()
	if err := extract.ValidateExtractionOutput(&result, *maxPages); err != nil {
		if errors.Is(err, extract.ErrPageLimit) {
			writeFailure(stderr, "page_limit", err)
		} else {
			writeFailure(stderr, "invalid_output", err)
		}
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

func pageLimitFromEnvironment() int {
	raw := strings.TrimSpace(os.Getenv("VERA_EXTRACT_MAX_PAGES"))
	if raw == "" {
		return extract.DefaultMaxPages
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return extract.DefaultMaxPages
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
