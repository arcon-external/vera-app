import { useEffect, useRef, useState } from "react";
import {
  AttachPurchaseOrderDocument,
  GetActivePurchaseOrderDocumentContent,
  GetPurchaseOrderDocumentState,
  RemovePurchaseOrderDocument,
} from "@backend/backend/Quotation";
import { backend } from "@backend/models";
import {
  base64ToBytes,
  bytesToBase64,
  documentErrorMessage,
  documentVersionLabel,
  formatDocumentSize,
  MAX_PURCHASE_ORDER_DOCUMENT_BYTES,
  NO_DOCUMENT_TEXT,
  type PurchaseOrderDocumentContentDTO,
  type PurchaseOrderDocumentDTO,
  type PurchaseOrderDocumentStateDTO,
} from "../purchaseOrderDocument";
import { Button, Card, SectionHeading, StatusBanner } from "./ui";

// Issue #472: the one optional customer PO PDF.
//
// The panel is deliberately not a supporting-file manager: it shows the single
// current document, its version against the retained correction history, and
// the four operations staff need. Attaching or replacing a PDF never saves the
// purchase order, so a checked draft stays exactly as entered.

export type DocumentPanelBusy = "idle" | "uploading" | "removing";

export interface PurchaseOrderDocumentPanelViewProps {
  state: PurchaseOrderDocumentStateDTO;
  busy?: DocumentPanelBusy;
  loading?: boolean;
  error?: string | null;
  /** The inline removal confirmation; no second dialog exists elsewhere. */
  pendingRemoval?: boolean;
  /** True when the saved record recorded a deliberate no-PDF final save. */
  absenceConfirmed?: boolean;
  preview?: { url: string; filename: string } | null;
  /**
   * Blocks every document control, including the hidden file input. The editor
   * sets it while a draft Save is in flight, so an upload cannot attach to a row
   * the Save is about to remount.
   */
  disabled?: boolean;
  onChooseFile: () => void;
  onView: () => void;
  onRemove: () => void;
  onConfirmRemove: () => void;
  onCancelRemove: () => void;
  onClosePreview?: () => void;
}

export function PurchaseOrderDocumentPanelView({
  state,
  busy = "idle",
  loading = false,
  error = null,
  pendingRemoval = false,
  absenceConfirmed = false,
  preview = null,
  disabled: disabledProp = false,
  onChooseFile,
  onView,
  onRemove,
  onConfirmRemove,
  onCancelRemove,
  onClosePreview,
}: PurchaseOrderDocumentPanelViewProps) {
  const active: PurchaseOrderDocumentDTO | null = state.Document ?? null;
  const disabled = busy !== "idle" || loading || disabledProp;
  const versionLabel = documentVersionLabel(state);
  const uploadedDate = active?.UploadedAt ? active.UploadedAt.slice(0, 10) : "";

  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <SectionHeading
          title="Customer PO PDF"
          hint="One original document, kept unchanged"
        />
        {active !== null ? (
          <span className="text-caption text-ink-500">{versionLabel}</span>
        ) : null}
      </div>

      {loading ? (
        <p className="mt-2 text-small text-ink-500" role="status">
          Loading the customer PO PDF…
        </p>
      ) : active === null ? (
        <div className="mt-2 grid gap-1">
          <p className="text-small text-ink-700" data-testid="po-document-none">
            {NO_DOCUMENT_TEXT}
          </p>
          {state.Versions.length > 0 ? (
            <p className="text-caption text-ink-500">
              {state.Versions.length}{" "}
              {state.Versions.length === 1 ? "earlier version is" : "earlier versions are"}{" "}
              kept in the correction history.
            </p>
          ) : null}
        </div>
      ) : (
        <div className="mt-2 grid gap-1 text-small text-ink-700">
          <p className="font-medium break-all text-ink-900">
            {active.OriginalFilename}
          </p>
          <p className="tnum whitespace-nowrap text-caption text-ink-500">
            {formatDocumentSize(active.SizeBytes)}
            {uploadedDate !== "" ? ` · uploaded ${uploadedDate}` : ""}
          </p>
        </div>
      )}

      {active === null && absenceConfirmed ? (
        <p className="mt-1 text-caption text-ink-500">
          The final save recorded this omission deliberately.
        </p>
      ) : null}

      {pendingRemoval && active !== null ? (
        <div
          role="alertdialog"
          aria-label="Remove the customer PO PDF?"
          className="mt-3 grid gap-2 rounded-lg border border-danger-200 bg-danger-50/40 p-3"
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              event.stopPropagation();
              onCancelRemove();
            }
          }}
        >
          <p className="text-small font-medium text-ink-900">
            Remove the customer PO PDF?
          </p>
          <p className="text-caption text-ink-600">
            The current file stops being attached. Earlier versions stay in the
            correction history, and no purchase order field changes.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              aria-label="Keep PDF"
              onClick={onCancelRemove}
              disabled={disabled}
              autoFocus
            >
              Keep PDF
            </Button>
            <Button
              variant="danger"
              aria-label="Remove PDF"
              onClick={onConfirmRemove}
              disabled={disabled}
            >
              {busy === "removing" ? "Removing…" : "Remove PDF"}
            </Button>
          </div>
        </div>
      ) : (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button
            aria-label={active === null ? "Attach customer PO PDF" : "Replace customer PO PDF"}
            onClick={onChooseFile}
            disabled={disabled}
          >
            {busy === "uploading"
              ? "Uploading…"
              : active === null
                ? "Attach"
                : "Replace"}
          </Button>
          {active !== null ? (
            <>
              <Button aria-label="View customer PO PDF" onClick={onView} disabled={disabled}>
                View
              </Button>
              <Button aria-label="Remove customer PO PDF" onClick={onRemove} disabled={disabled}>
                Remove
              </Button>
            </>
          ) : null}
        </div>
      )}

      {preview !== null ? (
        <div className="mt-3 grid gap-2 rounded-lg border border-rule p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-small font-medium text-ink-900">
              {preview.filename}
            </span>
            {onClosePreview ? (
              <Button size="sm" aria-label="Close PDF preview" onClick={onClosePreview}>
                Close preview
              </Button>
            ) : null}
          </div>
          <iframe
            title="Customer PO PDF preview"
            src={preview.url}
            className="h-[28rem] w-full rounded border border-rule bg-paper-subtle"
          />
        </div>
      ) : null}

      {error !== null ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
    </Card>
  );
}

export interface PurchaseOrderDocumentPanelProps {
  purchaseOrderId: number;
  /**
   * Persists a transient draft the first time an upload needs a stored owner.
   * Returns the created (or existing) id; the panel never creates a second row.
   */
  resolvePurchaseOrderId?: () => Promise<number>;
  /** Server state when the parent already has it (snapshots, saved views). */
  initialState?: PurchaseOrderDocumentStateDTO | null;
  absenceConfirmed?: boolean;
  onStateChange?: (state: PurchaseOrderDocumentStateDTO) => void;
  /**
   * Blocks every document control, including the hidden file input, and makes
   * `attach` refuse a synthetic or stale file event. The editor sets it while a
   * draft Save is in flight.
   */
  disabled?: boolean;
}

/**
 * The connected panel. It owns the file picker and the content preview; the
 * purchase order page only supplies the id. A successful attach refreshes the
 * state so history and version labels come from the server, never from a
 * guessed local increment.
 */
export function PurchaseOrderDocumentPanel({
  purchaseOrderId,
  resolvePurchaseOrderId,
  initialState = null,
  absenceConfirmed = false,
  onStateChange,
  disabled = false,
}: PurchaseOrderDocumentPanelProps) {
  const [state, setState] = useState<PurchaseOrderDocumentStateDTO>(
    initialState ?? { Document: null, Versions: [] }
  );
  const [loading, setLoading] = useState(initialState == null);
  const [busy, setBusy] = useState<DocumentPanelBusy>("idle");
  const [error, setError] = useState<string | null>(null);
  const [pendingRemoval, setPendingRemoval] = useState(false);
  const [preview, setPreview] = useState<{ url: string; filename: string } | null>(null);
  const fileInput = useRef<HTMLInputElement | null>(null);
  const previewUrl = useRef<string | null>(null);
  // Every state read carries the generation it was requested for. A read that
  // started before a replacement (or removal) must not overwrite the state the
  // later read reported, even if it lands afterwards.
  const stateGeneration = useRef(0);

  const applyState = (next: PurchaseOrderDocumentStateDTO) => {
    setState(next);
    onStateChange?.(next);
  };

  const refresh = async (id: number = purchaseOrderId) => {
    const generation = stateGeneration.current + 1;
    stateGeneration.current = generation;
    try {
      const next = await GetPurchaseOrderDocumentState(id);
      if (generation !== stateGeneration.current) return;
      applyState(next);
    } finally {
      if (generation === stateGeneration.current) setLoading(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    if (initialState != null) {
      setLoading(false);
      return;
    }
    if (purchaseOrderId < 1) {
      // A transient draft has no row to read. The first upload asks
      // resolvePurchaseOrderId to create one and then refreshes with that id.
      setLoading(false);
      return;
    }
    const generation = stateGeneration.current + 1;
    stateGeneration.current = generation;
    void (async () => {
      try {
        const next = await GetPurchaseOrderDocumentState(purchaseOrderId);
        if (!cancelled && generation === stateGeneration.current) {
          applyState(next);
        }
      } catch (loadError) {
        if (!cancelled && generation === stateGeneration.current) {
          setError(documentErrorMessage(loadError));
        }
      } finally {
        if (!cancelled && generation === stateGeneration.current) {
          setLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [purchaseOrderId, initialState]);

  useEffect(() => {
    const current = previewUrl.current;
    return () => {
      if (current !== null && typeof URL.revokeObjectURL === "function") {
        URL.revokeObjectURL(current);
      }
    };
  }, [preview]);

  // The hidden input follows the view's disabled rule, plus the parent's block.
  // The view also disables every visible control while `disabled` is true.
  const controlsDisabled = disabled || busy !== "idle" || loading;

  const chooseFile = () => {
    fileInput.current?.click();
  };

  const attach = async (file: File) => {
    if (disabled) return;
    if (file.size > MAX_PURCHASE_ORDER_DOCUMENT_BYTES) {
      setError(documentErrorMessage(new Error("the customer PO PDF is larger than 20 MB")));
      return;
    }
    setBusy("uploading");
    setError(null);
    let id = purchaseOrderId;
    if (id < 1) {
      if (resolvePurchaseOrderId === undefined) {
        setError("Save the draft before attaching a customer PO PDF.");
        setBusy("idle");
        return;
      }
      try {
        id = await resolvePurchaseOrderId();
      } catch (createError) {
        setError(documentErrorMessage(createError));
        setBusy("idle");
        return;
      }
    }
    try {
      const buffer = await file.arrayBuffer();
      await AttachPurchaseOrderDocument(
        new backend.PurchaseOrderDocumentUploadInput({
          PurchaseOrderID: id,
          DataBase64: bytesToBase64(new Uint8Array(buffer)),
          Filename: file.name,
        })
      );
      await refresh(id);
      setPendingRemoval(false);
    } catch (uploadError) {
      setError(documentErrorMessage(uploadError));
    } finally {
      setBusy("idle");
    }
  };

  const view = async () => {
    setBusy("uploading");
    setError(null);
    try {
      const content: PurchaseOrderDocumentContentDTO =
        await GetActivePurchaseOrderDocumentContent(purchaseOrderId);
      if (typeof URL.createObjectURL !== "function") {
        setError("This build cannot open a PDF preview. The document is still attached.");
        return;
      }
      const blob = new Blob([base64ToBytes(content.DataBase64)], {
        type: content.ContentType || "application/pdf",
      });
      const url = URL.createObjectURL(blob);
      if (previewUrl.current !== null) {
        URL.revokeObjectURL(previewUrl.current);
      }
      previewUrl.current = url;
      setPreview({
        url,
        filename: content.Filename || state.Document?.OriginalFilename || "customer-po.pdf",
      });
    } catch (viewError) {
      setError(documentErrorMessage(viewError));
    } finally {
      setBusy("idle");
    }
  };

  const closePreview = () => {
    if (previewUrl.current !== null && typeof URL.revokeObjectURL === "function") {
      URL.revokeObjectURL(previewUrl.current);
      previewUrl.current = null;
    }
    setPreview(null);
  };

  const remove = async () => {
    setBusy("removing");
    setError(null);
    try {
      await RemovePurchaseOrderDocument(purchaseOrderId, true);
      await refresh();
      setPendingRemoval(false);
    } catch (removeError) {
      setError(documentErrorMessage(removeError));
    } finally {
      setBusy("idle");
    }
  };

  return (
    <>
      <input
        ref={fileInput}
        type="file"
        accept="application/pdf,.pdf"
        aria-label="Customer PO PDF file"
        className="sr-only"
        disabled={controlsDisabled}
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = "";
          if (file) void attach(file);
        }}
      />
      <PurchaseOrderDocumentPanelView
        state={state}
        busy={busy}
        loading={loading}
        error={error}
        pendingRemoval={pendingRemoval}
        absenceConfirmed={absenceConfirmed}
        preview={preview}
        disabled={disabled}
        onChooseFile={chooseFile}
        onView={() => void view()}
        onRemove={() => setPendingRemoval(true)}
        onConfirmRemove={() => void remove()}
        onCancelRemove={() => setPendingRemoval(false)}
        onClosePreview={closePreview}
      />
    </>
  );
}
