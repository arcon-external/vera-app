import type { CurrencyInfo } from "../format";
import {
  APPLICABLE_HEADER_KEYS,
  EXTRACTION_FIELD_LABELS,
  EXTRACTION_FIELD_ORDER,
  EXTRACTION_LINES_KEY,
  EXTRACTION_MIN_CONFIDENCE,
  EXTRACTION_STATUS_LABELS,
  extractionAbstentionSummary,
  extractionCellConfidenceLabel,
  extractionEngineSummary,
  extractionEvidenceLabel,
  extractionFoundNoText,
  extractionLowConfidenceCellLabels,
  extractionPageModeSummary,
  extractionRegion,
  extractionUsesOCR,
  headerProposalApplicable,
  proposalCellKey,
  rowProposalApplicable,
  type ExtractionChoice,
  type ExtractionConflictPlan,
  type ExtractionFieldComparison,
  type ExtractionFieldKey,
  type ExtractionStatus,
  type PurchaseOrderExtractionCellDTO,
  type PurchaseOrderExtractionFieldDTO,
  type PurchaseOrderExtractionPageDTO,
  type PurchaseOrderExtractionResultDTO,
  type PurchaseOrderExtractionRowDTO,
} from "../poExtraction";
import type {
  PurchaseOrderHeader,
  PurchaseOrderLine,
} from "../purchaseOrder";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

// Issue #477: the extraction review.
//
// This is evidence, not a form. Every proposal shows its status and its page
// region, an absent value is visibly absent, and a `needs_review` value is
// never preselected. Applying is one explicit action for the ticked fields and
// rows; choosing a row replaces the line set, which is stated before the
// action. A scanned page is reported as an abstention, never OCRed here.

export interface PurchaseOrderExtractionPanelViewProps {
  result: PurchaseOrderExtractionResultDTO;
  header: PurchaseOrderHeader;
  lines: PurchaseOrderLine[];
  currency: CurrencyInfo;
  selection: Record<string, boolean>;
  onToggle: (key: string, selected: boolean) => void;
  onApply: () => void;
  onDiscard: () => void;
  applied?: { fields: number; rows: number } | null;
  /**
   * Issue #479: the resolved comparison. When present, a field or the line set
   * whose proposal conflicts shows an explicit choice instead of a checkbox.
   */
  plan?: ExtractionConflictPlan | null;
  /** The staff member's side per conflict key; absent means manual. */
  choices?: Record<string, ExtractionChoice>;
  onChoose?: (key: string, choice: ExtractionChoice) => void;
}

function statusTone(status: string): "success" | "internal" | "neutral" {
  if (status === "extracted") return "success";
  if (status === "needs_review") return "internal";
  return "neutral";
}

function statusLabel(status: string): string {
  return EXTRACTION_STATUS_LABELS[status as ExtractionStatus] ?? status;
}

function currentHeaderValue(header: PurchaseOrderHeader, key: ExtractionFieldKey): string {
  switch (key) {
    case "poNumber":
      return header.poNumber;
    case "poDate":
      return header.poDate;
    case "requestedDeliveryDate":
      return header.requestedDeliveryDate;
    case "paymentTerms":
      return header.paymentTerms;
    case "incoterms":
      return header.incoterms;
    default:
      return "";
  }
}

function PageMap({
  cell,
  pages,
}: {
  cell: PurchaseOrderExtractionCellDTO;
  pages: PurchaseOrderExtractionPageDTO[];
}) {
  const page = pages.find((entry) => entry.Number === cell.Page) ?? pages[0];
  const region = page ? extractionRegion(cell, page) : null;
  return (
    <span
      className="relative inline-block h-16 w-12 shrink-0 rounded border border-rule bg-paper-subtle"
      aria-hidden="true"
      data-testid="po-extraction-region"
    >
      {region !== null ? (
        <span
          className="absolute rounded-[1px] border border-accent-500 bg-accent-500/15"
          style={{
            left: `${region.left}%`,
            top: `${region.top}%`,
            width: `${Math.max(region.width, 2)}%`,
            height: `${Math.max(region.height, 2)}%`,
          }}
        />
      ) : null}
    </span>
  );
}

function FieldProposal({
  field,
  header,
  pages,
  currency,
  selection,
  onToggle,
  comparison,
  choice,
  onChoose,
}: {
  field: PurchaseOrderExtractionFieldDTO;
  header: PurchaseOrderHeader;
  pages: PurchaseOrderExtractionPageDTO[];
  currency: CurrencyInfo;
  selection: Record<string, boolean>;
  onToggle: (key: string, selected: boolean) => void;
  comparison: ExtractionFieldComparison | null;
  choice: ExtractionChoice;
  onChoose?: (key: string, choice: ExtractionChoice) => void;
}) {
  const key = field.Key as ExtractionFieldKey;
  const label = EXTRACTION_FIELD_LABELS[key] ?? field.Key;
  const applicable = headerProposalApplicable(field, currency);
  const protectedField = !APPLICABLE_HEADER_KEYS.includes(key);
  const selected = selection[proposalCellKey("header", field.Key)] === true;
  const current = currentHeaderValue(header, key).trim();
  const differs = current !== "" && current !== field.Cell.Value.trim();
  const conflict = comparison?.outcome === "conflict";
  return (
    <li
      className="grid gap-2 rounded-lg border border-rule bg-paper-subtle/40 p-3 sm:grid-cols-[auto_1fr_auto] sm:items-start"
      data-testid={`po-extraction-field-${field.Key}`}
    >
      <div className="flex items-start gap-2">
        {!conflict ? (
          <input
            type="checkbox"
            className="mt-1 h-4 w-4 accent-[var(--color-accent-600)]"
            aria-label={`Apply extracted ${label}`}
            checked={selected}
            disabled={!applicable}
            onChange={(event) => onToggle(proposalCellKey("header", field.Key), event.target.checked)}
          />
        ) : null}
        {field.Cell.Status !== "absent" ? <PageMap cell={field.Cell} pages={pages} /> : null}
      </div>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-small font-medium text-ink-900">{label}</span>
          <Badge tone={statusTone(field.Cell.Status)}>{statusLabel(field.Cell.Status)}</Badge>
        </div>
        <p className="mt-1 break-words text-body text-ink-900">
          {field.Cell.Status === "absent"
            ? "Not printed on this page."
            : field.Cell.Value || field.Cell.Raw || "—"}
        </p>
        {field.Cell.Raw !== "" && field.Cell.Raw !== field.Cell.Value ? (
          <p className="text-caption text-ink-500">Printed as “{field.Cell.Raw}”</p>
        ) : null}
        {protectedField ? (
          <p className="mt-1 text-caption text-ink-500">
            Protected by the Quotation; compare it here and correct the quotation link instead.
          </p>
        ) : null}
        {conflict && comparison ? (
          <fieldset
            className="mt-2 grid gap-1.5 rounded-md border border-rule bg-white p-2"
            data-testid={`po-extraction-conflict-${field.Key}`}
          >
            <legend className="px-1 text-caption text-ink-600">
              {comparison.touched
                ? "You changed this while extraction ran. Choose the value to use."
                : "The draft and the PDF differ. Choose the value to use."}
            </legend>
            <label className="flex items-start gap-2 text-small text-ink-900">
              <input
                type="radio"
                className="mt-1 accent-[var(--color-accent-600)]"
                name={`extraction-conflict-${field.Key}`}
                aria-label={`Keep draft ${label}`}
                checked={choice !== "extracted"}
                onChange={() => onChoose?.(comparison.key, "manual")}
              />
              <span>
                {current === "" ? "Keep it empty" : <>Keep draft “{current}”</>}
              </span>
            </label>
            <label className="flex items-start gap-2 text-small text-ink-900">
              <input
                type="radio"
                className="mt-1 accent-[var(--color-accent-600)]"
                name={`extraction-conflict-${field.Key}`}
                aria-label={`Use extracted ${label}`}
                checked={choice === "extracted"}
                onChange={() => onChoose?.(comparison.key, "extracted")}
              />
              <span>Use extracted “{comparison.extracted}”</span>
            </label>
          </fieldset>
        ) : null}
        {differs && applicable && !conflict ? (
          <p className="mt-1 text-caption text-ink-600">
            The draft currently holds “{current}”. Applying replaces it.
          </p>
        ) : null}
        {field.Cell.Note ? (
          <p className="mt-1 text-caption text-ink-600">{field.Cell.Note}</p>
        ) : null}
        {field.Cell.Confidence != null ? (
          <p className="mt-1 text-caption text-ink-500" data-testid={`po-extraction-confidence-${field.Key}`}>
            {extractionCellConfidenceLabel(field.Cell)}
          </p>
        ) : null}
      </div>
      <span className="tnum whitespace-nowrap text-caption text-ink-500 sm:text-right">
        {extractionEvidenceLabel(field.Cell)}
      </span>
    </li>
  );
}

function rowValue(cell: PurchaseOrderExtractionCellDTO, suffix = ""): string {
  if (cell.Status === "absent" || cell.Value.trim() === "") return "—";
  return `${cell.Value}${suffix}`;
}

function lowConfidence(cell: PurchaseOrderExtractionCellDTO): boolean {
  return cell.Confidence != null && cell.Confidence < EXTRACTION_MIN_CONFIDENCE;
}

export function PurchaseOrderExtractionRowCard({
  row,
  index,
  currency,
  selection,
  onToggle,
  locked = false,
}: {
  row: PurchaseOrderExtractionRowDTO;
  index: number;
  currency: CurrencyInfo;
  selection: Record<string, boolean>;
  onToggle: (key: string, selected: boolean) => void;
  /** Issue #479: a line conflict resolved as Keep my lines locks row apply. */
  locked?: boolean;
}) {
  const applicable = rowProposalApplicable(row, currency);
  const selected = selection[proposalCellKey("row", String(index))] === true;
  return (
    <li
      className="grid gap-2 rounded-lg border border-rule bg-white p-3"
      data-testid={`po-extraction-row-${index}`}
    >
      <div className="flex flex-wrap items-start gap-2">
        <input
          type="checkbox"
          className="mt-1 h-4 w-4 accent-[var(--color-accent-600)]"
          aria-label={`Apply extracted line ${row.LineNumber.Value || index + 1}`}
          checked={selected}
          disabled={!applicable || locked}
          onChange={(event) => onToggle(proposalCellKey("row", String(index)), event.target.checked)}
        />
        <span className="tnum text-caption font-medium text-ink-500">
          Line {row.LineNumber.Value || index + 1}
        </span>
        <Badge tone={statusTone(row.Status)}>{statusLabel(row.Status)}</Badge>
        <span className="tnum ml-auto whitespace-nowrap text-caption text-ink-500">
          Page {row.Page}
        </span>
      </div>
      <p className="break-words text-body text-ink-900">{row.Description.Value || "—"}</p>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-caption sm:grid-cols-4">
        <div>
          <dt className="text-ink-500">Quantity</dt>
          <dd className="tnum whitespace-nowrap text-ink-900">
            {rowValue(row.Quantity)}
            {lowConfidence(row.Quantity) ? (
              <span className="block text-caption text-ink-500">
                {extractionCellConfidenceLabel(row.Quantity)}
              </span>
            ) : null}
          </dd>
        </div>
        <div>
          <dt className="text-ink-500">UoM</dt>
          <dd className="text-ink-900">
            {rowValue(row.UoM)}
            {lowConfidence(row.UoM) ? (
              <span className="block text-caption text-ink-500">
                {extractionCellConfidenceLabel(row.UoM)}
              </span>
            ) : null}
          </dd>
        </div>
        <div>
          <dt className="text-ink-500">Unit price ({currency.code.toUpperCase()})</dt>
          <dd className="tnum whitespace-nowrap text-ink-900">
            {rowValue(row.UnitPrice)}
            {lowConfidence(row.UnitPrice) ? (
              <span className="block text-caption text-ink-500">
                {extractionCellConfidenceLabel(row.UnitPrice)}
              </span>
            ) : null}
          </dd>
        </div>
        <div>
          <dt className="text-ink-500">Amount ({currency.code.toUpperCase()})</dt>
          <dd className="tnum whitespace-nowrap text-ink-900">
            {rowValue(row.Amount)}
            {lowConfidence(row.Amount) ? (
              <span className="block text-caption text-ink-500">
                {extractionCellConfidenceLabel(row.Amount)}
              </span>
            ) : null}
          </dd>
        </div>
      </dl>
      {!applicable ? (
        <p className="text-caption text-ink-600">
          This line cannot be applied exactly: a required value is missing or not a
          number the draft can hold. Correct it by hand.
        </p>
      ) : null}
      {extractionLowConfidenceCellLabels(row) !== "" ? (
        <p className="text-caption text-ink-600" data-testid={`po-extraction-row-confidence-${index}`}>
          Low OCR confidence: {extractionLowConfidenceCellLabels(row)}.
        </p>
      ) : null}
      {row.Note ? <p className="text-caption text-ink-600">{row.Note}</p> : null}
    </li>
  );
}

export function PurchaseOrderExtractionPanelView({
  result,
  header,
  lines,
  currency,
  selection,
  onToggle,
  onApply,
  onDiscard,
  applied = null,
  plan = null,
  choices = {},
  onChoose,
}: PurchaseOrderExtractionPanelViewProps) {
  const fields = EXTRACTION_FIELD_ORDER.map((key) =>
    result.Fields.find((field) => field.Key === key)
  ).filter((field): field is PurchaseOrderExtractionFieldDTO => field != null);
  const lineConflict = plan?.lines?.outcome === "conflict";
  const lineChoice = choices[EXTRACTION_LINES_KEY] ?? "manual";
  const rowsLocked = lineConflict && lineChoice !== "extracted";
  const comparisonFor = (fieldKey: string) =>
    plan?.fields.find((field) => field.key === proposalCellKey("header", fieldKey)) ?? null;
  const selectedFields = fields.filter(
    (field) =>
      selection[proposalCellKey("header", field.Key)] === true &&
      headerProposalApplicable(field, currency)
  ).length;
  const selectedRows = result.Rows.filter(
    (row, index) =>
      !rowsLocked &&
      selection[proposalCellKey("row", String(index))] === true &&
      rowProposalApplicable(row, currency)
  ).length;
  const canApply = selectedFields + selectedRows > 0;
  const replaceLinesHint = `Applying line proposals replaces the current ${lines.length} ${
    lines.length === 1 ? "line" : "lines"
  } with the ticked rows.`;
  return (
    <div data-testid="po-extraction-panel">
      <Card className="p-4">
        <SectionHeading
        title="Extracted from the customer PO PDF"
        hint={`${result.Engine} ${result.EngineVersion}`}
      />
      <p className="text-small text-ink-600">
        These proposals are evidence from the document, not confirmed facts. Tick the
        values to apply, then review the draft. Nothing changes until you press Apply.
      </p>
      <p className="mt-1 text-small text-ink-600">
        Where the draft already held a different value, choose which one this field
        uses; extraction never replaces it on its own.
      </p>
      <p className="mt-1 text-caption text-ink-500">{extractionAbstentionSummary(result)}</p>
      <p className="mt-1 text-caption text-ink-600" data-testid="po-extraction-provenance">
        {extractionEngineSummary(result)}
      </p>
      {(result.PageModes ?? []).length > 0 ? (
        <ul className="mt-1 grid gap-0.5 text-caption text-ink-500" data-testid="po-extraction-pages">
          {(result.PageModes ?? []).map((page) => (
            <li key={page.Number}>{extractionPageModeSummary(page)}</li>
          ))}
        </ul>
      ) : null}

      {extractionFoundNoText(result) ? (
        <StatusBanner tone="info">
          No readable text was found on any page. Enter the values manually.
        </StatusBanner>
      ) : null}

      {result.PagesWithoutText > 0 && !extractionFoundNoText(result) ? (
        <p className="mt-2 text-caption text-ink-600">
          {result.PagesWithoutText} of {result.PageCount} pages{" "}
          {extractionUsesOCR(result) ? (
            <>
              had no usable text layer and were read with OCR
              {result.OCREngine ? ` (${result.OCREngine} ${result.OCREngineVersion})` : ""}.
              Check every line that needs review against the page image.
            </>
          ) : (
            <>
              had no selectable text and were not read. This stored result predates
              scanned-page extraction; run Extract again to read those pages.
            </>
          )}
        </p>
      ) : null}

      <div className="mt-4 grid gap-4">
        <section aria-label="Extracted header fields">
          <h3 className="mb-2 text-small font-medium text-ink-700">Header fields</h3>
          <ul className="grid gap-2">
            {fields.map((field) => (
              <FieldProposal
                key={field.Key}
                field={field}
                header={header}
                pages={result.Pages}
                currency={currency}
                selection={selection}
                onToggle={onToggle}
                comparison={comparisonFor(field.Key)}
                choice={choices[proposalCellKey("header", field.Key)] ?? "manual"}
                onChoose={onChoose}
              />
            ))}
          </ul>
        </section>

        <section aria-label="Extracted lines">
          <div className="mb-2 flex flex-wrap items-baseline gap-2">
            <h3 className="text-small font-medium text-ink-700">Lines</h3>
            <span className="text-caption text-ink-500">
              {rowsLocked
                ? "Your line changes are kept while this choice stands."
                : replaceLinesHint}
            </span>
          </div>
          {lineConflict && plan?.lines ? (
            <fieldset
              className="mb-2 grid gap-1.5 rounded-md border border-rule bg-paper-subtle/60 p-2"
              data-testid="po-extraction-line-conflict"
            >
              <legend className="px-1 text-caption text-ink-600">
                You changed the lines while extraction ran. Choose what the draft uses.
              </legend>
              <label className="flex items-start gap-2 text-small text-ink-900">
                <input
                  type="radio"
                  className="mt-1 accent-[var(--color-accent-600)]"
                  name="extraction-line-conflict"
                  aria-label="Keep my lines"
                  checked={lineChoice !== "extracted"}
                  onChange={() => onChoose?.(EXTRACTION_LINES_KEY, "manual")}
                />
                <span>
                  Keep my lines ({plan.lines.current})
                </span>
              </label>
              <label className="flex items-start gap-2 text-small text-ink-900">
                <input
                  type="radio"
                  className="mt-1 accent-[var(--color-accent-600)]"
                  name="extraction-line-conflict"
                  aria-label="Use extracted lines"
                  checked={lineChoice === "extracted"}
                  onChange={() => onChoose?.(EXTRACTION_LINES_KEY, "extracted")}
                />
                <span>Use extracted lines ({plan.lines.extracted})</span>
              </label>
            </fieldset>
          ) : null}
          {result.Rows.length === 0 ? (
            <p className="text-small text-ink-600">
              No line table was read. Enter the lines manually or check the page image.
            </p>
          ) : (
            <ul className="grid gap-2">
              {result.Rows.map((row, index) => (
                <PurchaseOrderExtractionRowCard
                  key={`${row.LineNumber.Value}-${index}`}
                  row={row}
                  index={index}
                  currency={currency}
                  selection={selection}
                  onToggle={onToggle}
                  locked={rowsLocked}
                />
              ))}
            </ul>
          )}
        </section>
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="primary" onClick={onApply} disabled={!canApply}>
          {selectedFields + selectedRows === 0
            ? "Apply selected"
            : `Apply selected (${selectedFields} ${selectedFields === 1 ? "field" : "fields"}${
                selectedRows > 0
                  ? ` · ${selectedRows} ${selectedRows === 1 ? "line" : "lines"}`
                  : ""
              })`}
        </Button>
        <Button onClick={onDiscard}>Discard proposals</Button>
        {applied !== null ? (
          <span className="text-caption text-success-700" role="status">
            Applied {applied.fields} {applied.fields === 1 ? "field" : "fields"}
            {applied.rows > 0
              ? ` and ${applied.rows} ${applied.rows === 1 ? "line" : "lines"}`
              : ""}{" "}
            to the draft. Save draft to keep them.
          </span>
        ) : null}
      </div>
      </Card>
    </div>
  );
}
