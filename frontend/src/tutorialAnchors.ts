// Issue #601 review: the tutorial anchors and events are shared vocabulary
// between the product components and the Features Tutorial scripts. They live
// in this neutral module so `components/` never imports from
// `featuresTutorial/`, which imports those same components back. Keep the
// values here stable: a product control writes one and a tutorial step reads
// it, so renaming one side alone silently breaks the guide.

// Issue #604: the quote-bar lesson. The reveal key forces the inline bar open
// without focusing the field; the choice key additionally lets the bar's two
// actions resolve the tour step. The event name is the bar's only tutorial
// output, so no resolution is ever reported outside that step.
export const QUOTE_BAR_REVEAL = "quote-bar";
export const QUOTE_BAR_CHOICE_REVEAL = "quote-bar-choice";
export const QUOTE_BAR_RESOLVED_EVENT = "po-tutorial-quote-resolved";

// Issue #606: the ghost lesson. The action group is the ✓/✕ pair the guide
// points at, and either action reports the same resolution event. The DOM
// anchor only exists while the PO editor is in tutorial mode, so a real ghost
// outside the tour never carries tutorial state.
export const GHOST_ACTIONS_ANCHOR = "po-ghost-actions";
export const GHOST_RESOLVED_EVENT = "po-ghost-resolved";
