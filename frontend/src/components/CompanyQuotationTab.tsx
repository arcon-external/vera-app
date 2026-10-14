import type { JSX } from "react";
import type { CompanyTheme } from "../company";
import type { CompanyLabelEntry } from "../companyLabels";
import { CompanyTemplatePanel } from "./CompanyTemplatePanel";
import { CompanyThemePanel } from "./CompanyThemePanel";
import { TemplatePreviewPanel } from "./TemplatePreviewPanel";

// The merged Quotation tab (issue #429): one sub-tab where Template and
// Quotation used to be two, in the Portal tab's shape — the document on
// top, everything that writes it below. Staff edit the wording and the
// colours while the real PDF is in view; splitting the halves across two
// tabs hid the preview from the wording editor and carried the colour
// panel twice.
//
// The two save paths stay separate and are named once, at the bottom: the
// wording autosaves through its own store, the four colours ride the
// profile save. The caption says both, and the preview shows both before
// either is saved.
//
// The blur/Enter capture wrapper covers the config stack only. The
// preview's Download button blurs too, and naming the whole tab would
// re-render the document on the way out of a download click — a render
// nobody asked for. Capture phase, so the wording inputs' own Enter
// blocker (which stops a partial profile save) does not swallow the
// refresh first.

export function CompanyQuotationTab(props: {
  /** The served wording entries, or null while they load. */
  labels: CompanyLabelEntry[] | null;
  /** The full theme; the preview and the colour panel read their four each. */
  theme: CompanyTheme;
  /** The parent's blur/Enter signal for the preview (see SettingsPage). */
  refreshKey: number;
  /** The live wording save state, owned by the parent's autosave. */
  labelsStatus: "saved" | "dirty" | "saving";
  /** A wording save error to show, or null. */
  labelsError: string | null;
  /** A non-fatal note (the save landed, the refresh did not), or null. */
  labelsNotice: string | null;
  /** Called with the entry key and its new raw value. */
  onLabelChange: (key: string, value: string) => void;
  /** Called with the theme field key and its new raw value. */
  onThemeChange: (key: keyof CompanyTheme, next: string) => void;
  /** Leaving a config field (capture phase) re-renders the preview. */
  onFieldBlur: () => void;
  /** Enter inside a config field (capture phase) re-renders the preview. */
  onFieldEnter: (event: React.KeyboardEvent) => void;
}): JSX.Element {
  const {
    labels,
    theme,
    refreshKey,
    labelsStatus,
    labelsError,
    labelsNotice,
    onLabelChange,
    onThemeChange,
    onFieldBlur,
    onFieldEnter,
  } = props;
  return (
    <div className="grid gap-6">
      <TemplatePreviewPanel labels={labels} theme={theme} refreshKey={refreshKey} />
      <div className="grid gap-6" onBlurCapture={onFieldBlur} onKeyDownCapture={onFieldEnter}>
        <CompanyTemplatePanel
          labels={labels}
          saveState={labelsStatus}
          error={labelsError}
          notice={labelsNotice}
          onChange={onLabelChange}
        />
        {/* The four quotation colours, through the same panel the old
            Quotation tab rendered: one UI over the theme columns, saved by
            the profile save below, never a second copy of a colour. */}
        <CompanyThemePanel section="Quotation" theme={theme} onChange={onThemeChange} />
        <p className="max-w-2xl text-caption leading-relaxed text-ink-400">
          Wording above saves automatically; the four colours save with{" "}
          <span className="font-medium text-ink-500">Save company profile</span>{" "}
          below — the preview shows both before you save.
        </p>
      </div>
    </div>
  );
}
