import type { JSX } from "react";
import type { CompanyTheme } from "../company";
import { COMPANY_TAB_SAVES } from "../companySaves";
import { CompanyThemePanel } from "./CompanyThemePanel";
import { PortalPreviewPanel } from "./PortalPreviewPanel";
import { SaveRow, type SaveState } from "./ui";

// The Portal tab (issue #340, save model #432): the verification page as a
// document on top, the four portal colours below, one save row for both. It is
// the shape the merged Quotation tab follows — the thing being judged is a
// page, and it has to come from the server that renders it.
//
// The portal's colours are their own save (#432): they used to ride the
// whole-record profile save, so recolouring the verification page meant
// resending every other column of the company record. Now the Portal tab saves
// its four and nothing else, and the wording of the page stays hard-coded on
// purpose — it is a security statement, not company copy.

export function CompanyPortalTab(props: {
  /** The full theme; this panel reads the four portal colours from it. */
  theme: CompanyTheme;
  /** Called with the theme field key and its new raw value. */
  onChange: (key: keyof CompanyTheme, next: string) => void;
  /** The live save state, shown on the save row's chip. */
  status: SaveState;
  /** True while the portal save is in flight. */
  busy: boolean;
  /** A failed save's reason, or null. */
  error: string | null;
  /** Saves the four portal colours. */
  onSave: () => void;
}): JSX.Element {
  const { theme, onChange, status, busy, error, onSave } = props;
  return (
    <div className="grid gap-6">
      {/* The portal is judged as a document, one state at a time: the four
          colours are only meaningful against the five verdicts a customer can
          meet. */}
      <PortalPreviewPanel theme={theme} />
      <CompanyThemePanel section="Portal" theme={theme} onChange={onChange} />
      <SaveRow
        label={COMPANY_TAB_SAVES.portal.button}
        busyLabel={COMPANY_TAB_SAVES.portal.busy}
        busy={busy}
        state={status}
        onSave={onSave}
        error={error}
        hint="The preview above shows the four colours before you save them."
      />
      <p className="max-w-2xl text-caption leading-relaxed text-ink-400">
        These four colours paint the page around the verdict. The verdict
        colours stay fixed — green valid, amber superseded, red mismatch, grey
        not-found — so no rebrand can make one verdict look like another.
      </p>
    </div>
  );
}
