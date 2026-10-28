package backend

import (
	"encoding/json"
	"strconv"
	"strings"

	"vera-app/backend/store"
)

// PO reconciliation workbench (#502, ticket 04): the retained header
// difference review. Keys are exact PascalCase with explicit nulls and [] for
// empty arrays, per docs/po-reconciliation-contract.md section 5. Timestamps
// are UTC RFC3339 strings; values are JSON (text, integer minor units and
// scaled quantities, Discount/TaxBasis objects, or null).

type POReconciliationSession struct {
	ID                int64
	PurchaseOrderID   int64
	SourceQuotationID int64
	SourceRevision    int16
	POVersion         int64
	Active            bool
	ReadOnly          bool
	CreatedBy         int64
	CreatedAt         string
	UpdatedAt         string
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
	POValue       any
	SourceValue   any
	Status        string
	Resolution    *string
	ActorID       int64
	UpdatedAt     string
	History       []POReconciliationHistoryEvent
}

type POReconciliationHistoryEvent struct {
	Kind          string
	SourceLineRef *string
	POValue       any
	SourceValue   any
	Status        string
	Resolution    *string
	ActorID       int64
	At            string
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

func rawToAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return trimmed
	}
	return value
}

func anyToRaw(value any) json.RawMessage {
	if value == nil {
		return json.RawMessage("null")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return raw
}

func mapPOReconciliationSession(session store.POReconciliationSession) POReconciliationSession {
	out := POReconciliationSession{
		ID:                session.ID,
		PurchaseOrderID:   session.PurchaseOrderID,
		SourceQuotationID: session.SourceQuotationID,
		SourceRevision:    session.SourceRevision,
		POVersion:         session.POVersion,
		Active:            session.Active,
		ReadOnly:          session.ReadOnly,
		CreatedBy:         session.CreatedBy,
		CreatedAt:         session.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:         session.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		LineIdentities:    make([]POReconciliationLineIdentity, 0, len(session.LineIdentities)),
		Items:             make([]POReconciliationItem, 0, len(session.Items)),
	}
	for _, identity := range session.LineIdentities {
		out.LineIdentities = append(out.LineIdentities, POReconciliationLineIdentity{
			Key:     identity.Key,
			ID:      identity.ID,
			Removed: identity.Removed,
		})
	}
	for _, item := range session.Items {
		history := make([]POReconciliationHistoryEvent, 0, len(item.History))
		for _, event := range item.History {
			history = append(history, POReconciliationHistoryEvent{
				Kind:          event.Kind,
				SourceLineRef: event.SourceLineRef,
				POValue:       rawToAny(event.POValue),
				SourceValue:   rawToAny(event.SourceValue),
				Status:        event.Status,
				Resolution:    event.Resolution,
				ActorID:       event.ActorID,
				At:            event.At.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
		out.Items = append(out.Items, POReconciliationItem{
			Key:           item.Key,
			Field:         item.Field,
			Kind:          item.Kind,
			LineKey:       item.LineKey,
			SourceLineRef: item.SourceLineRef,
			Label:         item.Label,
			POValue:       rawToAny(item.POValue),
			SourceValue:   rawToAny(item.SourceValue),
			Status:        item.Status,
			Resolution:    item.Resolution,
			ActorID:       item.ActorID,
			UpdatedAt:     item.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			History:       history,
		})
	}
	return out
}

func mapPOReconciliationStoreSession(session POReconciliationSession) store.POReconciliationSession {
	out := store.POReconciliationSession{
		ID:                session.ID,
		PurchaseOrderID:   session.PurchaseOrderID,
		SourceQuotationID: session.SourceQuotationID,
		SourceRevision:    session.SourceRevision,
		POVersion:         session.POVersion,
		Active:            session.Active,
		ReadOnly:          session.ReadOnly,
		CreatedBy:         session.CreatedBy,
		LineIdentities:    make([]store.POReconciliationLineIdentity, 0, len(session.LineIdentities)),
		Items:             make([]store.POReconciliationItem, 0, len(session.Items)),
	}
	for _, identity := range session.LineIdentities {
		out.LineIdentities = append(out.LineIdentities, store.POReconciliationLineIdentity{
			Key:     identity.Key,
			ID:      identity.ID,
			Removed: identity.Removed,
		})
	}
	for _, item := range session.Items {
		out.Items = append(out.Items, store.POReconciliationItem{
			Key:           item.Key,
			Field:         item.Field,
			Kind:          item.Kind,
			LineKey:       item.LineKey,
			SourceLineRef: item.SourceLineRef,
			Label:         item.Label,
			POValue:       anyToRaw(item.POValue),
			SourceValue:   anyToRaw(item.SourceValue),
			Status:        item.Status,
			Resolution:    item.Resolution,
		})
	}
	return out
}

func parsePOReconciliationID(poID string) (int64, error) {
	trimmed := strings.TrimSpace(poID)
	if trimmed == "" {
		return 0, store.ErrPurchaseOrderNotFound
	}
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id < 1 {
		return 0, store.ErrPurchaseOrderNotFound
	}
	return id, nil
}

func (q *Quotation) LoadPOReconciliation(poID string) (POReconciliationSession, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return POReconciliationSession{}, err
	}
	id, err := parsePOReconciliationID(poID)
	if err != nil {
		return POReconciliationSession{}, err
	}
	client, err := api()
	if err != nil {
		return POReconciliationSession{}, err
	}
	var response store.POReconciliationSession
	if err := client.Get("/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/reconciliation", &response); err != nil {
		return POReconciliationSession{}, err
	}
	return mapPOReconciliationSession(response), nil
}

func (q *Quotation) SavePOReconciliation(poID string, session POReconciliationSession) (POReconciliationSession, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return POReconciliationSession{}, err
	}
	id, err := parsePOReconciliationID(poID)
	if err != nil {
		return POReconciliationSession{}, err
	}
	client, err := api()
	if err != nil {
		return POReconciliationSession{}, err
	}
	request := mapPOReconciliationStoreSession(session)
	request.PurchaseOrderID = id
	var response store.POReconciliationSession
	if err := client.Put("/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/reconciliation", request, &response); err != nil {
		return POReconciliationSession{}, err
	}
	return mapPOReconciliationSession(response), nil
}
