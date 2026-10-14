/**
 * The wording half of the quotation template (issue #339).
 *
 * The served shape is the contract: GET /v1/company/labels answers one entry
 * per editable label, in document order, carrying the effective value, the
 * built-in default, the store's own length cap and whether the value is a
 * terms sentence. The client hard-codes none of the document's wording — a
 * field is named after its served default and prefilled from its served value,
 * so a new label on the server is a new field here without a release.
 *
 * What the client owns is the chrome around that data: the grouping the editor
 * shows, the edit-time mirror of the store's plain-text rules (so a save the
 * server would refuse is never offered), and the full-replacement payload the
 * save sends. Those pieces live here as pure functions so the panel and both
 * test files can share one implementation, the way companyUnits.ts does for
 * the unit list.
 *
 * The save itself — which endpoint, in what order, and the identity refresh
 * that follows it — lives in companySaves.ts (#432). It used to live here as
 * saveCompanyLabels; the wording stopped autosaving, and the sequence became
 * one of four per-tab sequences rather than the wording's private concern.
 */

/**
 * One served label. Structural on purpose (like CompanyUnitOption): tests
 * pass literals, and the generated backend.CompanyLabelEntry satisfies it.
 */
export interface CompanyLabelEntry {
  /** The stable name stored in company_labels, e.g. "metaQuotationNo". */
  Key: string;
  /** What the document prints today: the override, or the built-in default. */
  Value: string;
  /** The built-in wording — the field's name and its placeholder. */
  Default: string;
  /** The store's cap on an override, in bytes. */
  MaxLength: number;
  /** True for the four terms sentences, which may carry placeholder tokens. */
  Terms: boolean;
}

/**
 * The groups the editor shows, in display order. `keys` is the inclusion rule:
 * a key on no list is not dropped, it lands in the fallback group below, so a
 * label a future release adds is visible rather than silently uneditable.
 * Titles and hints are settings chrome, not document wording.
 */
export const LABEL_GROUPS: {
  id: string;
  title: string;
  hint: string;
  keys: string[];
}[] = [
  {
    id: "document",
    title: "Document",
    hint: "The title, the headings, the meta strip, the subject and the closing sentence.",
    keys: [
      "docTitle",
      "recipientHeading",
      "termsHeading",
      "metaQuotationNo",
      "metaIssueDate",
      "metaValidUntil",
      "metaRevision",
      "subject",
      "closing",
    ],
  },
  {
    id: "columns",
    title: "Table columns",
    hint: "The seven column headers of the item table.",
    keys: [
      "columnNo",
      "columnDescription",
      "columnQty",
      "columnUoM",
      "columnUnitPrice",
      "columnAfterDisc",
      "columnTotal",
    ],
  },
  {
    id: "totals",
    title: "Totals",
    hint: "The four labels down the totals column.",
    keys: ["totalSubTotal", "totalDiscount", "totalPpn", "totalGrandTotal"],
  },
  {
    id: "terms",
    title: "Terms",
    hint: "The numbered sentences under the terms heading. Each fills its figures from the quotation.",
    keys: ["termsPayment", "termsValidity", "termsLeadTime", "termsIncoterm"],
  },
  {
    id: "verification",
    title: "Verification",
    hint: "The two captions beside the verification code.",
    keys: ["verifyScan", "verifyCode"],
  },
];

/** The group a key the client does not know lands in, rather than vanish. */
const FALLBACK_GROUP = {
  id: "other",
  title: "Other",
  hint: "Labels this version of the app does not group yet.",
};

export interface CompanyLabelGroup {
  id: string;
  title: string;
  hint: string;
  entries: CompanyLabelEntry[];
}

/**
 * Buckets the served entries into the editor's groups, preserving the served
 * order inside each group and the group order above across them. A key on no
 * list is appended to the fallback group in served order; an empty group is
 * dropped rather than printed as a heading with nothing under it.
 */
export function groupCompanyLabels(entries: CompanyLabelEntry[]): CompanyLabelGroup[] {
  const byGroup = new Map<string, CompanyLabelEntry[]>();
  const unknown: CompanyLabelEntry[] = [];
  for (const group of LABEL_GROUPS) byGroup.set(group.id, []);
  for (const entry of entries) {
    const owner = LABEL_GROUPS.find((group) => group.keys.includes(entry.Key));
    if (owner) byGroup.get(owner.id)?.push(entry);
    else unknown.push(entry);
  }
  const groups: CompanyLabelGroup[] = [];
  for (const group of LABEL_GROUPS) {
    const owned = byGroup.get(group.id) ?? [];
    if (owned.length === 0) continue;
    groups.push({ id: group.id, title: group.title, hint: group.hint, entries: owned });
  }
  if (unknown.length > 0) {
    groups.push({ ...FALLBACK_GROUP, entries: unknown });
  }
  return groups;
}

/** One entry's value replaced. An unknown key leaves the list untouched. */
export function patchCompanyLabel(
  entries: CompanyLabelEntry[],
  key: string,
  value: string
): CompanyLabelEntry[] {
  return entries.map((entry) => (entry.Key === key ? { ...entry, Value: value } : entry));
}

/** The full-replacement payload: every key, its value, empties included —
 *  an empty value is how a label is cleared back to its built-in default. */
export function companyLabelPayload(
  entries: CompanyLabelEntry[]
): { Key: string; Value: string }[] {
  return entries.map((entry) => ({ Key: entry.Key, Value: entry.Value }));
}

/**
 * A stable fingerprint of what the server holds, used to tell "loaded" from
 * "edited" (and an edited value from a superseded one): equal fingerprints
 * mean there is nothing to save.
 */
export function serializeCompanyLabels(entries: CompanyLabelEntry[]): string {
  return JSON.stringify(companyLabelPayload(entries));
}

/**
 * The placeholder tokens a terms sentence may carry, taken from the served
 * default rather than hard-coded: the tokens are a protocol between the label
 * set and both renderers, and reading them off the shipped sentence means a
 * token added server-side arrives without a client change.
 */
export function placeholderTokens(entry: CompanyLabelEntry): string[] {
  const found: string[] = entry.Default.match(/\{[^}]*\}/g) ?? [];
  return found.filter((token, index) => found.indexOf(token) === index);
}

/**
 * The closed token set the store allows: the union over the served terms
 * defaults. Every allowed token appears in some built-in sentence, so the
 * union is the whole set — and a sentence combining two of them is accepted
 * here exactly as the store accepts it.
 */
function allowedPlaceholderTokens(entries: CompanyLabelEntry[]): string[] {
  const tokens: string[] = [];
  for (const entry of entries) {
    if (!entry.Terms) continue;
    for (const token of placeholderTokens(entry)) {
      if (!tokens.includes(token)) tokens.push(token);
    }
  }
  return tokens;
}

/** The store compares lengths in bytes, so the mirror does too. */
function byteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

// The messages mirror backend/labels.ValidateOverride word for word, the way
// validateCompanyUnits mirrors the units store: a refusal the UI raises must
// read the same as the one it prevented.
function checkValue(entry: CompanyLabelEntry, value: string, allowed: string[]): string | null {
  if (byteLength(value) > entry.MaxLength) {
    return `label "${entry.Key}" is over ${entry.MaxLength} characters`;
  }
  if (value.includes("<") || value.includes(">")) {
    return `label "${entry.Key}" may not contain "<" or ">": it is printed as plain text`;
  }
  if (value.includes("{{")) {
    return `label "${entry.Key}" may not contain "{{": it is printed as plain text`;
  }
  for (const char of value) {
    const code = char.codePointAt(0) ?? 0;
    if (code < 0x20 || code === 0x7f) {
      return `label "${entry.Key}" may not contain control characters: it prints on one line`;
    }
  }
  if (!entry.Terms) return null;
  return checkPlaceholders(entry.Key, value, allowed);
}

function checkPlaceholders(key: string, value: string, allowed: string[]): string | null {
  const kept: string[] = [];
  let index = 0;
  while (index < value.length) {
    const char = value[index];
    if (char !== "{") {
      kept.push(char);
      index += 1;
      continue;
    }
    const end = value.indexOf("}", index);
    if (end < 0) {
      return `label "${key}" has an unclosed placeholder: use one of ${allowed.join(", ")}`;
    }
    const token = value.slice(index, end + 1);
    if (!allowed.includes(token)) {
      return `label "${key}" uses the unknown placeholder ${token}: use one of ${allowed.join(", ")}`;
    }
    index = end + 1;
  }
  if (kept.some((char) => char === "{" || char === "}")) {
    return `label "${key}" has a stray "{" or "}": use one of ${allowed.join(", ")}`;
  }
  return null;
}

/**
 * The edit-time mirror of the store's whole-set validation, keyed so the panel
 * can mark the offending row. Only the plain-text rules are mirrored: the
 * duplicate-key rule the store also carries cannot be reached through this UI,
 * which edits one entry per key and never invents one.
 */
export function validateCompanyLabelSet(entries: CompanyLabelEntry[]): Record<string, string> {
  const allowed = allowedPlaceholderTokens(entries);
  const invalid: Record<string, string> = {};
  for (const entry of entries) {
    const message = checkValue(entry, entry.Value, allowed);
    if (message !== null) invalid[entry.Key] = message;
  }
  return invalid;
}

/**
 * Why the wording may not be saved, or null when it may.
 *
 * The store's own rules are mirrored here (validateCompanyLabelSet) so the save
 * row can refuse a set the server would bounce — the wording saves manually
 * now (#432), so the button is where that refusal has to surface. The sentence
 * matches the one the panel shows under its heading, because two different
 * sentences describing the same refusal reads as two different problems.
 */
export function companyLabelsRefusal(entries: CompanyLabelEntry[]): string | null {
  const invalidCount = Object.keys(validateCompanyLabelSet(entries)).length;
  if (invalidCount === 0) return null;
  return invalidCount === 1
    ? "One field is highlighted below and must be fixed before anything saves."
    : `${invalidCount} fields are highlighted below and must be fixed before anything saves.`;
}

