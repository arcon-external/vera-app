package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"vera-app/backend"
)

// InputDoc is the JSON shape of a quotation input file.
//
// It is deliberately not backend.QuotationInput. That struct is the wire shape,
// and it says UnitPrice for a line's cost and UnitCost for a sub-item's -- the
// same money under two names, which is a fine thing for a struct that mirrors
// the database and a confusing one for a file a person writes by hand.
//
// So this file uses one word, unitCost, everywhere cost is meant, and the
// conversion below is the only place that knows about the difference.
//
// Money is a plain integer of rupiah -- 15000000, not "15.000.000" and not a
// float. Anything else invites a rounding bug into a pricing system.
//
// Every command that reads one of these accepts `--input -`, which reads stdin,
// so a payload can come from another tool or a heredoc.
type InputDoc struct {
	// CustomerCode is the two-digit client code, the same number the desktop's
	// dropdown carries and the one printed inside the quote number.
	CustomerCode  int    `json:"customerCode"`
	Subject       string `json:"subject"`
	RecipientName string `json:"recipientName,omitempty"`
	Incoterm      string `json:"incoterm,omitempty"`
	Validity      int    `json:"validityDays,omitempty"`
	PaymentTerm   int    `json:"paymentTermDays,omitempty"`
	// The lead time is four independent units, each 0..255 and each optional:
	// a quotation may state none of them, which means no lead time at all.
	// They print as entered, with no conversion between them.
	LeadTimeYears   int    `json:"leadTimeYears,omitempty"`
	LeadTimeMonths  int    `json:"leadTimeMonths,omitempty"`
	LeadTimeWeeks   int    `json:"leadTimeWeeks,omitempty"`
	LeadTimeDays    int    `json:"leadTimeDays,omitempty"`
	TaxZone         string `json:"taxZone,omitempty"`
	TaxType         string `json:"taxType,omitempty"`
	ServiceCategory string `json:"serviceCategory,omitempty"`

	// DiscountPercent is the whole-quotation discount. It is retired from the
	// editor in favour of per-row discounts and only still here so an older
	// payload loads; new quotations should discount rows.
	DiscountPercent int `json:"discountPercent,omitempty"`

	Items []InputItem `json:"items"`
}

// InputItem is one line of the quotation.
type InputItem struct {
	Description string `json:"description"`
	// UoM must be one of the catalogue the editor offers (Lot, Pc, EA, LS, set,
	// meter, m2, m3, ...); the server refuses anything else, because an
	// arbitrary unit would print on the customer's document unchecked.
	UoM      string `json:"uom"`
	Qty      int    `json:"qty"`
	UnitCost int64  `json:"unitCost"`

	MarginEnabled   bool `json:"marginEnabled,omitempty"`
	MarginPercent   int  `json:"marginPercent,omitempty"`
	DiscountEnabled bool `json:"discountEnabled,omitempty"`
	DiscountPercent int  `json:"discountPercent,omitempty"`

	// SubItems split a line into priced parts. A line with parts does not price
	// itself: its total is their sum and it is always quantity 1.
	SubItems []InputSubItem `json:"subItems,omitempty"`

	// ShowSubPrices picks the customer presentation: true prints the line as a
	// title with each part priced beneath it, false leaves the whole price on
	// the line and prints the parts as unbilled scope.
	ShowSubPrices bool `json:"showSubPrices,omitempty"`

	// Details are the line's scope bullets, printed under its description. A
	// line with parts puts its detail lines on the parts instead.
	Details []string `json:"details,omitempty"`
}

// InputSubItem is one priced part of a split line.
type InputSubItem struct {
	Description string `json:"description"`
	UoM         string `json:"uom"`
	Qty         int    `json:"qty"`
	UnitCost    int64  `json:"unitCost"`

	MarginEnabled   bool `json:"marginEnabled,omitempty"`
	MarginPercent   int  `json:"marginPercent,omitempty"`
	DiscountEnabled bool `json:"discountEnabled,omitempty"`
	DiscountPercent int  `json:"discountPercent,omitempty"`

	// Details are the part's scope lines ("what this includes").
	Details []string `json:"details,omitempty"`
}

// LoadInputDoc reads and checks an input file.
//
// The checks here are the ones whose absence produces a confusing server error:
// a missing quantity, a missing unit. Everything else -- percentages in range,
// arithmetic, tax rules -- is the server's to decide, and duplicating those
// rules would give two answers to disagree.
func LoadInputDoc(path string) (*InputDoc, error) {
	raw, err := readInputFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	// Unknown fields are refused rather than ignored: a typo in a field name
	// would otherwise silently drop the value, which on a pricing document
	// means quoting a number nobody chose.
	decoder.DisallowUnknownFields()
	var doc InputDoc
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("read the input file: %w", err)
	}

	if doc.CustomerCode == 0 {
		return nil, fmt.Errorf(`"customerCode" is required; run 'vera clients list' for the codes`)
	}
	if doc.CustomerCode < 1 || doc.CustomerCode > 99 {
		return nil, fmt.Errorf(`"customerCode" must be between 1 and 99, got %d`, doc.CustomerCode)
	}
	// Each lead-time unit is a uint8 on the wire, so the server's own decoder
	// would reject anything above 255 with a JSON type error rather than a
	// sentence naming the field. Naming it here is the same courtesy
	// customerCode gets.
	for _, unit := range []struct {
		name  string
		value int
	}{
		{"leadTimeYears", doc.LeadTimeYears},
		{"leadTimeMonths", doc.LeadTimeMonths},
		{"leadTimeWeeks", doc.LeadTimeWeeks},
		{"leadTimeDays", doc.LeadTimeDays},
	} {
		if unit.value < 0 || unit.value > 255 {
			return nil, fmt.Errorf(`%q must be between 0 and 255, got %d`, unit.name, unit.value)
		}
	}
	if len(doc.Items) == 0 {
		return nil, fmt.Errorf(`"items" is empty; a quotation needs at least one line`)
	}
	for i, item := range doc.Items {
		where := fmt.Sprintf("items[%d]", i)
		if strings.TrimSpace(item.Description) == "" {
			return nil, fmt.Errorf(`%s."description" is required`, where)
		}
		if item.Qty == 0 && len(item.SubItems) == 0 {
			return nil, fmt.Errorf(`%s."qty" must be greater than zero`, where)
		}
		if strings.TrimSpace(item.UoM) == "" && len(item.SubItems) == 0 {
			return nil, fmt.Errorf(`%s."uom" is required`, where)
		}
		for j, sub := range item.SubItems {
			subWhere := fmt.Sprintf("%s.subItems[%d]", where, j)
			if strings.TrimSpace(sub.Description) == "" {
				return nil, fmt.Errorf(`%s."description" is required`, subWhere)
			}
			if sub.Qty == 0 {
				return nil, fmt.Errorf(`%s."qty" must be greater than zero`, subWhere)
			}
			if strings.TrimSpace(sub.UoM) == "" {
				return nil, fmt.Errorf(`%s."uom" is required`, subWhere)
			}
		}
	}
	return &doc, nil
}

// toQuotationInput converts the file's shape into the server's.
//
// The defaults are the same ones the editor applies to a new quotation, so a
// minimal file produces a sane document rather than a zeroed one: 30 days valid,
// 90 days payment, no lead time stated at all (a quotation need not have one),
// EXW, FTZ goods.
func (d *InputDoc) toQuotationInput() (backend.QuotationInput, error) {
	input := backend.QuotationInput{
		CustomerID:      uint8(d.CustomerCode),
		Subject:         d.Subject,
		RecipientName:   d.RecipientName,
		Incoterm:        defaultString(d.Incoterm, "EXW"),
		Validity:        uint8(defaultInt(d.Validity, 30)),
		PaymentTerm:     uint8(defaultInt(d.PaymentTerm, 90)),
		LeadTimeYears:   uint8(d.LeadTimeYears),
		LeadTimeMonths:  uint8(d.LeadTimeMonths),
		LeadTimeWeeks:   uint8(d.LeadTimeWeeks),
		LeadTimeDays:    uint8(d.LeadTimeDays),
		TaxZone:         defaultString(d.TaxZone, backend.TaxZoneFTZ),
		TaxType:         defaultString(d.TaxType, backend.TaxTypeGoods),
		ServiceCategory: d.ServiceCategory,
		DiscountPercent: uint8(d.DiscountPercent),
	}

	// The service category only means anything for services, and sending one for
	// goods is a validation error rather than a harmless extra.
	if input.TaxType != backend.TaxTypeServices {
		input.ServiceCategory = ""
	}

	for i, item := range d.Items {
		converted := backend.QuotationItemInput{
			Description:     item.Description,
			UoM:             item.UoM,
			Qty:             uint32(item.Qty),
			UnitPrice:       uint64(item.UnitCost),
			MarginEnabled:   item.MarginEnabled,
			MarginPercent:   uint16(item.MarginPercent),
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: uint16(item.DiscountPercent),
			ShowSubPrices:   item.ShowSubPrices,
		}
		if len(item.SubItems) > 0 {
			// A line with parts never prices itself: the server replaces these
			// with the rollup, and sending real figures here would be a second
			// answer to a question the parts already answer.
			converted.Qty = 1
			converted.UoM = "Lot"
			converted.UnitPrice = 0
			converted.MarginEnabled = false
			converted.MarginPercent = 0
			converted.DiscountEnabled = false
			converted.DiscountPercent = 0
			for _, sub := range item.SubItems {
				converted.SubItems = append(converted.SubItems, backend.QuotationSubItemInput{
					Description:     sub.Description,
					UoM:             sub.UoM,
					Qty:             uint32(sub.Qty),
					UnitCost:        uint64(sub.UnitCost),
					MarginEnabled:   sub.MarginEnabled,
					MarginPercent:   uint16(sub.MarginPercent),
					DiscountEnabled: sub.DiscountEnabled,
					DiscountPercent: uint16(sub.DiscountPercent),
					SubSubItems:     subSubItems(sub.Details),
				})
			}
		} else if len(item.Details) > 0 {
			// A line with no parts has nowhere to put scope lines: the domain
			// model carries them only beneath a priced part. Silently dropping
			// them would print a document missing text the caller wrote, so
			// this is refused and the fix is named.
			return backend.QuotationInput{}, fmt.Errorf(
				`%s."details" needs "subItems": scope lines print under a priced part. Split the line into subItems, or drop the details.`,
				itemWhere(i))
		}
		input.Items = append(input.Items, converted)
	}
	return input, nil
}

// itemWhere names a line in an error the way LoadInputDoc does, so the message
// points at the same JSON path whichever check caught the problem.
func itemWhere(index int) string {
	return fmt.Sprintf("items[%d]", index)
}

func subSubItems(details []string) []backend.QuotationSubSubItemInput {
	var out []backend.QuotationSubSubItemInput
	for _, detail := range details {
		if strings.TrimSpace(detail) == "" {
			continue
		}
		out = append(out, backend.QuotationSubSubItemInput{Description: detail})
	}
	return out
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
