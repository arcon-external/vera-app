import { describe, expect, it } from "vitest";
import type { DraftPayload } from "./drafts";
import {
  RECOVERY_VERSION,
  buildRecoverySnapshot,
  parseRecoverySnapshot,
  recoveryLabel,
} from "./recovery";

// Issue #537: the recovery envelope is the frontend's own shape. Go stores it
// verbatim, so the parser is the only thing standing between a crash and a
// restore prompt: it must accept a snapshot this build wrote, accept a draft an
// older build wrote, and refuse anything it cannot safely apply.

const draft: DraftPayload = {
  items: [
    {
      key: 0,
      description: "Cable tray",
      bullets: ["Hot-dip galvanised"],
      qty: "2",
      uom: "Lot",
      unitPrice: "1500000",
      marginEnabled: true,
      marginPercent: "10",
      discountEnabled: false,
      discountPercent: "0",
      subItems: [],
      showSubPrices: false,
    },
  ],
  details: {
    subject: "Supply and installation",
    recipientName: "Ada",
    validity: 30,
    paymentTerm: 90,
    leadTimeYears: 0,
    leadTimeMonths: 0,
    leadTimeWeeks: 0,
    leadTimeDays: 14,
    discountPercent: 0,
    incoterm: "EXW",
    taxZone: "FTZ",
    taxType: "Goods",
    serviceCategory: "",
    note: "**Deliver** by Friday",
  },
  customerId: 2,
};

describe("recovery snapshot envelope (#537)", () => {
  it("round-trips the draft, including the note", () => {
    const snapshot = buildRecoverySnapshot(draft, "2026-11-09T14:14:00");
    expect(snapshot.version).toBe(RECOVERY_VERSION);

    const parsed = parseRecoverySnapshot(JSON.stringify(snapshot));
    expect(parsed).not.toBeNull();
    expect(parsed?.savedAt).toBe("2026-11-09T14:14:00");
    expect(parsed?.draft).toEqual(draft);
    expect(parsed?.draft.details.note).toBe("**Deliver** by Friday");
  });

  it("rejects a wrong or absent version", () => {
    const snapshot = buildRecoverySnapshot(draft, "2026-11-09T14:14:00");
    expect(
      parseRecoverySnapshot(JSON.stringify({ ...snapshot, version: 2 }))
    ).toBeNull();
    expect(
      parseRecoverySnapshot(JSON.stringify({ ...snapshot, version: undefined }))
    ).toBeNull();
  });

  it("rejects malformed JSON and non-objects", () => {
    expect(parseRecoverySnapshot("{not json")).toBeNull();
    expect(parseRecoverySnapshot("null")).toBeNull();
    expect(parseRecoverySnapshot("[]")).toBeNull();
    expect(parseRecoverySnapshot("")).toBeNull();
  });

  it("rejects a missing or blank savedAt", () => {
    const snapshot = buildRecoverySnapshot(draft, "2026-11-09T14:14:00");
    expect(
      parseRecoverySnapshot(
        JSON.stringify({ version: 1, draft: snapshot.draft })
      )
    ).toBeNull();
    expect(
      parseRecoverySnapshot(JSON.stringify({ ...snapshot, savedAt: "   " }))
    ).toBeNull();
  });

  it("opens a draft written before the note existed", () => {
    const older = JSON.parse(JSON.stringify(draft)) as Record<string, unknown>;
    const details = older.details as Record<string, unknown>;
    delete details.note;
    const raw = JSON.stringify({
      version: 1,
      savedAt: "2026-11-09T14:14:00",
      draft: older,
    });

    const parsed = parseRecoverySnapshot(raw);
    expect(parsed).not.toBeNull();
    expect(parsed?.draft.details.note).toBe("");
    expect(parsed?.draft.details.subject).toBe("Supply and installation");
  });

  it("labels the snapshot with the saved time", () => {
    const snapshot = buildRecoverySnapshot(draft, "2026-11-09T14:14:00");
    expect(recoveryLabel(snapshot)).toBe("2:14 PM");
  });

  it("labels midnight and noon as 12, not 0", () => {
    expect(
      recoveryLabel(buildRecoverySnapshot(draft, "2026-11-09T00:05:00"))
    ).toBe("12:05 AM");
    expect(
      recoveryLabel(buildRecoverySnapshot(draft, "2026-11-09T12:05:00"))
    ).toBe("12:05 PM");
  });

  it("does not throw on an unreadable time; the label is only a hint", () => {
    const snapshot = buildRecoverySnapshot(draft, "not-a-date");
    expect(recoveryLabel(snapshot)).toBe("earlier");
  });
});
