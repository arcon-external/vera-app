package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The server-side reconciliation composer for a source repair (#502, #518 B2).
//
// Until B2 the draft workbench's items were supplied by the frontend and only
// validated here: ComparePurchaseOrder formats values for display, and the
// generic writer persists whatever the editor sends. A repair runs inside one
// store transaction and must seed a *terminal*, read-only generation without a
// client, so it needs a composer of its own.
//
// Three comparisons, one item set:
//
//   - Stage A compares the locked stored record with the target revision.
//   - The review projection compares the same locked scalars after applying the
//     admin's corrected source references. This is what a rematch/Use actually
//     reviewed: quantity 3 on the locked row against the newly matched line's 2.
//   - Stage B compares the final submitted record with the target.
//
// A field that matched in every observed projection produces no item. A field
// that differed before an explicit Use keeps that comparison in its history even
// though the final value matches, which is the point of the record.

// reconciliationRepairSeed is the in-memory terminal state a repair installs.
type reconciliationRepairSeed struct {
	Items      []POReconciliationItem
	Identities []POReconciliationLineIdentity
}

// repairHeaders are the editable header slots the comparison covers.
type repairHeaders struct {
	PaymentTerms    string
	Incoterms       string
	DiscountPercent int16
}

func (headers repairHeaders) state(lines []PurchaseOrderItem) PurchaseOrderReviewState {
	return PurchaseOrderReviewState{
		PaymentTerms:    headers.PaymentTerms,
		Incoterms:       headers.Incoterms,
		DiscountPercent: headers.DiscountPercent,
		Lines:           lines,
	}
}

// repairLine is one line in one stage of the comparison, with the identity the
// item keys use and the source line it references in the target revision.
type repairLine struct {
	line     PurchaseOrderItem
	key      string
	display  int
	ref      string
	sourceAt *PurchaseOrderSourceLine
}

type observedRepairValue struct {
	po  json.RawMessage
	src json.RawMessage
}

type repairItemDraft struct {
	key          string
	field        string
	kind         string
	lineKey      *string
	sourceRef    *string
	label        string
	observed     []observedRepairValue
	finalPO      json.RawMessage
	finalSource  json.RawMessage
	finalCompKey string
	hasFinal     bool
	removed      bool
	acknowledged bool
}

// buildRepairReconciliationSeed composes the terminal items and line identities
// for the generation a repair is about to install.
//
// `applied` is the final submitted line set (line numbers 1..n); `kept` is
// replacePurchaseOrderItems' resolved result in the same order; `removed` is
// the row ids it retired.
func buildRepairReconciliationSeed(
	stored PurchaseOrder,
	applied []PurchaseOrderItem,
	kept []PurchaseOrderItem,
	removed []int64,
	oldIdentities []POReconciliationLineIdentity,
	source PurchaseOrderSource,
	finalHeaders repairHeaders,
	decisions []PurchaseOrderDecision,
	acknowledgedMissing []string,
	acknowledgedExtra []int,
	actorID int64,
	now time.Time,
) (reconciliationRepairSeed, error) {
	seed := reconciliationRepairSeed{
		Items:      []POReconciliationItem{},
		Identities: []POReconciliationLineIdentity{},
	}

	// First-seen identity by row, from the generation this repair retires.
	firstSeen := map[int64]string{}
	for _, identity := range oldIdentities {
		if identity.ID != nil {
			firstSeen[*identity.ID] = identity.Key
		}
	}
	keyFor := func(rowID int64, clientKey string) string {
		if key, ok := firstSeen[rowID]; ok && rowID > 0 {
			return key
		}
		if clientKey != "" {
			return clientKey
		}
		return purchaseOrderLineKeyForRow(rowID)
	}

	storedByID := make(map[int64]PurchaseOrderItem, len(stored.Items))
	for _, item := range stored.Items {
		storedByID[item.ID] = item
	}
	chosen := make(map[string]string, len(decisions))
	for _, decision := range decisions {
		chosen[decision.Key] = decision.Choice
	}

	storedHeaders := repairHeaders{
		PaymentTerms:    stored.PaymentTerms,
		Incoterms:       stored.Incoterms,
		DiscountPercent: stored.DiscountPercent,
	}

	// --- Stage A: the locked stored record against the target revision.
	stageA, stateA := repairStageLines(stored.Items, storedHeaders, source, keyFor)
	comparisonA := ComparePurchaseOrder(source, stateA)

	// --- Projection: the locked scalars with the admin's corrected references.
	projected := make([]PurchaseOrderItem, 0, len(kept))
	for index := range kept {
		final := applied[index]
		base, locked := storedByID[kept[index].ID]
		if !locked {
			base = final
		}
		base.QuotationItemNo = final.QuotationItemNo
		base.QuotationSubNo = final.QuotationSubNo
		base.LineNumber = index + 1
		projected = append(projected, base)
	}
	stageP, stateP := repairStageLines(projected, storedHeaders, source, keyFor)
	comparisonP := ComparePurchaseOrder(source, stateP)

	// --- Stage B: the final submitted record against the target revision.
	stageB, stateB := repairStageLines(applied, finalHeaders, source, keyFor)
	comparisonB := ComparePurchaseOrder(source, stateB)

	drafts := map[string]*repairItemDraft{}
	order := []string{}
	draftFor := func(key, field, kind, label string, lineKey, sourceRef *string) *repairItemDraft {
		if draft, exists := drafts[key]; exists {
			return draft
		}
		draft := &repairItemDraft{
			key: key, field: field, kind: kind, label: label,
			lineKey: lineKey, sourceRef: sourceRef,
		}
		drafts[key] = draft
		order = append(order, key)
		return draft
	}
	addObservation := func(draft *repairItemDraft, po, src json.RawMessage) {
		if len(draft.observed) > 0 {
			last := draft.observed[len(draft.observed)-1]
			if sameReconciliationValue(last.po, po) && sameReconciliationValue(last.src, src) {
				return
			}
		}
		draft.observed = append(draft.observed, observedRepairValue{po: po, src: src})
	}

	for _, stage := range []struct {
		lines      []repairLine
		headers    repairHeaders
		comparison PurchaseOrderComparison
		final      bool
	}{
		{stageA, storedHeaders, comparisonA, false},
		{stageP, storedHeaders, comparisonP, false},
		{stageB, finalHeaders, comparisonB, true},
	} {
		for _, difference := range stage.comparison.Differences {
			field, lineNumber, isLine, err := splitReviewDifferenceKey(difference.Key)
			if err != nil {
				return seed, err
			}
			if !isLine {
				poRaw, err := canonicalHeaderValue(field, stage.headers)
				if err != nil {
					return seed, err
				}
				srcRaw, err := canonicalHeaderValue(field, sourceHeadersOf(source))
				if err != nil {
					return seed, err
				}
				draft := draftFor(field, field, POReconciliationKindDifference, difference.Label, nil, nil)
				addObservation(draft, poRaw, srcRaw)
				if stage.final {
					draft.hasFinal = true
					draft.finalPO = poRaw
					draft.finalSource = srcRaw
					draft.finalCompKey = difference.Key
				}
				continue
			}
			line := stage.lines[lineNumber-1]
			suffix, _ := lineItemKeySuffix(field)
			itemKey := "line:" + line.key + ":" + suffix
			lineKey := line.key
			draft := draftFor(itemKey, field, POReconciliationKindDifference, difference.Label,
				&lineKey, stringPointerOrNil(line.ref))
			poRaw, err := canonicalReconciliationLineValue(field, line.line)
			if err != nil {
				return seed, err
			}
			srcRaw, err := canonicalSourceLineValue(field, line.sourceAt)
			if err != nil {
				return seed, err
			}
			addObservation(draft, poRaw, srcRaw)
			if stage.final {
				draft.hasFinal = true
				draft.finalPO = poRaw
				draft.finalSource = srcRaw
				draft.finalCompKey = difference.Key
			}
		}
	}

	// Missing source lines: unioned across stages, so a rematch records that the
	// line it now claims was initially missing.
	missingSeen := map[string]bool{}
	for _, stage := range []struct {
		comparison PurchaseOrderComparison
		final      bool
	}{{comparisonA, false}, {comparisonP, false}, {comparisonB, true}} {
		for _, missing := range stage.comparison.Missing {
			ref := missing.Label()
			sourceRef := ref
			draft := draftFor("source:"+ref+":match", "source:match",
				POReconciliationKindMissingLine, "Quotation line "+ref, nil, &sourceRef)
			if !missingSeen[ref] {
				missingSeen[ref] = true
				poNull := json.RawMessage("null")
				raw := jsonString(ref)
				addObservation(draft, poNull, raw)
				draft.finalPO = poNull
				draft.finalSource = raw
			}
			if stage.final {
				draft.hasFinal = true
				if acknowledgedContains(acknowledgedMissing, ref) {
					draft.acknowledged = true
				}
			}
		}
	}

	// Extra purchase-order lines: acknowledged as submitted, or corrected when
	// the final record claims the reference, or retired when the row is gone.
	extraSeen := map[string]bool{}
	removedKeys := map[string]bool{}
	for _, id := range removed {
		removedKeys[keyFor(id, "")] = true
	}
	for _, stage := range []struct {
		lines      []repairLine
		comparison PurchaseOrderComparison
		final      bool
	}{{stageA, comparisonA, false}, {stageP, comparisonP, false}, {stageB, comparisonB, true}} {
		for _, lineNumber := range stage.comparison.ExtraLineNumbers {
			line := stage.lines[lineNumber-1]
			if extraSeen[line.key] {
				continue
			}
			extraSeen[line.key] = true
			lineKey := line.key
			draft := draftFor("line:"+line.key+":match", "line:match",
				POReconciliationKindExtraLine,
				"Line "+strconv.Itoa(line.display)+" match", &lineKey, nil)
			var poRaw json.RawMessage = json.RawMessage("null")
			if line.ref != "" {
				poRaw = jsonString(line.ref)
			}
			addObservation(draft, poRaw, json.RawMessage("null"))
			draft.finalPO = poRaw
			draft.finalSource = json.RawMessage("null")
			if stage.final && acknowledgedContainsInt(acknowledgedExtra, line.display) {
				draft.acknowledged = true
			}
			if removedKeys[line.key] {
				draft.removed = true
			}
		}
	}

	// A projection can create a temporary duplicate (a rematch claims a
	// reference another line still holds) that Stage B resolves by removal or
	// correction. B itself never carries an unresolved duplicate because the
	// repair validation refuses one before this composer runs.
	for _, lineNumber := range comparisonP.DuplicateLineNumbers {
		line := stageP[lineNumber-1]
		lineKey := line.key
		draft := draftFor("line:"+line.key+":match", "line:match",
			POReconciliationKindDuplicateMatch,
			"Line "+strconv.Itoa(line.display)+" match", &lineKey,
			stringPointerOrNil(line.ref))
		draft.kind = POReconciliationKindDuplicateMatch
		if line.ref != "" {
			draft.finalPO = jsonString(line.ref)
		}
	}

	// The final display position of each surviving row, so a decision recorded
	// against a slot that already matches can still be honoured (an explicit
	// Use whose applied value equals the target is not a "now matches" guess).
	stageBDisplay := make(map[string]int, len(stageB))
	stageBLine := make(map[string]repairLine, len(stageB))
	for _, line := range stageB {
		stageBDisplay[line.key] = line.display
		stageBLine[line.key] = line
	}

	// Resolve every draft and attach its terminal state.
	for _, key := range order {
		draft := drafts[key]
		// A field difference observed on a row this repair retires is evidence
		// of the removal, not of a value that now matches.
		if draft.lineKey != nil && removedKeys[*draft.lineKey] {
			draft.removed = true
		}
		var resolution string
		switch {
		case draft.removed:
			resolution = POReconciliationResolutionRemoved
		case draft.kind == POReconciliationKindMissingLine:
			if draft.acknowledged {
				resolution = POReconciliationResolutionAckMiss
			} else {
				resolution = POReconciliationResolutionCorrected
			}
		case draft.kind == POReconciliationKindExtraLine:
			switch {
			case draft.acknowledged:
				resolution = POReconciliationResolutionAckExtra
			case draft.removed:
				resolution = POReconciliationResolutionRemoved
			default:
				resolution = POReconciliationResolutionCorrected
			}
		case draft.kind == POReconciliationKindDuplicateMatch:
			if draft.removed {
				resolution = POReconciliationResolutionRemoved
			} else {
				resolution = POReconciliationResolutionCorrected
			}
		default:
			decisionKey := draft.finalCompKey
			if decisionKey == "" {
				if draft.lineKey != nil {
					if display, known := stageBDisplay[*draft.lineKey]; known {
						if suffix, isLine := lineItemKeySuffix(draft.field); isLine {
							decisionKey = "line:" + strconv.Itoa(display) + ":" + suffix
						}
					}
				} else if strings.HasPrefix(draft.field, "header:") {
					decisionKey = draft.field
				}
			}
			decision := chosen[decisionKey]
			switch {
			case draft.hasFinal && decision == PurchaseOrderChoicePO:
				resolution = POReconciliationResolutionKeptPO
			case draft.hasFinal && decision == PurchaseOrderChoiceQuotation:
				resolution = POReconciliationResolutionUsedQuote
			case draft.hasFinal:
				// Validation refuses an undecided final difference. Reaching
				// here means the composer and the validator disagree.
				return seed, fmt.Errorf("%w: %q has no decision", ErrPurchaseOrderUnresolvedDifference, key)
			case decision == PurchaseOrderChoiceQuotation:
				// An explicit Use whose applied value already equals the target:
				// the decision is the provenance, not a coincidental match.
				resolution = POReconciliationResolutionUsedQuote
			default:
				resolution = POReconciliationResolutionMatched
			}
		}

		// A slot observed before the final state still needs its final values,
		// even when Stage B found no difference there.
		if draft.kind == POReconciliationKindDifference && len(draft.finalPO) == 0 {
			switch {
			case draft.lineKey != nil:
				if line, alive := stageBLine[*draft.lineKey]; alive {
					value, err := canonicalReconciliationLineValue(draft.field, line.line)
					if err != nil {
						return seed, err
					}
					sourceValue, err := canonicalSourceLineValue(draft.field, line.sourceAt)
					if err != nil {
						return seed, err
					}
					draft.finalPO, draft.finalSource = value, sourceValue
				} else if len(draft.observed) > 0 {
					// The row was retired; its compared values are the evidence.
					last := draft.observed[len(draft.observed)-1]
					draft.finalPO, draft.finalSource = last.po, last.src
				}
			case strings.HasPrefix(draft.field, "header:"):
				value, err := canonicalHeaderValue(draft.field, finalHeaders)
				if err != nil {
					return seed, err
				}
				sourceValue, err := canonicalHeaderValue(draft.field, sourceHeadersOf(source))
				if err != nil {
					return seed, err
				}
				draft.finalPO, draft.finalSource = value, sourceValue
			}
		}

		item := POReconciliationItem{
			Key:           key,
			Field:         draft.field,
			Kind:          draft.kind,
			LineKey:       draft.lineKey,
			SourceLineRef: draft.sourceRef,
			Label:         draft.label,
			POValue:       draft.finalPO,
			SourceValue:   draft.finalSource,
			Status:        POReconciliationStatusResolved,
			Resolution:    &resolution,
			ActorID:       actorID,
			UpdatedAt:     now,
			History:       []POReconciliationHistoryEvent{},
		}
		if len(item.POValue) == 0 {
			item.POValue = json.RawMessage("null")
		}
		if len(item.SourceValue) == 0 {
			item.SourceValue = json.RawMessage("null")
		}
		for _, observation := range draft.observed {
			item.History = append(item.History, POReconciliationHistoryEvent{
				Kind:          draft.field,
				SourceLineRef: item.SourceLineRef,
				POValue:       observation.po,
				SourceValue:   observation.src,
				Status:        POReconciliationStatusUnresolved,
				ActorID:       actorID,
				At:            now,
			})
		}
		terminal := POReconciliationHistoryEvent{
			Kind:          draft.field,
			SourceLineRef: item.SourceLineRef,
			POValue:       item.POValue,
			SourceValue:   item.SourceValue,
			Status:        POReconciliationStatusResolved,
			Resolution:    &resolution,
			ActorID:       actorID,
			At:            now,
		}
		// The terminal decision is always its own event, even when it carries
		// the same values as the last observation: "compared then decided" is
		// the recorded state change, and the writer appends one event per
		// decision elsewhere.
		item.History = append(item.History, terminal)
		seed.Items = append(seed.Items, item)
	}

	// A repair always adopts the target's tax basis, so the basis is always part
	// of the record; the pre-adoption triple stays visible beside it.
	taxItem, err := repairTaxBasisItem(stored, source, actorID, now)
	if err != nil {
		return seed, err
	}
	seed.Items = append(seed.Items, taxItem)

	// Identities: every surviving row keeps its first-seen key; every retired
	// row keeps its key marked Removed so its history can never be inherited.
	seenIdentities := map[string]bool{}
	appendIdentity := func(key string, id int64, retired bool) {
		if seenIdentities[key] {
			return
		}
		seenIdentities[key] = true
		value := id
		seed.Identities = append(seed.Identities, POReconciliationLineIdentity{
			Key: key, ID: &value, Removed: retired,
		})
	}
	for index := range kept {
		appendIdentity(keyFor(kept[index].ID, kept[index].ClientKey), kept[index].ID, false)
	}
	for _, id := range removed {
		// A removed row's alias column disappears with it; its first-seen
		// identity is the durable address of the retained history.
		appendIdentity(keyFor(id, ""), id, true)
	}

	return seed, nil
}

// repairStageLines builds one comparison stage: line numbers normalized to the
// slice order, the identity key of each row, and the target line each row's
// reference names.
func repairStageLines(
	items []PurchaseOrderItem,
	headers repairHeaders,
	source PurchaseOrderSource,
	keyFor func(rowID int64, clientKey string) string,
) ([]repairLine, PurchaseOrderReviewState) {
	lines := make([]repairLine, len(items))
	normalized := make([]PurchaseOrderItem, len(items))
	sourceByRef := make(map[string]*PurchaseOrderSourceLine, len(source.Lines))
	for index := range source.Lines {
		sourceByRef[source.Lines[index].Label()] = &source.Lines[index]
	}
	for index, item := range items {
		item.LineNumber = index + 1
		normalized[index] = item
		ref := ""
		if reference := purchaseOrderLineReference(item); reference != nil {
			ref = *reference
		}
		lines[index] = repairLine{
			line:     item,
			key:      keyFor(item.ID, item.ClientKey),
			display:  index + 1,
			ref:      ref,
			sourceAt: sourceByRef[ref],
		}
	}
	return lines, headers.state(normalized)
}

// sourceHeadersOf projects the target revision's header slots.
func sourceHeadersOf(source PurchaseOrderSource) repairHeaders {
	return repairHeaders{
		PaymentTerms:    source.PaymentTerms,
		Incoterms:       source.Incoterms,
		DiscountPercent: source.DiscountPercent,
	}
}

// splitReviewDifferenceKey separates a comparison key into its field and, for a
// line slot, the positional line number within that stage.
func splitReviewDifferenceKey(key string) (field string, lineNumber int, isLine bool, err error) {
	if strings.HasPrefix(key, "header:") {
		return key, 0, false, nil
	}
	rest, ok := strings.CutPrefix(key, "line:")
	if !ok {
		return "", 0, false, fmt.Errorf("unexpected review key %q", key)
	}
	number, suffix, ok := strings.Cut(rest, ":")
	if !ok {
		return "", 0, false, fmt.Errorf("unexpected review key %q", key)
	}
	index, convErr := strconv.Atoi(number)
	if convErr != nil || index < 1 {
		return "", 0, false, fmt.Errorf("unexpected review line in %q", key)
	}
	return "line:" + suffix, index, true, nil
}

// canonicalHeaderValue renders a header slot's typed value.
func canonicalHeaderValue(field string, headers repairHeaders) (json.RawMessage, error) {
	switch field {
	case "header:paymentTerms":
		return json.Marshal(strings.TrimSpace(headers.PaymentTerms))
	case "header:incoterms":
		return json.Marshal(strings.TrimSpace(headers.Incoterms))
	case "header:discountPercent":
		return json.Marshal(headers.DiscountPercent)
	}
	return nil, fmt.Errorf("no canonical header value for %q", field)
}

// canonicalSourceLineValue renders a target line's typed value for a field.
func canonicalSourceLineValue(field string, line *PurchaseOrderSourceLine) (json.RawMessage, error) {
	if line == nil {
		return nil, fmt.Errorf("source line is missing for %q", field)
	}
	switch field {
	case "line:description":
		return json.Marshal(strings.TrimSpace(line.Description))
	case "line:uom":
		return json.Marshal(strings.TrimSpace(line.UoM))
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
	}
	return nil, fmt.Errorf("no canonical source value for %q", field)
}

func jsonString(value string) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func stringPointerOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func sameReconciliationValue(left, right json.RawMessage) bool {
	return string(normalizeForCompare(left)) == string(normalizeForCompare(right))
}

func acknowledgedContains(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func acknowledgedContainsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// repairTaxBasisItem always records the adopted basis: the pre-repair triple
// beside the target triple, resolved as used_quotation because a repair adopts
// the target's basis rather than offering an inline tax choice.
func repairTaxBasisItem(stored PurchaseOrder, source PurchaseOrderSource, actorID int64, now time.Time) (POReconciliationItem, error) {
	before := reconciliationTaxTriple(stored.TaxZone, stored.TaxType, stored.ServiceCategory, stored.TaxCategory)
	after := reconciliationTaxTriple(source.TaxZone, source.TaxType, source.ServiceCategory, source.TaxCategory)
	beforeRaw, err := json.Marshal(before)
	if err != nil {
		return POReconciliationItem{}, err
	}
	afterRaw, err := json.Marshal(after)
	if err != nil {
		return POReconciliationItem{}, err
	}
	resolution := POReconciliationResolutionUsedQuote
	item := POReconciliationItem{
		Key:         "link:taxBasis",
		Field:       "link:taxBasis",
		Kind:        POReconciliationKindTaxBasis,
		Label:       "Tax basis",
		POValue:     beforeRaw,
		SourceValue: afterRaw,
		Status:      POReconciliationStatusResolved,
		Resolution:  &resolution,
		ActorID:     actorID,
		UpdatedAt:   now,
		History: []POReconciliationHistoryEvent{{
			Kind:        "link:taxBasis",
			POValue:     beforeRaw,
			SourceValue: afterRaw,
			Status:      POReconciliationStatusResolved,
			Resolution:  &resolution,
			ActorID:     actorID,
			At:          now,
		}},
	}
	return item, nil
}

// insertTerminalReconciliationSessionTx installs the new generation already
// active and read-only, with its final identities and the repair link. It does
// not use the draft session writer: that writer cannot touch a saved record,
// and this row is construction-time evidence.
func insertTerminalReconciliationSessionTx(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	poID int64,
	source PurchaseOrderSource,
	generation int,
	version int64,
	identities []POReconciliationLineIdentity,
	correctionID, actorID int64,
) (int64, error) {
	identitiesRaw, err := json.Marshal(identities)
	if err != nil {
		return 0, fmt.Errorf("encode repair line identities: %w", err)
	}
	var sessionID int64
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_reconciliation_sessions (
			org_id, purchase_order_id, source_quotation_id, source_revision,
			source_revision_id, generation, active, readonly, po_version,
			line_identities, created_by, repair_correction_id
		) VALUES ($1,$2,$3,$4,$5,$6,TRUE,TRUE,$7,$8,$9,$10)
		RETURNING id`, schema),
		orgID, poID, source.QuotationID, source.RevisionNumber, source.RevisionID,
		generation, version, identitiesRaw, actorID, correctionID).Scan(&sessionID); err != nil {
		return 0, fmt.Errorf("install repair reconciliation session: %w", err)
	}
	return sessionID, nil
}

// insertRepairReconciliationItemsTx writes the terminal item set in the same
// transaction as the repair, inside migration 044's construction window.
func insertRepairReconciliationItemsTx(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	poID, sessionID int64,
	items []POReconciliationItem,
) error {
	for _, item := range items {
		historyRaw, err := json.Marshal(item.History)
		if err != nil {
			return fmt.Errorf("encode repair item history %q: %w", item.Key, err)
		}
		poValue := item.POValue
		if len(poValue) == 0 {
			poValue = json.RawMessage("null")
		}
		sourceValue := item.SourceValue
		if len(sourceValue) == 0 {
			sourceValue = json.RawMessage("null")
		}
		if _, err := q.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_reconciliation_items (
				org_id, session_id, purchase_order_id, key, field, kind,
				line_key, source_line_ref, label, po_value, source_value,
				status, resolution, actor_id, updated_at, history
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, schema),
			orgID, sessionID, poID, item.Key, item.Field, item.Kind,
			nullableString(item.LineKey), nullableString(item.SourceLineRef),
			item.Label, string(poValue), string(sourceValue), item.Status,
			nullableString(item.Resolution), item.ActorID, item.UpdatedAt, string(historyRaw),
		); err != nil {
			return fmt.Errorf("store repair reconciliation item %q: %w", item.Key, err)
		}
	}
	return nil
}

// loadLatestReconciliationIdentitiesTx reads the retained identity map of the
// generation this repair will retire (the highest generation, active or not).
func loadLatestReconciliationIdentitiesTx(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	poID int64,
) ([]POReconciliationLineIdentity, error) {
	var raw []byte
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT line_identities FROM %s.purchase_order_reconciliation_sessions
		WHERE purchase_order_id = $1 AND org_id = $2
		ORDER BY generation DESC, id DESC
		LIMIT 1`, schema), poID, orgID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []POReconciliationLineIdentity{}, nil
		}
		return nil, fmt.Errorf("read the latest reconciliation identities for purchase order %d: %w", poID, err)
	}
	identities := []POReconciliationLineIdentity{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &identities); err != nil {
			return nil, fmt.Errorf("decode the latest reconciliation identities for purchase order %d: %w", poID, err)
		}
	}
	return identities, nil
}
