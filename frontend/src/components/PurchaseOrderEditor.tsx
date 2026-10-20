import { useEffect, useReducer } from "react";
import { UpdatePurchaseOrderDraft } from "@backend/backend/Quotation";
import { backend } from "@backend/models";
import type { CompanyUnitOption } from "../companyUnits";
import type { CurrencyInfo } from "../format";
import {
  buildPurchaseOrderUpdate,
  initialPurchaseOrderEditorState,
  isPurchaseOrderDirty,
  purchaseOrderEditorReducer,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorAction,
  type PurchaseOrderEditorState,
} from "../purchaseOrder";
import { Select } from "./Select";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";

// Issue #468: the PO draft workspace. The header keeps the customer snapshot
// read-only (a directory edit must not rewrite this order) while the agreed
// commercial details stay editable; the lines are the customer's order, with
// quantity as a 4-decimal string and money in the PO's own currency.
//
// No totals, differences, PDF or confirmation number render here: those are
// #470/#471/#472/#474. A draft is deliberately unfinished, so Save draft only
// stores what was entered.

export interface PurchaseOrderEditorViewProps {
  state: PurchaseOrderEditorState;
  dispatch: (action: PurchaseOrderEditorAction) => void;
  currency: CurrencyInfo;
  uomOptions: CompanyUnitOption[];
  incotermOptions: { value: string; label: string }[];
  onSave: () => void;
  onCancel: () => void;
  onDiscard: () => void;
}

export interface PurchaseOrderEditorPageProps {
  order: PurchaseOrderDTO;
  currency: CurrencyInfo;
  uomOptions: CompanyUnitOption[];
  incotermOptions: { value: string; label: string }[];
  onSaved: (order: PurchaseOrderDTO) => void;
  onCancel: () => void;
  /**
   * Reports whether the draft holds unsaved changes, so the page that owns
   * navigation (App) can ask before unmounting this editor. The editor stays
   * the only place that decides dirtiness; App never re-derives it.
   */
  onDirtyChange?: (dirty: boolean) => void;
}

/**
 * The one discard confirmation used by both discard paths: the editor's own
 * Cancel panel and App's tab-navigation prompt. Keeping the copy in one place
 * means a staff member reads the same question wherever the leave happens.
 */
export function PurchaseOrderDiscardPrompt({
  onKeepEditing,
  onDiscard,
  overlay = false,
}: {
  onKeepEditing: () => void;
  onDiscard: () => void;
  overlay?: boolean;
}) {
  const panel = (
    <Card className="border-danger-200 p-4">
      <div role="alertdialog" aria-label="Discard unsaved changes?">
        <p className="text-lead font-medium text-ink-900">
          Discard unsaved changes?
        </p>
        <p className="mt-1 text-small text-ink-500">
          The draft was saved earlier. Changes since then will be lost.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button aria-label="Keep editing" onClick={onKeepEditing}>
            Keep editing
          </Button>
          <Button variant="danger" aria-label="Discard changes" onClick={onDiscard}>
            Discard changes
          </Button>
        </div>
      </div>
    </Card>
  );
  if (!overlay) return panel;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/45 p-5"
      role="presentation"
    >
      <div className="w-full max-w-md">{panel}</div>
    </div>
  );
}

function errorText(error: unknown): string {
  if (typeof error === "string" && error.trim() !== "") return error;
  if (error instanceof Error && error.message.trim() !== "") return error.message;
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message.trim() !== "") return message;
  }
  return "The purchase order could not be saved. Try again.";
}

function matchLabel(
  quotationItemNo: number | null,
  quotationSubNo: number | null
): string {
  if (quotationItemNo === null) return "Extra line (no quotation match)";
  if (quotationSubNo === null) return `Quotation item ${quotationItemNo}`;
  return `Quotation item ${quotationItemNo}.${quotationSubNo}`;
}

function Field({
  label,
  value,
  onChange,
  type = "text",
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: "text" | "date" | "email" | "number";
  placeholder?: string;
}) {
  return (
    <label className="grid gap-1.5">
      <span className="text-small font-medium text-ink-700">{label}</span>
      <input
        type={type}
        aria-label={label}
        className={fieldClass}
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function TextAreaField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="grid gap-1.5">
      <span className="text-small font-medium text-ink-700">{label}</span>
      <textarea
        aria-label={label}
        className={`${fieldClass} min-h-20 resize-y`}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function LineRow({
  line,
  index,
  currency,
  uomOptions,
  dispatch,
}: {
  line: PurchaseOrderEditorState["lines"][number];
  index: number;
  currency: CurrencyInfo;
  uomOptions: CompanyUnitOption[];
  dispatch: (action: PurchaseOrderEditorAction) => void;
}) {
  return (
    <div
      className="grid gap-3 rounded-lg border border-rule bg-white p-3 sm:grid-cols-[minmax(0,1fr)_120px_140px_170px_auto] sm:items-end"
      data-po-line={index + 1}
    >
      <label className="grid gap-1.5">
        <span className="text-small font-medium text-ink-700">
          Description
        </span>
        <input
          aria-label={`Line ${index + 1} description`}
          className={fieldClass}
          value={line.description}
          onChange={(event) =>
            dispatch({
              type: "lineChanged",
              key: line.key,
              patch: { description: event.target.value },
            })
          }
        />
        <span className="text-caption text-ink-400">
          {matchLabel(line.quotationItemNo, line.quotationSubNo)}
        </span>
      </label>
      <label className="grid gap-1.5">
        <span className="text-small font-medium text-ink-700">Quantity</span>
        <input
          inputMode="decimal"
          aria-label={`Line ${index + 1} quantity`}
          className={fieldClass}
          value={line.quantity}
          onChange={(event) =>
            dispatch({
              type: "lineChanged",
              key: line.key,
              patch: { quantity: event.target.value },
            })
          }
        />
      </label>
      <div className="grid gap-1.5">
        <span className="text-small font-medium text-ink-700">UoM</span>
        <Select
          ariaLabel={`Line ${index + 1} unit of measure`}
          value={line.uom}
          options={uomOptions.length > 0 ? uomOptions : [{ value: line.uom, label: line.uom || "—" }]}
          onChange={(value) =>
            dispatch({
              type: "lineChanged",
              key: line.key,
              patch: { uom: String(value) },
            })
          }
        />
      </div>
      <label className="grid gap-1.5">
        <span className="text-small font-medium text-ink-700">
          Unit price ({currency.code})
        </span>
        <input
          inputMode="decimal"
          aria-label={`Line ${index + 1} unit price`}
          className={fieldClass}
          value={line.unitPrice}
          onChange={(event) =>
            dispatch({
              type: "lineChanged",
              key: line.key,
              patch: { unitPrice: event.target.value },
            })
          }
        />
      </label>
      <Button
        size="sm"
        aria-label={`Remove line ${index + 1}`}
        onClick={() => dispatch({ type: "lineRemoved", key: line.key })}
      >
        Remove
      </Button>
    </div>
  );
}

export function PurchaseOrderEditorView({
  state,
  dispatch,
  currency,
  uomOptions,
  incotermOptions,
  onSave,
  onCancel,
  onDiscard,
}: PurchaseOrderEditorViewProps) {
  const dirty = isPurchaseOrderDirty(state);
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading title="Purchase Order" hint={`Draft ${state.orderId}`} />
          <h1 className="tnum text-display font-semibold text-ink-950">
            {state.quoteNumber}
          </h1>
          <p className="mt-1 text-body text-ink-500">
            {state.customerName} · revision {state.sourceRevision} ·{" "}
            {state.currency.toUpperCase()}
            {state.taxZone !== ""
              ? ` · ${state.taxZone}${
                  state.taxCategory !== "" ? ` / ${state.taxCategory}` : ""
                }`
              : ""}
          </p>
        </div>
        <Badge tone="accent">Draft</Badge>
      </div>

      {state.confirmDiscard ? (
        <PurchaseOrderDiscardPrompt
          onKeepEditing={() => dispatch({ type: "cancelDismissed" })}
          onDiscard={onDiscard}
        />
      ) : null}

      <Card className="p-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <TextAreaField
            label="Billing address"
            value={state.header.billingAddress}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { billingAddress: value } })
            }
          />
          <TextAreaField
            label="Delivery address"
            value={state.header.deliveryAddress}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { deliveryAddress: value } })
            }
          />
          <Field
            label="Ship-to code"
            value={state.header.shipToCode}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { shipToCode: value } })
            }
          />
          <Field
            label="Contact email"
            type="email"
            value={state.header.contactEmail}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { contactEmail: value } })
            }
          />
          <Field
            label="Contact phone"
            value={state.header.contactPhone}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { contactPhone: value } })
            }
          />
          <Field
            label="Customer PO number"
            value={state.header.poNumber}
            placeholder="Optional"
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { poNumber: value } })
            }
          />
          <Field
            label="PO date"
            type="date"
            value={state.header.poDate}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { poDate: value } })
            }
          />
          <Field
            label="Received date"
            type="date"
            value={state.header.receivedDate}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { receivedDate: value } })
            }
          />
          <Field
            label="Requested delivery date"
            type="date"
            value={state.header.requestedDeliveryDate}
            onChange={(value) =>
              dispatch({
                type: "headerChanged",
                patch: { requestedDeliveryDate: value },
              })
            }
          />
          <Field
            label="Payment terms"
            value={state.header.paymentTerms}
            placeholder="e.g. 50% advance, balance 30 days after delivery"
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { paymentTerms: value } })
            }
          />
          <div className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">Incoterms</span>
            <Select
              ariaLabel="Incoterms"
              value={state.header.incoterms}
              options={incotermOptions}
              onChange={(value) =>
                dispatch({ type: "headerChanged", patch: { incoterms: String(value) } })
              }
            />
          </div>
        </div>
        <p className="mt-4 text-caption text-ink-500">
          Customer, currency and tax basis are snapshots of the issued
          quotation revision and cannot be edited here.
        </p>
      </Card>

      <div>
        <SectionHeading
          title="Lines"
          hint={`${state.lines.length} ${state.lines.length === 1 ? "line" : "lines"}`}
          action={
            <Button
              size="sm"
              aria-label="Add line"
              onClick={() =>
                dispatch({ type: "lineAdded", uom: uomOptions[0]?.value ?? "" })
              }
            >
              Add line
            </Button>
          }
        />
        <div className="grid gap-3">
          {state.lines.length === 0 ? (
            <p className="rounded-lg border border-dashed border-rule px-4 py-8 text-center text-small text-ink-500">
              No lines yet. Add the customer's first line.
            </p>
          ) : (
            state.lines.map((line, index) => (
              <LineRow
                key={line.key}
                line={line}
                index={index}
                currency={currency}
                uomOptions={uomOptions}
                dispatch={dispatch}
              />
            ))
          )}
        </div>
      </div>

      {state.error !== null ? (
        <StatusBanner tone="danger">{state.error}</StatusBanner>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
        <span className="text-caption text-ink-500">
          {dirty ? "Unsaved changes" : "All changes saved"}
        </span>
        <div className="flex flex-wrap gap-2">
          <Button aria-label="Cancel draft" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            variant="primary"
            aria-label="Save draft"
            disabled={state.saving}
            onClick={onSave}
          >
            {state.saving ? "Saving…" : "Save draft"}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function PurchaseOrderEditorPage({
  order,
  currency,
  uomOptions,
  incotermOptions,
  onSaved,
  onCancel,
  onDirtyChange,
}: PurchaseOrderEditorPageProps) {
  const [state, dispatch] = useReducer(
    purchaseOrderEditorReducer,
    undefined,
    () => initialPurchaseOrderEditorState(order, currency)
  );

  // The dirty flag is lifted, not duplicated: the editor still owns the
  // baseline comparison, and App only consumes the answer so it can ask
  // before a tab navigation unmounts this component's reducer state.
  useEffect(() => {
    onDirtyChange?.(isPurchaseOrderDirty(state));
  }, [state, onDirtyChange]);

  const save = async () => {
    const result = buildPurchaseOrderUpdate(state, currency);
    if (result.input === null) {
      dispatch({ type: "saveFailed", error: result.error ?? "The draft is incomplete." });
      return;
    }
    dispatch({ type: "saveStarted" });
    try {
      const updated = await UpdatePurchaseOrderDraft(
        new backend.PurchaseOrderUpdateInput({
          ...result.input,
          Items: result.input.Items.map(
            (item) => new backend.PurchaseOrderItemInput(item)
          ),
        })
      );
      dispatch({ type: "saveSucceeded", order: updated, currency });
      onSaved(updated);
    } catch (error) {
      dispatch({ type: "saveFailed", error: errorText(error) });
    }
  };

  const requestCancel = () => {
    if (isPurchaseOrderDirty(state)) {
      dispatch({ type: "cancelRequested" });
      return;
    }
    onCancel();
  };

  return (
    <PurchaseOrderEditorView
      state={state}
      dispatch={dispatch}
      currency={currency}
      uomOptions={uomOptions}
      incotermOptions={incotermOptions}
      onSave={() => void save()}
      onCancel={requestCancel}
      onDiscard={onCancel}
    />
  );
}
