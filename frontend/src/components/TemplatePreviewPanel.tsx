import { useCallback, useEffect, useRef, useState, type JSX } from "react";
import {
  DownloadTemplateSample,
  PreviewTemplateDocument,
} from "@backend/backend/Quotation";
import { isCompleteHex, type CompanyTheme } from "../company";
import { serializeCompanyLabels, type CompanyLabelEntry } from "../companyLabels";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

// The template preview (issue #425): the real sample PDF beside the Template
// tab's inputs, rendered through the active engine by the PreviewTemplateDocument
// binding — never a client-side re-implementation, which is what would let the
// preview drift from what an export prints.
//
// What this panel deliberately does not do: rasterise pages into images. The
// engine renders PDF bytes and neither side ships a PDF rasteriser (adding one
// would be a cgo dependency that breaks the CGO_ENABLED=0 cross-compile
// contract, or a commercial pure-Go one), so the document is shown embedded in
// the webview's own viewer — the bytes are the export's, only the chrome
// around them is the platform's. Download sample saves those same bytes.
//
// Like the portal preview it owns its own fetch, but unlike it there is no
// debounce: the parent re-renders this panel on blur/Enter (never per
// keystroke), signalled through refreshKey. The effect reads the latest props
// through a ref so a keystroke alone can never start a render.

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
  const [pdfUrl, setPdfUrl] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [savedPath, setSavedPath] = useState("");
  const [downloading, setDownloading] = useState(false);
  // What the visible document was rendered from. The download reuses it, so
  // the saved file is the document on screen rather than a newer draft.
  const rendered = useRef<RenderedArgs | null>(null);
  // The latest props for the refreshKey effect, which must not depend on them
  // directly: depending on labels/theme would re-render per keystroke, which
  // is exactly what the blur/Enter contract forbids.
  const latest = useRef({ labels, theme });
  latest.current = { labels, theme };
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
    PreviewTemplateDocument(...args)
      .then((bytes) => {
        if (id !== request.current) return;
        const url = URL.createObjectURL(
          new Blob([new Uint8Array(bytes as unknown as number[])], { type: "application/pdf" })
        );
        setPdfUrl((previous) => {
          if (previous) URL.revokeObjectURL(previous);
          return url;
        });
        rendered.current = args;
        setLoading(false);
      })
      .catch(() => {
        if (id !== request.current) return;
        setError("We couldn't render the preview. The last good preview stays above.");
        setLoading(false);
      });
  }, [refreshKey]);

  useEffect(() => {
    return () => {
      setPdfUrl((previous) => {
        if (previous) URL.revokeObjectURL(previous);
        return "";
      });
    };
  }, []);

  const download = useCallback(async () => {
    const args = rendered.current;
    if (downloading || args === null) return;
    setDownloading(true);
    setSavedPath("");
    try {
      const path = (await DownloadTemplateSample(...args)) as string;
      setSavedPath(`Sample saved to ${path}.`);
    } catch {
      setError("We couldn't save the sample.");
    } finally {
      setDownloading(false);
    }
  }, [downloading]);

  return (
    <Card className="p-5">
      <SectionHeading
        title="Template preview"
        action={
          error && pdfUrl === "" ? (
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
        colours above typed but not yet saved. It re-renders when you leave a
        field or press Enter — never while you type.
      </p>
      {error ? (
        <div className="mt-4">
          <StatusBanner tone={pdfUrl === "" ? "danger" : "info"}>{error}</StatusBanner>
        </div>
      ) : null}
      <div className="mt-4 bg-paper-muted p-3 lg:p-5">
        {pdfUrl === "" ? (
          <div className="h-48 animate-pulse rounded-xl bg-paper-subtle" aria-label="Loading template preview" />
        ) : (
          <iframe
            title="Template sample preview"
            src={pdfUrl}
            className="h-[min(760px,72vh)] min-h-[420px] w-full border-0 bg-white shadow-card"
          />
        )}
      </div>
      <p className="mt-3 max-w-2xl text-caption leading-relaxed text-ink-400">
        A two-page sample with a fake quote number. Its QR points at a code
        that does not verify, so it can never be mistaken for a document.
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
