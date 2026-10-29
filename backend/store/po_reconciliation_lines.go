package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Line identity, link-time tax and line-item validation for the PO
// reconciliation workbench (#502, ticket 05).
//
// A review item never keys off the display position: it keys off the line's
// stable identity. A persisted line first entering a session is addressed as
// `id:<row>`; an editor line is addressed by its one-time `client:<UUID>`
// alias, which stays the item's LineKey for the lifetime of the session and
// is bound to the persisted row id on the first draft save. The mapping lives
// in the session's LineIdentities and, durably, in the row's client_key
// column (migration 042); the writer below refuses any second claimant.

// purchaseOrderLineKeyForRow is the persisted-line identity: `id:<row>`.
func purchaseOrderLineKeyForRow(id int64) string {
	return "id:" + strconv.FormatInt(id, 10)
}

// parsePurchaseOrderLineKeyID splits an `id:<row>` identity key.
func parsePurchaseOrderLineKeyID(key string) (int64, bool) {
	number, ok := strings.CutPrefix(key, "id:")
	if !ok || number == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(number, 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

// lineItemKeySuffix maps a line Field to its item-key suffix.
func lineItemKeySuffix(field string) (string, bool) {
	switch field {
	case "line:description":
		return "description", true
	case "line:uom":
		return "uom", true
	case "line:quantity":
		return "quantity", true
	case "line:unitPrice":
		return "unitPrice", true
	case "line:discount":
		return "discount", true
	case "line:match":
		return "match", true
	}
	return "", false
}

// lineItemLabelSuffix is the human suffix for a recomputed line label. Labels
// may track the displayed line number; keys never do (contract section 2).
func lineItemLabelSuffix(field string) string {
	switch field {
	case "line:uom":
		return "unit of measure"
	case "line:unitPrice":
		return "unit price"
	case "line:description":
		return "description"
	case "line:quantity":
		return "quantity"
	case "line:discount":
		return "discount"
	case "line:match":
		return "match"
	}
	return field
}

// purchaseOrderLineReference renders a PO line's source reference ("3" or
// "3.1"), or nil for an extra line the Quotation never carried.
func purchaseOrderLineReference(line PurchaseOrderItem) *string {
	if line.QuotationItemNo == nil {
		return nil
	}
	reference := strconv.Itoa(*line.QuotationItemNo)
	if line.QuotationSubNo != nil {
		reference += "." + strconv.Itoa(*line.QuotationSubNo)
	}
	return &reference
}

// reconciliationTaxTriple projects the persisted tax columns onto the frozen
// TaxBasis triple. A legacy row with no tax type resolves through the same
// legacy resolver the draft path uses; the database legacy field is untouched.
func reconciliationTaxTriple(zone, taxType, serviceCategory, legacyCategory string) POReconciliationTaxBasis {
	zone = strings.TrimSpace(zone)
	taxType = strings.TrimSpace(taxType)
	serviceCategory = strings.TrimSpace(serviceCategory)
	if taxType == "" {
		if mappedType, mappedService, ok := legacyBasisFor(strings.TrimSpace(legacyCategory)); ok {
			taxType, serviceCategory = mappedType, mappedService
		}
	}
	return POReconciliationTaxBasis{
		TaxZone:         zone,
		TaxType:         taxType,
		ServiceCategory: serviceCategory,
	}
}

// seedReconciliationLineIdentitiesTx records the first-seen identity for every
// current row of a new session: the editor alias where the row carries one,
// `id:<row>` otherwise. Creation and linking are the only writers; later
// saves only append bindings and retire removals, so a LineKey never re-keys
// mid-session.
func seedReconciliationLineIdentitiesTx(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, sessionID int64, rows []PurchaseOrderItem) error {
	identities := make([]POReconciliationLineIdentity, 0, len(rows))
	for _, row := range rows {
		key := purchaseOrderLineKeyForRow(row.ID)
		if row.ClientKey != "" {
			key = row.ClientKey
		}
		id := row.ID
		identities = append(identities, POReconciliationLineIdentity{Key: key, ID: &id})
	}
	raw, err := json.Marshal(identities)
	if err != nil {
		return fmt.Errorf("encode reconciliation line identities: %w", err)
	}
	if _, err := q.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_reconciliation_sessions SET
			line_identities = $2, updated_at = NOW()
		WHERE id = $1`, schema), sessionID, raw); err != nil {
		return fmt.Errorf("seed reconciliation line identities: %w", err)
	}
	return nil
}

// seedLinkTaxBasisItemTx persists the resolved link-time tax item for a new
// session (contract section 6): the complete triple from the chosen basis
// beside the source triple, with the link choice. It is read-only context
// afterwards; the generic writer refuses any later mutation.
func seedLinkTaxBasisItemTx(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, sessionID, poID int64, poTriple, sourceTriple POReconciliationTaxBasis, resolution string, actorID int64) error {
	now := time.Now().UTC()
	poRaw, err := json.Marshal(poTriple)
	if err != nil {
		return fmt.Errorf("encode reconciliation tax basis: %w", err)
	}
	sourceRaw, err := json.Marshal(sourceTriple)
	if err != nil {
		return fmt.Errorf("encode reconciliation source tax basis: %w", err)
	}
	history, err := json.Marshal([]POReconciliationHistoryEvent{{
		Kind:        POReconciliationKindTaxBasis,
		POValue:     poRaw,
		SourceValue: sourceRaw,
		Status:      POReconciliationStatusResolved,
		Resolution:  &resolution,
		ActorID:     actorID,
		At:          now,
	}})
	if err != nil {
		return fmt.Errorf("encode reconciliation tax history: %w", err)
	}
	if _, err := q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_items (
			org_id, session_id, purchase_order_id, key, field, kind,
			line_key, source_line_ref, label, po_value, source_value,
			status, resolution, actor_id, history
		) VALUES ($1,$2,$3,'link:taxBasis','link:taxBasis','tax_basis',
			NULL,NULL,'Tax basis',$4,$5,'resolved',$6,$7,$8)`, schema),
		orgID, sessionID, poID, string(poRaw), string(sourceRaw),
		resolution, actorID, string(history),
	); err != nil {
		return fmt.Errorf("seed reconciliation tax basis item: %w", err)
	}
	return nil
}

// syncReconciliationLineStateTx retires removed rows and records new aliases
// after a draft save (ticket 05). It runs in the save transaction, after the
// line upsert, and only touches a live mutable session; a draft without a
// session (never-linked) and a frozen session need nothing.
//
//   - Every kept row without any identity entry gains one: the editor alias
//     where the row carries one, `id:<row>` otherwise. First-seen wins: an
//     existing entry is never re-keyed.
//   - Every removed row flips its entries to Removed and resolves each of
//     their items as line_removed, retaining keys, values and history.
//   - A duplicate conflict cleared by a removal unblocks its survivor as
//     match_corrected (R-08): the surviving line stays matched.
//   - Kept lines' item labels are refreshed to the current display order.
//     Labels may track display position; keys never do. The refresh appends
//     no history and changes no decision.
func syncReconciliationLineStateTx(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, poID int64, kept []PurchaseOrderItem, removed []int64, actorID int64) error {
	session, sessionID, err := loadReconciliationSession(ctx, q, schema, orgID, poID)
	if err != nil {
		if errors.Is(err, ErrPOReconciliationNotFound) {
			return nil
		}
		return err
	}
	if session.ReadOnly {
		return fmt.Errorf("%w: the reconciliation session is frozen", ErrPOReconciliationReadOnly)
	}
	now := time.Now().UTC()
	identities := append([]POReconciliationLineIdentity{}, session.LineIdentities...)
	byKey := make(map[string]int, len(identities))
	byID := make(map[int64][]int, len(identities))
	for index, identity := range identities {
		byKey[identity.Key] = index
		if identity.ID != nil {
			byID[*identity.ID] = append(byID[*identity.ID], index)
		}
	}
	identitiesDirty := false
	ensureIdentity := func(key string, rowID int64) {
		if _, ok := byKey[key]; ok {
			return
		}
		// First-seen wins: a row already addressed by any session key keeps
		// that key. Adopting the row-carried editor alias must not emit a
		// second identity for the same row; the alias stays on the row for
		// persistence addressing.
		if len(byID[rowID]) > 0 {
			return
		}
		identities = append(identities, POReconciliationLineIdentity{Key: key, ID: &rowID})
		byKey[key] = len(identities) - 1
		byID[rowID] = append(byID[rowID], len(identities)-1)
		identitiesDirty = true
	}
	keptByID := make(map[int64]PurchaseOrderItem, len(kept))
	for _, row := range kept {
		keptByID[row.ID] = row
		if row.ClientKey != "" {
			ensureIdentity(row.ClientKey, row.ID)
		} else {
			ensureIdentity(purchaseOrderLineKeyForRow(row.ID), row.ID)
		}
	}
	removedSet := make(map[int64]bool, len(removed))
	for _, id := range removed {
		removedSet[id] = true
	}
	removedKeys := map[string]bool{}
	for _, id := range removed {
		for _, index := range byID[id] {
			if !identities[index].Removed {
				identities[index].Removed = true
				identitiesDirty = true
			}
			removedKeys[identities[index].Key] = true
		}
	}

	// A duplicate conflict cleared by a removal: group the surviving claims.
	claims := map[string]int{}
	for _, row := range kept {
		if reference := purchaseOrderLineReference(row); reference != nil {
			claims[*reference]++
		}
	}

	type itemChange struct {
		key        string
		status     string
		resolution string
		label      string
	}
	changes := make([]itemChange, 0)
	for _, item := range session.Items {
		if item.LineKey == nil {
			continue
		}
		lineKey := *item.LineKey
		if removedKeys[lineKey] {
			if item.Status == POReconciliationStatusResolved &&
				item.Resolution != nil && *item.Resolution == POReconciliationResolutionRemoved {
				continue
			}
			changes = append(changes, itemChange{key: item.Key, status: POReconciliationStatusResolved, resolution: POReconciliationResolutionRemoved, label: item.Label})
			continue
		}
		if item.Kind == POReconciliationKindDuplicateMatch &&
			item.Status == POReconciliationStatusUnresolved &&
			item.SourceLineRef != nil && claims[*item.SourceLineRef] <= 1 {
			changes = append(changes, itemChange{key: item.Key, status: POReconciliationStatusResolved, resolution: POReconciliationResolutionCorrected, label: item.Label})
			continue
		}
		if rowID, ok := identityRowID(identities, byKey, lineKey); ok {
			if row, ok := keptByID[rowID]; ok {
				if label := "Line " + strconv.Itoa(row.LineNumber) + " " + lineItemLabelSuffix(item.Field); label != item.Label {
					changes = append(changes, itemChange{key: item.Key, status: item.Status, resolution: valueOrEmpty(item.Resolution), label: label})
				}
			}
		}
	}
	if identitiesDirty {
		raw, err := json.Marshal(identities)
		if err != nil {
			return fmt.Errorf("encode reconciliation line identities: %w", err)
		}
		if _, err := q.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_order_reconciliation_sessions SET
				line_identities = $2, updated_at = NOW()
			WHERE id = $1`, schema), sessionID, raw); err != nil {
			return fmt.Errorf("sync reconciliation line identities: %w", err)
		}
	}
	for _, change := range changes {
		var stored POReconciliationItem
		for _, item := range session.Items {
			if item.Key == change.key {
				stored = item
				break
			}
		}
		event := POReconciliationHistoryEvent{
			Kind:          stored.Kind,
			SourceLineRef: stored.SourceLineRef,
			POValue:       stored.POValue,
			SourceValue:   stored.SourceValue,
			Status:        change.status,
			Resolution:    &change.resolution,
			ActorID:       actorID,
			At:            now,
		}
		// A label-only refresh is display maintenance, not a decision: it
		// appends no history event.
		history := stored.History
		if change.resolution != valueOrEmpty(stored.Resolution) || change.status != stored.Status {
			history = append(append([]POReconciliationHistoryEvent{}, stored.History...), event)
		}
		historyRaw, err := json.Marshal(history)
		if err != nil {
			return fmt.Errorf("encode reconciliation history: %w", err)
		}
		var resolutionParam any
		if change.resolution != "" {
			resolutionParam = change.resolution
		}
		if _, err := q.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_order_reconciliation_items SET
				status = $3, resolution = $4, label = $5, actor_id = $6,
				history = $7
			WHERE session_id = $1 AND key = $2`, schema),
			sessionID, change.key, change.status, resolutionParam,
			change.label, actorID, string(historyRaw),
		); err != nil {
			return fmt.Errorf("resolve reconciliation item %q: %w", change.key, err)
		}
	}
	return nil
}

// identityRowID resolves a LineKey to its persisted row through the session
// identities. The second result is false for a key the session never recorded.
func identityRowID(identities []POReconciliationLineIdentity, byKey map[string]int, lineKey string) (int64, bool) {
	index, ok := byKey[lineKey]
	if !ok || identities[index].ID == nil {
		return 0, false
	}
	return *identities[index].ID, true
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// validateReconciliationLineItemShape checks key/field/kind/reference
// consistency and the kind/resolution matrix for line and tax items. Header
// items keep their existing checks; existence against identities and rows is
// enforced on the write path, where the stored set is available.
func validateReconciliationLineItemShape(item POReconciliationItem) error {
	switch item.Field {
	case "header:paymentTerms", "header:incoterms", "header:discountPercent":
		return nil
	case "derived:totals":
		return nil
	}
	suffix, isLine := lineItemKeySuffix(item.Field)
	switch {
	case isLine:
		if item.LineKey == nil || strings.TrimSpace(*item.LineKey) == "" {
			return fmt.Errorf("%w: item %q needs a line key", ErrPurchaseOrderInvalid, item.Key)
		}
		if want := "line:" + *item.LineKey + ":" + suffix; item.Key != want {
			return fmt.Errorf("%w: item %q must be keyed %q", ErrPurchaseOrderInvalid, item.Key, want)
		}
		if _, isID := parsePurchaseOrderLineKeyID(*item.LineKey); !isID && !strings.HasPrefix(*item.LineKey, "client:") {
			return fmt.Errorf("%w: item %q carries an unknown line identity", ErrPurchaseOrderInvalid, item.Key)
		}
		var wantKinds []string
		if item.Field == "line:match" {
			wantKinds = []string{POReconciliationKindExtraLine, POReconciliationKindDuplicateMatch}
		} else {
			wantKinds = []string{POReconciliationKindDifference}
			if item.SourceLineRef == nil || strings.TrimSpace(*item.SourceLineRef) == "" {
				return fmt.Errorf("%w: item %q needs its source reference", ErrPurchaseOrderInvalid, item.Key)
			}
		}
		if !containsReconciliationKind(wantKinds, item.Kind) {
			return fmt.Errorf("%w: item %q cannot be a %q", ErrPurchaseOrderInvalid, item.Key, item.Kind)
		}
		if item.Field == "line:match" && item.Kind == POReconciliationKindDuplicateMatch &&
			(item.SourceLineRef == nil || strings.TrimSpace(*item.SourceLineRef) == "") {
			return fmt.Errorf("%w: item %q needs the contested reference", ErrPurchaseOrderInvalid, item.Key)
		}
		if item.Field == "line:match" && item.Kind == POReconciliationKindExtraLine && item.SourceLineRef != nil {
			return fmt.Errorf("%w: item %q is purchase-order side and carries no source reference", ErrPurchaseOrderInvalid, item.Key)
		}
	case item.Field == "source:match":
		if item.LineKey != nil {
			return fmt.Errorf("%w: item %q is source side and carries no line key", ErrPurchaseOrderInvalid, item.Key)
		}
		if item.SourceLineRef == nil || strings.TrimSpace(*item.SourceLineRef) == "" {
			return fmt.Errorf("%w: item %q needs its source reference", ErrPurchaseOrderInvalid, item.Key)
		}
		if want := "source:" + *item.SourceLineRef + ":match"; item.Key != want {
			return fmt.Errorf("%w: item %q must be keyed %q", ErrPurchaseOrderInvalid, item.Key, want)
		}
		if item.Kind != POReconciliationKindMissingLine {
			return fmt.Errorf("%w: item %q cannot be a %q", ErrPurchaseOrderInvalid, item.Key, item.Kind)
		}
	case item.Field == "link:taxBasis":
		if item.Key != "link:taxBasis" {
			return fmt.Errorf("%w: item %q must be keyed %q", ErrPurchaseOrderInvalid, item.Key, "link:taxBasis")
		}
		if item.LineKey != nil || item.SourceLineRef != nil {
			return fmt.Errorf("%w: item %q is session context and names no line", ErrPurchaseOrderInvalid, item.Key)
		}
		if item.Kind != POReconciliationKindTaxBasis {
			return fmt.Errorf("%w: item %q cannot be a %q", ErrPurchaseOrderInvalid, item.Key, item.Kind)
		}
	default:
		return fmt.Errorf("%w: %q is not a review value", ErrPurchaseOrderUnresolvedDifference, item.Field)
	}
	if item.Status == POReconciliationStatusResolved && item.Resolution != nil {
		var want []string
		switch item.Kind {
		case POReconciliationKindDifference:
			// line_removed retires every item of a removed line, including
			// its field differences; the writer's removed-line rule below
			// requires the matching Removed identity for it.
			want = []string{POReconciliationResolutionKeptPO, POReconciliationResolutionUsedQuote, POReconciliationResolutionMatched, POReconciliationResolutionRemoved}
		case POReconciliationKindMissingLine:
			want = []string{POReconciliationResolutionAckMiss, POReconciliationResolutionCorrected}
		case POReconciliationKindExtraLine:
			want = []string{POReconciliationResolutionAckExtra, POReconciliationResolutionCorrected, POReconciliationResolutionRemoved}
		case POReconciliationKindDuplicateMatch:
			want = []string{POReconciliationResolutionCorrected, POReconciliationResolutionRemoved}
		case POReconciliationKindTaxBasis:
			want = []string{POReconciliationResolutionKeptPO, POReconciliationResolutionUsedQuote}
		}
		if !containsReconciliationResolution(want, *item.Resolution) {
			return fmt.Errorf("%w: item %q cannot resolve as %q", ErrPurchaseOrderInvalid, item.Key, *item.Resolution)
		}
		if item.Kind == POReconciliationKindDuplicateMatch && *item.Resolution != POReconciliationResolutionCorrected &&
			*item.Resolution != POReconciliationResolutionRemoved {
			return fmt.Errorf("%w: a duplicate match is never merely acknowledged", ErrPurchaseOrderInvalid)
		}
	}
	// A recorded source reference is always canonical reference text, never
	// display prose: values below bind decisions to it.
	if item.SourceLineRef != nil && !isReconciliationReference(strings.TrimSpace(*item.SourceLineRef)) {
		return fmt.Errorf("%w: item %q carries a malformed source reference", ErrPurchaseOrderInvalid, item.Key)
	}
	return validateReconciliationLineValue(item)
}

func containsReconciliationKind(kinds []string, kind string) bool {
	for _, candidate := range kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

func containsReconciliationResolution(resolutions []string, resolution string) bool {
	for _, candidate := range resolutions {
		if candidate == resolution {
			return true
		}
	}
	return false
}

// validateReconciliationLineValue checks the value shape a Field determines:
// text, integers in stored units, the Discount object, reference strings and
// the TaxBasis triple. Monetary and quantity values reuse the plain-integer,
// safe-range normalization; display strings are never compared here.
func validateReconciliationLineValue(item POReconciliationItem) error {
	nonNull := func(raw json.RawMessage) bool {
		trimmed := bytes.TrimSpace(raw)
		return len(trimmed) > 0 && string(trimmed) != "null"
	}
	switch item.Field {
	case "line:description", "line:uom":
		for _, raw := range []json.RawMessage{item.POValue, item.SourceValue} {
			if !nonNull(raw) {
				return fmt.Errorf("%w: item %q must carry text values", ErrPurchaseOrderInvalid, item.Key)
			}
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return fmt.Errorf("%w: item %q must carry text values", ErrPurchaseOrderInvalid, item.Key)
			}
		}
		return nil
	case "line:quantity", "line:unitPrice":
		for _, raw := range []json.RawMessage{item.POValue, item.SourceValue} {
			canonical, err := normalizeReconciliationJSON(raw)
			if err != nil || !nonNull(canonical) {
				return fmt.Errorf("%w: item %q must carry an integer", ErrPurchaseOrderInvalid, item.Key)
			}
			var number int64
			if err := json.Unmarshal(canonical, &number); err != nil {
				return fmt.Errorf("%w: item %q must carry an integer", ErrPurchaseOrderInvalid, item.Key)
			}
			if item.Field == "line:quantity" && number <= 0 {
				return fmt.Errorf("%w: item %q must carry a positive quantity", ErrPurchaseOrderInvalid, item.Key)
			}
			if item.Field == "line:unitPrice" && number < 0 {
				return fmt.Errorf("%w: item %q must not carry a negative price", ErrPurchaseOrderInvalid, item.Key)
			}
		}
		return nil
	case "line:discount":
		for _, raw := range []json.RawMessage{item.POValue, item.SourceValue} {
			if !nonNull(raw) {
				return fmt.Errorf("%w: item %q must carry discount values", ErrPurchaseOrderInvalid, item.Key)
			}
			members, err := exactReconciliationObject(raw, "discount", "Enabled", "Percent")
			if err != nil {
				return fmt.Errorf("%w: item %q must carry a discount value: %v", ErrPurchaseOrderInvalid, item.Key, err)
			}
			if isReconciliationNull(members["enabled"]) || isReconciliationNull(members["percent"]) {
				return fmt.Errorf("%w: item %q Enabled and Percent must be non-null", ErrPurchaseOrderInvalid, item.Key)
			}
			var enabled bool
			if err := json.Unmarshal(members["enabled"], &enabled); err != nil {
				return fmt.Errorf("%w: item %q Enabled must be a boolean", ErrPurchaseOrderInvalid, item.Key)
			}
			canonical, err := normalizeReconciliationJSON(members["percent"])
			if err != nil {
				return fmt.Errorf("%w: item %q Percent must be an integer", ErrPurchaseOrderInvalid, item.Key)
			}
			var percent int64
			if err := json.Unmarshal(canonical, &percent); err != nil || percent < 0 || percent > 100 {
				return fmt.Errorf("%w: item %q must carry a percent between 0 and 100", ErrPurchaseOrderInvalid, item.Key)
			}
		}
		return nil
	case "line:match":
		if nonNull(item.SourceValue) {
			return fmt.Errorf("%w: item %q is purchase-order side and carries no source value", ErrPurchaseOrderInvalid, item.Key)
		}
		if item.Kind == POReconciliationKindDuplicateMatch {
			if !nonNull(item.POValue) {
				return fmt.Errorf("%w: item %q must carry the contested reference", ErrPurchaseOrderInvalid, item.Key)
			}
			reference, err := reconciliationReferenceValue(item.POValue)
			if err != nil {
				return fmt.Errorf("%w: item %q must carry a reference value", ErrPurchaseOrderInvalid, item.Key)
			}
			if item.SourceLineRef == nil || strings.TrimSpace(*item.SourceLineRef) != reference {
				return fmt.Errorf("%w: item %q must contest its own reference", ErrPurchaseOrderInvalid, item.Key)
			}
			return nil
		}
		if nonNull(item.POValue) {
			if _, err := reconciliationReferenceValue(item.POValue); err != nil {
				return fmt.Errorf("%w: item %q must carry a reference value", ErrPurchaseOrderInvalid, item.Key)
			}
		}
		return nil
	case "source:match":
		if nonNull(item.POValue) {
			return fmt.Errorf("%w: item %q is source side and carries no purchase-order value", ErrPurchaseOrderInvalid, item.Key)
		}
		if !nonNull(item.SourceValue) {
			return fmt.Errorf("%w: item %q must carry the missing reference", ErrPurchaseOrderInvalid, item.Key)
		}
		reference, err := reconciliationReferenceValue(item.SourceValue)
		if err != nil || reference != strings.TrimSpace(*item.SourceLineRef) {
			return fmt.Errorf("%w: item %q must carry its missing reference", ErrPurchaseOrderInvalid, item.Key)
		}
		return nil
	case "link:taxBasis":
		for _, raw := range []json.RawMessage{item.POValue, item.SourceValue} {
			if !nonNull(raw) {
				return fmt.Errorf("%w: item %q must carry tax basis values", ErrPurchaseOrderInvalid, item.Key)
			}
			members, err := exactReconciliationObject(raw, "tax basis", "TaxZone", "TaxType", "ServiceCategory")
			if err != nil {
				return fmt.Errorf("%w: item %q must carry a tax basis value: %v", ErrPurchaseOrderInvalid, item.Key, err)
			}
			for _, member := range []string{"taxzone", "taxtype", "servicecategory"} {
				if isReconciliationNull(members[member]) {
					return fmt.Errorf("%w: item %q %s must be text", ErrPurchaseOrderInvalid, item.Key, member)
				}
				var text string
				if err := json.Unmarshal(members[member], &text); err != nil {
					return fmt.Errorf("%w: item %q %s must be text", ErrPurchaseOrderInvalid, item.Key, member)
				}
			}
		}
		return nil
	}
	return nil
}

// isReconciliationNull reports an absent or JSON-null value.
func isReconciliationNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || string(trimmed) == "null"
}

// exactReconciliationObject decodes a JSON object and requires exactly the
// wanted member names, matched ASCII case-insensitively per Go's documented
// field behavior, each present once. Missing, null-extra and unknown members
// are refused; Go zero-values must never stand in for absent members.
func exactReconciliationObject(raw json.RawMessage, what string, want ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(raw)))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%s must be an object: %v", what, err)
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("%s must be an object", what)
	}
	// Member names are detected at the token level BEFORE map insertion:
	// identically spelled, mixed-case and escaped-equivalent duplicates
	// collide even with identical values, because map decoding would
	// collapse them with last-value-wins first.
	folded := make(map[string]json.RawMessage)
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%s carries a malformed member: %v", what, err)
		}
		name, ok := nameToken.(string)
		if !ok {
			return nil, fmt.Errorf("%s carries a malformed member name", what)
		}
		lowered := asciiFoldReconciliationKey(name)
		if _, duplicate := folded[lowered]; duplicate {
			return nil, fmt.Errorf("%s repeats member %q", what, name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("%s carries a malformed value for %q: %v", what, name, err)
		}
		folded[lowered] = value
	}
	closing, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%s is truncated: %v", what, err)
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return nil, fmt.Errorf("%s is malformed", what)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("%s carries trailing data", what)
	}
	if len(folded) != len(want) {
		return nil, fmt.Errorf("%s must carry exactly %s", what, strings.Join(want, ", "))
	}
	for _, name := range want {
		if _, ok := folded[asciiFoldReconciliationKey(name)]; !ok {
			return nil, fmt.Errorf("%s must carry %q", what, name)
		}
	}
	return folded, nil
}

// asciiFoldReconciliationKey folds ASCII A-Z to a-z for member-name
// comparison, matching the documented compatible casing. Unicode lookalikes
// are not folded and stay unknown members.
func asciiFoldReconciliationKey(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, name)
}

// reconciliationReferenceValue decodes a canonical source reference:
// digits with one optional dotted part ("9", "2.1"). Display prose is never
// a reference.
func reconciliationReferenceValue(raw json.RawMessage) (string, error) {
	var reference string
	if err := json.Unmarshal(bytes.TrimSpace(raw), &reference); err != nil {
		return "", err
	}
	if !isReconciliationReference(strings.TrimSpace(reference)) {
		return "", fmt.Errorf("%q is not a source reference", reference)
	}
	return strings.TrimSpace(reference), nil
}

// isReconciliationReference reports the canonical reference shape.
func isReconciliationReference(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) < 1 || len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// checkReconciliationLineIdentities requires every submitted line reference
// to name a recorded session identity exactly. The server owns the mapping:
// draft saves, links and creation record one first-seen key per row, and the
// pane resolves its editor alias to that key before writing (the DTO rows
// carry both ID and ClientKey for the translation). Unknown keys are refused
// with guidance to save the draft first, so the pane cannot invent decisions
// for rows that do not exist.
func checkReconciliationLineIdentities(stored []POReconciliationLineIdentity, items []POReconciliationItem) error {
	recorded := make(map[string]bool, len(stored))
	for _, identity := range stored {
		recorded[identity.Key] = true
	}
	for _, item := range items {
		if item.LineKey == nil {
			continue
		}
		if !recorded[*item.LineKey] {
			return fmt.Errorf("%w: item %q names an unknown line identity; reload the session after saving the draft",
				ErrPurchaseOrderInvalid, item.Key)
		}
	}
	return nil
}

// checkReconciliationRemovedItems enforces that an item pointing at a removed
// line resolves as line_removed: a removed line keeps its keys, values and
// history, and no other decision applies to it.
func checkReconciliationRemovedItems(identities []POReconciliationLineIdentity, items []POReconciliationItem) error {
	removed := map[string]bool{}
	for _, identity := range identities {
		if identity.Removed {
			removed[identity.Key] = true
		}
	}
	for _, item := range items {
		if item.LineKey == nil || !removed[*item.LineKey] {
			continue
		}
		if item.Status != POReconciliationStatusResolved || item.Resolution == nil ||
			*item.Resolution != POReconciliationResolutionRemoved {
			return fmt.Errorf("%w: item %q names a removed line and must resolve as line_removed",
				ErrPurchaseOrderInvalid, item.Key)
		}
	}
	return nil
}

// checkReconciliationTaxItemsUnchanged protects the link-time tax basis: after
// linking it is read-only context. The generic writer refuses any mutation
// and never mints a tax item; omission retains the stored one.
func checkReconciliationTaxItemsUnchanged(stored, submitted []POReconciliationItem) error {
	byKey := make(map[string]POReconciliationItem, len(stored))
	for _, item := range stored {
		if item.Field == "link:taxBasis" {
			byKey[item.Key] = item
		}
	}
	for _, item := range submitted {
		if item.Field != "link:taxBasis" {
			continue
		}
		previous, ok := byKey[item.Key]
		if !ok {
			return fmt.Errorf("%w: the link-time tax basis is recorded at link time, not through this writer",
				ErrPurchaseOrderInvalid)
		}
		if item.Kind != previous.Kind || item.Label != previous.Label ||
			!sameOptionalString(item.LineKey, previous.LineKey) ||
			!sameOptionalString(item.SourceLineRef, previous.SourceLineRef) ||
			item.Status != previous.Status ||
			!sameOptionalString(item.Resolution, previous.Resolution) ||
			!reconciliationJSONEqual(normalizeForCompare(item.POValue), normalizeForCompare(previous.POValue)) ||
			!reconciliationJSONEqual(normalizeForCompare(item.SourceValue), normalizeForCompare(previous.SourceValue)) {
			return fmt.Errorf("%w: the link-time tax basis is read-only context",
				ErrPurchaseOrderInvalid)
		}
	}
	return nil
}

func normalizeForCompare(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage("null")
	}
	return raw
}

// canonicalReconciliationLineValue renders a submitted line's value for a
// field in the canonical stored form, so the Final Save gate can bind a
// persisted decision to the value actually submitted.
func canonicalReconciliationLineValue(field string, line PurchaseOrderItem) (json.RawMessage, error) {
	switch field {
	case "line:description":
		raw, err := json.Marshal(strings.TrimSpace(line.Description))
		if err != nil {
			return nil, err
		}
		return raw, nil
	case "line:uom":
		raw, err := json.Marshal(strings.TrimSpace(line.UoM))
		if err != nil {
			return nil, err
		}
		return raw, nil
	case "line:quantity":
		return json.Marshal(line.QuantityScaled)
	case "line:unitPrice":
		return json.Marshal(line.UnitPriceMinor)
	case "line:discount":
		percent := line.DiscountPercent
		if !line.DiscountEnabled {
			percent = 0
		}
		return json.Marshal(POReconciliationDiscount{Enabled: line.DiscountEnabled, Percent: percent})
	case "line:match":
		if reference := purchaseOrderLineReference(line); reference != nil {
			return json.Marshal(*reference)
		}
		return json.RawMessage("null"), nil
	}
	return nil, fmt.Errorf("no canonical submitted value for %q", field)
}

// canonicalReconciliationStoredDiscount canonicalizes a stored discount value
// the same way: a disabled discount carries Percent 0 whatever dormant
// percentage the editor holds.
func canonicalReconciliationStoredDiscount(raw json.RawMessage) json.RawMessage {
	var discount POReconciliationDiscount
	if err := json.Unmarshal(raw, &discount); err != nil {
		return raw
	}
	if !discount.Enabled {
		discount.Percent = 0
	}
	canonical, err := json.Marshal(discount)
	if err != nil {
		return raw
	}
	return canonical
}
