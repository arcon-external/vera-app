import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  HEADER_FIELDS,
  applyKeepResolution,
  applyUseResolution,
  commitHeaderField,
  emptyLocalSession,
  orderPaneItems,
  paneFilterCounts,
  resolveKept,
  type CommitNotice,
  type CommitResult,
  type HeaderFieldDef,
  type HeaderFieldKey,
  type LatestNotice,
  type PaneFilter,
  type ReconciliationItem,
  type ReconciliationSession,
  type ReconciliationSourceValues,
} from "./poReconciliation";
import {
  acknowledgeLineMatch,
  commitLineField,
  deriveLineSet,
  lineFieldLabel,
  reconcileLineSession,
  sessionBlockers,
  LINE_FIELD_SUFFIX,
  sourceLineRef,
  prefillLineDecisions,
  type LineCommitResult,
  type LineField,
  type LineValuesInput,
  sessionLineKey,
  type SessionBlocker,
} from "./poReconciliation.lines";
import { formatQuantityScaled, formatUnitPriceMinor } from "./purchaseOrder";
import { formatMoney } from "./format";
import type { CurrencyInfo } from "./format";
import type { PurchaseOrderSourceLineDTO } from "./poFinalize";
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

export interface ReconciliationEditorLine {
  clientKey: string;
  persistedId?: number | null;
  dtoAlias?: string | null;
  displayNumber: number;
  quotationItemNo: number | null;
  quotationSubNo: number | null | undefined;
  values: LineValuesInput;
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
  /** Source lines for line matching; null until the review source loads. */
  sourceLines: PurchaseOrderSourceLineDTO[] | null;
  /** Editor lines with raw values, in display order. */
  lines: ReconciliationEditorLine[];
  currency: CurrencyInfo;
  actor: ReconciliationActor;
  getFieldValue: (field: HeaderFieldKey) => string;
  applyFieldValue: (field: HeaderFieldKey, value: string) => void;
  applyLineFieldValue: (
    lineKey: string,
    field: LineField,
    value: string | { enabled: boolean; percent: string }
  ) => void;
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
    sourceLines,
    lines,
    currency,
    actor,
  } = options;
  const getFieldValueRef = useRef(options.getFieldValue);
  getFieldValueRef.current = options.getFieldValue;
  const applyFieldValueRef = useRef(options.applyFieldValue);
  applyFieldValueRef.current = options.applyFieldValue;
  const applyLineFieldValueRef = useRef(options.applyLineFieldValue);
  applyLineFieldValueRef.current = options.applyLineFieldValue;
  const sourceRef = useRef(source);
  sourceRef.current = source;
  const sourceLinesRef = useRef(sourceLines);
  sourceLinesRef.current = sourceLines;
  const linesRef = useRef(lines);
  linesRef.current = lines;
  const currencyRef = useRef(currency);
  currencyRef.current = currency;
  const actorRef = useRef(actor);
  actorRef.current = actor;

  const [session, setSession] = useState<ReconciliationSession | null>(null);
  const [status, setStatus] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const [error, setError] = useState<string | null>(null);
  const [notices, setNotices] = useState<Record<HeaderFieldKey, CommitNotice>>({
    "header:paymentTerms": null,
    "header:incoterms": null,
  });
  const [validationErrors, setValidationErrors] = useState<
    Record<HeaderFieldKey, string | null>
  >({
    "header:paymentTerms": null,
    "header:incoterms": null,
  });
  // Ticket 05: line commit feedback, keyed `clientKey:field`.
  const [lineNotices, setLineNotices] = useState<Record<string, CommitNotice>>({});
  const [lineValidationErrors, setLineValidationErrors] = useState<
    Record<string, string | null>
  >({});
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

  // Ticket 05: declarative line-set sync. Rematches and removals flow through
  // this effect, never through imperative calls: the editor edits its lines,
  // and the session's identities plus missing/extra/unknown/duplicate items
  // follow. Idempotent: reconcile returns the same reference when nothing
  // changed, so depending on the session cannot loop.
  const lineSignature = JSON.stringify(
    lines.map((line) => [
      line.clientKey,
      line.persistedId ?? null,
      line.dtoAlias ?? null,
      line.displayNumber,
      line.quotationItemNo,
      line.quotationSubNo ?? null,
    ])
  );
  const sourcesSignature = JSON.stringify(
    sourceLines === null
      ? null
      : sourceLines.map((entry) => sourceLineRef(entry.QuotationItemNo, entry.QuotationSubNo))
  );
  useEffect(() => {
    if (readOnlyRef.current || !liveRef.current) return;
    if (sourcesSignature === "null") return;
    setSession((current) => {
      const base =
        current ??
        emptyLocalSession({
          purchaseOrderId: orderId,
          sourceQuotationId,
          sourceRevision,
          poVersion,
        });
      const identities = base.LineIdentities;
      const { session: next, changed } = reconcileLineSession({
        session: base,
        lines: linesRef.current.map((entry) => ({
          sessionKey: sessionLineKey(identities, entry),
          persistedId: entry.persistedId,
          displayNumber: entry.displayNumber,
          quotationItemNo: entry.quotationItemNo,
          quotationSubNo: entry.quotationSubNo,
        })),
        sources: sourceLinesRef.current ?? [],
        actorId: actorRef.current.id,
        now: new Date().toISOString(),
      });
      return changed ? next : current;
    });
    // Signatures pin the inputs; the session dep re-syncs after a commit or
    // save rebuilds it without touching the line set.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lineSignature, sourcesSignature, orderId, sourceQuotationId, sourceRevision, poVersion, session]);

  const sessionKeyFor = useCallback(
    (clientKey: string): string => {
      const line = linesRef.current.find((entry) => entry.clientKey === clientKey);
      if (line === undefined) return clientKey;
      return sessionLineKey(session?.LineIdentities ?? [], line);
    },
    [session]
  );

  const clientKeyForSessionKey = useCallback(
    (sessionKey: string): string | null => {
      const identities = session?.LineIdentities ?? [];
      const line = linesRef.current.find(
        (entry) => sessionLineKey(identities, entry) === sessionKey
      );
      return line?.clientKey ?? null;
    },
    [session]
  );

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
    (
      field: HeaderFieldKey,
      explicitValue?: string,
      options?: { autoKeep?: boolean }
    ): CommitResult | null => {
      if (readOnlyRef.current || !liveRef.current) return null;
      const currentSource = sourceRef.current;
      if (currentSource === null) return null;
      const def = fieldDef(field);
      // F3: selects commit the NEW value, not React's previous render.
      const poValue = explicitValue ?? getFieldValueRef.current(field);
      const sourceValue =
        field === "header:paymentTerms"
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
      let item = result.item;
      // The inline editor has no pane: a differing field is a visible "using
      // my value" state, so the commit itself records that keep instead of
      // leaving an unresolved review item nobody can resolve.
      if (
        options?.autoKeep === true &&
        item !== null &&
        item.Status === "unresolved"
      ) {
        item = resolveKept(item, actor.id, new Date().toISOString());
      }
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
      if (item !== null) {
        upsertItem(item, result.itemChanged);
        if (result.itemChanged && item.Status === "unresolved") {
          const count = [
            ...(session?.Items.filter((entry) => entry.Key !== item?.Key) ?? []),
            item,
          ].filter((entry) => entry.Status === "unresolved").length;
          setAnnouncement(
            `${def.label}: differs from the Quotation. ${count} ${count === 1 ? "item" : "items"} to resolve.`
          );
        }
      }
      return { ...result, item };
    },
    // session read for the count only; the transition itself is pure.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [actor.id, findItem, upsertItem, session]
  );

  /**
   * Ticket 05: one blur-commit of one line field. Same notice/review
   * behaviour as commitField, keyed by the stable line identity. Unmatched
   * lines never compare; invalid numbers are validation errors, never items.
   */
  const commitLine = useCallback(
    (
      lineKey: string,
      field: LineField,
      explicitRaw?: string | { enabled: boolean; percent: string | number },
      options?: { autoKeep?: boolean }
    ): LineCommitResult | null => {
      if (readOnlyRef.current || !liveRef.current) return null;
      const currentSources = sourceLinesRef.current;
      if (currentSources === null) return null;
      const line = linesRef.current.find((entry) => entry.clientKey === lineKey);
      if (line === undefined) return null;
      const identities = session?.LineIdentities ?? [];
      const sessionKey = sessionLineKey(identities, line);
      const set = deriveLineSet(
        linesRef.current.map((entry) => ({
          sessionKey: sessionLineKey(identities, entry),
          lineNumber: entry.displayNumber,
          quotationItemNo: entry.quotationItemNo,
          quotationSubNo: entry.quotationSubNo,
        })),
        currentSources
      );
      const pair = set.matched.find((entry) => entry.line.sessionKey === sessionKey);
      const feedbackKey = `${sessionKey}:${field}`;
      const raw =
        explicitRaw !== undefined
          ? explicitRaw
          : field === "discount"
            ? { enabled: line.values.discountEnabled, percent: line.values.discountPercent }
            : line.values[field];
      const result = commitLineField({
        lineKey: sessionKey,
        displayNumber: line.displayNumber,
        field,
        poValue: raw,
        source: pair?.source ?? null,
        existingItem: findItem(`line:${sessionKey}:${LINE_FIELD_SUFFIX[field]}`),
        actorId: actorRef.current.id,
        now: new Date().toISOString(),
        currency: currencyRef.current,
      });
      let item = result.item;
      if (
        options?.autoKeep === true &&
        item !== null &&
        item.Status === "unresolved"
      ) {
        item = resolveKept(item, actorRef.current.id, new Date().toISOString());
      }
      setLineNotices((current) => ({ ...current, [feedbackKey]: result.notice }));
      setLineValidationErrors((current) => ({
        ...current,
        [feedbackKey]: result.validationError,
      }));
      if (result.notice !== null) {
        const label = lineFieldLabel(line.displayNumber, field);
        setLatestNotice({ field: `line:${sessionKey}:${field}`, label, kind: result.notice });
        setAnnouncement(
          result.notice === "empty"
            ? `${label} is empty.`
            : `${label}: matches the Quotation.`
        );
      }
      if (result.validationError !== null) {
        setAnnouncement(result.validationError);
      }
      if (item !== null) {
        upsertItem(item, result.itemChanged);
        if (result.itemChanged && item.Status === "unresolved") {
          const label = lineFieldLabel(line.displayNumber, field);
          setAnnouncement(`${label}: differs from the Quotation.`);
        }
      }
      return { ...result, item };
    },
    // sessionKey read for the identity; the transition itself is pure.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [findItem, upsertItem, session]
  );

  const acknowledgeMissing = useCallback(
    (key: string) => {
      if (readOnlyRef.current) return;
      setSession((current) => {
        if (current === null) return current;
        const { session: next, changed } = acknowledgeLineMatch(
          current,
          key,
          "acknowledged_missing",
          actorRef.current.id,
          new Date().toISOString()
        );
        if (changed) {
          setAnnouncement("Omission acknowledged. It stays recorded on the session.");
        }
        return changed ? next : current;
      });
    },
    []
  );

  const acknowledgeExtra = useCallback(
    (key: string, options?: { silent?: boolean }) => {
      if (readOnlyRef.current) return;
      setSession((current) => {
        if (current === null) return current;
        const { session: next, changed } = acknowledgeLineMatch(
          current,
          key,
          "acknowledged_extra",
          actorRef.current.id,
          new Date().toISOString()
        );
        if (changed && options?.silent !== true) {
          setAnnouncement("Extra line acknowledged. Unknown references still block Final Save until corrected.");
        }
        return changed ? next : current;
      });
    },
    []
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
      const remaining = (session?.Items ?? [])
        .map((entry) => (entry.Key === key ? kept : entry))
        .filter((entry) => entry.Status === "unresolved").length;
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
      } else if (typeof used.Field === "string" && used.Field.startsWith("line:")) {
        // #517: write the source value back into the editor line. Item keys
        // carry the stable session key (`line:{sessionKey}:{suffix}`), so it
        // is mapped back to the editor alias before applying the value.
        const segments = used.Key.split(":");
        const suffix = segments[segments.length - 1];
        const sessionKey = segments.slice(1, -1).join(":");
        const identities = session?.LineIdentities ?? [];
        const lineKey =
          linesRef.current.find((entry) => sessionLineKey(identities, entry) === sessionKey)
            ?.clientKey ?? sessionKey;
        const currency = currencyRef.current;
        if (suffix === "quantity" && typeof value === "number") {
          applyLineFieldValueRef.current(lineKey, "quantity", formatQuantityScaled(value));
        } else if (suffix === "unitPrice" && typeof value === "number") {
          applyLineFieldValueRef.current(
            lineKey,
            "unitPrice",
            formatUnitPriceMinor(currency, value)
          );
        } else if (
          suffix === "discount" &&
          value !== null &&
          typeof value === "object" &&
          "Enabled" in value &&
          "Percent" in value
        ) {
          const discount = value as { Enabled: unknown; Percent: unknown };
          applyLineFieldValueRef.current(lineKey, "discount", {
            enabled: discount.Enabled === true,
            percent: String(discount.Percent ?? 0),
          });
        } else if (suffix === "description" || suffix === "uom") {
          applyLineFieldValueRef.current(
            lineKey,
            suffix,
            typeof value === "string" ? value : String(value ?? "")
          );
        }
      }
      const remaining = (session?.Items ?? [])
        .map((entry) => (entry.Key === key ? used : entry))
        .filter((entry) => entry.Status === "unresolved").length;
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
   * is skipped; a session failure never fails the draft save. The returned
   * session is the post-call truth (the server's answer after a save, the
   * current session when skipped) so callers never reason from a stale
   * render closure about whether the session is persisted yet (N4/U2).
   */
  const persistAfterDraftSave = useCallback(
    async (
      rowVersion: number
    ): Promise<{ ok: boolean; message?: string; version?: number; session: ReconciliationSession | null }> => {
      if (readOnlyRef.current || !liveRef.current) return { ok: true, session };
      const current = session;
      // Nothing compared and no stored row: nothing to persist.
      if (current === null) return { ok: true, session: null };
      const serialized = JSON.stringify(current.Items);
      if (current.ID > 0 && serialized === savedItemsRef.current) {
        return { ok: true, session: current };
      }
      const result = await saveSession(transport, String(orderId), {
        ...current,
        POVersion: rowVersion,
      });
      if (result.status === "saved") {
        savedItemsRef.current = JSON.stringify(result.session.Items);
        setSession(result.session);
        // The session save bumps the row version (R-04) exactly like a draft
        // save does. Callers adopt the returned version so the next submit
        // names the row as the server now stands, not the draft-save one.
        return { ok: true, version: result.session.POVersion, session: result.session };
      }
      return { ok: false, message: result.message, session: current };
    },
    [transport, orderId, session]
  );

  // #517: named Final Save blockers from the live line set. Unresolved
  // items of any kind plus unacknowledged missing/extra lines, every
  // duplicate group and every unknown reference (acknowledged or not).
  // Memoized on content signatures: the caller builds fresh arrays every
  // render, and the shell gate effect depends on this reference — a new
  // identity per render would re-fire the gate and its setState forever.
  const derivedLineSet = useMemo(
    () =>
      sourceLines === null
        ? null
        : deriveLineSet(
            lines.map((entry) => ({
              sessionKey: sessionLineKey(session?.LineIdentities ?? [], entry),
              lineNumber: entry.displayNumber,
              quotationItemNo: entry.quotationItemNo,
              quotationSubNo: entry.quotationSubNo,
            })),
            sourceLines
          ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [session, lineSignature, sourcesSignature]
  );
  const ackedMissingRefs = new Set(
    (session?.Items ?? [])
      .filter(
        (entry) =>
          entry.Kind === "missing_line" &&
          entry.Status === "resolved" &&
          typeof entry.SourceLineRef === "string"
      )
      .map((entry) => entry.SourceLineRef as string)
  );
  const ackedExtraKeys = new Set(
    (session?.Items ?? [])
      .filter(
        (entry) =>
          entry.Kind === "extra_line" &&
          (entry.Status === "resolved" ||
            entry.History.some((event) => event.Resolution === "acknowledged_extra")) &&
          typeof entry.LineKey === "string"
      )
      .map((entry) => entry.LineKey as string)
  );
  const blockers: SessionBlocker[] = useMemo(
    () =>
      session === null || derivedLineSet === null
        ? (session?.Items ?? [])
            .filter((entry) => entry.Status === "unresolved")
            .map((entry) => ({ kind: "unresolved" as const, label: entry.Label }))
        : sessionBlockers({
            items: session.Items,
            missing: derivedLineSet.missing.map((entry) => ({
              quotationItemNo: entry.QuotationItemNo,
              quotationSubNo: entry.QuotationSubNo,
              acknowledged: ackedMissingRefs.has(
                sourceLineRef(entry.QuotationItemNo, entry.QuotationSubNo)
              ),
            })),
            extra: derivedLineSet.extra.map((entry) => ({
              sessionKey: entry.sessionKey,
              label: `Extra line · Line ${entry.lineNumber}`,
              acknowledged: ackedExtraKeys.has(entry.sessionKey),
            })),
            duplicates: derivedLineSet.duplicates,
            unknown: derivedLineSet.unknown.map((entry) => ({
              sessionKey: entry.sessionKey,
              reference: entry.reference,
              acknowledged: ackedExtraKeys.has(entry.sessionKey),
            })),
          }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [session, derivedLineSet]
  );

  /**
   * Ticket 05: section 5.5 for lines. Kept line decisions with unchanged
   * values pre-fill the keep choice under the review's positional keys.
   */
  const prefillMissing = useCallback(
    (sources: PurchaseOrderSourceLineDTO[]): string[] => {
      if (session === null) return [];
      const acked = new Set(
        session.Items.filter(
          (entry) =>
            entry.Kind === "missing_line" &&
            entry.Status === "resolved" &&
            typeof entry.SourceLineRef === "string"
        ).map((entry) => entry.SourceLineRef as string)
      );
      // U2: the review keys acknowledgements by the source label it renders.
      // The label prefers LineLabel and falls back to the numeric reference;
      // accept either so a session ack survives a label/reference mismatch.
      return sources
        .map((source) => ({
          label:
            source.LineLabel !== ""
              ? source.LineLabel
              : source.QuotationSubNo === null || source.QuotationSubNo === undefined
                ? String(source.QuotationItemNo)
                : `${source.QuotationItemNo}.${source.QuotationSubNo}`,
          ref: sourceLineRef(source.QuotationItemNo, source.QuotationSubNo),
        }))
        .filter((entry) => acked.has(entry.label) || acked.has(entry.ref))
        .map((entry) => entry.label);
    },
    [session]
  );

  const prefillExtra = useCallback((): number[] => {
    if (session === null) return [];
    const ackedKeys = new Set(
      session.Items.filter(
        (entry) =>
          entry.Kind === "extra_line" &&
          (entry.Status === "resolved" ||
            entry.History.some((event) => event.Resolution === "acknowledged_extra")) &&
          typeof entry.LineKey === "string"
      ).map((entry) => entry.LineKey as string)
    );
    return linesRef.current
      .filter((entry) =>
        ackedKeys.has(sessionLineKey(session?.LineIdentities ?? [], entry))
      )
      .map((entry) => entry.displayNumber);
  }, [session]);

  const prefillLines = useCallback((): { [key: string]: "po" | "quotation" } => {
    const currentSources = sourceLinesRef.current;
    if (currentSources === null) return {};
    return prefillLineDecisions(
      session?.Items ?? [],
      linesRef.current.map((entry) => ({
        sessionKey: sessionLineKey(session?.LineIdentities ?? [], entry),
        displayNumber: entry.displayNumber,
        values: entry.values,
      })),
      currentSources,
      currencyRef.current
    );
  }, [session]);

  const allUnresolvedCount = (session?.Items ?? []).filter(
    (entry) => entry.Status === "unresolved"
  ).length;
  const unresolvedCount = allUnresolvedCount;
  const ordered = orderPaneItems(session?.Items ?? [], filter);
  const counts = paneFilterCounts(session?.Items ?? []);

  const announce = useCallback((message: string) => {
    setAnnouncement(message);
  }, []);

  return {
    session,
    status,
    announce,
    error,
    notices,
    validationErrors,
    lineNotices,
    lineValidationErrors,
    latestNotice,
    filter,
    setFilter,
    keepConfirmKey,
    unresolvedCount,
    blockers,
    blockingCount: blockers.length,
    ordered,
    counts,
    announcement,
    blockedAnnouncement,
    setBlockedAnnouncement,
    commitField,
    commitLine,
    sessionKeyFor,
    clientKeyForSessionKey,
    acknowledgeMissing,
    acknowledgeExtra,
    prefillLines,
    prefillMissing,
    prefillExtra,
    hasSession: session !== null,
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
