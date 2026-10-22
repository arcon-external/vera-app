package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// Issue #496 Judge round 2: the legacy one-time population must never drift the
// effective basis. A linked/former row with tax_type='' and
// tax_category=SupplyInstallConstruction means Services/Construction; a
// service-only submission of NonConstruction must be refused, not written.

func legacyServicesDraft(t *testing.T, database *Store, ctx context.Context, user AuthUser, former bool) PurchaseOrder {
	t.Helper()
	quotation := seedFinalizeQuotation(t, database, 2, "PT. F2b", "F2b "+t.Name())
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	legacyBasis(t, database, draft.ID, "SupplyInstallConstruction")
	if former {
		admin := deletionUser(t, database, "admin")
		preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
		if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeRecord, Reason: "f2b legacy former",
			TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
			ActorID: admin.ID,
		}); err != nil {
			t.Fatalf("delete source for former F2b row: %v", err)
		}
	}
	order, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read F2b draft: %v", err)
	}
	return order
}

func f2bUpdate(order PurchaseOrder, user AuthUser, patch PurchaseOrderUpdate) PurchaseOrderUpdate {
	return PurchaseOrderUpdate{
		ID:                    order.ID,
		BillingAddress:        patch.BillingAddress,
		DeliveryAddress:       patch.DeliveryAddress,
		ShipToCode:            patch.ShipToCode,
		ContactEmail:          patch.ContactEmail,
		ContactPhone:          patch.ContactPhone,
		PONumber:              order.PONumber,
		PODate:                order.PODate,
		ReceivedDate:          datePointer(order.ReceivedDate),
		RequestedDeliveryDate: order.RequestedDeliveryDate,
		PaymentTerms:          order.PaymentTerms,
		Incoterms:             order.Incoterms,
		AdjustmentReason:      order.AdjustmentReason,
		Items:                 order.Items,
		DiscountPercent:       order.DiscountPercent,
		AdjustmentMinor:       order.AdjustmentMinor,
		Currency:              patch.Currency,
		TaxZone:               patch.TaxZone,
		TaxType:               patch.TaxType,
		ServiceCategory:       patch.ServiceCategory,
		TaxCategory:           patch.TaxCategory,
		ExpectedVersion:       order.Version,
		UpdatedBy:             user.ID,
	}
}

func TestLegacyBasisUpdateCannotDriftTheEffectiveBasis(t *testing.T) {
	cases := []struct {
		name        string
		former      bool
		patch       PurchaseOrderUpdate
		wantErr     bool
		wantType    string
		wantService string
	}{
		{
			name:     "linked empty keeps and populates the exact legacy basis",
			wantType: "Services", wantService: "Construction",
		},
		{
			name:     "linked type-only matching populates",
			patch:    PurchaseOrderUpdate{TaxType: "Services"},
			wantType: "Services", wantService: "Construction",
		},
		{
			name:     "linked service-only matching populates",
			patch:    PurchaseOrderUpdate{ServiceCategory: "Construction"},
			wantType: "Services", wantService: "Construction",
		},
		{
			name:     "linked full matching populates",
			patch:    PurchaseOrderUpdate{TaxType: "Services", ServiceCategory: "Construction"},
			wantType: "Services", wantService: "Construction",
		},
		{
			name:    "linked service-only differing is refused",
			patch:   PurchaseOrderUpdate{ServiceCategory: "NonConstruction"},
			wantErr: true,
		},
		{
			name:    "linked type-only differing is refused",
			patch:   PurchaseOrderUpdate{TaxType: "Goods"},
			wantErr: true,
		},
		{
			name:    "linked full differing is refused",
			patch:   PurchaseOrderUpdate{TaxType: "Services", ServiceCategory: "NonConstruction"},
			wantErr: true,
		},
		{
			name:     "former empty keeps and populates the exact legacy basis",
			former:   true,
			wantType: "Services", wantService: "Construction",
		},
		{
			name:     "former service-only matching populates",
			former:   true,
			patch:    PurchaseOrderUpdate{ServiceCategory: "Construction"},
			wantType: "Services", wantService: "Construction",
		},
		{
			name:    "former service-only differing is refused",
			former:  true,
			patch:   PurchaseOrderUpdate{ServiceCategory: "NonConstruction"},
			wantErr: true,
		},
	}

	for index, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			database := openTestSchema(t, fmt.Sprintf("vera_po_f2b_%d", index))
			user := poTestUser(t, database)
			ctx := poContext(t, database)
			order := legacyServicesDraft(t, database, ctx, user, testCase.former)
			_, err := database.UpdatePurchaseOrderDraft(ctx, f2bUpdate(order, user, testCase.patch))
			if testCase.wantErr {
				if !errors.Is(err, ErrPurchaseOrderInvalid) {
					t.Fatalf("legacy basis drift update = %v; want ErrPurchaseOrderInvalid", err)
				}
				unchanged, readErr := database.GetPurchaseOrder(ctx, order.ID)
				if readErr != nil {
					t.Fatalf("reread after refused drift: %v", readErr)
				}
				if unchanged.TaxType != "" || unchanged.ServiceCategory != "" {
					t.Errorf("refused drift wrote basis %q/%q; want the legacy fields untouched",
						unchanged.TaxType, unchanged.ServiceCategory)
				}
				if unchanged.TaxCategory != "SupplyInstallConstruction" {
					t.Errorf("refused drift changed legacy category to %q", unchanged.TaxCategory)
				}
				return
			}
			if err != nil {
				t.Fatalf("legacy basis update = %v; want success", err)
			}
			saved, readErr := database.GetPurchaseOrder(ctx, order.ID)
			if readErr != nil {
				t.Fatalf("reread after legacy basis update: %v", readErr)
			}
			if saved.TaxType != testCase.wantType || saved.ServiceCategory != testCase.wantService {
				t.Errorf("legacy basis = %q/%q; want %q/%q",
					saved.TaxType, saved.ServiceCategory, testCase.wantType, testCase.wantService)
			}
			if saved.PpnBps != order.PpnBps {
				t.Errorf("legacy population changed PPN %d -> %d; want frozen", order.PpnBps, saved.PpnBps)
			}
		})
	}
}

func TestFrozenNonEmptyLegacyBasisRefusesDrift(t *testing.T) {
	database := openTestSchema(t, "vera_po_f2b_frozen")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	order := legacyServicesDraft(t, database, ctx, user, false)

	populated, err := database.UpdatePurchaseOrderDraft(ctx, f2bUpdate(order, user, PurchaseOrderUpdate{}))
	if err != nil {
		t.Fatalf("populate legacy basis: %v", err)
	}
	if populated.TaxType != "Services" || populated.ServiceCategory != "Construction" {
		t.Fatalf("populated basis = %q/%q", populated.TaxType, populated.ServiceCategory)
	}
	if _, err := database.UpdatePurchaseOrderDraft(ctx, f2bUpdate(populated, user, PurchaseOrderUpdate{
		ServiceCategory: "NonConstruction",
	})); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("frozen service drift = %v; want ErrPurchaseOrderInvalid", err)
	}
	unchanged, err := database.GetPurchaseOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("reread frozen basis: %v", err)
	}
	if unchanged.TaxType != "Services" || unchanged.ServiceCategory != "Construction" {
		t.Errorf("frozen basis moved to %q/%q", unchanged.TaxType, unchanged.ServiceCategory)
	}
}
