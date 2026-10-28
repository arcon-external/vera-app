package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
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
			var generic any
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			if err := decoder.Decode(&generic); err != nil {
				return fmt.Errorf("%w: item %q must carry an integer percent", ErrPurchaseOrderInvalid, item.Key)
			}
			number, ok := generic.(json.Number)
			if !ok {
				// A quoted number ("10") or a bool/object is not a percent.
				return fmt.Errorf("%w: item %q must carry an integer percent", ErrPurchaseOrderInvalid, item.Key)
			}
			text := number.String()
			if strings.ContainsAny(text, ".eE") {
				return fmt.Errorf("%w: item %q must carry an integer percent", ErrPurchaseOrderInvalid, item.Key)
			}
			percent, err := strconv.ParseInt(text, 10, 16)
			if err != nil || percent < 0 || percent > 100 {
				return fmt.Errorf("%w: item %q must carry a percent between 0 and 100",
					ErrPurchaseOrderInvalid, item.Key)
			}
		}
	}
	return nil
}

// createReconciliationSessionTx installs the active session for one exact
// source revision. The link-time tax item (contract section 6) is seeded by
// #517; this slice records the session shell only.
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
		) VALUES ($1,$2,$3,$4,$5,$6,TRUE,FALSE,$7,$8,$9)`, schema),
		orgID, poID, quotationID, revision, revisionIDParam, generation,
		poVersion, lineIdentities, createdBy,
	)
	if err != nil {
		return fmt.Errorf("create reconciliation session for purchase order %d: %w", poID, err)
	}
	return nil
}

// deactivateReconciliationSessionsTx retires the active session before a new
// generation is installed (a relink or, later, a source repair).
func deactivateReconciliationSessionsTx(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, poID int64) error {
	if _, err := q.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_reconciliation_sessions
		SET active = FALSE, updated_at = NOW()
		WHERE purchase_order_id = $1 AND org_id = $2 AND active`, schema),
		poID, orgID); err != nil {
		return fmt.Errorf("deactivate reconciliation session for purchase order %d: %w", poID, err)
	}
	return nil
}

// nextReconciliationGeneration keeps a return to a previously used source
// revision legal under the historical uniqueness rule (R-06).
func nextReconciliationGeneration(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, poID int64) (int, error) {
	var generation int
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(MAX(generation), 0) + 1
		FROM %s.purchase_order_reconciliation_sessions
		WHERE purchase_order_id = $1 AND org_id = $2`, schema),
		poID, orgID).Scan(&generation); err != nil {
		return 0, fmt.Errorf("read next reconciliation generation for purchase order %d: %w", poID, err)
	}
	return generation, nil
}

// syncReconciliationVersionTx keeps the session's recorded PO version current
// after an ordinary draft save, so the next Load returns a usable token (R-04).
func syncReconciliationVersionTx(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, poID int64) error {
	if _, err := q.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_reconciliation_sessions s
		SET po_version = p.version, updated_at = NOW()
		FROM %s.purchase_orders p
		WHERE s.purchase_order_id = $1 AND s.org_id = $2
		  AND s.active AND NOT s.readonly
		  AND p.id = s.purchase_order_id AND p.org_id = s.org_id`, schema, schema),
		poID, orgID); err != nil {
		return fmt.Errorf("sync reconciliation version for purchase order %d: %w", poID, err)
	}
	return nil
}

func loadReconciliationSession(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, poID int64) (POReconciliationSession, int64, error) {
	var session POReconciliationSession
	var createdAt, updatedAt time.Time
	var lineIdentitiesRaw []byte
	var sourceRevision int16
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT s.id, s.purchase_order_id, s.source_quotation_id, s.source_revision,
		       p.version, s.active, s.readonly, s.created_by, s.created_at, s.updated_at,
		       s.line_identities
		FROM %s.purchase_order_reconciliation_sessions s
		JOIN %s.purchase_orders p
		  ON p.id = s.purchase_order_id AND p.org_id = s.org_id
		WHERE s.purchase_order_id = $1 AND s.org_id = $2 AND s.active`, schema, schema),
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
	if session.ID != stored.ID {
		return POReconciliationSession{}, fmt.Errorf(
			"%w: reconciliation session %d is not the active session %d for this purchase order",
			ErrPurchaseOrderInvalid, session.ID, stored.ID)
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
	for _, item := range session.Items {
		previous, ok := byKey[item.Key]
		poValue, err := normalizeReconciliationJSON(item.POValue)
		if err != nil {
			return POReconciliationSession{}, fmt.Errorf("%w: item %q has an invalid value", ErrPurchaseOrderInvalid, item.Key)
		}
		sourceValue, err := normalizeReconciliationJSON(item.SourceValue)
		if err != nil {
			return POReconciliationSession{}, fmt.Errorf("%w: item %q has an invalid value", ErrPurchaseOrderInvalid, item.Key)
		}
		// Repeated identical saves are idempotent: no new event, no new
		// attribution, no history growth (contract section 4).
		if ok && reconciliationItemUnchanged(previous, item, poValue, sourceValue) {
			continue
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
	// Items absent from the submitted array are retained. The client cannot
	// erase a resolved decision or its history by omission; a removed line is
	// resolved explicitly as line_removed (contract sections 4-6).
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

// enforceReconciliationFinalGate is the server-side half of the Final Save
// gate. It refuses while any persisted item is unresolved, and it refuses a
// resolved choice that disagrees with the decision recomputed from the
// submitted values. A pre-041 record has no session and is unaffected (R-07).
func enforceReconciliationFinalGate(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, poID int64, comparison PurchaseOrderComparison, decisions []PurchaseOrderDecision) error {
	session, _, err := loadReconciliationSession(ctx, q, schema, orgID, poID)
	if errors.Is(err, ErrPOReconciliationNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	unresolved := 0
	for _, item := range session.Items {
		if item.Status == POReconciliationStatusUnresolved {
			unresolved++
		}
	}
	if unresolved > 0 {
		return fmt.Errorf("%w: %d reconciliation item(s) are unresolved",
			ErrPurchaseOrderUnresolvedDifference, unresolved)
	}
	chosen := make(map[string]string, len(decisions))
	for _, decision := range decisions {
		chosen[decision.Key] = decision.Choice
	}
	differing := make(map[string]PurchaseOrderDifference, len(comparison.Differences))
	for _, difference := range comparison.Differences {
		differing[difference.Key] = difference
	}
	for _, item := range session.Items {
		if item.Resolution == nil {
			continue
		}
		difference, differs := differing[item.Field]
		switch *item.Resolution {
		case POReconciliationResolutionKeptPO:
			if !differs {
				continue
			}
			if chosen[item.Field] != PurchaseOrderChoicePO {
				return fmt.Errorf("%w: %q was kept from the purchase order but the final save chose otherwise",
					ErrPurchaseOrderUnresolvedDifference, item.Field)
			}
			// The frozen receipt must describe the value actually saved. A
			// recorded decision for an older value is stale, even when the
			// final choice is the same.
			if !reconciliationItemValueMatches(item, difference.POValue) {
				return fmt.Errorf("%w: the recorded decision for %q no longer matches the value submitted for final save",
					ErrPurchaseOrderUnresolvedDifference, item.Field)
			}
		case POReconciliationResolutionUsedQuote:
			if differs && chosen[item.Field] != PurchaseOrderChoiceQuotation {
				return fmt.Errorf("%w: %q was taken from the quotation but the final save chose otherwise",
					ErrPurchaseOrderUnresolvedDifference, item.Field)
			}
		}
	}
	return nil
}

// reconciliationItemValueMatches binds a persisted header decision to the
// value the final save submits. Only the header slice is frozen here; the
// line and tax decisions arrive with #517.
func reconciliationItemValueMatches(item POReconciliationItem, submitted string) bool {
	switch item.Field {
	case "header:paymentTerms", "header:incoterms":
		var text string
		if err := json.Unmarshal(item.POValue, &text); err != nil {
			return false
		}
		return strings.TrimSpace(text) == submitted
	case "header:discountPercent":
		var percent int
		if err := json.Unmarshal(item.POValue, &percent); err != nil {
			return false
		}
		return fmt.Sprintf("%d%%", percent) == submitted
	default:
		return true
	}
}

// maxSafeJSONInteger is the largest integer a JavaScript number represents
// exactly. Values crossing to the desktop are bounded by it (AGENTS.md §10).
const maxSafeJSONInteger = int64(1<<53 - 1)

func reconciliationItemUnchanged(previous, next POReconciliationItem, poValue, sourceValue json.RawMessage) bool {
	if previous.Kind != next.Kind || previous.Field != next.Field || previous.Label != next.Label {
		return false
	}
	if !sameOptionalString(previous.LineKey, next.LineKey) ||
		!sameOptionalString(previous.SourceLineRef, next.SourceLineRef) ||
		!sameOptionalString(previous.Resolution, next.Resolution) {
		return false
	}
	if previous.Status != next.Status {
		return false
	}
	return reconciliationJSONEqual(previous.POValue, poValue) &&
		reconciliationJSONEqual(previous.SourceValue, sourceValue)
}

func sameOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func reconciliationJSONEqual(a, b json.RawMessage) bool {
	var left, right any
	if err := json.Unmarshal(a, &left); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
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
		text := value.String()
		// Only a plain integer is a valid stored number. A decimal or an
		// exponent can hide a value outside JavaScript's exact range.
		if strings.ContainsAny(text, ".eE") {
			return nil, fmt.Errorf("reconciliation numbers must be integers")
		}
		integer, err := value.Int64()
		if err != nil {
			return nil, err
		}
		if integer > maxSafeJSONInteger || integer < -maxSafeJSONInteger {
			return nil, fmt.Errorf("reconciliation integers must stay within 2^53-1")
		}
		return json.Marshal(integer)
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
