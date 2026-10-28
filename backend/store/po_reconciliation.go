package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrPOReconciliationNotFound = errors.New("no reconciliation recorded")
	ErrPOReconciliationReadOnly = errors.New("purchase order reconciliation is read-only")
)

const (
	POReconciliationKindDifference      = "difference"
	POReconciliationKindMissingLine     = "missing_line"
	POReconciliationKindExtraLine       = "extra_line"
	POReconciliationKindDuplicateMatch  = "duplicate_match"
	POReconciliationKindTaxBasis        = "tax_basis"
	POReconciliationStatusUnresolved    = "unresolved"
	POReconciliationStatusResolved      = "resolved"
	POReconciliationResolutionKeptPO    = "kept_po"
	POReconciliationResolutionUsedQuote = "used_quotation"
	POReconciliationResolutionMatched   = "values_now_match"
	POReconciliationResolutionAckMiss   = "acknowledged_missing"
	POReconciliationResolutionAckExtra  = "acknowledged_extra"
	POReconciliationResolutionCorrected = "match_corrected"
	POReconciliationResolutionRemoved   = "line_removed"
)

type POReconciliationSession struct {
	ID                int64
	PurchaseOrderID   int64
	SourceQuotationID int64
	SourceRevision    int16
	POVersion         int64
	Active            bool
	ReadOnly          bool
	CreatedBy         int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LineIdentities    []POReconciliationLineIdentity
	Items             []POReconciliationItem
}

type POReconciliationLineIdentity struct {
	Key     string
	ID      *int64
	Removed bool
}

type POReconciliationItem struct {
	Key           string
	Field         string
	Kind          string
	LineKey       *string
	SourceLineRef *string
	Label         string
	POValue       json.RawMessage
	SourceValue   json.RawMessage
	Status        string
	Resolution    *string
	ActorID       int64
	UpdatedAt     time.Time
	History       []POReconciliationHistoryEvent
}

type POReconciliationHistoryEvent struct {
	Kind          string
	SourceLineRef *string
	POValue       json.RawMessage
	SourceValue   json.RawMessage
	Status        string
	Resolution    *string
	ActorID       int64
	At            time.Time
}

type POReconciliationDiscount struct {
	Enabled bool
	Percent int16
}

type POReconciliationTaxBasis struct {
	TaxZone         string
	TaxType         string
	ServiceCategory string
}

func poReconciliationCatalogue() map[string]bool {
	return map[string]bool{
		"header:paymentTerms":    true,
		"header:incoterms":       true,
		"header:discountPercent": true,
		"line:description":       true,
		"line:uom":               true,
		"line:quantity":          true,
		"line:unitPrice":         true,
		"line:discount":          true,
		"line:match":             true,
		"source:match":           true,
		"link:taxBasis":          true,
		"derived:totals":         true,
	}
}

func poReconciliationKindValid(kind string) bool {
	switch kind {
	case POReconciliationKindDifference, POReconciliationKindMissingLine,
		POReconciliationKindExtraLine, POReconciliationKindDuplicateMatch,
		POReconciliationKindTaxBasis:
		return true
	}
	return false
}

func poReconciliationResolutionValid(resolution string) bool {
	switch resolution {
	case POReconciliationResolutionKeptPO, POReconciliationResolutionUsedQuote,
		POReconciliationResolutionMatched, POReconciliationResolutionAckMiss,
		POReconciliationResolutionAckExtra, POReconciliationResolutionCorrected,
		POReconciliationResolutionRemoved:
		return true
	}
	return false
}

func normalizeReconciliationSlices(session *POReconciliationSession) {
	if session.LineIdentities == nil {
		session.LineIdentities = []POReconciliationLineIdentity{}
	}
	if session.Items == nil {
		session.Items = []POReconciliationItem{}
	}
	for i := range session.Items {
		if session.Items[i].History == nil {
			session.Items[i].History = []POReconciliationHistoryEvent{}
		}
		if session.Items[i].POValue == nil {
			session.Items[i].POValue = json.RawMessage("null")
		}
		if session.Items[i].SourceValue == nil {
			session.Items[i].SourceValue = json.RawMessage("null")
		}
	}
}

func validateReconciliationSessionInput(session POReconciliationSession) error {
	catalogue := poReconciliationCatalogue()
	if session.PurchaseOrderID < 1 {
		return fmt.Errorf("%w: purchase order id is required", ErrPurchaseOrderInvalid)
	}
	if session.SourceQuotationID < 1 {
		return fmt.Errorf("%w: source quotation is required", ErrPurchaseOrderInvalid)
	}
	if session.SourceRevision < 0 {
		return fmt.Errorf("%w: source revision must not be negative", ErrPurchaseOrderInvalid)
	}
	seenIdentities := map[string]bool{}
	seenIDs := map[int64]string{}
	for _, identity := range session.LineIdentities {
		if strings.TrimSpace(identity.Key) == "" {
			return fmt.Errorf("%w: line identity key is required", ErrPurchaseOrderInvalid)
		}
		if seenIdentities[identity.Key] {
			return fmt.Errorf("%w: line identity %q is duplicated", ErrPurchaseOrderInvalid, identity.Key)
		}
		seenIdentities[identity.Key] = true
		if identity.ID != nil {
			if *identity.ID < 1 {
				return fmt.Errorf("%w: line identity id must be positive", ErrPurchaseOrderInvalid)
			}
			if other, taken := seenIDs[*identity.ID]; taken {
				return fmt.Errorf("%w: line id %d is claimed by %q and %q",
					ErrPurchaseOrderInvalid, *identity.ID, other, identity.Key)
			}
			seenIDs[*identity.ID] = identity.Key
		}
	}
	seenKeys := map[string]bool{}
	for _, item := range session.Items {
		if strings.TrimSpace(item.Key) == "" {
			return fmt.Errorf("%w: reconciliation item key is required", ErrPurchaseOrderInvalid)
		}
		if seenKeys[item.Key] {
			return fmt.Errorf("%w: reconciliation item %q is duplicated", ErrPurchaseOrderInvalid, item.Key)
		}
		seenKeys[item.Key] = true
		if !catalogue[item.Field] {
			return fmt.Errorf("%w: %q is not a review value", ErrPurchaseOrderUnresolvedDifference, item.Field)
		}
		if !poReconciliationKindValid(item.Kind) {
			return fmt.Errorf("%w: item %q has an unknown kind", ErrPurchaseOrderInvalid, item.Key)
		}
		if item.Status != POReconciliationStatusUnresolved && item.Status != POReconciliationStatusResolved {
			return fmt.Errorf("%w: item %q has an unknown status", ErrPurchaseOrderInvalid, item.Key)
		}
		if item.Status == POReconciliationStatusResolved {
			if item.Resolution == nil || !poReconciliationResolutionValid(*item.Resolution) {
				return fmt.Errorf("%w: item %q is resolved without a valid resolution",
					ErrPurchaseOrderInvalid, item.Key)
			}
		} else if item.Resolution != nil {
			return fmt.Errorf("%w: item %q is unresolved but carries a resolution",
				ErrPurchaseOrderInvalid, item.Key)
		}
		if strings.TrimSpace(item.Label) == "" {
			return fmt.Errorf("%w: item %q needs a label", ErrPurchaseOrderInvalid, item.Key)
		}
		if item.Field == "derived:totals" {
			return fmt.Errorf("%w: derived totals never create an item", ErrPurchaseOrderInvalid)
		}
		if err := validateReconciliationHeaderValue(item); err != nil {
			return err
		}
	}
	return nil
}

func validateReconciliationHeaderValue(item POReconciliationItem) error {
	switch item.Field {
	case "header:paymentTerms", "header:incoterms":
		for _, raw := range []json.RawMessage{item.POValue, item.SourceValue} {
			if len(raw) == 0 || string(raw) == "null" {
				continue
			}
			var text string
			decoder := json.NewDecoder(bytes.NewReader(raw))
			if err := decoder.Decode(&text); err != nil {
				return fmt.Errorf("%w: item %q must carry text values", ErrPurchaseOrderInvalid, item.Key)
			}
		}
	case "header:discountPercent":
		for _, raw := range []json.RawMessage{item.POValue, item.SourceValue} {
			if len(raw) == 0 || string(raw) == "null" {
				continue
			}
			var number int16
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			var generic any
			if err := decoder.Decode(&generic); err != nil {
				return fmt.Errorf("%w: item %q must carry an integer percent", ErrPurchaseOrderInvalid, item.Key)
			}
			_ = number
			switch value := generic.(type) {
			case json.Number:
				if strings.Contains(value.String(), ".") {
					return fmt.Errorf("%w: item %q must carry an integer percent", ErrPurchaseOrderInvalid, item.Key)
				}
			case float64:
				return fmt.Errorf("%w: item %q must carry an integer percent", ErrPurchaseOrderInvalid, item.Key)
			}
		}
	}
	return nil
}

func createReconciliationSessionTx(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, poID, quotationID int64, revision int16, revisionID int64, generation int, createdBy, poVersion int64) error {
	lineIdentities, err := json.Marshal([]POReconciliationLineIdentity{})
	if err != nil {
		return fmt.Errorf("encode reconciliation line identities: %w", err)
	}
	var revisionIDParam any
	if revisionID > 0 {
		revisionIDParam = revisionID
	}
	_, err = q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_sessions (
			org_id, purchase_order_id, source_quotation_id, source_revision,
			source_revision_id, generation, active, readonly, po_version,
			line_identities, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,TRUE,FALSE,$7,$8,$9)
		ON CONFLICT DO NOTHING`, schema),
		orgID, poID, quotationID, revision, revisionIDParam, generation,
		poVersion, lineIdentities, createdBy,
	)
	if err != nil {
		return fmt.Errorf("create reconciliation session for purchase order %d: %w", poID, err)
	}
	return nil
}

func loadReconciliationSession(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, poID int64) (POReconciliationSession, int64, error) {
	var session POReconciliationSession
	var createdAt, updatedAt time.Time
	var lineIdentitiesRaw []byte
	var sourceRevision int16
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, purchase_order_id, source_quotation_id, source_revision,
		       po_version, active, readonly, created_by, created_at, updated_at,
		       line_identities
		FROM %s.purchase_order_reconciliation_sessions
		WHERE purchase_order_id = $1 AND org_id = $2 AND active`, schema),
		poID, orgID).Scan(
		&session.ID, &session.PurchaseOrderID,
		&session.SourceQuotationID, &sourceRevision,
		&session.POVersion, &session.Active, &session.ReadOnly,
		&session.CreatedBy, &createdAt, &updatedAt, &lineIdentitiesRaw,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return POReconciliationSession{}, 0, ErrPOReconciliationNotFound
		}
		return POReconciliationSession{}, 0, fmt.Errorf("load reconciliation session for purchase order %d: %w", poID, err)
	}
	session.SourceRevision = sourceRevision
	session.CreatedAt = createdAt.UTC()
	session.UpdatedAt = updatedAt.UTC()
	if len(lineIdentitiesRaw) > 0 {
		if err := json.Unmarshal(lineIdentitiesRaw, &session.LineIdentities); err != nil {
			return POReconciliationSession{}, 0, fmt.Errorf("decode reconciliation line identities: %w", err)
		}
	}
	if session.LineIdentities == nil {
		session.LineIdentities = []POReconciliationLineIdentity{}
	}
	items, err := loadReconciliationItems(ctx, q, schema, orgID, session.ID)
	if err != nil {
		return POReconciliationSession{}, 0, err
	}
	session.Items = items
	normalizeReconciliationSlices(&session)
	return session, session.ID, nil
}

func loadReconciliationItems(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, sessionRowID int64) ([]POReconciliationItem, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT key, field, kind, line_key, source_line_ref, label,
		       po_value, source_value, status, resolution, actor_id,
		       updated_at, history
		FROM %s.purchase_order_reconciliation_items
		WHERE session_id = $1 AND org_id = $2
		ORDER BY key`, schema), sessionRowID, orgID)
	if err != nil {
		return nil, fmt.Errorf("load reconciliation items: %w", err)
	}
	defer rows.Close()
	items := []POReconciliationItem{}
	for rows.Next() {
		var item POReconciliationItem
		var lineKey, sourceRef, resolution *string
		var poValue, sourceValue []byte
		var historyRaw []byte
		var updatedAt time.Time
		if err := rows.Scan(&item.Key, &item.Field, &item.Kind, &lineKey,
			&sourceRef, &item.Label, &poValue, &sourceValue, &item.Status,
			&resolution, &item.ActorID, &updatedAt, &historyRaw); err != nil {
			return nil, fmt.Errorf("scan reconciliation item: %w", err)
		}
		item.LineKey = lineKey
		item.SourceLineRef = sourceRef
		item.Resolution = resolution
		item.UpdatedAt = updatedAt.UTC()
		if len(poValue) > 0 {
			item.POValue = json.RawMessage(append([]byte(nil), poValue...))
		} else {
			item.POValue = json.RawMessage("null")
		}
		if len(sourceValue) > 0 {
			item.SourceValue = json.RawMessage(append([]byte(nil), sourceValue...))
		} else {
			item.SourceValue = json.RawMessage("null")
		}
		if len(historyRaw) > 0 {
			if err := json.Unmarshal(historyRaw, &item.History); err != nil {
				return nil, fmt.Errorf("decode reconciliation history for %q: %w", item.Key, err)
			}
		}
		if item.History == nil {
			item.History = []POReconciliationHistoryEvent{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation items: %w", err)
	}
	return items, nil
}

func (s *Store) GetPOReconciliation(ctx context.Context, poID int64) (POReconciliationSession, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return POReconciliationSession{}, err
	}
	if poID < 1 {
		return POReconciliationSession{}, ErrPurchaseOrderNotFound
	}
	pool, err := s.poolOrError()
	if err != nil {
		return POReconciliationSession{}, err
	}
	schema := s.configSchema()
	if err := requirePurchaseOrderInOrg(ctx, pool, schema, org.ID, poID); err != nil {
		return POReconciliationSession{}, err
	}
	session, _, err := loadReconciliationSession(ctx, pool, schema, org.ID, poID)
	if err != nil {
		return POReconciliationSession{}, err
	}
	return session, nil
}

func (s *Store) SavePOReconciliation(ctx context.Context, session POReconciliationSession, actorID int64) (POReconciliationSession, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return POReconciliationSession{}, err
	}
	if session.PurchaseOrderID < 1 {
		return POReconciliationSession{}, ErrPurchaseOrderNotFound
	}
	if actorID < 1 {
		return POReconciliationSession{}, fmt.Errorf("%w: the actor is required", ErrPurchaseOrderInvalid)
	}
	if err := validateReconciliationSessionInput(session); err != nil {
		return POReconciliationSession{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return POReconciliationSession{}, err
	}
	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return POReconciliationSession{}, fmt.Errorf("begin reconciliation save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lock, err := lockPurchaseOrderWrite(ctx, tx, schema, org.ID, session.PurchaseOrderID,
		session.POVersion, PurchaseOrderStateDraft)
	if err != nil {
		return POReconciliationSession{}, err
	}
	stored, _, err := loadReconciliationSession(ctx, tx, schema, org.ID, session.PurchaseOrderID)
	if err != nil {
		return POReconciliationSession{}, err
	}
	if stored.ReadOnly {
		return POReconciliationSession{}, ErrPOReconciliationReadOnly
	}
	if stored.SourceQuotationID != session.SourceQuotationID || stored.SourceRevision != session.SourceRevision {
		return POReconciliationSession{}, fmt.Errorf("%w: the reconciliation source is fixed for this session",
			ErrPurchaseOrderInvalid)
	}
	now := time.Now().UTC()
	lineIdentities, err := json.Marshal(session.LineIdentities)
	if err != nil {
		return POReconciliationSession{}, fmt.Errorf("encode reconciliation line identities: %w", err)
	}
	newPOVersion := lock.Version + 1
	var sessionRowID int64
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_reconciliation_sessions SET
			po_version = $3, line_identities = $4, updated_at = NOW()
		WHERE purchase_order_id = $1 AND org_id = $2 AND active
		RETURNING id`, schema),
		session.PurchaseOrderID, org.ID, newPOVersion, lineIdentities).Scan(&sessionRowID)
	if err != nil {
		return POReconciliationSession{}, fmt.Errorf("update reconciliation session: %w", err)
	}
	existing, err := loadReconciliationItems(ctx, tx, schema, org.ID, sessionRowID)
	if err != nil {
		return POReconciliationSession{}, err
	}
	byKey := map[string]POReconciliationItem{}
	for _, item := range existing {
		byKey[item.Key] = item
	}
	submitted := map[string]bool{}
	for _, item := range session.Items {
		submitted[item.Key] = true
		previous, ok := byKey[item.Key]
		poValue, err := normalizeReconciliationJSON(item.POValue)
		if err != nil {
			return POReconciliationSession{}, fmt.Errorf("%w: item %q has an invalid value", ErrPurchaseOrderInvalid, item.Key)
		}
		sourceValue, err := normalizeReconciliationJSON(item.SourceValue)
		if err != nil {
			return POReconciliationSession{}, fmt.Errorf("%w: item %q has an invalid value", ErrPurchaseOrderInvalid, item.Key)
		}
		event := POReconciliationHistoryEvent{
			Kind:          item.Kind,
			SourceLineRef: item.SourceLineRef,
			POValue:       poValue,
			SourceValue:   sourceValue,
			Status:        item.Status,
			Resolution:    item.Resolution,
			ActorID:       actorID,
			At:            now,
		}
		var history []POReconciliationHistoryEvent
		if ok {
			history = append(append([]POReconciliationHistoryEvent{}, previous.History...), event)
		} else {
			history = []POReconciliationHistoryEvent{event}
		}
		historyRaw, err := json.Marshal(history)
		if err != nil {
			return POReconciliationSession{}, fmt.Errorf("encode reconciliation history: %w", err)
		}
		_, err = tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_reconciliation_items (
				org_id, session_id, purchase_order_id, key, field, kind,
				line_key, source_line_ref, label, po_value, source_value,
				status, resolution, actor_id, history
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (session_id, key) DO UPDATE SET
				field = EXCLUDED.field, kind = EXCLUDED.kind,
				line_key = EXCLUDED.line_key, source_line_ref = EXCLUDED.source_line_ref,
				label = EXCLUDED.label, po_value = EXCLUDED.po_value,
				source_value = EXCLUDED.source_value, status = EXCLUDED.status,
				resolution = EXCLUDED.resolution, actor_id = EXCLUDED.actor_id,
				history = EXCLUDED.history`, schema),
			org.ID, sessionRowID, session.PurchaseOrderID, item.Key, item.Field,
			item.Kind, nullableString(item.LineKey), nullableString(item.SourceLineRef),
			item.Label, string(poValue), string(sourceValue), item.Status,
			nullableString(item.Resolution), actorID, string(historyRaw),
		)
		if err != nil {
			return POReconciliationSession{}, fmt.Errorf("save reconciliation item %q: %w", item.Key, err)
		}
	}
	for _, item := range existing {
		if !submitted[item.Key] {
			if _, err := tx.Exec(ctx, fmt.Sprintf(`
				DELETE FROM %s.purchase_order_reconciliation_items
				WHERE session_id = $1 AND org_id = $2 AND key = $3`, schema),
				sessionRowID, org.ID, item.Key); err != nil {
				return POReconciliationSession{}, fmt.Errorf("remove reconciliation item %q: %w", item.Key, err)
			}
		}
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET updated_by = $3, updated_at = NOW()
		WHERE id = $1 AND org_id = $2`, schema),
		session.PurchaseOrderID, org.ID, actorID); err != nil {
		return POReconciliationSession{}, fmt.Errorf("bump purchase order %d version: %w", session.PurchaseOrderID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return POReconciliationSession{}, fmt.Errorf("commit reconciliation save: %w", err)
	}
	updated, err := s.GetPOReconciliation(ctx, session.PurchaseOrderID)
	if err != nil {
		return POReconciliationSession{}, err
	}
	return updated, nil
}

func normalizeReconciliationJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("null"), nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return json.RawMessage("null"), nil
	}
	var generic any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&generic); err != nil {
		return nil, err
	}
	switch value := generic.(type) {
	case json.Number:
		if _, err := value.Int64(); err != nil {
			number, parseErr := value.Float64()
			if parseErr != nil {
				return nil, parseErr
			}
			if number != float64(int64(number)) {
				return nil, fmt.Errorf("reconciliation numbers must be integers")
			}
			return json.Marshal(int64(number))
		}
		return json.Marshal(generic)
	default:
		return json.RawMessage(append([]byte(nil), trimmed...)), nil
	}
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
