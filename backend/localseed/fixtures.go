package localseed

import (
	"context"
	"fmt"
	"time"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// createdQuotation is one seeded quotation as the manifest knows it: the
// latest revision plus every revision number, so a rerun can tell whether the
// revision set changed.
type createdQuotation struct {
	Spec        QuotationSpec
	QuoteNumber string
	UUID        string
	Record      store.QuotationRecord
	Revisions   []int16
}

func (c createdQuotation) RevisionNumbers() []int16 {
	return c.Revisions
}

func createQuotation(ctx context.Context, database *store.Store, spec QuotationSpec) (createdQuotation, error) {
	yy := store.CurrentYY()
	sequence, _, err := database.NextQuoteSequence(ctx, int(spec.CustomerCode), yy)
	if err != nil {
		return createdQuotation{}, fmt.Errorf("allocate the mock quote number: %w", err)
	}
	quoteNumber := apicontract.FormatQuoteNumber(int(spec.CustomerCode), yy, sequence)
	uuid, err := store.NewQuotationUUID()
	if err != nil {
		return createdQuotation{}, fmt.Errorf("allocate the mock quotation identity: %w", err)
	}

	created := createdQuotation{Spec: spec, QuoteNumber: quoteNumber, UUID: uuid}
	for index, revision := range spec.Revisions {
		subject := revision.Subject
		if subject == "" {
			subject = spec.Subject
		}
		items, err := priceItems(revision.Items)
		if err != nil {
			return createdQuotation{}, err
		}
		totals, err := priceQuotationTotals(
			items, revision.DiscountPercent,
			spec.TaxZone, spec.TaxType, spec.ServiceCategory)
		if err != nil {
			return createdQuotation{}, err
		}
		pdf := quotationPDF(quoteNumber, int16(index), spec.CustomerName, subject, items, totals)
		snapshot := quotationSnapshot(quoteNumber, int16(index), spec.CustomerName, subject)
		taxRateBps, err := fixtureTaxRateBps(spec.TaxZone, spec.TaxType, spec.ServiceCategory)
		if err != nil {
			return createdQuotation{}, err
		}
		draft := store.RevisionDraft{
			QuoteNumber:      quoteNumber,
			QuotationUUID:    uuid,
			RevisionNumber:   int16(index),
			CustomerCode:     spec.CustomerCode,
			CustomerName:     spec.CustomerName,
			CustomerAddress:  spec.CustomerAddress,
			RecipientName:    spec.RecipientName,
			RecipientAddress: spec.RecipientAddress,
			Subject:          subject,
			Incoterm:         spec.Incoterm,
			Currency:         "IDR",
			TaxZone:          spec.TaxZone,
			TaxCategory:      legacyTaxCategory(spec.TaxZone, spec.TaxType, spec.ServiceCategory),
			TaxType:          spec.TaxType,
			ServiceCategory:  spec.ServiceCategory,
			TaxRateBps:       taxRateBps,
			DiscountPercent:  revision.DiscountPercent,
			PaymentTermDays:  spec.PaymentTermDays,
			ValidityDays:     spec.ValidityDays,
			LeadTimeDays:     spec.LeadTimeDays,
			LeadTimeWeeks:    spec.LeadTimeWeeks,
			LeadTimeMonths:   spec.LeadTimeMonths,
			LeadTimeYears:    spec.LeadTimeYears,
			Items:            items,
			Totals:           totals,
			PDFData:          pdf,
			HTMLSnapshot:     snapshot,
		}
		record, err := database.SaveRevision(ctx, draft)
		if err != nil {
			return createdQuotation{}, fmt.Errorf("save revision %d: %w", index, err)
		}
		created.Record = record
		created.Revisions = append(created.Revisions, int16(index))
	}
	return created, nil
}

func priceItems(specs []ItemSpec) ([]store.RevisionItem, error) {
	items := make([]store.RevisionItem, 0, len(specs))
	for index, spec := range specs {
		item, err := priceItem(spec, index+1)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func legacyTaxCategory(zone, taxType, serviceCategory string) string {
	if taxType == "Goods" {
		if zone == "NonFTZ" {
			return "SupplyOnly"
		}
		return ""
	}
	switch serviceCategory {
	case "NonConstruction":
		return "SupplyInstall"
	case "Construction":
		return "SupplyInstallConstruction"
	default:
		return ""
	}
}

func fixtureTaxRateBps(zone, taxType, serviceCategory string) (int16, error) {
	ppn, err := store.PurchaseOrderPPNBps(zone, taxType, serviceCategory, "")
	if err != nil {
		return 0, fmt.Errorf("fixture tax basis: %w", err)
	}
	service := int16(0)
	switch serviceCategory {
	case "NonConstruction":
		service = 200
	case "Construction":
		service = 175
	}
	return ppn + service, nil
}

func createPurchaseOrder(
	ctx context.Context,
	database *store.Store,
	actor store.AuthUser,
	spec POSpec,
	source *createdQuotation,
) (store.PurchaseOrder, error) {
	if spec.SourceState == sourceNever {
		return createNeverLinkedPurchaseOrder(ctx, database, actor)
	}
	if source == nil {
		return store.PurchaseOrder{}, fmt.Errorf("fixture has no source quotation")
	}
	revision := source.Record.RevisionNumber
	order, err := database.CreatePurchaseOrderDraft(ctx, store.PurchaseOrderCreate{
		QuotationID:    source.Record.ID,
		RevisionNumber: revision,
		CreatedBy:      actor.ID,
	})
	if err != nil {
		return store.PurchaseOrder{}, err
	}

	switch spec.Key {
	case "po-linked-incomplete-draft":
		return updateDraft(ctx, database, actor, order, func(input *store.PurchaseOrderUpdate) {
			input.DeliveryAddress = "Gudang fiksi lantai 2, Jalan Tiruan No. 3, Surabaya (MOCK SHIP-TO)"
			input.ShipToCode = "MOCK-SHIP-01"
			input.ContactEmail = "fiksi.rawat@example.invalid"
			input.ContactPhone = "+62 000 0000 0001"
			// PONumber deliberately stays empty: this is the incomplete draft.
		})
	case "po-linked-draft-differences":
		return updateDraft(ctx, database, actor, order, func(input *store.PurchaseOrderUpdate) {
			fillHeader(input, "MOCK-CUSTOMER-PO-002", daysAgo(6))
			input.DeliveryAddress = "Lokasi proyek fiksi, Jalan Khayalan No. 8, Bekasi (MOCK SHIP-TO)"
			input.ShipToCode = "MOCK-SITE-02"
			if len(input.Items) > 0 {
				input.Items[0].QuantityScaled += store.QuantityScale
			}
			input.Items = append(input.Items, extraLine("Mock site crane hire (MOCK)", 2, 3_750_000))
		})
	case "po-finalized-pdf":
		order, err = updateDraft(ctx, database, actor, order, func(input *store.PurchaseOrderUpdate) {
			fillHeader(input, "MOCK-CUSTOMER-PO-003", daysAgo(12))
			input.DeliveryAddress = "Dermaga fiksi, Jalan Fiksi No. 1, Batam (MOCK SHIP-TO)"
			input.ShipToCode = "MOCK-PORT-03"
			input.ContactEmail = "fiksi.laut@example.invalid"
			input.ContactPhone = "+62 000 0000 0003"
		})
		if err != nil {
			return store.PurchaseOrder{}, err
		}
		if _, err := attachTextPDF(ctx, database, actor, order, "mock-customer-po-003.pdf", source.QuoteNumber); err != nil {
			return store.PurchaseOrder{}, err
		}
		return finalizeMatching(ctx, database, actor, order, false, false)
	case "po-finalized-differences-adjustment":
		return finalizeWithDifferences(ctx, database, actor, order)
	case "po-finalized-no-pdf-confirmation-correction":
		order, err = finalize(ctx, database, actor, order, true, func(input *store.PurchaseOrderFinalize) {
			fillHeader(&input.PurchaseOrderUpdate, "MOCK-CUSTOMER-PO-005", daysAgo(4))
			input.DeliveryAddress = "Gudang fiksi, Jalan Fiktif No. 17, Bandung (MOCK SHIP-TO)"
			input.ShipToCode = "MOCK-LAB-05"
			input.ContactEmail = "fiksi.laboratorium@example.invalid"
			input.ContactPhone = "+62 000 0000 0005"
		}, true)
		if err != nil {
			return store.PurchaseOrder{}, err
		}
		return correctDeliveryAddress(ctx, database, actor, order)
	case "po-former-linked-draft":
		return updateDraft(ctx, database, actor, order, func(input *store.PurchaseOrderUpdate) {
			fillHeader(input, "MOCK-CUSTOMER-PO-007", daysAgo(9))
			input.DeliveryAddress = "Area fiksi, Jalan Rekaan No. 22, Jakarta Utara (MOCK SHIP-TO)"
			input.ContactEmail = "fiksi.sumber@example.invalid"
			if len(input.Items) > 0 {
				input.Items[0].QuantityScaled += store.QuantityScale
			}
		})
	case "po-former-linked-finalized":
		order, err = updateDraft(ctx, database, actor, order, func(input *store.PurchaseOrderUpdate) {
			fillHeader(input, "MOCK-CUSTOMER-PO-008", daysAgo(15))
			input.DeliveryAddress = "Lokasi fiksi, Jalan Tiruan No. 3, Surabaya (MOCK SHIP-TO)"
			input.ContactEmail = "fiksi.sumber@example.invalid"
		})
		if err != nil {
			return store.PurchaseOrder{}, err
		}
		if _, err := attachTextPDF(ctx, database, actor, order, "mock-customer-po-008.pdf", source.QuoteNumber); err != nil {
			return store.PurchaseOrder{}, err
		}
		return finalizeMatching(ctx, database, actor, order, false, false)
	default:
		return store.PurchaseOrder{}, fmt.Errorf("no creation recipe for fixture %q", spec.Key)
	}
}

func createNeverLinkedPurchaseOrder(
	ctx context.Context,
	database *store.Store,
	actor store.AuthUser,
) (store.PurchaseOrder, error) {
	order, err := database.CreatePurchaseOrderBlankDraft(ctx, store.PurchaseOrderBlankCreate{
		TaxZone:     "NonFTZ",
		TaxType:     "Goods",
		TaxCategory: "SupplyOnly",
		CreatedBy:   actor.ID,
	})
	if err != nil {
		return store.PurchaseOrder{}, err
	}
	order, err = updateDraft(ctx, database, actor, order, func(input *store.PurchaseOrderUpdate) {
		fillHeader(input, "MOCK-CUSTOMER-PO-006", daysAgo(3))
		input.DeliveryAddress = "Gudang fiksi, Jalan Contoh No. 5, Medan (MOCK SHIP-TO)"
		input.ShipToCode = "MOCK-SCAN-06"
		input.ContactEmail = "fiksi.scan@example.invalid"
		input.ContactPhone = "+62 000 0000 0006"
		input.PaymentTerms = "30 days"
		input.Items = []store.PurchaseOrderItem{
			extraLine("Manual entry: temporary power distribution (MOCK)", 1, 8_500_000),
			extraLine("Manual entry from the scanned page: site consumables (MOCK)", 12, 250_000),
		}
		input.DiscountPercent = 0
	})
	if err != nil {
		return store.PurchaseOrder{}, err
	}
	mixed := purchaseOrderMixedPDF(order.PONumber, order.Items, order.GrandTotalMinor)
	if _, err := database.AttachPurchaseOrderDocument(ctx, store.PurchaseOrderDocumentUpload{
		PurchaseOrderID: order.ID,
		Data:            mixed,
		Filename:        "mock-customer-po-006-mixed-scan.pdf",
		UploadedBy:      actor.ID,
	}); err != nil {
		return store.PurchaseOrder{}, err
	}
	return database.GetPurchaseOrder(ctx, order.ID)
}

func finalizeMatching(
	ctx context.Context,
	database *store.Store,
	actor store.AuthUser,
	order store.PurchaseOrder,
	confirmedAbsent bool,
	assignConfirmation bool,
) (store.PurchaseOrder, error) {
	return finalize(ctx, database, actor, order, confirmedAbsent, nil, assignConfirmation)
}

func finalizeWithDifferences(
	ctx context.Context,
	database *store.Store,
	actor store.AuthUser,
	order store.PurchaseOrder,
) (store.PurchaseOrder, error) {
	return finalize(ctx, database, actor, order, true, func(input *store.PurchaseOrderFinalize) {
		fillHeader(&input.PurchaseOrderUpdate, "MOCK-CUSTOMER-PO-004", daysAgo(8))
		input.DeliveryAddress = "Lokasi fiksi, Jalan Rekaan No. 22, Jakarta Utara (MOCK SHIP-TO)"
		input.ShipToCode = "MOCK-CLEAN-04"
		input.ContactEmail = "fiksi.bangun@example.invalid"
		input.PaymentTerms = "45 days"
		if len(input.Items) > 0 {
			input.Items[0].QuantityScaled += store.QuantityScale
		}
		input.Items = append(input.Items, extraLine("Mock weekend installation surcharge (MOCK)", 1, 2_250_000))
		input.AdjustmentMinor = 250_000
		input.AdjustmentReason = "Mock adjustment: customer requested additional packing and labels."
	}, false)
}

func finalize(
	ctx context.Context,
	database *store.Store,
	actor store.AuthUser,
	order store.PurchaseOrder,
	confirmedAbsent bool,
	mutate func(*store.PurchaseOrderFinalize),
	assignConfirmation bool,
) (store.PurchaseOrder, error) {
	review, err := database.GetPurchaseOrderReview(ctx, order.ID)
	if err != nil {
		return store.PurchaseOrder{}, err
	}
	input := store.PurchaseOrderFinalize{
		PurchaseOrderUpdate:      baseUpdate(order, actor.ID),
		DocumentConfirmed:        confirmedAbsent,
		AssignConfirmationNumber: assignConfirmation,
	}
	if mutate != nil {
		mutate(&input)
	}
	decisions, missing, extra := acceptDifferences(review.Source, input.PurchaseOrderUpdate)
	input.Decisions = decisions
	input.AcknowledgedMissing = missing
	input.AcknowledgedExtra = extra
	return database.FinalizePurchaseOrder(ctx, input)
}

func correctDeliveryAddress(
	ctx context.Context,
	database *store.Store,
	actor store.AuthUser,
	order store.PurchaseOrder,
) (store.PurchaseOrder, error) {
	input := store.PurchaseOrderCorrectionInput{
		PurchaseOrderUpdate: baseUpdate(order, actor.ID),
		Reason:              "Mock correction: the fictional customer revised its delivery address.",
	}
	input.DeliveryAddress = "Alamat koreksi fiktif, Jalan Fiktif No. 18, Bandung (MOCK CORRECTED SHIP-TO)"
	return database.CorrectPurchaseOrder(ctx, input)
}

func updateDraft(
	ctx context.Context,
	database *store.Store,
	actor store.AuthUser,
	order store.PurchaseOrder,
	mutate func(*store.PurchaseOrderUpdate),
) (store.PurchaseOrder, error) {
	input := baseUpdate(order, actor.ID)
	mutate(&input)
	return database.UpdatePurchaseOrderDraft(ctx, input)
}

// baseUpdate starts from the values the record already carries. A draft save
// replaces every editable field, so leaving one out would clear it.
func baseUpdate(order store.PurchaseOrder, actorID int64) store.PurchaseOrderUpdate {
	received := order.ReceivedDate
	items := make([]store.PurchaseOrderItem, len(order.Items))
	copy(items, order.Items)
	return store.PurchaseOrderUpdate{
		ID:                    order.ID,
		ExpectedVersion:       order.Version,
		UpdatedBy:             actorID,
		BillingAddress:        order.BillingAddress,
		DeliveryAddress:       order.DeliveryAddress,
		ShipToCode:            order.ShipToCode,
		ContactEmail:          order.ContactEmail,
		ContactPhone:          order.ContactPhone,
		PONumber:              order.PONumber,
		Incoterms:             order.Incoterms,
		PaymentTerms:          order.PaymentTerms,
		PODate:                order.PODate,
		ReceivedDate:          &received,
		RequestedDeliveryDate: order.RequestedDeliveryDate,
		Items:                 items,
		DiscountPercent:       order.DiscountPercent,
		AdjustmentMinor:       order.AdjustmentMinor,
		AdjustmentReason:      order.AdjustmentReason,
	}
}

func fillHeader(input *store.PurchaseOrderUpdate, poNumber string, received time.Time) {
	input.PONumber = poNumber
	input.ReceivedDate = &received
	input.PODate = &received
	requested := received.AddDate(0, 0, 21)
	input.RequestedDeliveryDate = &requested
}

func extraLine(description string, qty int64, unitPrice int64) store.PurchaseOrderItem {
	return store.PurchaseOrderItem{
		Description:    description,
		QuantityScaled: qty * store.QuantityScale,
		UoM:            "Lot",
		UnitPriceMinor: unitPrice,
	}
}

func attachTextPDF(
	ctx context.Context,
	database *store.Store,
	actor store.AuthUser,
	order store.PurchaseOrder,
	filename string,
	sourceQuote string,
) (store.PurchaseOrderDocument, error) {
	pdf := purchaseOrderTextPDF(order.PONumber, sourceQuote, order.CustomerName, order.Items, order.GrandTotalMinor)
	return database.AttachPurchaseOrderDocument(ctx, store.PurchaseOrderDocumentUpload{
		PurchaseOrderID: order.ID,
		Data:            pdf,
		Filename:        filename,
		UploadedBy:      actor.ID,
	})
}

// acceptDifferences records the deliberate "use the PO value" choice for every
// difference and acknowledges each missing or extra line. The seed is creating
// a known-good example, so it must pass the same review a staff member would;
// it may not skip it.
func acceptDifferences(source store.PurchaseOrderSource, input store.PurchaseOrderUpdate) (
	[]store.PurchaseOrderDecision, []string, []int,
) {
	// The final save numbers the lines before comparing; the review keys carry
	// those numbers, so this pass must use the same numbering.
	for index := range input.Items {
		input.Items[index].LineNumber = index + 1
	}
	comparison := store.ComparePurchaseOrder(source, store.PurchaseOrderReviewState{
		PaymentTerms:    input.PaymentTerms,
		Incoterms:       input.Incoterms,
		DiscountPercent: input.DiscountPercent,
		Lines:           input.Items,
	})
	decisions := make([]store.PurchaseOrderDecision, 0, len(comparison.Differences))
	for _, difference := range comparison.Differences {
		decisions = append(decisions, store.PurchaseOrderDecision{
			Key:    difference.Key,
			Choice: store.PurchaseOrderChoicePO,
		})
	}
	missing := make([]string, 0, len(comparison.Missing))
	for _, line := range comparison.Missing {
		missing = append(missing, line.Label())
	}
	extra := append([]int(nil), comparison.ExtraLineNumbers...)
	return decisions, missing, extra
}

func deleteSyntheticSource(ctx context.Context, database *store.Store, actor store.AuthUser, created createdQuotation) error {
	preview, err := database.PreviewDeletion(ctx, store.DeletionPreviewInput{
		TargetType: store.DeletionTargetQuotation,
		TargetID:   created.Record.ID,
		ActorID:    actor.ID,
	})
	if err != nil {
		return err
	}
	_, err = database.ConfirmDeletion(ctx, store.DeletionConfirmInput{
		TargetType:          store.DeletionTargetQuotation,
		TargetID:            created.Record.ID,
		Scope:               store.DeletionScopeRecord,
		Reason:              "Synthetic mock seed: delete the source after its fictional purchase order is recorded (#500).",
		TypedNumber:         preview.DocumentNumber,
		ExpectedFingerprint: preview.Fingerprint,
		ActorID:             actor.ID,
	})
	return err
}

func daysAgo(days int) time.Time {
	return time.Now().AddDate(0, 0, -days)
}
