import { useCallback, useEffect, useRef, useState, type JSX } from "react";
import { DownloadTemplateSample, PreviewTemplateHTML } from "@backend/backend/Quotation";
import { isCompleteHex, type CompanyTheme } from "../company";
import { serializeCompanyLabels, type CompanyLabelEntry } from "../companyLabels";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

// The template preview (issue #425, terminal swapped in #432): the real sample
// document on the merged Quotation tab (issue #429), above the colours and the
// wording inputs, rendered through the active engine by the
// PreviewTemplateHTML binding — never a client-side re-implementation, which is
// what would let the preview drift from what an export prints.
//
// What this panel deliberately does not do: rasterise pages into images. The
// engine renders PDF bytes and neither side ships a PDF rasteriser (adding one
// would be a cgo dependency that breaks the CGO_ENABLED=0 cross-compile
// contract, or a commercial pure-Go one). The document is therefore shown as
// what the engine executed — the template's own HTML, stylesheet inlined,
// artwork and QR as data URIs — in a sandboxed iframe: no scripts, no forms,
// no same-origin access, because a preview is a picture of a document, not the
// document. Download sample saves the real PDF bytes for the same draft.
//
// Why the terminal is HTML rather than the PDF bytes (#432): the blob-URL
// iframe the PDF used is blank on some platforms, and a settings editor that
// shows nothing judges nothing. The HTML is not a second rendering — it is the
// renderer's own input, the string the engines draw the PDF from — so the
// preview and the printed document come from one execution of one template. No
// pixel parity is promised: the native engine agrees with the browser to the
// extent the equivalence tests prove.
//
// Like the portal preview it owns its own fetch, but unlike it there is no
// debounce: the parent re-renders this panel on blur/Enter (never per
// keystroke), signalled through refreshKey, and once more when the served
// wording lands — the panel can mount mid-fetch, and [refreshKey] alone never
// re-ran then (#429). The effect reads the latest props through a ref so a
// keystroke alone can never start a render.

/** The four quotation colours, trimmed, in the order the binding takes them. */
export function templatePreviewColors(theme: CompanyTheme): [string, string, string, string] {
  return [theme.qDeep.trim(), theme.qAccent.trim(), theme.qSoft.trim(), theme.qInk.trim()];
}

/** The argument list for the preview binding. Labels pass through verbatim in
 *  their served shape; the backend reads Key/Value and resolves the rest. */
export function templatePreviewArgs(
  labels: CompanyLabelEntry[],
  theme: CompanyTheme
): [CompanyLabelEntry[], string, string, string, string] {
  return [labels, ...templatePreviewColors(theme)];
}

/** The first quotation colour that is neither empty (the stylesheet default)
 *  nor a complete hex, or null when the set renders. Named after the field so
 *  the error reads as a field error. */
export function templatePreviewColorError(theme: CompanyTheme): string | null {
  const names: [string, string][] = [
    ["Deep", theme.qDeep.trim()],
    ["Accent", theme.qAccent.trim()],
    ["Soft", theme.qSoft.trim()],
    ["Ink", theme.qInk.trim()],
  ];
  for (const [name, value] of names) {
    if (value !== "" && !isCompleteHex(value)) {
      return name;
    }
  }
  return null;
}

/** The fingerprint the parent compares to decide whether a blur or an Enter
 *  carries new values. Wording serializes the way the autosave does, so one
 *  canonical string covers both halves of the tab. */
export function templatePreviewFingerprint(
  labels: CompanyLabelEntry[] | null,
  theme: CompanyTheme
): string {
  if (labels === null) return "";
  return `${serializeCompanyLabels(labels)}\n${templatePreviewColors(theme).join("\n")}`;
}

/** The blur/Enter decision, pure so it is pinned rather than assumed: given
 *  the fingerprint of what the preview shows, return the fingerprint to
 *  render when the draft changed, or null when nothing is due (still loading,
 *  or tabbing through unchanged fields). */
export function templatePreviewRefresh(
  renderedFingerprint: string,
  labels: CompanyLabelEntry[] | null,
  theme: CompanyTheme
): string | null {
  const fingerprint = templatePreviewFingerprint(labels, theme);
  if (fingerprint === "" || fingerprint === renderedFingerprint) return null;
  return fingerprint;
}

/** The preview effect's trigger beyond refreshKey (issue #429): false while
 *  the wording fetch is in flight, true once a set has landed.
 *
 *  The panel can mount while that fetch is still running — the profile
 *  fetch that clears the loading skeleton is awaited before the labels fetch
 *  that follows it in the same load(), so labels lands strictly after the
 *  skeleton clears and the panel mounts. Keyed on [refreshKey] alone, the
 *  effect then ran once, found labels null, and never ran again: the preview
 *  sat on its "still loading" error for the rest of the session, and even a
 *  blur would not save it, because the fingerprint is unchanged and
 *  templatePreviewRefresh answers null. That is the blank preview.
 *
 *  This flips exactly once, on the null → loaded transition. A keystroke
 *  swaps the entries array without touching it, which is what keeps the
 *  latest-ref contract intact: no render per keystroke. */
export function templatePreviewLabelsReady(labels: CompanyLabelEntry[] | null): boolean {
  return labels !== null;
}

/** The backend's reason for a failed preview, when it sent one. The Wails
 *  runtime rejects with the Go error — a string, or an Error wrapping it —
 *  and those messages are descriptive ("no clients available for template
 *  sample", "template sample: deep colour ... is not a #RRGGBB colour").
 *  The catch used to discard them (#425), which left the #429 blank preview
 *  with a generic sentence and nothing to diagnose from. Empty when the
 *  rejection carries nothing readable, so the caller keeps its fallback. */
export function templatePreviewErrorDetail(err: unknown): string {
  if (typeof err === "string") return err.trim();
  if (err instanceof Error) return err.message.trim();
  return "";
}

type RenderedArgs = [CompanyLabelEntry[], string, string, string, string];

export function TemplatePreviewPanel(props: {
  /** The served wording entries, or null while they load. */
  labels: CompanyLabelEntry[] | null;
  /** The full theme; this panel reads the four quotation colours from it. */
  theme: CompanyTheme;
  /** Bumped by the parent on blur/Enter when the draft changed. */
  refreshKey: number;
}): JSX.Element {
  const { labels, theme, refreshKey } = props;
  // The executed document, a whole HTML page for the iframe's srcDoc. Empty
  // until the first render lands, which is also the pending state.
  const [html, setHtml] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [savedPath, setSavedPath] = useState("");
  const [downloading, setDownloading] = useState(false);
  // What the visible document was rendered from. The download reuses it, so
  // the saved file is the PDF bytes for the document on screen rather than a
  // newer draft.
  const rendered = useRef<RenderedArgs | null>(null);
  // The latest props for the effect, which must not depend on them
  // directly: depending on labels/theme would re-render per keystroke, which
  // is exactly what the blur/Enter contract forbids.
  const latest = useRef({ labels, theme });
  latest.current = { labels, theme };
  // The one labels change the effect must answer beyond refreshKey: the set
  // landing. Everything else about labels is a keystroke, and keystrokes
  // never render (see templatePreviewLabelsReady).
  const labelsReady = templatePreviewLabelsReady(labels);
  const request = useRef(0);

  useEffect(() => {
    const { labels: current, theme: draft } = latest.current;
    if (current === null) {
      setError("The document wording is still loading.");
      setLoading(false);
      return;
    }
    const bad = templatePreviewColorError(draft);
    if (bad !== null) {
      // A field error, and the last good preview stays: the invalid value
      // never reaches the binding, so the document on screen is always one
      // the engine actually rendered.
      setError(`The Quotation ${bad} colour is not a complete hex — fix it to refresh the preview.`);
      setLoading(false);
      return;
    }
    const id = ++request.current;
    setLoading(true);
    setError("");
    const args = templatePreviewArgs(current, draft);
    PreviewTemplateHTML(...args)
      .then((document) => {
        if (id !== request.current) return;
        setHtml(document);
        rendered.current = args;
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (id !== request.current) return;
        // The backend's reason is the diagnosis — swallowed here, a blank
        // preview is a generic sentence and a guess (#429). It is logged for
        // the console and shown in the banner, which is admin-only settings.
        console.error("PreviewTemplateHTML failed", err);
        const detail = templatePreviewErrorDetail(err);
        setError(
          detail === ""
            ? "We couldn't render the preview. The last good preview stays below."
            : `We couldn't render the preview: ${detail}. The last good preview stays below.`
        );
        setLoading(false);
      });
  }, [refreshKey, labelsReady]);

  const download = useCallback(async () => {
    const args = rendered.current;
    if (downloading || args === null) return;
    setDownloading(true);
    setSavedPath("");
    try {
      const path = (await DownloadTemplateSample(...args)) as string;
      setSavedPath(`Sample saved to ${path}.`);
    } catch (err: unknown) {
      // Same obligation as the render catch above: the backend says why
      // ("user cancelled", a write failure), and the banner must carry it.
      console.error("DownloadTemplateSample failed", err);
      const detail = templatePreviewErrorDetail(err);
      setError(detail === "" ? "We couldn't save the sample." : `We couldn't save the sample: ${detail}`);
    } finally {
      setDownloading(false);
    }
  }, [downloading]);

  return (
    <Card className="p-5">
      <SectionHeading
        title="Template preview"
        action={
          error && html === "" ? (
            <Badge tone="neutral">Preview failed</Badge>
          ) : loading ? (
            <Badge tone="internal">Rendering…</Badge>
          ) : (
            <Badge tone="success">Live</Badge>
          )
        }
      />
      <p className="max-w-2xl text-body text-ink-600">
        The sample quotation as it prints right now, with the wording and
        colours below typed but not yet saved. It re-renders when you leave a
        field or press Enter — never while you type.
      </p>
      {error ? (
        <div className="mt-4">
          <StatusBanner tone={html === "" ? "danger" : "info"}>{error}</StatusBanner>
        </div>
      ) : null}
      <div className="mt-4 bg-paper-muted p-3 lg:p-5">
        {html === "" ? (
          <div className="h-48 animate-pulse rounded-xl bg-paper-subtle" aria-label="Loading template preview" />
        ) : (
          <iframe
            title="Template sample preview"
            srcDoc={html}
            // No scripts, no forms, no same-origin access: the preview is a
            // picture of a page, not the page.
            sandbox=""
            className="h-[min(760px,72vh)] min-h-[420px] w-full border-0 bg-white shadow-card"
          />
        )}
      </div>
      <p className="mt-3 max-w-2xl text-caption leading-relaxed text-ink-400">
        A two-page sample with a fake quote number. Its QR points at a code
        that does not verify, so it can never be mistaken for a document.
      </p>
      <p className="mt-3 max-w-2xl text-caption leading-relaxed text-ink-400">
        The preview is the document's own HTML — the template the print engine
        runs, shown as it executes. Download sample saves the real PDF bytes
        for the same draft.
      </p>
      <div className="mt-4 flex flex-wrap items-center gap-3">
        <Button variant="secondary" onClick={() => void download()} disabled={downloading || rendered.current === null}>
          {downloading ? "Preparing…" : "Download sample"}
        </Button>
        {savedPath ? (
          <span className="text-caption leading-relaxed text-ink-500">{savedPath}</span>
        ) : null}
      </div>
    </Card>
  );
}
