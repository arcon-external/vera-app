import type { JSX } from "react";
import {
  groupCompanyLabels,
  placeholderTokens,
  validateCompanyLabelSet,
  type CompanyLabelEntry,
} from "../companyLabels";
import { Badge, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";

// Presentational slice of the Company settings Quotation tab: the 26 editable
// labels grouped the way the document reads. Like the Units panel it holds no
// state and no save — the parent (CompanyProfileCard) owns the served set, the
// autosave and the identity refresh, and forwards edits up. The colours that
// share this tab are the existing CompanyThemePanel over the theme columns;
// this panel never touches a colour.
//
// The wording is served data, never hard-coded here: each field is named after
// its built-in default and prefilled from its effective value, so the client
// carries no copy of the document's words and a label added on the server
// appears without a release (in the fallback group until it is grouped).

/** Enter inside a field must not submit the surrounding profile form: it would
 *  save the profile while the wording autosave is still pending — a partial
 *  save that reads as success. The Units panel blocks Enter for the same
 *  reason. */
function blockEnter(event: React.KeyboardEvent): void {
  if (event.key === "Enter") event.preventDefault();
}

/** The live save readout. Wrapped in role="status" so the state change is
 *  announced rather than only repainted. */
function SaveStateChip({ state }: { state: "saved" | "dirty" | "saving" }): JSX.Element {
  if (state === "saving") {
    return (
      <span role="status" className="flex items-center">
        <Badge tone="internal">Saving…</Badge>
      </span>
    );
  }
  if (state === "dirty") {
    return (
      <span role="status" className="flex items-center">
        <Badge tone="accent">Unsaved changes</Badge>
      </span>
    );
  }
  return (
    <span role="status" className="flex items-center gap-1.5 text-caption text-ink-400">
      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
        <path d="M3 8.5l3.2 3.2L13 5" />
      </svg>
      Saved
    </span>
  );
}

/** A value this long never fits two columns: the closing sentence and the four
 *  terms sentences span the full width of the grid. */
function spansBothColumns(entry: CompanyLabelEntry): boolean {
  return entry.MaxLength > 60;
}

export function CompanyTemplatePanel(props: {
  /** The served entries in document order, or null while they load. */
  labels: CompanyLabelEntry[] | null;
  /** The live save state, owned by the parent's autosave. */
  saveState: "saved" | "dirty" | "saving";
  /** A save error to show, or null. */
  error: string | null;
  /** A non-fatal note (the save landed, the refresh did not), or null. */
  notice: string | null;
  /** Called with the entry key and its new raw value. */
  onChange: (key: string, value: string) => void;
}): JSX.Element {
  const { labels, saveState, error, notice, onChange } = props;

  if (labels === null) {
    return (
      <Card className="p-5">
        <SectionHeading title="Document wording" />
        <div className="h-48 animate-pulse rounded-xl bg-paper-muted" aria-label="Loading document wording" />
      </Card>
    );
  }

  // The store's own rules, mirrored here so a row the server would refuse is
  // marked before anything is sent (validateCompanyLabelSet). The parent reads
  // the same verdict to hold the autosave.
  const invalid = validateCompanyLabelSet(labels);
  const invalidCount = Object.keys(invalid).length;
  const groups = groupCompanyLabels(labels);

  return (
    <Card className="p-5">
      <SectionHeading title="Document wording" action={<SaveStateChip state={saveState} />} />
      <p className="max-w-2xl text-body text-ink-600">
        What prints on every customer quotation. Each field is named after its
        built-in wording and holds what the document says today; type over a
        value to change it, and empty a field to restore the built-in. Changes
        save automatically, and quotations already issued keep the wording they
        were exported with.
      </p>
      {error ? (
        <div className="mt-4">
          <StatusBanner tone="danger">{error}</StatusBanner>
        </div>
      ) : null}
      {error === null && notice !== null ? (
        <div className="mt-4">
          <StatusBanner tone="info">{notice}</StatusBanner>
        </div>
      ) : null}
      {invalidCount > 0 ? (
        <p className="mt-4 text-caption leading-relaxed text-danger-600">
          {invalidCount === 1
            ? "One field is highlighted below and must be fixed before anything saves."
            : `${invalidCount} fields are highlighted below and must be fixed before anything saves.`}
        </p>
      ) : null}

      <div className="mt-5 grid gap-6">
        {groups.map((group) => (
          <section key={group.id}>
            <h3 className="mb-1 text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
              {group.title}
            </h3>
            <p className="mb-3 max-w-2xl text-caption leading-relaxed text-ink-400">
              {group.hint}
            </p>
            <div className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
              {group.entries.map((entry) => {
                const cleared = entry.Value.trim() === "";
                const overridden = !cleared && entry.Value !== entry.Default;
                const message = invalid[entry.Key];
                const tokens = entry.Terms ? placeholderTokens(entry) : [];
                return (
                  // min-w-0 on the grid item: a grid item will not shrink past
                  // its content's intrinsic width otherwise, and the closing
                  // label is served text shown on one line (truncate) — its
                  // min-content is the whole sentence, so without this the
                  // single-column track is sized to that sentence and every
                  // field in the panel overflows the card at narrow widths.
                  <div
                    key={entry.Key}
                    className={`min-w-0${spansBothColumns(entry) ? " sm:col-span-2" : ""}`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-caption font-medium text-ink-600">
                          {entry.Default}
                        </span>
                        {cleared ? <Badge tone="neutral">Built-in</Badge> : null}
                      </span>
                      {overridden ? (
                        <button
                          type="button"
                          onClick={() => onChange(entry.Key, "")}
                          aria-label={`Reset ${entry.Default} to the built-in wording`}
                          className="shrink-0 rounded px-1 text-caption text-ink-400 hover:text-danger-600 hover:underline"
                        >
                          Reset
                        </button>
                      ) : null}
                    </div>
                    {/* The field carries an explicit aria-label rather than a
                        wrapping <label>: the visible name sits beside the
                        Reset button, and a label forwards clicks to its
                        control — the button must not be inside one. */}
                    <input
                      type="text"
                      value={entry.Value}
                      placeholder={entry.Default}
                      maxLength={entry.MaxLength}
                      spellCheck={false}
                      autoComplete="off"
                      aria-label={`Document wording: ${entry.Default}`}
                      aria-invalid={message ? true : undefined}
                      onKeyDown={blockEnter}
                      onChange={(event) => onChange(entry.Key, event.target.value)}
                      className={`${fieldClass} ${
                        message ? "border-danger-300 focus:border-danger-400" : ""
                      }`}
                    />
                    {entry.Terms && tokens.length > 0 ? (
                      <p className="mt-1 text-caption leading-snug text-ink-400">
                        {`Placeholders: ${tokens.join(", ")}. Each fills in from the quotation when it prints.`}
                      </p>
                    ) : null}
                    {message ? (
                      <p className="mt-1 text-caption leading-snug text-danger-600">
                        {message}
                      </p>
                    ) : null}
                  </div>
                );
              })}
            </div>
          </section>
        ))}
      </div>
    </Card>
  );
}
