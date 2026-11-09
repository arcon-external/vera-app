package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Issue #472: storage for the one optional customer PO PDF.
//
// The document is versioned evidence. A replace inserts a new row and marks
// the old one replaced; a removal marks the current row removed. Neither ever
// rewrites the bytes or a purchase-order field. Reads are always joined to the
// owning purchase order so an id from another organization is indistinguishable
// from a missing one.
const PurchaseOrderDocumentContentType = "application/pdf"

// PurchaseOrderDocumentUpload is one attach-or-replace request.
type PurchaseOrderDocumentUpload struct {
	PurchaseOrderID int64
	Data            []byte
	Filename        string
	UploadedBy      int64
}

// PurchaseOrderDocumentRemoval is the deliberate-removal request. Confirmed
// must be true: the API and editor both ask before calling, and the store
// refuses a caller that omitted the intent rather than treating "no body" as
// consent.
type PurchaseOrderDocumentRemoval struct {
	PurchaseOrderID int64
	RemovedBy       int64
	Confirmed       bool
}

// PurchaseOrderDocument is one stored version. Data is populated on attach and
// on a content read, and omitted from history listings so a version list does
// not copy every byte.
type PurchaseOrderDocument struct {
	UUID             string
	OrgID            string
	OriginalFilename string
	ContentType      string
	SHA256           string
	Data             []byte
	ID               int64
	PurchaseOrderID  int64
	Version          int32
	SizeBytes        int64
	UploadedBy       int64
	UploadedAt       time.Time
	Active           bool
	ReplacedAt       *time.Time
	RemovedAt        *time.Time
	RemovedBy        *int64
}

// PurchaseOrderDocumentState is the current document plus the retained
// correction history, newest first. A record with no upload has a nil Active
// and an empty Versions slice.
type PurchaseOrderDocumentState struct {
	Active   *PurchaseOrderDocument
	Versions []PurchaseOrderDocument
}

// AttachPurchaseOrderDocument stores a validated PDF as the next version of
// the purchase order's document. The purchase-order row is locked for the whole
// active-pointer swap, so two concurrent replacements cannot both claim the
// same version or leave two active rows. The old active row is marked replaced
// inside the same transaction; on any failure it stays active.
func (s *Store) AttachPurchaseOrderDocument(ctx context.Context, input PurchaseOrderDocumentUpload) (PurchaseOrderDocument, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderDocument{}, err
	}
	if input.PurchaseOrderID < 1 {
		return PurchaseOrderDocument{}, ErrPurchaseOrderNotFound
	}
	if input.UploadedBy < 1 {
		return PurchaseOrderDocument{}, fmt.Errorf("%w: the uploader is required", ErrPurchaseOrderDocumentInvalid)
	}
	if err := ValidatePurchaseOrderPDF(input.Data); err != nil {
		return PurchaseOrderDocument{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderDocument{}, err
	}
	schema := s.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrderDocument{}, fmt.Errorf("begin purchase order document transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockPurchaseOrderForDocument(ctx, tx, schema, org.ID, input.PurchaseOrderID); err != nil {
		return PurchaseOrderDocument{}, err
	}
	tag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_documents
		SET active = FALSE, replaced_at = NOW()
		WHERE purchase_order_id = $1 AND active`, schema), input.PurchaseOrderID)
	if err != nil {
		return PurchaseOrderDocument{}, fmt.Errorf("close the active purchase order document: %w", err)
	}
	if tag.RowsAffected() > 1 {
		return PurchaseOrderDocument{}, fmt.Errorf("purchase order %d had %d active documents despite the active-pointer index",
			input.PurchaseOrderID, tag.RowsAffected())
	}

	var version int32
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(MAX(version), 0) + 1
		FROM %s.purchase_order_documents
		WHERE purchase_order_id = $1`, schema), input.PurchaseOrderID).Scan(&version); err != nil {
		return PurchaseOrderDocument{}, fmt.Errorf("allocate purchase order document version: %w", err)
	}

	sum := sha256.Sum256(input.Data)
	filename := cleanPurchaseOrderDocumentFilename(input.Filename)
	document := PurchaseOrderDocument{
		OrgID:            org.ID,
		PurchaseOrderID:  input.PurchaseOrderID,
		Version:          version,
		Data:             input.Data,
		SHA256:           hex.EncodeToString(sum[:]),
		SizeBytes:        int64(len(input.Data)),
		ContentType:      PurchaseOrderDocumentContentType,
		OriginalFilename: filename,
		UploadedBy:       input.UploadedBy,
		Active:           true,
	}
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_documents (
			org_id, purchase_order_id, version, pdf_data, sha256, size_bytes,
			content_type, original_filename, uploaded_by, active
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,TRUE)
		RETURNING id, uuid::text, uploaded_at`, schema),
		org.ID, input.PurchaseOrderID, version, input.Data, document.SHA256,
		document.SizeBytes, document.ContentType, filename, input.UploadedBy,
	).Scan(&document.ID, &document.UUID, &document.UploadedAt)
	if err != nil {
		return PurchaseOrderDocument{}, fmt.Errorf("store purchase order document: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrderDocument{}, fmt.Errorf("commit purchase order document: %w", err)
	}
	return document, nil
}

// GetPurchaseOrderDocumentState reads the current document and its retained
// history inside the caller's organization.
func (s *Store) GetPurchaseOrderDocumentState(ctx context.Context, purchaseOrderID int64) (PurchaseOrderDocumentState, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderDocumentState{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderDocumentState{}, err
	}
	schema := s.configSchema()
	if err := requirePurchaseOrderInOrg(ctx, pool, schema, org.ID, purchaseOrderID); err != nil {
		if errors.Is(err, ErrPurchaseOrderNotFound) {
			return PurchaseOrderDocumentState{}, ErrPurchaseOrderDocumentNotFound
		}
		return PurchaseOrderDocumentState{}, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT id, uuid::text, org_id::text, purchase_order_id, version,
		       sha256, size_bytes, content_type, original_filename,
		       uploaded_by, uploaded_at, active, replaced_at, removed_at, removed_by
		FROM %s.purchase_order_documents
		WHERE purchase_order_id = $1
		ORDER BY version DESC`, schema), purchaseOrderID)
	if err != nil {
		return PurchaseOrderDocumentState{}, fmt.Errorf("load purchase order documents: %w", err)
	}
	defer rows.Close()
	state := PurchaseOrderDocumentState{Versions: make([]PurchaseOrderDocument, 0, 4)}
	for rows.Next() {
		document, err := scanPurchaseOrderDocument(rows, false)
		if err != nil {
			return PurchaseOrderDocumentState{}, err
		}
		state.Versions = append(state.Versions, document)
		if document.Active {
			active := document
			state.Active = &active
		}
	}
	if err := rows.Err(); err != nil {
		return PurchaseOrderDocumentState{}, fmt.Errorf("iterate purchase order documents: %w", err)
	}
	return state, nil
}

// GetActivePurchaseOrderDocument returns the current PDF's bytes and metadata.
func (s *Store) GetActivePurchaseOrderDocument(ctx context.Context, purchaseOrderID int64) (PurchaseOrderDocument, error) {
	return s.getPurchaseOrderDocument(ctx, purchaseOrderID, 0)
}

// GetPurchaseOrderDocumentVersion returns one retained version, including a
// replaced or removed one. Zero is refused rather than read as "active": the
// active document has its own method. A missing or foreign-organization
// version is the same not-found answer.
func (s *Store) GetPurchaseOrderDocumentVersion(ctx context.Context, purchaseOrderID int64, version int32) (PurchaseOrderDocument, error) {
	if version < 1 {
		return PurchaseOrderDocument{}, fmt.Errorf("%w: document version must be positive", ErrPurchaseOrderDocumentInvalid)
	}
	return s.getPurchaseOrderDocument(ctx, purchaseOrderID, version)
}

func (s *Store) getPurchaseOrderDocument(ctx context.Context, purchaseOrderID int64, version int32) (PurchaseOrderDocument, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderDocument{}, err
	}
	if purchaseOrderID < 1 {
		return PurchaseOrderDocument{}, ErrPurchaseOrderDocumentNotFound
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderDocument{}, err
	}
	schema := s.configSchema()
	query := fmt.Sprintf(`
		SELECT d.id, d.uuid::text, d.org_id::text, d.purchase_order_id,
		       d.version, d.pdf_data, d.sha256, d.size_bytes, d.content_type,
		       d.original_filename, d.uploaded_by, d.uploaded_at, d.active,
		       d.replaced_at, d.removed_at, d.removed_by
		FROM %s.purchase_order_documents d
		JOIN %s.purchase_orders po ON po.id = d.purchase_order_id
		WHERE d.purchase_order_id = $1 AND po.org_id = $2`, schema, schema)
	args := []any{purchaseOrderID, org.ID}
	if version == 0 {
		query += " AND d.active"
	} else {
		query += " AND d.version = $3"
		args = append(args, version)
	}
	document, err := scanPurchaseOrderDocument(pool.QueryRow(ctx, query, args...), true)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrderDocument{}, ErrPurchaseOrderDocumentNotFound
	}
	if err != nil {
		return PurchaseOrderDocument{}, err
	}
	return document, nil
}

// RemovePurchaseOrderDocument marks the active document removed. The row keeps
// its bytes and gains removed_at/removed_by, so the correction history still
// answers what the customer sent and who removed it. The confirmation is
// checked before any database work.
func (s *Store) RemovePurchaseOrderDocument(ctx context.Context, input PurchaseOrderDocumentRemoval) (PurchaseOrderDocument, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderDocument{}, err
	}
	if input.PurchaseOrderID < 1 {
		return PurchaseOrderDocument{}, ErrPurchaseOrderDocumentNotFound
	}
	if !input.Confirmed {
		return PurchaseOrderDocument{}, ErrPurchaseOrderDocumentRemovalNotConfirmed
	}
	if input.RemovedBy < 1 {
		return PurchaseOrderDocument{}, fmt.Errorf("%w: the actor is required", ErrPurchaseOrderDocumentInvalid)
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderDocument{}, err
	}
	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrderDocument{}, fmt.Errorf("begin purchase order document removal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockPurchaseOrderForDocument(ctx, tx, schema, org.ID, input.PurchaseOrderID); err != nil {
		if errors.Is(err, ErrPurchaseOrderNotFound) {
			return PurchaseOrderDocument{}, ErrPurchaseOrderDocumentNotFound
		}
		return PurchaseOrderDocument{}, err
	}
	document, err := scanPurchaseOrderDocument(tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_documents
		SET active = FALSE, removed_at = NOW(), removed_by = $2
		WHERE purchase_order_id = $1 AND active
		RETURNING id, uuid::text, org_id::text, purchase_order_id, version,
		          sha256, size_bytes, content_type, original_filename,
		          uploaded_by, uploaded_at, active, replaced_at, removed_at, removed_by`, schema),
		input.PurchaseOrderID, input.RemovedBy), false)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrderDocument{}, ErrPurchaseOrderDocumentNotFound
	}
	if err != nil {
		return PurchaseOrderDocument{}, fmt.Errorf("remove purchase order document: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrderDocument{}, fmt.Errorf("commit purchase order document removal: %w", err)
	}
	return document, nil
}

// lockPurchaseOrderForDocument is the serialization point for attach and
// remove. It both proves the PO belongs to the organization and holds the row
// until the active-pointer change commits.
func lockPurchaseOrderForDocument(ctx context.Context, tx pgx.Tx, schema, orgID string, purchaseOrderID int64) error {
	var id int64
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM %s.purchase_orders
		WHERE id = $1 AND org_id = $2
		FOR UPDATE`, schema), purchaseOrderID, orgID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPurchaseOrderNotFound
	}
	if err != nil {
		return fmt.Errorf("lock purchase order %d for its document: %w", purchaseOrderID, err)
	}
	return nil
}

func requirePurchaseOrderInOrg(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, purchaseOrderID int64) error {
	var id int64
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM %s.purchase_orders WHERE id = $1 AND org_id = $2`, schema),
		purchaseOrderID, orgID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPurchaseOrderNotFound
	}
	if err != nil {
		return fmt.Errorf("check purchase order %d: %w", purchaseOrderID, err)
	}
	return nil
}

// scanPurchaseOrderDocument reads the shared column list. withData selects the
// content shape; history listings omit the bytea column.
func scanPurchaseOrderDocument(row pgx.Row, withData bool) (PurchaseOrderDocument, error) {
	var document PurchaseOrderDocument
	var replacedAt, removedAt pgtype.Timestamptz
	var removedBy pgtype.Int8
	destinations := []any{
		&document.ID, &document.UUID, &document.OrgID, &document.PurchaseOrderID,
		&document.Version,
	}
	if withData {
		destinations = append(destinations, &document.Data)
	}
	destinations = append(destinations,
		&document.SHA256, &document.SizeBytes, &document.ContentType,
		&document.OriginalFilename, &document.UploadedBy, &document.UploadedAt,
		&document.Active, &replacedAt, &removedAt, &removedBy)
	if err := row.Scan(destinations...); err != nil {
		return PurchaseOrderDocument{}, err
	}
	if replacedAt.Valid {
		value := replacedAt.Time
		document.ReplacedAt = &value
	}
	if removedAt.Valid {
		value := removedAt.Time
		document.RemovedAt = &value
	}
	if removedBy.Valid {
		value := removedBy.Int64
		document.RemovedBy = &value
	}
	return document, nil
}

// cleanPurchaseOrderDocumentFilename keeps a display name, never a path. It
// accepts either separator because the desktop may pass a Windows path, drops
// control characters that would break a header, and bounds the result to the
// schema's 255 characters.
func cleanPurchaseOrderDocumentFilename(raw string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	name := path.Base(normalized)
	if name == "." || name == "/" || name == ".." {
		name = ""
	}
	var builder strings.Builder
	count := 0
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			continue
		}
		if count >= 255 {
			break
		}
		builder.WriteRune(character)
		count++
	}
	cleaned := strings.TrimSpace(builder.String())
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "customer-po.pdf"
	}
	return cleaned
}
