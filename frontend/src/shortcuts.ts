// Central keyboard-shortcut registry: defaults, per-device overrides,
// matching, and conflict validation. Every consumer (App handler, editor
// grid, help overlay, Keyboard settings) reads this module, so what the
// user configured is always what fires and what the docs show.
//
// Storage is localStorage, never the server: bindings are device-local
// (macOS Cmd vs Windows Ctrl, personal keyboards), must work offline, and
// carry no security value.

export type ActionId =
  | "exportPdf"
  | "prepare"
  | "newQuotation"
  | "gotoEditor"
  | "gotoArchive"
  | "gotoClients"
  | "gotoTeam"
  | "gotoSettings"
  | "toggleHelp"
  | "gridUp"
  | "gridDown"
  | "gridLeft"
  | "gridRight"
  | "gridEnter"
  | "gridEnterBack"
  | "archiveUp"
  | "archiveDown"
  | "archiveOpen";

export interface Binding {
  /** Command on macOS, Control on Windows/Linux. */
  mod: boolean;
  ctrl: boolean;
  shift: boolean;
  alt: boolean;
  /** Single key name: "s", "Enter", "ArrowUp", "?", "1" … */
  key: string;
}

export interface ActionDef {
  id: ActionId;
  label: string;
  hint: string;
  default: Binding;
}

export const SHORTCUT_ACTIONS: ActionDef[] = [
  { id: "exportPdf", label: "Export PDF", hint: "From the editor, when ready", default: { mod: true, ctrl: false, shift: false, alt: false, key: "s" } },
  { id: "prepare", label: "Prepare quotation", hint: "From the editor", default: { mod: true, ctrl: false, shift: false, alt: false, key: "Enter" } },
  { id: "newQuotation", label: "New quotation", hint: "Discard-confirm flow still applies", default: { mod: true, ctrl: false, shift: false, alt: false, key: "n" } },
  { id: "gotoEditor", label: "Go to Editor", hint: "", default: { mod: false, ctrl: false, shift: false, alt: true, key: "1" } },
  { id: "gotoArchive", label: "Go to Archive", hint: "", default: { mod: false, ctrl: false, shift: false, alt: true, key: "2" } },
  { id: "gotoClients", label: "Go to Clients", hint: "Admins only", default: { mod: false, ctrl: false, shift: false, alt: true, key: "3" } },
  { id: "gotoTeam", label: "Go to Team", hint: "Admins only", default: { mod: false, ctrl: false, shift: false, alt: true, key: "4" } },
  { id: "gotoSettings", label: "Go to Settings", hint: "", default: { mod: false, ctrl: false, shift: false, alt: true, key: "5" } },
  { id: "toggleHelp", label: "Shortcuts overlay", hint: "Also closes it", default: { mod: false, ctrl: false, shift: false, alt: false, key: "?" } },
  { id: "gridUp", label: "Grid: move up", hint: "Editor cells", default: { mod: false, ctrl: false, shift: false, alt: false, key: "ArrowUp" } },
  { id: "gridDown", label: "Grid: move down", hint: "Editor cells", default: { mod: false, ctrl: false, shift: false, alt: false, key: "ArrowDown" } },
  { id: "gridLeft", label: "Grid: move left", hint: "Editor cells", default: { mod: false, ctrl: false, shift: false, alt: false, key: "ArrowLeft" } },
  { id: "gridRight", label: "Grid: move right", hint: "Editor cells", default: { mod: false, ctrl: false, shift: false, alt: false, key: "ArrowRight" } },
  { id: "gridEnter", label: "Grid: Tab advance", hint: "Tab moves right (Enter moves down); Shift reverses either", default: { mod: false, ctrl: false, shift: false, alt: false, key: "Tab" } },
  { id: "gridEnterBack", label: "Grid: Shift+Tab advance", hint: "Shift+Tab moves left (Shift+Enter moves up)", default: { mod: false, ctrl: false, shift: true, alt: false, key: "Tab" } },
  { id: "archiveUp", label: "Archive: previous", hint: "Quotation list", default: { mod: false, ctrl: false, shift: false, alt: false, key: "ArrowUp" } },
  { id: "archiveDown", label: "Archive: next", hint: "Quotation list", default: { mod: false, ctrl: false, shift: false, alt: false, key: "ArrowDown" } },
  { id: "archiveOpen", label: "Archive: open", hint: "Quotation list", default: { mod: false, ctrl: false, shift: false, alt: false, key: "Enter" } },
];

export const SHORTCUT_STORAGE_KEY = "vera-shortcuts";

export type BindingMap = Partial<Record<ActionId, Binding>>;

function isMac(): boolean {
  try {
    return /mac/i.test(window.navigator.platform);
  } catch {
    return false;
  }
}

/** Merge stored overrides over defaults. Unknown ids are ignored. */
export function loadBindings(): Record<ActionId, Binding> {
  const merged = {} as Record<ActionId, Binding>;
  for (const action of SHORTCUT_ACTIONS) merged[action.id] = { ...action.default };
  try {
    const raw = window.localStorage.getItem(SHORTCUT_STORAGE_KEY);
    if (!raw) return merged;
    const parsed = JSON.parse(raw) as BindingMap;
    for (const action of SHORTCUT_ACTIONS) {
      const override = parsed[action.id];
      if (override && typeof override.key === "string" && override.key !== "") {
        merged[action.id] = {
          mod: !!override.mod,
          ctrl: !!override.ctrl,
          shift: !!override.shift,
          alt: !!override.alt,
          key: override.key,
        };
      }
    }
  } catch {
    // Corrupt JSON means defaults; never break the app over shortcuts.
  }
  return merged;
}

export function saveBindings(overrides: BindingMap): void {
  window.localStorage.setItem(SHORTCUT_STORAGE_KEY, JSON.stringify(overrides));
}

export function resetBindings(): void {
  try {
    window.localStorage.removeItem(SHORTCUT_STORAGE_KEY);
  } catch {
    // Best-effort only.
  }
}

/** True when the event target is an editable field. */
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName.toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return true;
  return target.isContentEditable;
}

function eventMatches(event: KeyboardEvent, binding: Binding): boolean {
  const modPressed = isMac() ? event.metaKey : event.ctrlKey;
  // On macOS Ctrl counts only when explicitly bound (avoids hijacking
  // Ctrl+click-style chords); on Windows mod IS ctrl.
  const ctrlPressed = isMac() ? event.ctrlKey : false;
  if (!!binding.mod !== modPressed) return false;
  if (!!binding.ctrl !== ctrlPressed) return false;
  if (!!binding.alt !== event.altKey) return false;
  const want = binding.key;
  const got = event.key.length === 1 ? event.key.toLowerCase() : event.key;
  if (got !== want.toLowerCase() && got !== want) return false;
  // Shifted punctuation ("?") implies its own Shift: the character alone
  // disambiguates, so don't demand shift equality for it. Letters keep
  // exact Shift matching so Shift+S stays distinct from s.
  if (want.length === 1 && !/[a-zA-Z0-9]/.test(want)) return true;
  return !!binding.shift === event.shiftKey;
}

/**
 * Match a keydown to an action id within one scope. Grid and archive
 * scopes are evaluated by their own containers, where focus sitting in an
 * input is expected — so the typing guard applies to the global scope
 * only, keeping bare keys (?, E, Alt+digits) out of text fields while
 * letting mod-bearing shortcuts (mod+S) fire anywhere.
 */
export function matchShortcut(
  event: KeyboardEvent,
  bindings: Record<ActionId, Binding>,
  scope: "global" | "grid" | "archive"
): ActionId | null {
  const typing = scope === "global" && isTypingTarget(event.target);
  const pool =
    scope === "grid"
      ? SHORTCUT_ACTIONS.filter((a) => a.id.startsWith("grid"))
      : scope === "archive"
        ? SHORTCUT_ACTIONS.filter((a) => a.id.startsWith("archive"))
        : SHORTCUT_ACTIONS.filter((a) => !a.id.startsWith("grid") && !a.id.startsWith("archive"));
  for (const action of pool) {
    if (typing) {
      const binding = bindings[action.id];
      const bare = !binding.mod && !binding.ctrl && !binding.alt;
      if (bare) continue;
    }
    if (eventMatches(event, bindings[action.id])) return action.id;
  }
  return null;
}

/** Human label for a binding, platform-aware (⌘ on macOS, Ctrl elsewhere). */
export function formatBinding(binding: Binding): string {
  const parts: string[] = [];
  if (binding.mod) parts.push(isMac() ? "⌘" : "Ctrl");
  if (binding.ctrl) parts.push("Ctrl");
  if (binding.alt) parts.push(isMac() ? "⌥" : "Alt");
  if (binding.shift) parts.push("⇧");
  let key = binding.key;
  if (key === " ") key = "Space";
  else if (key.length === 1) key = key.toUpperCase();
  parts.push(key);
  return parts.join(isMac() ? "" : "+");
}

/** Reject duplicate bindings across actions; returns a human error or "". */
export function validateBindings(bindings: Record<ActionId, Binding>): string {
  const seen = new Map<string, ActionId>();
  for (const action of SHORTCUT_ACTIONS) {
    const binding = bindings[action.id];
    const signature = [
      binding.mod ? "mod" : "",
      binding.ctrl ? "ctrl" : "",
      binding.shift ? "shift" : "",
      binding.alt ? "alt" : "",
      binding.key,
    ].join("+");
    const clash = seen.get(signature);
    if (clash) {
      const first = SHORTCUT_ACTIONS.find((a) => a.id === clash);
      return `${formatBinding(binding)} is already ${first?.label ?? clash}.`;
    }
    seen.set(signature, action.id);
  }
  return "";
}

/** Capture a keydown as a Binding for the rebind recorder. */
export function bindingFromEvent(event: KeyboardEvent): Binding | null {
  if (["Control", "Shift", "Alt", "Meta"].includes(event.key)) return null;
  return {
    mod: isMac() ? event.metaKey : event.ctrlKey,
    ctrl: isMac() ? event.ctrlKey : false,
    shift: event.shiftKey,
    alt: event.altKey,
    key: event.key.length === 1 ? event.key.toLowerCase() : event.key,
  };
}
