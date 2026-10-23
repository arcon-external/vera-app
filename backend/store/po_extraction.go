package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Issue #477: storage for text-PDF extraction runs.
//
// A run describes one document generation: the document row, its version and
// its SHA-256. The result is evidence about that exact file. When the file is
// replaced or removed, the run becomes stale and its result is discarded in
// the same statement, so no reader can serve proposals from a PDF that is no
// longer attached.
//
// The store owns the lifecycle transitions; the extraction service owns the
// subprocess. Every transition is conditional on the status it expects, so a
// late worker cannot overwrite a stale-marked run.
const (
	PurchaseOrderExtractionStatusQueued    = "queued"
	PurchaseOrderExtractionStatusRunning   = "running"
	PurchaseOrderExtractionStatusSucceeded = "succeeded"
	PurchaseOrderExtractionStatusFailed    = "failed"
	PurchaseOrderExtractionStatusStale     = "stale"
)

const (
	// PurchaseOrderExtractionErrorDocumentChanged is the stale reason: the
	// PDF this run read is no longer the active one.
	PurchaseOrderExtractionErrorDocumentChanged = "document_changed"
	// PurchaseOrderExtractionErrorInterrupted is the restart reason. A server
	// restart cannot resume a subprocess, so the run fails cleanly.
	PurchaseOrderExtractionErrorInterrupted = "interrupted"
)

var (
	// ErrPurchaseOrderExtractionNotFound answers an unknown run id (or one
	// outside the caller's organization).
	ErrPurchaseOrderExtractionNotFound = errors.New("purchase order extraction not found")
	// ErrPurchaseOrderExtractionDocumentRequired is the 409 a start receives
	// when the purchase order has no attached PDF.
	ErrPurchaseOrderExtractionDocumentRequired = errors.New("attach a customer PO PDF before extracting its data")
	// ErrPurchaseOrderExtractionNotActive means a worker tried to finish a run
	// that was already stale, failed or succeeded. The worker's answer is late
	// and must not replace the stored one.
	ErrPurchaseOrderExtractionNotActive = errors.New("purchase order extraction is no longer active")
)

// PurchaseOrderExtraction is one run, including its result when the status is
// succeeded. Result is the raw extraction contract JSON; the API decodes it.
type PurchaseOrderExtraction struct {
	UUID            string
	Status          string
	Engine          string
	EngineVersion   string
	DocumentSHA256  string
	Result          []byte
	ErrorCode       *string
	ErrorMessage    *string
	ID              int64
	PurchaseOrderID int64
	DocumentID      int64
	DocumentVersion int32
	PageCount       int
	RequestedBy     int64
	CreatedAt       time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	DurationMS      *int64
}

// PurchaseOrderExtractionCreate starts (or joins) the active run for a PO.
type PurchaseOrderExtractionCreate struct {
	PurchaseOrderID int64
	RequestedBy     int64
}

// PurchaseOrderExtractionFinish records a worker's answer. Status is either
// succeeded or failed; the store decides whether it instead becomes stale.
type PurchaseOrderExtractionFinish struct {
	Status        string
	Engine        string
	EngineVersion string
	Result        []byte
	ErrorCode     string
	ErrorMessage  string
	ID            int64
	PageCount     int
	DurationMS    int64
}

// PurchaseOrderExtractionState is the latest run beside the currently active
// document generation. Run is nil when the record has never been extracted.
type PurchaseOrderExtractionState struct {
	Run      *PurchaseOrderExtraction
	Document *PurchaseOrderDocument
}

const extractionColumns = `e.id, e.uuid::text, e.purchase_order_id, e.document_id,
	e.document_version, e.document_sha256, e.status, e.engine, e.engine_version,
	e.page_count, e.result, e.error_code, e.error_message, e.requested_by,
	e.created_at, e.started_at, e.finished_at, e.duration_ms`

// CreatePurchaseOrderExtraction binds a new run to the active document. It is
// idempotent while that run is still active: a second click returns the run
// already in flight. A run bound to an older generation is marked stale before
// the replacement is inserted, and the purchase-order row lock serializes
// this with attach/replace.
func (s *Store) CreatePurchaseOrderExtraction(ctx context.Context, input PurchaseOrderExtractionCreate) (PurchaseOrderExtraction, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderExtraction{}, err
	}
	if input.PurchaseOrderID < 1 {
		return PurchaseOrderExtraction{}, ErrPurchaseOrderNotFound
	}
	if input.RequestedBy < 1 {
		return PurchaseOrderExtraction{}, fmt.Errorf("the requester is required")
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderExtraction{}, err
	}
	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("begin extraction transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockPurchaseOrderForDocument(ctx, tx, schema, org.ID, input.PurchaseOrderID); err != nil {
		return PurchaseOrderExtraction{}, err
	}
	document, err := activeDocumentInTx(ctx, tx, schema, input.PurchaseOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrderExtraction{}, ErrPurchaseOrderExtractionDocumentRequired
	}
	if err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("load the active document: %w", err)
	}

	active, err := scanPurchaseOrderExtraction(tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM %s.purchase_order_extractions e
		WHERE e.purchase_order_id = $1 AND e.status IN ('queued', 'running')
		FOR UPDATE`, extractionColumns, schema), input.PurchaseOrderID))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrderExtraction{}, fmt.Errorf("load the active extraction: %w", err)
	}
	if err == nil {
		if active.DocumentID == document.ID && active.DocumentSHA256 == document.SHA256 {
			if err := tx.Commit(ctx); err != nil {
				return PurchaseOrderExtraction{}, fmt.Errorf("commit idempotent extraction: %w", err)
			}
			return active, nil
		}
		if err := markExtractionStale(ctx, tx, schema, active.ID,
			"The PDF was replaced before extraction started; the older run was discarded."); err != nil {
			return PurchaseOrderExtraction{}, err
		}
	}

	run, err := scanPurchaseOrderExtraction(tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_extractions AS e (
			org_id, purchase_order_id, document_id, document_version,
			document_sha256, status, requested_by
		) VALUES ($1,$2,$3,$4,$5,'queued',$6)
		RETURNING %s`, schema, extractionColumns),
		org.ID, input.PurchaseOrderID, document.ID, document.Version,
		document.SHA256, input.RequestedBy))
	if err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("start purchase order extraction: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("commit extraction start: %w", err)
	}
	return run, nil
}

// BeginPurchaseOrderExtraction claims a queued run for a worker. A run that
// was already marked stale or failed is not claimable.
func (s *Store) BeginPurchaseOrderExtraction(ctx context.Context, id int64) (PurchaseOrderExtraction, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderExtraction{}, err
	}
	if id < 1 {
		return PurchaseOrderExtraction{}, ErrPurchaseOrderExtractionNotFound
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderExtraction{}, err
	}
	schema := s.configSchema()
	run, err := scanPurchaseOrderExtraction(pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_extractions e
		SET status = 'running', started_at = NOW()
		FROM %s.purchase_orders po
		WHERE e.id = $1 AND e.status = 'queued'
		  AND po.id = e.purchase_order_id AND po.org_id = $2
		RETURNING %s`, schema, schema, extractionColumns), id, org.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrderExtraction{}, ErrPurchaseOrderExtractionNotActive
	}
	if err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("begin purchase order extraction: %w", err)
	}
	return run, nil
}

// FinishPurchaseOrderExtraction records a worker's answer. If the active
// document no longer matches the run's generation, the run becomes stale and
// its result is not stored: the proposals belong to a different file.
func (s *Store) FinishPurchaseOrderExtraction(ctx context.Context, input PurchaseOrderExtractionFinish) (PurchaseOrderExtraction, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderExtraction{}, err
	}
	if input.Status != PurchaseOrderExtractionStatusSucceeded &&
		input.Status != PurchaseOrderExtractionStatusFailed {
		return PurchaseOrderExtraction{}, fmt.Errorf("invalid extraction finish status %q", input.Status)
	}
	if input.ID < 1 {
		return PurchaseOrderExtraction{}, ErrPurchaseOrderExtractionNotFound
	}
	if input.DurationMS < 0 {
		input.DurationMS = 0
	}
	if input.Status == PurchaseOrderExtractionStatusSucceeded &&
		(len(input.Result) == 0 || input.Engine == "" || input.EngineVersion == "" || input.PageCount < 1) {
		return PurchaseOrderExtraction{}, fmt.Errorf("a succeeded extraction needs its engine, page count and result")
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderExtraction{}, err
	}
	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("begin extraction finish: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanPurchaseOrderExtraction(tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM %s.purchase_order_extractions e
		JOIN %s.purchase_orders po ON po.id = e.purchase_order_id
		WHERE e.id = $1 AND po.org_id = $2
		FOR UPDATE OF e`, extractionColumns, schema, schema), input.ID, org.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrderExtraction{}, ErrPurchaseOrderExtractionNotFound
	}
	if err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("lock extraction for finish: %w", err)
	}
	if current.Status != PurchaseOrderExtractionStatusQueued &&
		current.Status != PurchaseOrderExtractionStatusRunning {
		return PurchaseOrderExtraction{}, ErrPurchaseOrderExtractionNotActive
	}

	document, documentErr := activeDocumentInTx(ctx, tx, schema, current.PurchaseOrderID)
	documentMatches := documentErr == nil &&
		document.ID == current.DocumentID && document.SHA256 == current.DocumentSHA256
	if documentErr != nil && !errors.Is(documentErr, pgx.ErrNoRows) {
		return PurchaseOrderExtraction{}, fmt.Errorf("read the active document: %w", documentErr)
	}
	if !documentMatches {
		if err := markExtractionStale(ctx, tx, schema, current.ID,
			"The PDF changed while extraction was running; the proposals were discarded."); err != nil {
			return PurchaseOrderExtraction{}, err
		}
		stale, err := scanPurchaseOrderExtraction(tx.QueryRow(ctx, fmt.Sprintf(
			`SELECT %s FROM %s.purchase_order_extractions e WHERE e.id = $1`,
			extractionColumns, schema), current.ID))
		if err != nil {
			return PurchaseOrderExtraction{}, fmt.Errorf("read the stale extraction: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return PurchaseOrderExtraction{}, fmt.Errorf("commit stale extraction: %w", err)
		}
		return stale, nil
	}

	var result []byte
	if input.Status == PurchaseOrderExtractionStatusSucceeded {
		result = input.Result
	}
	var errorCode, errorMessage *string
	if input.Status == PurchaseOrderExtractionStatusFailed {
		code := input.ErrorCode
		if code == "" {
			code = "engine_failed"
		}
		message := input.ErrorMessage
		if message == "" {
			message = "The text extractor failed."
		}
		errorCode, errorMessage = &code, &message
	}
	finished, err := scanPurchaseOrderExtraction(tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_extractions e
		SET status = $2, engine = $3, engine_version = $4, page_count = $5,
		    result = $6, error_code = $7, error_message = $8,
		    finished_at = NOW(), duration_ms = $9,
		    started_at = COALESCE(started_at, NOW())
		WHERE e.id = $1
		RETURNING %s`, schema, extractionColumns),
		current.ID, input.Status, input.Engine, input.EngineVersion,
		input.PageCount, result, errorCode, errorMessage, input.DurationMS))
	if err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("finish purchase order extraction: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrderExtraction{}, fmt.Errorf("commit extraction finish: %w", err)
	}
	return finished, nil
}

// LatestPurchaseOrderExtraction returns the newest run beside the active
// document. If a successful run's generation no longer matches, the run is
// marked stale and its result is discarded before it is returned.
func (s *Store) LatestPurchaseOrderExtraction(ctx context.Context, purchaseOrderID int64) (PurchaseOrderExtractionState, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderExtractionState{}, err
	}
	if purchaseOrderID < 1 {
		return PurchaseOrderExtractionState{}, ErrPurchaseOrderExtractionNotFound
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderExtractionState{}, err
	}
	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrderExtractionState{}, fmt.Errorf("begin extraction read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Share-lock the record so an attach cannot replace the document between
	// the run read and the generation comparison. Attach takes FOR UPDATE.
	if err := requirePurchaseOrderInOrgForUpdate(ctx, tx, schema, org.ID, purchaseOrderID); err != nil {
		return PurchaseOrderExtractionState{}, err
	}
	document, err := activeDocumentInTx(ctx, tx, schema, purchaseOrderID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrderExtractionState{}, fmt.Errorf("read the active document: %w", err)
	}
	state := PurchaseOrderExtractionState{}
	if err == nil {
		state.Document = &document
	}

	run, err := scanPurchaseOrderExtraction(tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM %s.purchase_order_extractions e
		WHERE e.purchase_order_id = $1
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT 1
		FOR UPDATE OF e`, extractionColumns, schema), purchaseOrderID))
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return PurchaseOrderExtractionState{}, fmt.Errorf("commit empty extraction read: %w", err)
		}
		return state, nil
	}
	if err != nil {
		return PurchaseOrderExtractionState{}, fmt.Errorf("read the latest extraction: %w", err)
	}
	if run.Status == PurchaseOrderExtractionStatusQueued ||
		run.Status == PurchaseOrderExtractionStatusRunning ||
		run.Status == PurchaseOrderExtractionStatusSucceeded {
		matches := state.Document != nil &&
			state.Document.ID == run.DocumentID && state.Document.SHA256 == run.DocumentSHA256
		if !matches {
			if err := markExtractionStale(ctx, tx, schema, run.ID,
				"The PDF changed after this extraction; its proposals were discarded."); err != nil {
				return PurchaseOrderExtractionState{}, err
			}
			stale, err := scanPurchaseOrderExtraction(tx.QueryRow(ctx, fmt.Sprintf(
				`SELECT %s FROM %s.purchase_order_extractions e WHERE e.id = $1`,
				extractionColumns, schema), run.ID))
			if err != nil {
				return PurchaseOrderExtractionState{}, fmt.Errorf("read the stale extraction: %w", err)
			}
			run = stale
		}
	}
	state.Run = &run
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrderExtractionState{}, fmt.Errorf("commit extraction read: %w", err)
	}
	return state, nil
}

// RecoverInterruptedPurchaseOrderExtractions fails every queued/running run.
// It is called once at server startup: no worker survives a restart, so those
// rows can never finish. It is deliberately not org-scoped -- it is a
// process-level lifecycle operation, not a tenant read.
func (s *Store) RecoverInterruptedPurchaseOrderExtractions(ctx context.Context) (int64, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, err
	}
	tag, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_extractions
		SET status = 'failed',
		    error_code = $1,
		    error_message = $2,
		    result = NULL,
		    started_at = COALESCE(started_at, NOW()),
		    finished_at = NOW(),
		    duration_ms = CASE
		        WHEN started_at IS NULL THEN 0
		        ELSE GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint)
		    END
		WHERE status IN ('queued', 'running')`, s.configSchema()),
		PurchaseOrderExtractionErrorInterrupted,
		"The server restarted before extraction finished. Run Extract again.")
	if err != nil {
		return 0, fmt.Errorf("recover interrupted extractions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func markExtractionStale(ctx context.Context, tx pgx.Tx, schema string, id int64, message string) error {
	_, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_extractions
		SET status = 'stale',
		    result = NULL,
		    error_code = $2,
		    error_message = $3,
		    started_at = COALESCE(started_at, NOW()),
		    finished_at = NOW(),
		    duration_ms = CASE
		        WHEN started_at IS NULL THEN 0
		        ELSE GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint)
		    END
		WHERE id = $1 AND status IN ('queued', 'running', 'succeeded')`, schema),
		id, PurchaseOrderExtractionErrorDocumentChanged, message)
	if err != nil {
		return fmt.Errorf("mark extraction %d stale: %w", id, err)
	}
	return nil
}

type extractionQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func activeDocumentInTx(ctx context.Context, q extractionQuerier, schema string, purchaseOrderID int64) (PurchaseOrderDocument, error) {
	return scanPurchaseOrderDocument(q.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, uuid::text, org_id::text, purchase_order_id, version,
		       sha256, size_bytes, content_type, original_filename,
		       uploaded_by, uploaded_at, active, replaced_at, removed_at, removed_by
		FROM %s.purchase_order_documents
		WHERE purchase_order_id = $1 AND active`, schema), purchaseOrderID), false)
}

func requirePurchaseOrderInOrgForUpdate(ctx context.Context, q extractionQuerier, schema, orgID string, purchaseOrderID int64) error {
	var id int64
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM %s.purchase_orders WHERE id = $1 AND org_id = $2 FOR SHARE`, schema),
		purchaseOrderID, orgID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPurchaseOrderNotFound
	}
	if err != nil {
		return fmt.Errorf("check purchase order %d: %w", purchaseOrderID, err)
	}
	return nil
}

func scanPurchaseOrderExtraction(row pgx.Row) (PurchaseOrderExtraction, error) {
	var run PurchaseOrderExtraction
	var startedAt, finishedAt pgtype.Timestamptz
	var duration pgtype.Int8
	var errorCode, errorMessage pgtype.Text
	err := row.Scan(
		&run.ID, &run.UUID, &run.PurchaseOrderID, &run.DocumentID,
		&run.DocumentVersion, &run.DocumentSHA256, &run.Status,
		&run.Engine, &run.EngineVersion, &run.PageCount, &run.Result,
		&errorCode, &errorMessage, &run.RequestedBy, &run.CreatedAt,
		&startedAt, &finishedAt, &duration,
	)
	if err != nil {
		return PurchaseOrderExtraction{}, err
	}
	if startedAt.Valid {
		value := startedAt.Time
		run.StartedAt = &value
	}
	if finishedAt.Valid {
		value := finishedAt.Time
		run.FinishedAt = &value
	}
	if duration.Valid {
		value := duration.Int64
		run.DurationMS = &value
	}
	if errorCode.Valid {
		value := errorCode.String
		run.ErrorCode = &value
	}
	if errorMessage.Valid {
		value := errorMessage.String
		run.ErrorMessage = &value
	}
	return run, nil
}
