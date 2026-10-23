package extract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"vera-app/backend/store"
)

// Issue #477: the extraction run orchestration.
//
// Start queues a run bound to the active document generation and launches one
// worker outside the request's lifetime. The worker claims the run (only one
// claimant can move queued -> running), reads the exact document version it
// was bound to, runs the restricted engine, and stores the answer. A document
// replaced mid-run makes the answer stale inside the store, not merely late.
//
// Nothing here writes a purchase-order field. Extraction is assistance; the
// editor applies chosen proposals through its own explicit action.

// Service owns the engine and the run lifecycle. It is deliberately separate
// from the HTTP layer so tests can drive a stub engine through the real store.
type Service struct {
	Store    *store.Store
	Engine   Engine
	Timeout  time.Duration
	MaxPages int
	// Log receives operational failures that have no other visible surface.
	// It must never be given document content.
	Log func(format string, args ...any)
}

func NewService(database *store.Store, engine Engine) *Service {
	return &Service{
		Store:    database,
		Engine:   engine,
		Timeout:  DefaultExtractTimeout,
		MaxPages: DefaultMaxPages,
	}
}

// Available reports whether this deployment can extract at all, using the
// same Prober preflight Start runs. It is a typed deployment answer for the
// desktop's banner: not an error string, and never a simulated success.
// An engine without the optional Prober is assumed available, exactly as
// Start assumes it.
func (s *Service) Available(ctx context.Context) bool {
	if s == nil || s.Store == nil || s.Engine == nil {
		return false
	}
	if prober, ok := s.Engine.(Prober); ok {
		return prober.Available(ctx) == nil
	}
	return true
}

// Start queues an extraction and returns the run that is active for the
// document. Starting again while that run is still queued/running returns the
// same run; the extra worker loses the queued -> running claim and exits.
func (s *Service) Start(ctx context.Context, purchaseOrderID, actorID int64) (store.PurchaseOrderExtraction, error) {
	if s.Store == nil || s.Engine == nil {
		return store.PurchaseOrderExtraction{}, fmt.Errorf("%w: no extraction service is configured", ErrExtractorUnavailable)
	}
	// Preflight before a row exists. An extractor that cannot be started is a
	// deployment answer (503), not a queued job that fails later: a job would
	// leave history that says extraction was attempted when it never ran.
	if prober, ok := s.Engine.(Prober); ok {
		if err := prober.Available(ctx); err != nil {
			return store.PurchaseOrderExtraction{}, err
		}
	}
	run, err := s.Store.CreatePurchaseOrderExtraction(ctx, store.PurchaseOrderExtractionCreate{
		PurchaseOrderID: purchaseOrderID,
		RequestedBy:     actorID,
	})
	if err != nil {
		return store.PurchaseOrderExtraction{}, err
	}
	if run.Status != store.PurchaseOrderExtractionStatusQueued {
		return run, nil
	}
	// The request context ends when the response is written. The job must
	// outlive it, but keep the organization value the store requires.
	jobContext := context.WithoutCancel(ctx)
	go s.execute(jobContext, run)
	return run, nil
}

// Latest reads the newest run beside the active document. The store marks a
// generation mismatch stale and discards the result before returning.
func (s *Service) Latest(ctx context.Context, purchaseOrderID int64) (store.PurchaseOrderExtractionState, error) {
	return s.Store.LatestPurchaseOrderExtraction(ctx, purchaseOrderID)
}

// Recover fails every run a previous process left active. Called once at
// startup; safe to call again.
func (s *Service) Recover(ctx context.Context) (int64, error) {
	return s.Store.RecoverInterruptedPurchaseOrderExtractions(ctx)
}

func (s *Service) execute(ctx context.Context, queued store.PurchaseOrderExtraction) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultExtractTimeout
	}
	// Claiming the run and reading the exact document are database work, not
	// extraction work. Give them their own bound so a slow database cannot
	// consume the engine's wall-time and leave the run looking stuck.
	prepareContext, cancelPrepare := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	run, err := s.Store.BeginPurchaseOrderExtraction(prepareContext, queued.ID)
	if err != nil {
		cancelPrepare()
		// Another worker claimed this run, or it was discarded before it
		// started. There is nothing left for this worker to do.
		if !errors.Is(err, store.ErrPurchaseOrderExtractionNotActive) {
			s.logf("extraction %d could not be claimed: %v", queued.ID, err)
		}
		return
	}
	document, err := s.Store.GetPurchaseOrderDocumentVersion(prepareContext, run.PurchaseOrderID, run.DocumentVersion)
	cancelPrepare()
	if err != nil {
		s.finishFailure(ctx, run, 0, "document_missing",
			"The PDF this extraction read is no longer available. Run Extract again.")
		return
	}
	if document.SHA256 != run.DocumentSHA256 {
		s.finishFailure(ctx, run, 0, "document_changed",
			"The PDF changed before extraction could read it. Run Extract again.")
		return
	}

	runContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	started := time.Now()
	output, engineErr := s.Engine.Extract(runContext, document.Data)
	duration := time.Since(started).Milliseconds()
	if engineErr != nil {
		code, message := extractionFailure(engineErr, timeout)
		s.finishFailure(ctx, run, duration, code, message)
		return
	}
	maxPages := s.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultMaxPages
	}
	if err := ValidateExtractionOutput(&output, maxPages); err != nil {
		code, message := extractionFailure(err, timeout)
		s.finishFailure(ctx, run, duration, code, message)
		return
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		s.finishFailure(ctx, run, duration, "invalid_output", "The extractor returned a result that could not be stored.")
		return
	}
	// The job deadline may be nearly expired; recording the answer must not be
	// lost to it. WithoutCancel keeps the organization value.
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, err = s.Store.FinishPurchaseOrderExtraction(finishContext, store.PurchaseOrderExtractionFinish{
		ID:            run.ID,
		Status:        store.PurchaseOrderExtractionStatusSucceeded,
		Engine:        output.Engine,
		EngineVersion: output.EngineVersion,
		PageCount:     output.PageCount,
		Result:        encoded,
		DurationMS:    duration,
	})
	if err != nil && !errors.Is(err, store.ErrPurchaseOrderExtractionNotActive) {
		s.logf("extraction %d could not be finished: %v", run.ID, err)
	}
}

func (s *Service) finishFailure(ctx context.Context, run store.PurchaseOrderExtraction, duration int64, code, message string) {
	// The job's own context may carry the timeout that caused this failure,
	// and the failure still has to be recorded. WithoutCancel preserves the
	// organization value the store requires while dropping the expired
	// deadline.
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, err := s.Store.FinishPurchaseOrderExtraction(finishContext, store.PurchaseOrderExtractionFinish{
		ID:           run.ID,
		Status:       store.PurchaseOrderExtractionStatusFailed,
		ErrorCode:    code,
		ErrorMessage: message,
		DurationMS:   duration,
	})
	if err != nil && !errors.Is(err, store.ErrPurchaseOrderExtractionNotActive) {
		s.logf("extraction %d failure could not be recorded: %v", run.ID, err)
	}
}

func (s *Service) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

func extractionFailure(err error, timeout time.Duration) (string, string) {
	switch {
	case errors.Is(err, ErrPageLimit):
		return "page_limit", "This PDF has more pages than this release extracts. Read it and enter the order manually."
	case errors.Is(err, ErrOCRLimit):
		return "ocr_page_limit", "This PDF has more scanned pages than this release reads. Read it and enter the order manually."
	case errors.Is(err, ErrPixelLimit):
		return "page_too_large", "A page in this PDF is too large to read automatically. Enter the values manually."
	case errors.Is(err, ErrOCREngine):
		return "ocr_unavailable", "This server could not read the scanned pages. Enter the values manually."
	case errors.Is(err, ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout", fmt.Sprintf("Extraction took longer than %s. Try again or enter the values manually.", timeout.Round(time.Second))
	case errors.Is(err, ErrOutputTooLarge):
		return "output_too_large", "The extractor returned more data than this release accepts. Enter the values manually."
	case errors.Is(err, ErrInvalidOutput):
		return "invalid_output", "The extractor could not read this PDF's pages into usable values. Enter the values manually."
	case errors.Is(err, ErrExtractorUnavailable):
		return "extractor_unavailable", "Automatic extraction is not configured on this server. Enter the values manually."
	case errors.Is(err, ErrInputTooLarge):
		return "document_too_large", "The PDF is larger than the extraction limit. Enter the values manually."
	case errors.Is(err, context.Canceled):
		return "cancelled", "Extraction was stopped before it finished. Run Extract again."
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "The text extractor failed. Enter the values manually."
	}
	if len(message) > 300 {
		message = message[:300] + "…"
	}
	return "engine_failed", message
}
