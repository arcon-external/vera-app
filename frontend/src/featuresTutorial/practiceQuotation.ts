import type { DetailsState } from "../components/DetailsPanel";
import type { EditorItem } from "../components/ItemEditor";
import { TUTORIAL_CUSTOMER, TUTORIAL_SUBJECT } from "./tutorialData";

// Issue #605: the Quotation tutorial's client-only practice quotation. Every
// value here is a literal fixture, never a read of the real draft, the archive
// or the client directory. The workspace that renders it holds the editable
// copy in local state, so finishing, skipping or leaving the tour drops it and
// the real editor comes back exactly as it was.

/**
 * The single customer option the practice panel offers. It is a local option,
 * not a row from ListCustomers, so the practice never touches the client
 * directory -- and the id is far outside any seeded or generated code so it
 * can never collide with a real one.
 */
export const TUTORIAL_QUOTATION_CUSTOMER_ID = 9001;

export const TUTORIAL_QUOTATION_CUSTOMER_OPTIONS = [
  { value: TUTORIAL_QUOTATION_CUSTOMER_ID, label: TUTORIAL_CUSTOMER },
] as const;

/**
 * One blank practice line, shaped exactly like App's `newItem` default. It
 * lives here rather than being imported from App because App imports this
 * tutorial module; the practice editor must never create a cycle back into
 * the real workspace.
 */
export function blankPracticeItem(key: number, uom: string): EditorItem {
  return {
    key,
    description: "",
    bullets: [],
    qty: "1",
    uom,
    unitPrice: "",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    subItems: [],
    showSubPrices: false,
  };
}

export function tutorialQuotationDetails(): DetailsState {
  return {
    subject: TUTORIAL_SUBJECT,
    recipientName: "Practice Recipient (MOCK)",
    validity: 30,
    paymentTerm: 90,
    leadTimeYears: 0,
    leadTimeMonths: 0,
    leadTimeWeeks: 0,
    leadTimeDays: 0,
    discountPercent: 0,
    incoterm: "EXW",
    taxZone: "FTZ",
    taxType: "Goods",
    serviceCategory: "",
    note: "Practice note (MOCK) — safe to edit and format.",
  };
}

/**
 * Two lines: one plain line with margin on and a 10% row discount ready (off
 * until the tour's discount step turns it on), and one split package whose
 * sub-items carry their own margin and row discount. The split line prices
 * through the real rollup, so the rail exercises the same arithmetic a real
 * package would. The unit is the company's served canonical unit (#337) so
 * the practice rows never name a unit the dropdown cannot display.
 */
export function tutorialQuotationItems(uom: string): EditorItem[] {
  return [
    {
      key: 1,
      description: "Tutorial installation service (MOCK)",
      bullets: ["Practice scope line (MOCK)"],
      qty: "2",
      uom,
      unitPrice: "5000000",
      marginEnabled: true,
      marginPercent: "15",
      discountEnabled: false,
      discountPercent: "10",
      subItems: [],
      showSubPrices: false,
    },
    {
      key: 2,
      description: "Tutorial equipment package (MOCK)",
      bullets: [],
      qty: "1",
      uom,
      unitPrice: "",
      marginEnabled: false,
      marginPercent: "0",
      discountEnabled: false,
      discountPercent: "0",
      showSubPrices: true,
      subItems: [
        {
          id: 1,
          description: "Tutorial panel A (MOCK)",
          qty: "2",
          uom,
          unitCost: "2500000",
          marginEnabled: true,
          marginPercent: "10",
          discountEnabled: false,
          discountPercent: "0",
          bullets: ["Practice scope A (MOCK)"],
        },
        {
          id: 2,
          description: "Tutorial panel B (MOCK)",
          qty: "1",
          uom,
          unitCost: "1750000",
          marginEnabled: false,
          marginPercent: "0",
          discountEnabled: true,
          discountPercent: "5",
          bullets: [],
        },
      ],
    },
  ];
}
