package backend

func NewQuotationItem(desc string, qty uint32, uom string, unitPrice uint64) QuotationItem {
	total := uint64(qty) * unitPrice
	return QuotationItem{
		Description:      desc,
		DisplayDesc:      formatDescription(desc),
		Qty:              qty,
		UoM:              uom,
		UnitPrice:        unitPrice,
		Total:            total,
		DisplayUnitPrice: formatRupiah(unitPrice),
		DisplayTotal:     formatRupiah(total),
	}
}

var itemList = []QuotationItem{
	NewQuotationItem(
		"Supply & Install lighting sensor system (PIR Schneider) at AHU block3 ,2A 2, Mezzanine Front & Rear with following scopes:\n- 6 sensors at Front AHU and 6 sensors at Rear AHU\n- 6-units of batten fixtures set with guard (fixtures 2x 1200mm LED tubes/lamps) for additional lightings\n- Rewiring with Auto-Manual selector and lighting switch.",
		1,
		"Lot",
		115725000,
	),
	NewQuotationItem(
		"Supply & Install lighting sensor system (PIR Schneider) 14 sensor at CDA blok 4.3",
		1,
		"Lot",
		59970000,
	),
	NewQuotationItem(
		"Supply & Install lighting sensor system (PIR Schneider) 5 sensor office blok9",
		1,
		"Lot",
		22170000,
	),
	NewQuotationItem(
		"Supply and install aircond automation system at mesjid of Batam1, consisted of:\n- Metal enclosure panel Timers\n- Zelio Schneider.",
		1,
		"Lot",
		34050000,
	),
	NewQuotationItem(
		"Supply & Install lighting sensor system (PIR Schneider) at AHU block3 ,2A 2, Mezzanine Front & Rear with following scopes:\n- 6 sensors at Front AHU and 6 sensors at Rear AHU\n- 6-units of batten fixtures set with guard (fixtures 2x 1200mm LED tubes/lamps) for additional lightings\n- Rewiring with Auto-Manual selector and lighting switch.",
		1,
		"Lot",
		115725000,
	),
	NewQuotationItem(
		"Supply & Install lighting sensor system (PIR Schneider) 14 sensor at CDA blok 4.3",
		1,
		"Lot",
		59970000,
	),
	NewQuotationItem(
		"Supply & Install lighting sensor system (PIR Schneider) 5 sensor office blok9",
		1,
		"Lot",
		22170000,
	),
	NewQuotationItem(
		"Supply and install aircond automation system at mesjid of Batam1, consisted of:\n- Metal enclosure panel Timers\n- Zelio Schneider.",
		1,
		"Lot",
		34050000,
	),
}
