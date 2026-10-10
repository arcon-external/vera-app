package main

import (
	"flag"
)

// localCommands need no server at all, which is what makes them useful when
// something is wrong with the server.
func localCommands() []*command {
	return []*command{schemaCommand()}
}

// schemaCommand prints a worked input file.
//
// It exists because the input shape is the one thing a CLI user cannot guess:
// the field names, the fact that money is plain rupiah, that a split line takes
// its price from its parts, and that `--input -` reads stdin. Printing a file
// that runs as-is beats documenting it and hoping.
func schemaCommand() *command {
	var blank bool
	return &command{
		name:    "schema",
		summary: "Print an example quotation input file, and the field reference.",
		usage:   "vera schema [--blank]",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&blank, "blank", false, "print an empty skeleton instead of a worked example")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			doc := exampleInput
			if blank {
				doc = blankInput
			}
			if err := a.emitJSON(map[string]any{"example": doc, "reference": inputReference()}); err != nil {
				return err
			}
			a.line("%s", doc)
			a.line("")
			a.line("%s", inputReference())
			return nil
		},
	}
}

// exampleInput is a quotation that exercises every shape the format has: a plain
// margined line, a discounted line, a line split into priced parts, and scope
// bullets at both levels.
const exampleInput = `{
  "customerCode": 2,
  "subject": "Supply and installation of electrical and mechanical systems",
  "recipientName": "Dr. A. Wibowo",
  "incoterm": "DAP",
  "validityDays": 30,
  "paymentTermDays": 90,
  "leadTimeDays": 60,
  "taxZone": "FTZ",
  "taxType": "Goods",
  "items": [
    {
      "description": "Control panel replacement",
      "uom": "Lot",
      "qty": 2,
      "unitCost": 15000000,
      "marginEnabled": true,
      "marginPercent": 20
    },
    {
      "description": "Commissioning and handover",
      "uom": "LS",
      "qty": 1,
      "unitCost": 4000000,
      "marginEnabled": true,
      "marginPercent": 25,
      "discountEnabled": true,
      "discountPercent": 10
    },
    {
      "description": "Trauma system upgrade",
      "showSubPrices": true,
      "subItems": [
        {
          "description": "Panel beds",
          "uom": "Pc",
          "qty": 2,
          "unitCost": 8500000,
          "marginEnabled": true,
          "marginPercent": 20,
          "details": ["Powder-coated, with mounting hardware"]
        },
        {
          "description": "Monitor arms and cabling",
          "uom": "LS",
          "qty": 1,
          "unitCost": 1200000,
          "discountEnabled": true,
          "discountPercent": 10
        }
      ]
    }
  ]
}`

// blankInput is the skeleton: every required field present, nothing filled in.
const blankInput = `{
  "customerCode": 0,
  "subject": "",
  "items": [
    {
      "description": "",
      "uom": "Lot",
      "qty": 1,
      "unitCost": 0
    }
  ]
}`

// inputReference is the field list, with the two things that are easy to get
// wrong called out where they are.
func inputReference() string {
	return `Fields
  customerCode      required. The two-digit client code; 'vera clients list' shows them.
  subject           required. The quotation's subject line.
  recipientName     required. Who it is addressed to.
  incoterm          EXW (default), DAP, FCA, FOB, ...
  validityDays      30 by default.
  paymentTermDays   90 by default.
  leadTimeDays      60 by default.
  taxZone           FTZ (default) or NonFTZ.
  taxType           Goods (default) or Services.
  serviceCategory   services only, and only then: NonConstruction (2% PPH22) or
                    Construction (1.75%). Any other value is refused.
  discountPercent   whole-quotation discount. Retired from the editor in favour
                    of per-row discounts; still accepted for older payloads.
  items[]           at least one line.
    description     required, and printed on the customer's document.
    uom             one the editor offers: Lot, Pc, EA, LS, set, meter, m2, m3, ...
    qty             whole number, greater than zero.
    unitCost        plain rupiah as an integer: 15000000, not "15.000.000".
    marginEnabled / marginPercent     adds to the line's own price.
    discountEnabled / discountPercent comes off the marginalised price, so the
                                      list price printed is the one staff saw.
    showSubPrices   true prints the line as a title with each part priced under
                    it; false leaves the whole price on the line and prints the
                    parts as unbilled scope.
    subItems[]      priced parts. A line with parts does not price itself: its
                    total is their sum and it is always quantity 1.
      details[]     scope bullets for the part, printed beneath it and numbered
                    X.Y.Z. Scope lines belong to a priced part, so a line with
                    no subItems has nowhere to put them -- it is refused rather
                    than printed with the text missing.

Rules the server enforces, so this tool does not have to guess
  - Margin is applied first, then the row's own discount; the whole-quotation
    discount comes off the summed subtotal.
  - A discount never reduces what we pay: a row sold below cost shows a negative
    margin, and nett profit is signed.
  - PPN (11%, non-FTZ goods) is added on top, and is collected for the state --
    it is never profit. PPH22 (services) is internal and never printed.
  - Money only ever moves as integers of rupiah.

Reading the file
  --input quotation.json    a file
  --input -                 stdin, for piping from another tool`
}
