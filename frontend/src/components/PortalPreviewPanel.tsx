import { useCallback, useEffect, useRef, useState, type JSX } from "react";
import { GetPortalPreviewHTML } from "@backend/backend/AdminAPI";
import type { CompanyTheme } from "../company";
import { Badge, Card, SectionHeading } from "./ui";

// The portal preview (issue #340): the verification page as it renders now,
// with the colours typed but not yet saved, one state at a time.
//
// It is deliberately the only panel in Settings that owns its own fetch. The
// colours belong to the parent (the profile save writes them, and the Quotation
// panel edits the other four), but what is being judged here is a *document*,
// and that document has to come from the server that renders it: the endpoint
// executes the real verify_template.html through the same builder the live
// portal uses, so the preview cannot drift from the page a customer sees. A
// client-side re-implementation would be a second copy of the one surface that
// must never be ambiguous.
//
// The five states are shown one at a time on purpose. Stacked, five verdicts
// read as a gallery; one at a time, each is judged on its own — and the test
// that matters (a brand colour must never make `mismatch` look like `valid`) is
// a judgement a person makes against one page, not five.

/** The five states, in the order the toggle shows them. */
const VERDICTS: { id: string; label: string }[] = [
  { id: "valid", label: "Valid" },
  { id: "superseded", label: "Superseded" },
  { id: "mismatch", label: "Mismatch" },
  { id: "notfound", label: "Not found" },
  { id: "unavailable", label: "Unavailable" },
];

/** How long after the last colour keystroke the preview re-renders. A colour
 *  picker drag fires continuously, and one round trip per frame would both
 *  hammer the endpoint and rebuild the iframe dozens of times for a single
 *  choice. The same pause the wording autosave uses. */
const PREVIEW_DEBOUNCE_MS = 700;

/** The four portal colours, in the order the endpoint takes them.
 *
 *  Reading the wrong four here — the quotation's keys instead of the portal's —
 *  would preview a brand the verification page never shows, which is the exact
 *  failure this feature exists to prevent. So the mapping is exported and
 *  pinned rather than written inline. */
export function portalColors(theme: CompanyTheme): [string, string, string, string] {
  return [theme.pDeep, theme.pAccent, theme.pSoft, theme.pInk];
}

/** The argument list for the preview binding. */
export function portalPreviewArgs(
  verdict: string,
  portal: [string, string, string, string]
): [string, string, string, string, string] {
  return [verdict, ...portal];
}

export function PortalPreviewPanel(props: {
  /** The full theme; this panel reads the four portal colours from it. */
  theme: CompanyTheme;
}): JSX.Element {
  const { theme } = props;
  const [verdict, setVerdict] = useState(VERDICTS[0].id);
  const [previewHtml, setPreviewHtml] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  // The colours actually rendered. Kept apart from the live theme so a
  // keystroke does not re-render the document mid-word.
  const [rendered, setRendered] = useState<[string, string, string, string]>(() =>
    portalColors(theme)
  );
  const latest = useRef(0);

  useEffect(() => {
    const [deep, accent, soft, ink] = portalColors(theme);
    if (deep === rendered[0] && accent === rendered[1] && soft === rendered[2] && ink === rendered[3]) {
      return;
    }
    const timer = setTimeout(() => setRendered([deep, accent, soft, ink]), PREVIEW_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [theme, rendered]);

  const load = useCallback(async () => {
    const request = ++latest.current;
    setLoading(true);
    setError("");
    try {
      const html = await GetPortalPreviewHTML(...portalPreviewArgs(verdict, rendered));
      if (request !== latest.current) return;
      setPreviewHtml(html);
    } catch {
      if (request !== latest.current) return;
      setError("We couldn't render the preview. The colours are still saved.");
    } finally {
      if (request === latest.current) setLoading(false);
    }
  }, [verdict, rendered]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <Card className="p-5">
      <SectionHeading
        title="Portal preview"
        action={
          error ? (
            <Badge tone="neutral">Preview failed</Badge>
          ) : loading ? (
            <Badge tone="internal">Rendering…</Badge>
          ) : (
            <Badge tone="success">Live</Badge>
          )
        }
      />
      <p className="max-w-2xl text-body text-ink-600">
        What the verification page looks like right now, with the colours below
        typed but not yet saved. Pick one state at a time: each has to be
        unmistakable on its own, because a customer reads exactly one of them.
      </p>
      <div className="mt-4 flex flex-wrap gap-2" role="group" aria-label="Verification state">
        {VERDICTS.map((option) => (
          <button
            key={option.id}
            type="button"
            onClick={() => setVerdict(option.id)}
            aria-pressed={verdict === option.id}
            className={`rounded-md border px-3 py-1.5 text-small transition-colors duration-150 ${
              verdict === option.id
                ? "border-accent-500 bg-accent-50 text-accent-700"
                : "border-rule bg-white text-ink-600 hover:border-ink-300 hover:text-ink-900"
            }`}
          >
            {option.label}
          </button>
        ))}
      </div>
      {error ? (
        <div className="mt-4">
          <p className="text-caption text-danger-600">{error}</p>
        </div>
      ) : null}
      <div className="mt-4 bg-paper-muted p-3 lg:p-5">
        {previewHtml === "" ? (
          <div className="h-48 animate-pulse rounded-xl bg-paper-subtle" aria-label="Loading portal preview" />
        ) : (
          <iframe
            title={`Verification page preview — ${verdict}`}
            srcDoc={previewHtml}
            // No scripts, no forms, no same-origin access: the preview is a
            // picture of a page, not the page.
            sandbox=""
            className="h-[min(760px,72vh)] min-h-[420px] w-full border-0 bg-white shadow-card"
          />
        )}
      </div>
      <p className="mt-3 max-w-2xl text-caption leading-relaxed text-ink-400">
        The wording of the portal is fixed on purpose — “this document does not
        match our records” is a security statement, and softening it would weaken
        the one message that has to be unambiguous. Only the four colours are
        editable.
      </p>
    </Card>
  );
}
