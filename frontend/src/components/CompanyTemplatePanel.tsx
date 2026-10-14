import type { JSX } from "react";
import {
  companyLabelsRefusal,
  groupCompanyLabels,
  placeholderTokens,
  validateCompanyLabelSet,
  type CompanyLabelEntry,
} from "../companyLabels";
import { Badge, Card, SaveStateChip, SectionHeading, StatusBanner, fieldClass } from "./ui";

// Presentational slice of the Company settings Quotation tab: the 26 editable
// labels grouped the way the document reads. Like the Units panel it holds no
// state and no save — the parent (CompanyProfileCard) owns the served set and
// the save, and forwards edits up. The colours that share this tab are the
// existing CompanyThemePanel over the theme columns; this panel never touches a
// colour.
//
// The wording is served data, never hard-coded here: each field is named after
// its built-in default and prefilled from its effective value, so the client
// carries no copy of the document's words and a label added on the server
// appears without a release (in the fallback group until it is grouped).
//
// The wording saves manually, from the tab's own save row (#432). The autosave
// it used to have cost a save per pause in typing and a save nobody saw; the
// button, the Unsaved changes chip and the tab-switch guard replace it, and the
// copy below says so rather than promising an autosave. The Enter blocker this
// panel used to carry retired with the shared <form> it existed to protect:
// there is no form to submit, and Enter in a field still refreshes the preview
// through the tab's capture-phase handler.

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
  // the same verdict to disable the save row — one sentence, from one place, so
  // the row and the panel cannot describe the same refusal differently.
  const invalid = validateCompanyLabelSet(labels);
  const refusal = companyLabelsRefusal(labels);
  const groups = groupCompanyLabels(labels);

  return (
    <Card className="p-5">
      <SectionHeading title="Document wording" action={<SaveStateChip state={saveState} />} />
      <p className="max-w-2xl text-body text-ink-600">
        What prints on every customer quotation. Each field is named after its
        built-in wording and holds what the document says today; type over a
        value to change it, and empty a field to restore the built-in. The
        wording saves with the button below, together with the four colours,
        and quotations already issued keep the wording they were exported with.
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
      {refusal ? (
        <p className="mt-4 text-caption leading-relaxed text-danger-600">
          {refusal}
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
