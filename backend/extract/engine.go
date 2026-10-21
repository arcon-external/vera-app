package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"vera-app/backend/apicontract"
)

// Issue #477: the restricted extractor process.
//
// The engine is a separate executable. It receives the PDF on stdin and
// returns the extraction contract on stdout; it never receives a database
// DSN, a signing key or any other application secret. The runner bounds the
// input, the wall time and the output size, and it validates the answer
// before the server stores a single field.
//
// This is the text-layer slice only. The command is Poppler-backed in the
// image (#476 decision), but the same boundary lets the benchmark harness
// swap a different text engine without changing the server.
const (
	DefaultExtractorCommand = "vextract"
	DefaultExtractTimeout   = 45 * time.Second
	DefaultMaxOutputBytes   = 8 << 20
	DefaultMaxInputBytes    = 20 << 20 // the customer PDF bound, plus nothing
	DefaultMaxStderrBytes   = 64 << 10
)

var (
	// ErrExtractorUnavailable means the configured executable is missing or
	// not runnable. It is a deployment failure, not a document failure.
	ErrExtractorUnavailable = errors.New("the PDF extractor is not available")
	// ErrEngineFailed means the extractor ran and exited unsuccessfully.
	ErrEngineFailed = errors.New("the PDF extractor failed")
	// ErrTimeout is the wall-time bound. The child process group is killed.
	ErrTimeout = errors.New("the PDF extractor timed out")
	// ErrOutputTooLarge refuses an extractor answer above the output bound.
	ErrOutputTooLarge = errors.New("the PDF extractor returned too much output")
	// ErrInputTooLarge refuses a PDF above the stored document bound.
	ErrInputTooLarge = errors.New("the PDF is too large to extract")
	// ErrInvalidOutput is a contract violation: malformed JSON, a wrong
	// schema version, a missing field, an unknown status or a page map that
	// does not match the page count.
	ErrInvalidOutput = errors.New("the PDF extractor returned invalid output")
)

// Engine is the one seam the runner depends on. Tests use a stub; production
// uses CommandEngine.
type Engine interface {
	Extract(ctx context.Context, pdf []byte) (apicontract.ExtractionOutput, error)
}

// CommandEngine runs an executable with the PDF on stdin.
type CommandEngine struct {
	// Command is the executable and its fixed arguments. The page cap is
	// passed through VERA_EXTRACT_MAX_PAGES so a wrapper can forward it.
	Command []string
	// Environment is appended to the process environment. Nil means only the
	// inherited environment plus VERA_EXTRACT_MAX_PAGES.
	Environment    []string
	Timeout        time.Duration
	MaxInputBytes  int
	MaxOutputBytes int
	MaxStderrBytes int
	MaxPages       int
}

// NewCommandEngineFromEnvironment reads VERA_EXTRACTOR_CMD (default
// "vextract"). An empty value falls back to the default rather than running
// the caller's shell with no arguments.
func NewCommandEngineFromEnvironment() *CommandEngine {
	command := SplitCommand(os.Getenv("VERA_EXTRACTOR_CMD"))
	if len(command) == 0 {
		command = []string{DefaultExtractorCommand}
	}
	return &CommandEngine{
		Command:        command,
		Timeout:        DefaultExtractTimeout,
		MaxInputBytes:  DefaultMaxInputBytes,
		MaxOutputBytes: DefaultMaxOutputBytes,
		MaxStderrBytes: DefaultMaxStderrBytes,
		MaxPages:       DefaultMaxPages,
	}
}

// SplitCommand splits a configured command line on whitespace. The variable is
// operator configuration, not document input; an executable plus fixed flags
// is all the server needs, so shell quoting and metacharacters are deliberately
// not interpreted.
func SplitCommand(line string) []string {
	return strings.Fields(line)
}

// Extract runs the engine and validates its answer.
func (engine *CommandEngine) Extract(ctx context.Context, pdf []byte) (apicontract.ExtractionOutput, error) {
	if len(engine.Command) == 0 {
		return apicontract.ExtractionOutput{}, fmt.Errorf("%w: no command is configured", ErrExtractorUnavailable)
	}
	timeout := engine.Timeout
	if timeout <= 0 {
		timeout = DefaultExtractTimeout
	}
	maxInput := engine.MaxInputBytes
	if maxInput <= 0 {
		maxInput = DefaultMaxInputBytes
	}
	if len(pdf) > maxInput {
		return apicontract.ExtractionOutput{}, fmt.Errorf("%w: %d bytes, limit %d", ErrInputTooLarge, len(pdf), maxInput)
	}
	maxOutput := engine.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutputBytes
	}
	maxStderr := engine.MaxStderrBytes
	if maxStderr <= 0 {
		maxStderr = DefaultMaxStderrBytes
	}
	maxPages := engine.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultMaxPages
	}

	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	command := exec.CommandContext(runContext, engine.Command[0], engine.Command[1:]...)
	command.Stdin = bytes.NewReader(pdf)
	stdout := newLimitedBuffer(maxOutput)
	stderr := newLimitedBuffer(maxStderr)
	command.Stdout = stdout
	command.Stderr = stderr
	// Deliberately not os.Environ(): the API server holds the Postgres DSN and
	// the signing key, and the extractor has no business seeing either. Only
	// the executable search path, a writable temp home for a dev wrapper, and
	// the page cap cross this boundary; explicit Environment entries are
	// operator/test configuration.
	command.Env = extractorEnvironment(engine.Environment, maxPages)
	configureProcessGroup(command)

	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return apicontract.ExtractionOutput{}, fmt.Errorf("%w: %s", ErrExtractorUnavailable, engine.Command[0])
		}
		return apicontract.ExtractionOutput{}, fmt.Errorf("%w: start %s: %v", ErrExtractorUnavailable, engine.Command[0], err)
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()

	select {
	case <-runContext.Done():
		killProcessGroup(command)
		<-done
		return apicontract.ExtractionOutput{}, fmt.Errorf("%w after %s", ErrTimeout, timeout)
	case waitErr := <-done:
		if stdout.overflow {
			return apicontract.ExtractionOutput{}, fmt.Errorf("%w: more than %d bytes", ErrOutputTooLarge, maxOutput)
		}
		if waitErr != nil {
			return apicontract.ExtractionOutput{}, ClassifyFailure(stderr.String(), waitErr)
		}
	}

	var output apicontract.ExtractionOutput
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return apicontract.ExtractionOutput{}, fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apicontract.ExtractionOutput{}, fmt.Errorf("%w: trailing content after the result", ErrInvalidOutput)
	}
	if err := ValidateExtractionOutput(&output, maxPages); err != nil {
		return apicontract.ExtractionOutput{}, err
	}
	return output, nil
}

// extractorEnvironment is the complete environment the child receives. It is
// intentionally tiny: the API process holds credentials the extractor must not
// inherit. A dev wrapper needs PATH and a writable HOME; the default binary
// only needs PATH and the page cap.
func extractorEnvironment(extra []string, maxPages int) []string {
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}
	environment := []string{
		"PATH=" + path,
		"HOME=/tmp",
		"TMPDIR=/tmp",
		"LANG=C.UTF-8",
		fmt.Sprintf("VERA_EXTRACT_MAX_PAGES=%d", maxPages),
	}
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		environment = append(environment, "DOCKER_HOST="+host)
	}
	return append(environment, extra...)
}

// ValidateExtractionOutput is the shared contract check. The extractor uses it
// before printing, the server uses it after reading, and the stub engine in
// tests uses it too, so a broken answer cannot be stored by one path and
// refused by another.
func ValidateExtractionOutput(output *apicontract.ExtractionOutput, maxPages int) error {
	if output.SchemaVersion != apicontract.ExtractionSchemaVersion {
		return fmt.Errorf("%w: schema version %d", ErrInvalidOutput, output.SchemaVersion)
	}
	if strings.TrimSpace(output.Engine) == "" || strings.TrimSpace(output.EngineVersion) == "" {
		return fmt.Errorf("%w: the engine name and version are required", ErrInvalidOutput)
	}
	if output.PageCount < 1 {
		return fmt.Errorf("%w: it names no pages", ErrInvalidOutput)
	}
	if maxPages > 0 && output.PageCount > maxPages {
		return fmt.Errorf("%w: %d pages, limit %d", ErrPageLimit, output.PageCount, maxPages)
	}
	if len(output.Pages) != output.PageCount {
		return fmt.Errorf("%w: %d pages named but %d page sizes supplied", ErrInvalidOutput, output.PageCount, len(output.Pages))
	}
	for index, page := range output.Pages {
		if page.Number != index+1 || page.Width <= 0 || page.Height <= 0 {
			return fmt.Errorf("%w: page map entry %d is unusable", ErrInvalidOutput, index)
		}
	}
	if output.PagesWithText+output.PagesWithoutText != output.PageCount {
		return fmt.Errorf("%w: page text counts do not add up", ErrInvalidOutput)
	}
	if len(output.Fields) != len(apicontract.ExtractionFieldKeys) {
		return fmt.Errorf("%w: %d header fields, want %d", ErrInvalidOutput, len(output.Fields), len(apicontract.ExtractionFieldKeys))
	}
	seen := map[string]bool{}
	for _, field := range output.Fields {
		if seen[field.Key] {
			return fmt.Errorf("%w: duplicate header field %q", ErrInvalidOutput, field.Key)
		}
		seen[field.Key] = true
		if !allowedStatus(field.Status) {
			return fmt.Errorf("%w: header field %q has status %q", ErrInvalidOutput, field.Key, field.Status)
		}
		if field.Status == apicontract.ExtractionStatusAbsent && field.Value != "" {
			return fmt.Errorf("%w: absent header field %q carries a value", ErrInvalidOutput, field.Key)
		}
	}
	for _, key := range apicontract.ExtractionFieldKeys {
		if !seen[key] {
			return fmt.Errorf("%w: header field %q is missing", ErrInvalidOutput, key)
		}
	}
	for index := range output.Rows {
		row := &output.Rows[index]
		if row.Status != apicontract.ExtractionStatusExtracted &&
			row.Status != apicontract.ExtractionStatusNeedsReview {
			return fmt.Errorf("%w: row %d has status %q", ErrInvalidOutput, index, row.Status)
		}
		if row.Page < 1 || row.Page > output.PageCount {
			return fmt.Errorf("%w: row %d names page %d", ErrInvalidOutput, index, row.Page)
		}
		for label, cell := range map[string]apicontract.ExtractionCell{
			"lineNumber": row.LineNumber, "description": row.Description,
			"quantity": row.Quantity, "uom": row.UoM,
			"unitPrice": row.UnitPrice, "amount": row.Amount,
		} {
			if !allowedStatus(cell.Status) {
				return fmt.Errorf("%w: row %d %s has status %q", ErrInvalidOutput, index, label, cell.Status)
			}
			if cell.Status != apicontract.ExtractionStatusAbsent && strings.TrimSpace(cell.Value) == "" {
				return fmt.Errorf("%w: row %d %s is not absent but has no value", ErrInvalidOutput, index, label)
			}
		}
	}
	output.Abstentions = countAbstentions(*output)
	return nil
}

func allowedStatus(status string) bool {
	switch status {
	case apicontract.ExtractionStatusExtracted,
		apicontract.ExtractionStatusNeedsReview,
		apicontract.ExtractionStatusAbsent:
		return true
	}
	return false
}

func countAbstentions(output apicontract.ExtractionOutput) apicontract.ExtractionAbstentions {
	counts := apicontract.ExtractionAbstentions{}
	for _, field := range output.Fields {
		if field.Status != apicontract.ExtractionStatusExtracted {
			counts.Fields++
		}
	}
	for _, row := range output.Rows {
		if row.Status != apicontract.ExtractionStatusExtracted {
			counts.Rows++
		}
	}
	return counts
}

func firstLine(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
		trimmed = trimmed[:newline]
	}
	if len(trimmed) > 300 {
		trimmed = trimmed[:300] + "…"
	}
	return trimmed
}

// ClassifyFailure maps the extractor's typed stderr prefix to the error the
// service stores. The prefixes are part of the command contract documented in
// docs/po-extraction.md; an unprefixed failure stays a generic engine failure.
func ClassifyFailure(stderr string, waitErr error) error {
	line := firstLine(stderr)
	lower := strings.ToLower(line)
	message := line
	if colon := strings.IndexByte(line, ':'); colon >= 0 {
		message = strings.TrimSpace(line[colon+1:])
	}
	switch {
	case strings.HasPrefix(lower, "page_limit:"):
		return fmt.Errorf("%w: %s", ErrPageLimit, message)
	case strings.HasPrefix(lower, "timeout:"):
		return fmt.Errorf("%w: %s", ErrTimeout, message)
	case strings.HasPrefix(lower, "invalid_output:"):
		return fmt.Errorf("%w: %s", ErrInvalidOutput, message)
	case strings.HasPrefix(lower, "invalid_pdf:"), strings.HasPrefix(lower, "invalid_input:"):
		return fmt.Errorf("%w: %s", ErrEngineFailed, message)
	}
	if line == "" && waitErr != nil {
		line = waitErr.Error()
	}
	if line == "" {
		line = "the extractor exited unsuccessfully"
	}
	return fmt.Errorf("%w: %s", ErrEngineFailed, line)
}

// limitedBuffer keeps at most limit bytes and records that it dropped the
// rest. It keeps accepting writes so a child cannot block on a full pipe while
// the parent is waiting for it to exit.
type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.overflow = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.buffer.Write(data[:remaining])
		b.overflow = true
		return len(data), nil
	}
	_, _ = b.buffer.Write(data)
	return len(data), nil
}

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }
func (b *limitedBuffer) String() string {
	return b.buffer.String()
}
