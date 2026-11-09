package localseed

import (
	"fmt"
	"strings"
)

// ManifestVersion identifies the fixture set. It is stored beside the run
// marker so a later version of the seed never mistakes an older set for its
// own work and silently rebuilds it.
const ManifestVersion = 1

// Fictional marks every fixture. It is part of every subject, every mock
// customer name and every document's first line, so nothing seeded here can be
// mistaken for a real quotation or a customer's purchase order.
const Fictional = "[MOCK]"

// Manifest is the exact, reviewable inventory #500 asked for: six live mock
// Quotations, eight mock Purchase Orders, five occupied and one available, plus
// two synthetic source Quotations that are created and deleted to produce the
// former-source states. The seed never invents a fixture outside this list.
type Manifest struct {
	Quotations       []QuotationSpec
	PurchaseOrders   []POSpec
	SyntheticSources []QuotationSpec
}

// QuotationSpec is one fictional Quotation and the revisions it is issued in.
type QuotationSpec struct {
	Key              string
	Subject          string
	CustomerName     string
	CustomerAddress  string
	RecipientName    string
	RecipientAddress string
	Incoterm         string
	TaxZone          string
	TaxType          string
	ServiceCategory  string
	LeadTimeDays     int16
	LeadTimeWeeks    int16
	LeadTimeMonths   int16
	LeadTimeYears    int16
	PaymentTermDays  int16
	ValidityDays     int16
	CustomerCode     int16
	Revisions        []QuotationRevisionSpec
}

// QuotationRevisionSpec is one immutable issued revision. Revision 0 is the
// first; each later spec is saved as the next revision of the same quotation.
type QuotationRevisionSpec struct {
	Subject         string
	DiscountPercent int16
	Items           []ItemSpec
}

// ItemSpec is one quotation line. A line with SubItems is a package: its price
// is the sum of its priced parts and ShowSubPrices chooses whether the customer
// sees those parts as priced rows.
type ItemSpec struct {
	Description     string
	UoM             string
	MarginPercent   int16
	DiscountPercent int16
	Qty             int64
	UnitCost        int64
	ShowSubPrices   bool
	MarginEnabled   bool
	DiscountEnabled bool
	SubItems        []SubItemSpec
}

// SubItemSpec is one priced part of a package, with the description-only
// detail lines beneath it.
type SubItemSpec struct {
	Description     string
	UoM             string
	MarginPercent   int16
	DiscountPercent int16
	Qty             int64
	UnitCost        int64
	MarginEnabled   bool
	DiscountEnabled bool
	Details         []string
}

// POSpec is one mock Purchase Order. SourceKey names a live QuotationSpec or a
// SyntheticSources entry; an empty SourceKey is the never-linked draft.
type POSpec struct {
	Key           string
	Label         string
	SourceKey     string
	SourceState   string // live, never, former
	ExpectedState string // draft, saved
	Document      string // none, text, mixed
	Differences   bool
	Adjustment    bool
	Correction    bool
	Confirmation  bool
}

// State values shared with the store's public vocabulary.
const (
	stateDraft = "draft"
	stateSaved = "saved"

	sourceLive   = "live"
	sourceNever  = "never"
	sourceFormer = "former"

	documentNone  = "none"
	documentText  = "text"
	documentMixed = "mixed"
)

// MockManifest is the approved dataset. Amounts are whole rupiah (IDR has
// exponent 0); supplier costs, margins and choices are fictional but arithmetically
// coherent, so the editor and the customer document show the same numbers a
// staff member would see on a real quotation.
func MockManifest() Manifest {
	return Manifest{
		Quotations: []QuotationSpec{
			{
				Key:              "ftz-goods-finalized-pdf",
				Subject:          Fictional + " FTZ compressed-air package",
				CustomerName:     "PT Contoh Bahari Nusantara (MOCK)",
				CustomerAddress:  "Jalan Fiksi No. 1, Batam, Kepulauan Riau (MOCK ADDRESS)",
				RecipientName:    "Ibu Fiksi Laut",
				RecipientAddress: "Jalan Fiksi No. 1, Batam, Kepulauan Riau (MOCK ADDRESS)",
				Incoterm:         "FOB",
				TaxZone:          "FTZ",
				TaxType:          "Goods",
				PaymentTermDays:  30,
				ValidityDays:     30,
				LeadTimeDays:     14,
				CustomerCode:     90,
				Revisions: []QuotationRevisionSpec{
					{
						Items: []ItemSpec{
							{
								Description:   "Precision air compressor package (MOCK)",
								UoM:           "Lot",
								Qty:           1,
								UnitCost:      185_000_000,
								MarginEnabled: true,
								MarginPercent: 18,
							},
							{
								Description:   "Installation supervision (MOCK)",
								UoM:           "Day",
								Qty:           5,
								UnitCost:      2_750_000,
								MarginEnabled: true,
								MarginPercent: 12,
							},
						},
					},
					{
						Items: []ItemSpec{
							{
								Description:   "Precision air compressor package (MOCK)",
								UoM:           "Lot",
								Qty:           1,
								UnitCost:      185_000_000,
								MarginEnabled: true,
								MarginPercent: 18,
							},
							{
								Description:   "Installation supervision (MOCK)",
								UoM:           "Day",
								Qty:           5,
								UnitCost:      2_750_000,
								MarginEnabled: true,
								MarginPercent: 12,
							},
							{
								Description:   "Commissioning and handover (MOCK)",
								UoM:           "LS",
								Qty:           1,
								UnitCost:      7_500_000,
								MarginEnabled: true,
								MarginPercent: 10,
							},
						},
					},
				},
			},
			{
				Key:              "nonftz-goods-finalized-differences",
				Subject:          Fictional + " Non-FTZ clean-room package with PPN",
				CustomerName:     "PT Contoh Konstruksi Prima (MOCK)",
				CustomerAddress:  "Jalan Rekaan No. 22, Jakarta Utara (MOCK ADDRESS)",
				RecipientName:    "Bapak Fiksi Bangun",
				RecipientAddress: "Jalan Rekaan No. 22, Jakarta Utara (MOCK ADDRESS)",
				Incoterm:         "DAP",
				TaxZone:          "NonFTZ",
				TaxType:          "Goods",
				PaymentTermDays:  45,
				ValidityDays:     30,
				LeadTimeMonths:   1,
				CustomerCode:     91,
				Revisions: []QuotationRevisionSpec{
					{
						DiscountPercent: 5,
						Items: []ItemSpec{
							{
								Description:   "Modular clean-room panel system (MOCK)",
								UoM:           "Lot",
								Qty:           1,
								ShowSubPrices: true,
								SubItems: []SubItemSpec{
									{
										Description:   "Wall panels (MOCK)",
										UoM:           "Pc",
										Qty:           24,
										UnitCost:      1_850_000,
										MarginEnabled: true,
										MarginPercent: 15,
									},
									{
										Description:     "Ceiling grid (MOCK)",
										UoM:             "Set",
										Qty:             12,
										UnitCost:        2_400_000,
										MarginEnabled:   true,
										MarginPercent:   12,
										DiscountEnabled: true,
										DiscountPercent: 5,
									},
									{
										Description:   "Door sets (MOCK)",
										UoM:           "Unit",
										Qty:           4,
										UnitCost:      6_750_000,
										MarginEnabled: true,
										MarginPercent: 20,
									},
								},
							},
							{
								Description:   "Freight and insurance (MOCK)",
								UoM:           "LS",
								Qty:           1,
								UnitCost:      12_000_000,
								MarginEnabled: true,
								MarginPercent: 8,
							},
						},
					},
				},
			},
			{
				Key:              "services-nonconstruction-incomplete-draft",
				Subject:          Fictional + " Non-construction maintenance service",
				CustomerName:     "PT Contoh Layanan Teknik (MOCK)",
				CustomerAddress:  "Jalan Tiruan No. 3, Surabaya (MOCK ADDRESS)",
				RecipientName:    "Ibu Fiksi Rawat",
				RecipientAddress: "Jalan Tiruan No. 3, Surabaya (MOCK ADDRESS)",
				Incoterm:         "DAP",
				TaxZone:          "NonFTZ",
				TaxType:          "Services",
				ServiceCategory:  "NonConstruction",
				PaymentTermDays:  14,
				ValidityDays:     30,
				CustomerCode:     92,
				Revisions: []QuotationRevisionSpec{
					{
						Items: []ItemSpec{
							{
								Description:   "Annual electrical maintenance service (MOCK)",
								UoM:           "Month",
								Qty:           12,
								UnitCost:      4_500_000,
								MarginEnabled: true,
								MarginPercent: 15,
							},
						},
					},
				},
			},
			{
				Key:              "services-construction-draft-differences",
				Subject:          Fictional + " Construction service with a row discount",
				CustomerName:     "PT Contoh Mekanikal Utama (MOCK)",
				CustomerAddress:  "Jalan Khayalan No. 8, Bekasi (MOCK ADDRESS)",
				RecipientName:    "Bapak Fiksi Pasang",
				RecipientAddress: "Jalan Khayalan No. 8, Bekasi (MOCK ADDRESS)",
				Incoterm:         "FOB",
				TaxZone:          "NonFTZ",
				TaxType:          "Services",
				ServiceCategory:  "Construction",
				PaymentTermDays:  30,
				ValidityDays:     45,
				LeadTimeWeeks:    2,
				CustomerCode:     93,
				Revisions: []QuotationRevisionSpec{
					{
						Items: []ItemSpec{
							{
								Description:     "Structural steel erection (MOCK)",
								UoM:             "Ton",
								Qty:             18,
								UnitCost:        9_800_000,
								MarginEnabled:   true,
								MarginPercent:   18,
								DiscountEnabled: true,
								DiscountPercent: 10,
							},
							{
								Description:   "Site safety supervision (MOCK)",
								UoM:           "Month",
								Qty:           3,
								UnitCost:      5_200_000,
								MarginEnabled: true,
								MarginPercent: 10,
							},
						},
					},
				},
			},
			{
				Key:              "nonftz-subitems-no-pdf-correction",
				Subject:          Fictional + " Non-FTZ laboratory package, no customer PDF",
				CustomerName:     "PT Contoh Niaga Sentosa (MOCK)",
				CustomerAddress:  "Jalan Fiktif No. 17, Bandung (MOCK ADDRESS)",
				RecipientName:    "Ibu Fiksi Laboratorium",
				RecipientAddress: "Jalan Fiktif No. 17, Bandung (MOCK ADDRESS)",
				Incoterm:         "CIF",
				TaxZone:          "NonFTZ",
				TaxType:          "Goods",
				PaymentTermDays:  60,
				ValidityDays:     30,
				LeadTimeWeeks:    6,
				CustomerCode:     94,
				Revisions: []QuotationRevisionSpec{
					{
						Items: []ItemSpec{
							{
								Description:   "Laboratory fume-hood package (MOCK)",
								UoM:           "Lot",
								Qty:           1,
								ShowSubPrices: true,
								SubItems: []SubItemSpec{
									{
										Description:   "Fume hood body (MOCK)",
										UoM:           "Unit",
										Qty:           6,
										UnitCost:      14_500_000,
										MarginEnabled: true,
										MarginPercent: 16,
									},
									{
										Description:   "Ducting and scrubber (MOCK)",
										UoM:           "LS",
										Qty:           1,
										UnitCost:      22_000_000,
										MarginEnabled: true,
										MarginPercent: 14,
									},
								},
							},
							{
								Description:   "Calibration kit (MOCK)",
								UoM:           "Set",
								Qty:           2,
								UnitCost:      3_250_000,
								MarginEnabled: true,
								MarginPercent: 20,
							},
						},
					},
					{
						Items: []ItemSpec{
							{
								Description:   "Laboratory fume-hood package (MOCK)",
								UoM:           "Lot",
								Qty:           1,
								ShowSubPrices: true,
								SubItems: []SubItemSpec{
									{
										Description:   "Fume hood body (MOCK)",
										UoM:           "Unit",
										Qty:           6,
										UnitCost:      14_500_000,
										MarginEnabled: true,
										MarginPercent: 16,
									},
									{
										Description:   "Ducting and scrubber (MOCK)",
										UoM:           "LS",
										Qty:           1,
										UnitCost:      22_000_000,
										MarginEnabled: true,
										MarginPercent: 14,
									},
								},
							},
							{
								Description:   "Calibration kit (MOCK)",
								UoM:           "Set",
								Qty:           3,
								UnitCost:      3_250_000,
								MarginEnabled: true,
								MarginPercent: 18,
							},
						},
					},
				},
			},
			{
				Key:              "ftz-goods-available",
				Subject:          Fictional + " FTZ container spreader, available for linking",
				CustomerName:     "PT Contoh Tersedia Jaya (MOCK)",
				CustomerAddress:  "Jalan Contoh No. 5, Medan (MOCK ADDRESS)",
				RecipientName:    "Bapak Fiksi Tersedia",
				RecipientAddress: "Jalan Contoh No. 5, Medan (MOCK ADDRESS)",
				Incoterm:         "FOB",
				TaxZone:          "FTZ",
				TaxType:          "Goods",
				PaymentTermDays:  30,
				ValidityDays:     30,
				CustomerCode:     95,
				Revisions: []QuotationRevisionSpec{
					{
						Items: []ItemSpec{
							{
								Description:   "Container handling spreader (MOCK)",
								UoM:           "Unit",
								Qty:           2,
								UnitCost:      48_000_000,
								MarginEnabled: true,
								MarginPercent: 15,
							},
						},
					},
					{
						Items: []ItemSpec{
							{
								Description:   "Container handling spreader (MOCK)",
								UoM:           "Unit",
								Qty:           3,
								UnitCost:      48_000_000,
								MarginEnabled: true,
								MarginPercent: 15,
							},
						},
					},
				},
			},
		},
		PurchaseOrders: []POSpec{
			{
				Key:           "po-linked-incomplete-draft",
				Label:         "Linked incomplete draft",
				SourceKey:     "services-nonconstruction-incomplete-draft",
				SourceState:   sourceLive,
				ExpectedState: stateDraft,
				Document:      documentNone,
			},
			{
				Key:           "po-linked-draft-differences",
				Label:         "Linked draft with deliberate differences",
				SourceKey:     "services-construction-draft-differences",
				SourceState:   sourceLive,
				ExpectedState: stateDraft,
				Document:      documentNone,
				Differences:   true,
			},
			{
				Key:           "po-finalized-pdf",
				Label:         "Matching finalized PO with a synthetic PDF",
				SourceKey:     "ftz-goods-finalized-pdf",
				SourceState:   sourceLive,
				ExpectedState: stateSaved,
				Document:      documentText,
			},
			{
				Key:           "po-finalized-differences-adjustment",
				Label:         "Finalized with accepted differences and a reasoned adjustment",
				SourceKey:     "nonftz-goods-finalized-differences",
				SourceState:   sourceLive,
				ExpectedState: stateSaved,
				Document:      documentNone,
				Differences:   true,
				Adjustment:    true,
			},
			{
				Key:           "po-finalized-no-pdf-confirmation-correction",
				Label:         "Finalized without a PDF, with confirmation number and correction history",
				SourceKey:     "nonftz-subitems-no-pdf-correction",
				SourceState:   sourceLive,
				ExpectedState: stateSaved,
				Document:      documentNone,
				Confirmation:  true,
				Correction:    true,
			},
			{
				Key:           "po-never-linked-mixed-pdf",
				Label:         "Never-linked Source missing draft with a mixed manual/scanned PDF",
				SourceState:   sourceNever,
				ExpectedState: stateDraft,
				Document:      documentMixed,
			},
			{
				Key:           "po-former-linked-draft",
				Label:         "Formerly-linked Source missing draft",
				SourceKey:     "synthetic-former-draft-source",
				SourceState:   sourceFormer,
				ExpectedState: stateDraft,
				Document:      documentNone,
			},
			{
				Key:           "po-former-linked-finalized",
				Label:         "Formerly-linked Source missing finalized PO ready for source repair",
				SourceKey:     "synthetic-former-finalized-source",
				SourceState:   sourceFormer,
				ExpectedState: stateSaved,
				Document:      documentText,
			},
		},
		SyntheticSources: []QuotationSpec{
			{
				Key:              "synthetic-former-draft-source",
				Subject:          Fictional + " synthetic source for the former-source draft",
				CustomerName:     "PT Contoh Konstruksi Prima (MOCK)",
				CustomerAddress:  "Jalan Rekaan No. 22, Jakarta Utara (MOCK ADDRESS)",
				RecipientName:    "Bapak Fiksi Sumber",
				RecipientAddress: "Jalan Rekaan No. 22, Jakarta Utara (MOCK ADDRESS)",
				Incoterm:         "DAP",
				TaxZone:          "NonFTZ",
				TaxType:          "Goods",
				PaymentTermDays:  30,
				ValidityDays:     30,
				CustomerCode:     91,
				Revisions: []QuotationRevisionSpec{
					{
						Items: []ItemSpec{
							{
								Description:   "Synthetic former source line (MOCK)",
								UoM:           "Lot",
								Qty:           1,
								UnitCost:      10_000_000,
								MarginEnabled: true,
								MarginPercent: 10,
							},
						},
					},
				},
			},
			{
				Key:              "synthetic-former-finalized-source",
				Subject:          Fictional + " synthetic source for the former-source finalized PO",
				CustomerName:     "PT Contoh Layanan Teknik (MOCK)",
				CustomerAddress:  "Jalan Tiruan No. 3, Surabaya (MOCK ADDRESS)",
				RecipientName:    "Ibu Fiksi Sumber",
				RecipientAddress: "Jalan Tiruan No. 3, Surabaya (MOCK ADDRESS)",
				Incoterm:         "DAP",
				TaxZone:          "NonFTZ",
				TaxType:          "Services",
				ServiceCategory:  "NonConstruction",
				PaymentTermDays:  30,
				ValidityDays:     30,
				CustomerCode:     92,
				Revisions: []QuotationRevisionSpec{
					{
						Items: []ItemSpec{
							{
								Description:   "Synthetic former service source line (MOCK)",
								UoM:           "LS",
								Qty:           1,
								UnitCost:      8_000_000,
								MarginEnabled: true,
								MarginPercent: 10,
							},
						},
					},
				},
			},
		},
	}
}

// Coverage is the acceptance matrix's feature checklist, derived from the
// manifest rather than maintained as a second list.
type Coverage struct {
	FTZGoods                bool
	NonFTZGoodsWithPPN      bool
	ServicesNonConstruction bool
	ServicesConstruction    bool
	WholeDiscount           bool
	RowDiscount             bool
	SubItems                bool
	MultipleRevisions       bool
}

// Coverage reports which shapes the six live quotations actually cover.
func (m Manifest) Coverage() Coverage {
	var coverage Coverage
	for _, quotation := range m.Quotations {
		if quotation.TaxZone == "FTZ" && quotation.TaxType == "Goods" {
			coverage.FTZGoods = true
		}
		if quotation.TaxZone == "NonFTZ" && quotation.TaxType == "Goods" {
			coverage.NonFTZGoodsWithPPN = true
		}
		if quotation.TaxType == "Services" && quotation.ServiceCategory == "NonConstruction" {
			coverage.ServicesNonConstruction = true
		}
		if quotation.TaxType == "Services" && quotation.ServiceCategory == "Construction" {
			coverage.ServicesConstruction = true
		}
		if len(quotation.Revisions) > 1 {
			coverage.MultipleRevisions = true
		}
		for _, revision := range quotation.Revisions {
			if revision.DiscountPercent > 0 {
				coverage.WholeDiscount = true
			}
			for _, item := range revision.Items {
				if len(item.SubItems) > 0 {
					coverage.SubItems = true
				}
				if item.DiscountEnabled {
					coverage.RowDiscount = true
				}
			}
		}
	}
	return coverage
}

// OccupiedQuotations counts the live Quotations a mock PO follows. Available
// is the rest; the manifest must report five and one.
func (m Manifest) OccupiedQuotations() (occupied, available int) {
	used := make(map[string]bool)
	for _, order := range m.PurchaseOrders {
		if order.SourceState == sourceLive {
			used[order.SourceKey] = true
		}
	}
	for _, quotation := range m.Quotations {
		if used[quotation.Key] {
			occupied++
		} else {
			available++
		}
	}
	return occupied, available
}

// Validate is the manifest's own guard. It is called before any write, so a
// drift in the fixture list fails the run instead of producing a half-approved
// dataset.
func (m Manifest) Validate() error {
	if len(m.Quotations) != 6 {
		return fmt.Errorf("manifest has %d live Quotations; want exactly 6", len(m.Quotations))
	}
	if len(m.PurchaseOrders) != 8 {
		return fmt.Errorf("manifest has %d Purchase Orders; want exactly 8", len(m.PurchaseOrders))
	}
	if len(m.SyntheticSources) != 2 {
		return fmt.Errorf("manifest has %d synthetic sources; want exactly 2", len(m.SyntheticSources))
	}
	keys := make(map[string]bool)
	for _, quotation := range append(append([]QuotationSpec{}, m.Quotations...), m.SyntheticSources...) {
		if quotation.Key == "" {
			return fmt.Errorf("a quotation fixture has no key")
		}
		if keys[quotation.Key] {
			return fmt.Errorf("duplicate fixture key %q", quotation.Key)
		}
		keys[quotation.Key] = true
		if !strings.Contains(quotation.Subject, Fictional) || !strings.Contains(quotation.CustomerName, "(MOCK)") {
			return fmt.Errorf("quotation %q is not clearly marked fictional", quotation.Key)
		}
		if len(quotation.Revisions) == 0 {
			return fmt.Errorf("quotation %q has no revisions", quotation.Key)
		}
		for index, revision := range quotation.Revisions {
			if len(revision.Items) == 0 {
				return fmt.Errorf("quotation %q revision %d has no items", quotation.Key, index)
			}
		}
	}
	liveSources := make(map[string]bool)
	for _, quotation := range m.Quotations {
		liveSources[quotation.Key] = true
	}
	customerNames := make(map[int16]string)
	for _, quotation := range append(append([]QuotationSpec{}, m.Quotations...), m.SyntheticSources...) {
		if existing, seen := customerNames[quotation.CustomerCode]; seen && existing != quotation.CustomerName {
			return fmt.Errorf("customer code %d is used for both %q and %q", quotation.CustomerCode, existing, quotation.CustomerName)
		}
		customerNames[quotation.CustomerCode] = quotation.CustomerName
	}
	for _, order := range m.PurchaseOrders {
		if order.Key == "" || order.Label == "" {
			return fmt.Errorf("a purchase order fixture has no key or label")
		}
		switch order.SourceState {
		case sourceLive:
			if !liveSources[order.SourceKey] {
				return fmt.Errorf("purchase order %q names unknown live source %q", order.Key, order.SourceKey)
			}
		case sourceFormer:
			if !keys[order.SourceKey] {
				return fmt.Errorf("purchase order %q names unknown former source %q", order.Key, order.SourceKey)
			}
			if liveSources[order.SourceKey] {
				return fmt.Errorf("purchase order %q says former but %q is live", order.Key, order.SourceKey)
			}
		case sourceNever:
			if order.SourceKey != "" {
				return fmt.Errorf("purchase order %q is never-linked but names a source", order.Key)
			}
		default:
			return fmt.Errorf("purchase order %q has unknown source state %q", order.Key, order.SourceState)
		}
		if order.ExpectedState != stateDraft && order.ExpectedState != stateSaved {
			return fmt.Errorf("purchase order %q has unknown state %q", order.Key, order.ExpectedState)
		}
		if order.SourceState == sourceNever && order.ExpectedState == stateSaved {
			return fmt.Errorf("purchase order %q would be a finalized record with no source", order.Key)
		}
	}
	occupied, available := m.OccupiedQuotations()
	if occupied != 5 || available != 1 {
		return fmt.Errorf("manifest occupies %d quotations and leaves %d; want 5 and 1", occupied, available)
	}
	coverage := m.Coverage()
	if !coverage.FTZGoods || !coverage.NonFTZGoodsWithPPN ||
		!coverage.ServicesNonConstruction || !coverage.ServicesConstruction ||
		!coverage.WholeDiscount || !coverage.RowDiscount ||
		!coverage.SubItems || !coverage.MultipleRevisions {
		return fmt.Errorf("manifest coverage is incomplete: %+v", coverage)
	}
	return nil
}
