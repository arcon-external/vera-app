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

// #493 final Judge F1: a pre-039 row from a Non-FTZ services revision stores an
// empty legacy taxonomy and a frozen PPN of 0. The editor derives Goods for
// display; saving that derived basis must not add 11% PPN or move a stored
// total, now or on the next save of the same row.
func TestFormerLegacyEmptyCategorySaveKeepsItsFrozenRate(t *testing.T) {
	database := openTestSchema(t, "vera_po_f1_legacy_empty")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	seedFormer := func() PurchaseOrder {
		t.Helper()
		quotation := seedRepairQuotationWith(t, database, repairQuotationOptions{
			CustomerCode: 2, CustomerName: "PT. Legacy Services",
			Subject: "Legacy services " + t.Name(), Address: "Jl. Legacy",
			Currency: "IDR", TaxZone: "NonFTZ", TaxType: "Services",
			UnitPrice: 100, PaymentTermDays: 45,
		})
		if _, err := pool.Exec(ctx, `
			UPDATE `+database.configSchema()+`.quotation_revisions
			SET service_category = 'NonConstruction'
			WHERE quotation_id = $1 AND revision_number = $2`,
			quotation.ID, quotation.RevisionNumber); err != nil {
			t.Fatalf("set the revision service category: %v", err)
		}
		draft := createFinalizeDraft(t, database, ctx, user, quotation)
		if _, err := pool.Exec(ctx, `
			UPDATE `+database.configSchema()+`.purchase_orders
			SET tax_type = '', service_category = '', tax_category = '',
			    tax_zone = 'NonFTZ', ppn_bps = 0, ppn_minor = 0
			WHERE id = $1`, draft.ID); err != nil {
			t.Fatalf("make the row a pre-039 legacy row: %v", err)
		}
		admin := deletionUser(t, database, "admin")
		preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
		if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeRecord, Reason: "legacy former PPN",
			TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
			ActorID: admin.ID,
		}); err != nil {
			t.Fatalf("delete the source: %v", err)
		}
		former, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("read the former row: %v", err)
		}
		if former.SourceState != PurchaseOrderSourceStateFormer {
			t.Fatalf("source state = %q; want former", former.SourceState)
		}
		return former
	}
	editorPayload := func(order PurchaseOrder, currency, taxType, service string) PurchaseOrderUpdate {
		return PurchaseOrderUpdate{
			ID:              order.ID,
			Currency:        currency,
			TaxZone:         order.TaxZone,
			TaxType:         taxType,
			ServiceCategory: service,
			BillingAddress:  order.BillingAddress,
			DeliveryAddress: order.DeliveryAddress,
			ReceivedDate:    datePointer(order.ReceivedDate),
			PaymentTerms:    order.PaymentTerms,
			Incoterms:       order.Incoterms,
			UpdatedBy:       user.ID,
			ExpectedVersion: order.Version,
			Items:           order.Items,
		}
	}
	// 2 x 100 with no PPN and no adjustment: the totals the parent kept.
	assertFrozenMoney := func(t *testing.T, before, saved PurchaseOrder) {
		t.Helper()
		if saved.PpnBps != before.PpnBps {
			t.Errorf("a plain save moved the stored PPN %d -> %d", before.PpnBps, saved.PpnBps)
		}
		if saved.PpnMinor != before.PpnMinor {
			t.Errorf("a plain save moved the stored PPN amount %d -> %d",
				before.PpnMinor, saved.PpnMinor)
		}
		if saved.GrandTotalMinor != before.GrandTotalMinor {
			t.Errorf("a plain save moved the grand total %d -> %d",
				before.GrandTotalMinor, saved.GrandTotalMinor)
		}
	}

	t.Run("derived goods save", func(t *testing.T) {
		former := seedFormer()
		if former.PpnBps != 0 || former.PpnMinor != 0 || former.GrandTotalMinor != 200 {
			t.Fatalf("seeded legacy row = %d bps, %d PPN, grand %d; want 0/0/200",
				former.PpnBps, former.PpnMinor, former.GrandTotalMinor)
		}
		saved, err := database.UpdatePurchaseOrderDraft(ctx, editorPayload(former, "IDR", "Goods", ""))
		if err != nil {
			t.Fatalf("save the display-derived basis: %v", err)
		}
		if saved.TaxType != "Goods" || saved.ServiceCategory != "" {
			t.Errorf("stored basis = %q/%q; want the derived Goods", saved.TaxType, saved.ServiceCategory)
		}
		assertFrozenMoney(t, former, saved)

		// The stored type is now Goods. Saving the same row again must not
		// start charging PPN after the first save.
		again, err := database.UpdatePurchaseOrderDraft(ctx, editorPayload(saved, "IDR", "Goods", ""))
		if err != nil {
			t.Fatalf("save the row a second time: %v", err)
		}
		assertFrozenMoney(t, saved, again)
	})

	t.Run("old empty basis save", func(t *testing.T) {
		former := seedFormer()
		saved, err := database.UpdatePurchaseOrderDraft(ctx, editorPayload(former, "IDR", "", ""))
		if err != nil {
			t.Fatalf("save the old empty basis: %v", err)
		}
		assertFrozenMoney(t, former, saved)
	})

	t.Run("currency change keeps the rate", func(t *testing.T) {
		former := seedFormer()
		saved, err := database.UpdatePurchaseOrderDraft(ctx, editorPayload(former, "USD", "Goods", ""))
		if err != nil {
			t.Fatalf("change the currency: %v", err)
		}
		if saved.Currency != "USD" {
			t.Errorf("currency = %q; want USD", saved.Currency)
		}
		assertFrozenMoney(t, former, saved)
	})
}

// A former row whose effective basis staff really change recomputes PPN; the
// frozen rate only covers a save that leaves the commercial basis alone.
func TestFormerBasisChangeRecomputesTheRate(t *testing.T) {
	database := openTestSchema(t, "vera_po_f1_former_recompute")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Former Recompute", "Former recompute")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeRecord, Reason: "former basis change",
		TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete the source: %v", err)
	}
	former, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read the former row: %v", err)
	}
	// The seeded source is FTZ/Goods/USD: moving it to Non-FTZ goods is a real
	// basis change and must charge PPN.
	saved, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID:              former.ID,
		Currency:        "USD",
		TaxZone:         "NonFTZ",
		TaxType:         "Goods",
		ServiceCategory: "",
		ReceivedDate:    datePointer(former.ReceivedDate),
		UpdatedBy:       user.ID,
		ExpectedVersion: former.Version,
		Items:           former.Items,
	})
	if err != nil {
		t.Fatalf("change a former row's basis: %v", err)
	}
	if saved.PpnBps != 1100 || saved.PpnMinor != 22 {
		t.Errorf("changed basis PPN = %d bps / %d; want 1100 / 22",
			saved.PpnBps, saved.PpnMinor)
	}
	if saved.GrandTotalMinor != former.GrandTotalMinor+22 {
		t.Errorf("changed basis grand = %d; want %d", saved.GrandTotalMinor, former.GrandTotalMinor+22)
	}
}
