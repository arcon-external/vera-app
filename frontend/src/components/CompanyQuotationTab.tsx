import type { JSX } from "react";
import type { CompanyTheme } from "../company";
import type { CompanyLabelEntry } from "../companyLabels";
import { COMPANY_TAB_SAVES } from "../companySaves";
import { CompanyTemplatePanel } from "./CompanyTemplatePanel";
import { CompanyThemePanel } from "./CompanyThemePanel";
import { TemplatePreviewPanel, templatePreviewColorError } from "./TemplatePreviewPanel";
import { SaveRow, type SaveState } from "./ui";

// The merged Quotation tab (issue #429, save model #432): one sub-tab where
// Template and Quotation used to be two, in the Portal tab's shape — the
// document on top, everything that writes it below. Staff edit the colours and
// the wording while the real document is in view; splitting the halves across
// two tabs hid the preview from the wording editor and carried the colour
// panel twice.
//
// The order is deliberate and pinned: preview, colours, wording, the save row,
// then the two standing notes. The colours sit above the wording because they
// are the smaller decision — four fields against twenty-six — and the save row
// sits below both because it saves both. Nothing here autosaves (#432): the
// wording used to save itself after a pause, which cost a round trip per typo
// and a save nobody watched. One button, one chip, one guard now.
//
// The blur/Enter capture wrapper covers the config stack only. The preview's
// Download button blurs too, and naming the whole tab would re-render the
// document on the way out of a download click — a render nobody asked for.
// Capture phase, so the wording inputs' own Enter blocker does not swallow the
// refresh first.

export function CompanyQuotationTab(props: {
  /** The served wording entries, or null while they load. */
  labels: CompanyLabelEntry[] | null;
  /** The full theme; the preview and the colour panel read their four each. */
  theme: CompanyTheme;
  /** The parent's blur/Enter signal for the preview (see SettingsPage). */
  refreshKey: number;
  /** The wording half's live save state, for the wording panel's chip. */
  wordingStatus: SaveState;
  /** The whole tab's live save state, for the save row's chip. */
  tabStatus: SaveState;
  /** A wording save error to show, or null. */
  labelsError: string | null;
  /** A non-fatal note (the save landed, the refresh did not), or null. */
  labelsNotice: string | null;
  /** A colour save error to show, or null. */
  themeError: string | null;
  /** Why the save is unavailable: a wording value the store would refuse. */
  invalid: string | null;
  /** True while either half's save is in flight. */
  busy: boolean;
  /** Called with the entry key and its new raw value. */
  onLabelChange: (key: string, value: string) => void;
  /** Called with the theme field key and its new raw value. */
  onThemeChange: (key: keyof CompanyTheme, next: string) => void;
  /** Saves the wording and the colours, each only when it changed. */
  onSave: () => void;
  /** Leaving a config field (capture phase) re-renders the preview. */
  onFieldBlur: () => void;
  /** Enter inside a config field (capture phase) re-renders the preview. */
  onFieldEnter: (event: React.KeyboardEvent) => void;
}): JSX.Element {
  const {
    labels,
    theme,
    refreshKey,
    wordingStatus,
    tabStatus,
    labelsError,
    labelsNotice,
    themeError,
    invalid,
    busy,
    onLabelChange,
    onThemeChange,
    onSave,
    onFieldBlur,
    onFieldEnter,
  } = props;
  // The four colours are strict hex or empty (the stylesheet default), and the
  // store refuses anything else — so the row refuses it first, naming the field
  // the same way the preview panel does. A button that can only bounce is a
  // button that teaches people to distrust the save.
  const badHex = templatePreviewColorError(theme);
  const colourRefusal =
    badHex === null ? null : `The Quotation ${badHex} colour is not a complete hex — fix it before saving.`;
  const refusal = invalid ?? colourRefusal;
  return (
    <div className="grid gap-6">
      <TemplatePreviewPanel labels={labels} theme={theme} refreshKey={refreshKey} />
      <div className="grid gap-6" onBlurCapture={onFieldBlur} onKeyDownCapture={onFieldEnter}>
        {/* The four quotation colours, through the same panel the old
            Quotation tab rendered: one UI over the theme columns, saved by the
            row below, never a second copy of a colour. */}
        <CompanyThemePanel section="Quotation" theme={theme} onChange={onThemeChange} />
        <CompanyTemplatePanel
          labels={labels}
          saveState={wordingStatus}
          error={labelsError}
          notice={labelsNotice}
          onChange={onLabelChange}
        />
      </div>
      <SaveRow
        label={COMPANY_TAB_SAVES.quotation.button}
        busyLabel={COMPANY_TAB_SAVES.quotation.busy}
        busy={busy}
        state={tabStatus}
        onSave={onSave}
        invalid={refusal}
        error={themeError}
        hint="The wording and the four colours save together with this button. The preview above shows both before you save."
      />
      <p className="max-w-2xl text-caption leading-relaxed text-ink-400">
        These four colours paint the whole document. The white page and the
        green discount row stay fixed, so no rebrand can wash out the paper or
        turn a discount into a warning.
      </p>
      <p className="max-w-2xl text-caption leading-relaxed text-ink-400">
        The preview above shows the draft — wording and colours together —
        before either is saved.
      </p>
    </div>
  );
}
