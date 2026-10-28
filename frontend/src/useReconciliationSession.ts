import { useCallback, useEffect, useRef, useState } from "react";
import {
  HEADER_FIELDS,
  applyKeepResolution,
  applyUseResolution,
  commitHeaderField,
  countUnresolvedHeaderItems,
  emptyLocalSession,
  orderPaneItems,
  paneFilterCounts,
  type CommitNotice,
  type HeaderFieldDef,
  type HeaderFieldKey,
  type LatestNotice,
  type PaneFilter,
  type ReconciliationItem,
  type ReconciliationSession,
  type ReconciliationSourceValues,
} from "./poReconciliation";
import {
  loadSession,
  saveSession,
  type ReconciliationTransport,
} from "./poReconciliationAdapter";
import type { PurchaseOrderSourceState } from "./purchaseOrder";

export interface ReconciliationActor {
  id: number;
  name: string;
}

export interface UseReconciliationSessionOptions {
  transport: ReconciliationTransport;
  orderId: number;
  sourceState: PurchaseOrderSourceState;
  sourceQuotationId: number;
  sourceRevision: number;
  /** Latest known PO row version: the R-04 concurrency token for saves. */
  poVersion: number;
  /** Correction mode and saved views display the stored session read-only. */
  readOnly: boolean;
  /** Source scalars for live comparison; null until the review source loads. */
  source: ReconciliationSourceValues | null;
  actor: ReconciliationActor;
  getFieldValue: (field: HeaderFieldKey) => string;
  applyFieldValue: (field: HeaderFieldKey, value: string) => void;
}

/**
 * The live reconciliation session controller for the PO editor. Owns the
 * session the server created for the linked revision, the blur-commit state
 * machine, notices, filters, the keep-step and announcements. The editor owns
 * the form values; this hook only reads them at commit time and writes them
 * back on Use Quotation.
 */
export function useReconciliationSession(options: UseReconciliationSessionOptions) {
  const {
    transport,
    orderId,
    sourceState,
    sourceQuotationId,
    sourceRevision,
    poVersion,
    readOnly,
    source,
    actor,
  } = options;
  const getFieldValueRef = useRef(options.getFieldValue);
  getFieldValueRef.current = options.getFieldValue;
  const applyFieldValueRef = useRef(options.applyFieldValue);
  applyFieldValueRef.current = options.applyFieldValue;
  const sourceRef = useRef(source);
  sourceRef.current = source;

  const [session, setSession] = useState<ReconciliationSession | null>(null);
  const [status, setStatus] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const [error, setError] = useState<string | null>(null);
  const [notices, setNotices] = useState<Record<HeaderFieldKey, CommitNotice>>({
    "header:paymentTerms": null,
    "header:incoterms": null,
    "header:discountPercent": null,
  });
  const [validationErrors, setValidationErrors] = useState<
    Record<HeaderFieldKey, string | null>
  >({
    "header:paymentTerms": null,
    "header:incoterms": null,
    "header:discountPercent": null,
  });
  const [latestNotice, setLatestNotice] = useState<LatestNotice | null>(null);
  const [filter, setFilter] = useState<PaneFilter>("all");
  const [keepConfirmKey, setKeepConfirmKey] = useState<string | null>(null);
  const [loadToken, setLoadToken] = useState(0);
  const [announcement, setAnnouncement] = useState("");
  const [blockedAnnouncement, setBlockedAnnouncement] = useState("");
  // The items as last successfully saved; a save with no changes is skipped
  // so an untouched pane never churns versions.
  const savedItemsRef = useRef<string | null>(null);
  const statusRef = useRef(status);
  statusRef.current = status;

  const live = sourceState === "live" && orderId > 0;
  const liveRef = useRef(live);
  liveRef.current = live;
  const readOnlyRef = useRef(readOnly);
  readOnlyRef.current = readOnly;

  // Loads the stored session once the editor mounts with a live source. A
  // missing session is not an error: the first commit builds the local shell
  // and the draft save persists it.
  useEffect(() => {
    if (!liveRef.current) {
      setStatus("idle");
      setSession(null);
      setError(null);
      return;
    }
    let cancelled = false;
    setStatus("loading");
    setError(null);
    void loadSession(transport, String(orderId)).then(
      (result) => {
        if (cancelled) return;
        if (result.status === "ready") {
          setSession(result.session);
          savedItemsRef.current = JSON.stringify(result.session.Items);
          setStatus("ready");
        } else if (result.status === "absent") {
          setSession(null);
          savedItemsRef.current = JSON.stringify([]);
          setStatus("ready");
        } else {
          setError(result.message);
          setStatus("error");
        }
      },
      () => {
        // A missing Wails runtime rejects outside the adapter's mapping;
        // the pane shows its error state with a retry.
        if (cancelled) return;
        setError("The reconciliation could not be loaded.");
        setStatus("error");
      }
    );
    return () => {
      cancelled = true;
    };
    // orderId identity only: a version bump must not reload the session.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [transport, orderId, sourceState, loadToken]);

  const reload = useCallback(() => setLoadToken((token) => token + 1), []);

  const fieldDef = (field: HeaderFieldKey): HeaderFieldDef => HEADER_FIELDS[field];

  const findItem = useCallback(
    (key: string): ReconciliationItem | null =>
      session?.Items.find((entry) => entry.Key === key) ?? null,
    [session]
  );

  const upsertItem = useCallback(
    (next: ReconciliationItem | null, changed: boolean) => {
      if (next === null || !changed) return;
      setSession((current) => {
        const base =
          current ??
          emptyLocalSession({
            purchaseOrderId: orderId,
            sourceQuotationId,
            sourceRevision,
            poVersion,
          });
        const kept = base.Items.filter((entry) => entry.Key !== next.Key);
        return { ...base, Items: [...kept, next] };
      });
    },
    [orderId, sourceQuotationId, sourceRevision, poVersion]
  );

  /**
   * One blur-commit of one header field. Never runs per keystroke: the caller
   * wires onBlur (and Select onChange, which is itself a commit).
   */
  const commitField = useCallback(
    (field: HeaderFieldKey) => {
      if (readOnlyRef.current || !liveRef.current) return;
      const currentSource = sourceRef.current;
      if (currentSource === null) return;
      const def = fieldDef(field);
      const poValue = getFieldValueRef.current(field);
      const sourceValue =
        field === "header:discountPercent"
          ? currentSource.discountPercent
          : field === "header:paymentTerms"
            ? currentSource.paymentTerms
            : currentSource.incoterms;
      const result = commitHeaderField({
        field: def,
        poValue,
        sourceValue,
        existingItem: findItem(field),
        actorId: actor.id,
        now: new Date().toISOString(),
      });
      setNotices((current) => ({ ...current, [field]: result.notice }));
      setValidationErrors((current) => ({
        ...current,
        [field]: result.validationError,
      }));
      if (result.notice !== null) {
        setLatestNotice({ field, label: def.label, kind: result.notice });
        setAnnouncement(
          result.notice === "empty"
            ? `${def.label} is empty.`
            : `${def.label}: matches the Quotation.`
        );
      }
      if (result.validationError !== null) {
        setAnnouncement(result.validationError);
      }
      if (result.item !== null) {
        upsertItem(result.item, result.itemChanged);
        if (result.itemChanged && result.item.Status === "unresolved") {
          const count = countUnresolvedHeaderItems([
            ...(session?.Items.filter((entry) => entry.Key !== result.item?.Key) ?? []),
            result.item,
          ]);
          setAnnouncement(
            `${def.label}: differs from the Quotation. ${count} ${count === 1 ? "item" : "items"} to resolve.`
          );
        }
      }
    },
    // session read for the count only; the transition itself is pure.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [actor.id, findItem, upsertItem, session]
  );

  const requestKeep = useCallback((key: string) => {
    setKeepConfirmKey(key);
    const target = findItem(key);
    if (target !== null) {
      setAnnouncement(`Keeping the PO value for ${target.Label}. Is this correct?`);
    }
  }, [findItem]);

  const cancelKeep = useCallback(() => setKeepConfirmKey(null), []);

  const confirmKeep = useCallback(
    (key: string) => {
      const target = findItem(key);
      if (target === null) {
        setKeepConfirmKey(null);
        return;
      }
      const kept = applyKeepResolution(target, actor.id, new Date().toISOString());
      upsertItem(kept, true);
      setKeepConfirmKey(null);
      const remaining = countUnresolvedHeaderItems(
        (session?.Items ?? []).map((entry) => (entry.Key === key ? kept : entry))
      );
      setAnnouncement(
        `${target.Label} kept from the PO. ${remaining} ${remaining === 1 ? "item" : "items"} to resolve.`
      );
    },
    [actor.id, findItem, session, upsertItem]
  );

  const useQuotation = useCallback(
    (key: string) => {
      const target = findItem(key);
      if (target === null) return;
      const { item: used, value } = applyUseResolution(
        target,
        actor.id,
        new Date().toISOString()
      );
      upsertItem(used, true);
      const field = used.Field as HeaderFieldKey;
      if (HEADER_FIELDS[field] !== undefined) {
        applyFieldValueRef.current(field, String(value));
      }
      const remaining = countUnresolvedHeaderItems(
        (session?.Items ?? []).map((entry) => (entry.Key === key ? used : entry))
      );
      setAnnouncement(
        `${target.Label} updated from the Quotation. ${remaining} ${remaining === 1 ? "item" : "items"} to resolve.`
      );
    },
    [actor.id, findItem, session, upsertItem]
  );

  const showAll = useCallback(() => setFilter("all"), []);

  /**
   * Persists the session after a successful draft save, carrying the row
   * version the draft save just produced (R-04). A save with no new content
   * is skipped; a session failure never fails the draft save.
   */
  const persistAfterDraftSave = useCallback(
    async (rowVersion: number): Promise<{ ok: boolean; message?: string }> => {
      if (readOnlyRef.current || !liveRef.current) return { ok: true };
      const current = session;
      // Nothing compared and no stored row: nothing to persist.
      if (current === null) return { ok: true };
      const serialized = JSON.stringify(current.Items);
      if (current.ID > 0 && serialized === savedItemsRef.current) {
        return { ok: true };
      }
      const result = await saveSession(transport, String(orderId), {
        ...current,
        POVersion: rowVersion,
      });
      if (result.status === "saved") {
        savedItemsRef.current = JSON.stringify(result.session.Items);
        setSession(result.session);
        return { ok: true };
      }
      return { ok: false, message: result.message };
    },
    [transport, orderId, session]
  );

  const unresolvedCount = countUnresolvedHeaderItems(session?.Items ?? []);
  const ordered = orderPaneItems(session?.Items ?? [], filter);
  const counts = paneFilterCounts(session?.Items ?? []);

  return {
    session,
    status,
    error,
    notices,
    validationErrors,
    latestNotice,
    filter,
    setFilter,
    keepConfirmKey,
    unresolvedCount,
    ordered,
    counts,
    announcement,
    blockedAnnouncement,
    setBlockedAnnouncement,
    commitField,
    requestKeep,
    cancelKeep,
    confirmKeep,
    useQuotation,
    showAll,
    persistAfterDraftSave,
    items: session?.Items ?? [],
    reload,
  };
}
