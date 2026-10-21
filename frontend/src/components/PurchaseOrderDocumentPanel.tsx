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
  documentStateOf,
  documentVersionLabel,
  extractAndCompareAvailability,
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
//
// "Extract and compare" is visible but disabled until #477 ships, with the
// reason stated beside it. A dead control with no explanation would imply the
// feature exists and is broken.

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
   * Starts a text extraction. Omitted by a read-only saved view, whose panel
   * explains the correction path instead of starting an unappliable job.
   */
  onExtract?: () => void;
  /** The run's queued/running status, for honest progress on the button. */
  extractionStatus?: "queued" | "running" | null;
  /** True while the start request itself is in flight. */
  extractionStarting?: boolean;
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
  onExtract,
  extractionStatus = null,
  extractionStarting = false,
  onChooseFile,
  onView,
  onRemove,
  onConfirmRemove,
  onCancelRemove,
  onClosePreview,
}: PurchaseOrderDocumentPanelViewProps) {
  const active: PurchaseOrderDocumentDTO | null = state.Document ?? null;
  const documentState = documentStateOf(state);
  const extract = extractAndCompareAvailability(documentState, onExtract != null);
  const extractInFlight = extractionStatus === "queued" || extractionStatus === "running";
  const extractBusy = extractionStarting || extractInFlight;
  const disabled = busy !== "idle" || loading;
  const versionLabel = documentVersionLabel(state);
  const uploadedDate = active?.UploadedAt ? active.UploadedAt.slice(0, 10) : "";
  const extractHint =
    extractionStarting
      ? "Starting extraction…"
      : extractionStatus === "queued"
        ? "Queued for extraction…"
        : extractionStatus === "running"
          ? "Reading the PDF text…"
          : extract.enabled
            ? extract.hint
            : extract.reason;

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
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <Button
              aria-label="Extract and compare"
              onClick={onExtract}
              disabled={disabled || extractBusy || !extract.enabled}
              title={extract.enabled ? undefined : extract.reason}
            >
              {extractBusy ? "Extracting…" : "Extract and compare"}
            </Button>
            <span className="max-w-[18rem] text-caption text-ink-500">
              {extractHint}
            </span>
          </div>
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
  /** Server state when the parent already has it (snapshots, saved views). */
  initialState?: PurchaseOrderDocumentStateDTO | null;
  absenceConfirmed?: boolean;
  onStateChange?: (state: PurchaseOrderDocumentStateDTO) => void;
  /**
   * Called synchronously when a successful attach/replace/remove has changed
   * the active document, before the state refresh returns. The editor clears
   * any extraction result and selection here, so a refresh that is still in
   * flight cannot leave proposals for the old PDF actionable.
   */
  onDocumentChanged?: () => void;
  /** Omitted by a saved view: its document panel explains the correction path. */
  onExtract?: () => void;
  extractionStatus?: "queued" | "running" | null;
  extractionStarting?: boolean;
}

/**
 * The connected panel. It owns the file picker and the content preview; the
 * purchase order page only supplies the id. A successful attach refreshes the
 * state so history and version labels come from the server, never from a
 * guessed local increment.
 */
export function PurchaseOrderDocumentPanel({
  purchaseOrderId,
  initialState = null,
  absenceConfirmed = false,
  onStateChange,
  onDocumentChanged,
  onExtract,
  extractionStatus = null,
  extractionStarting = false,
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

  const refresh = async () => {
    const generation = stateGeneration.current + 1;
    stateGeneration.current = generation;
    try {
      const next = await GetPurchaseOrderDocumentState(purchaseOrderId);
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

  const chooseFile = () => {
    fileInput.current?.click();
  };

  const attach = async (file: File) => {
    if (file.size > MAX_PURCHASE_ORDER_DOCUMENT_BYTES) {
      setError(documentErrorMessage(new Error("the customer PO PDF is larger than 20 MB")));
      return;
    }
    setBusy("uploading");
    setError(null);
    try {
      const buffer = await file.arrayBuffer();
      await AttachPurchaseOrderDocument(
        new backend.PurchaseOrderDocumentUploadInput({
          PurchaseOrderID: purchaseOrderId,
          DataBase64: bytesToBase64(new Uint8Array(buffer)),
          Filename: file.name,
        })
      );
      // Notify before refreshing: the editor must not keep showing proposals
      // for the previous PDF while the new state is still being read.
      onDocumentChanged?.();
      await refresh();
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
      onDocumentChanged?.();
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
        onExtract={onExtract}
        extractionStatus={extractionStatus}
        extractionStarting={extractionStarting}
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
