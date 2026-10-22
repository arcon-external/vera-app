import { useMemo } from "react";
import {
  SHORTCUT_ACTIONS,
  formatBinding,
  loadBindings,
  type ActionId,
} from "../shortcuts";
import { Button } from "./ui";

const GROUPS: { title: string; ids: ActionId[] }[] = [
  {
    title: "Editor",
    ids: ["exportPdf", "prepare", "newQuotation", "gridUp", "gridDown", "gridLeft", "gridRight", "gridEnter", "gridEnterBack"],
  },
  {
    title: "Pages",
    ids: ["gotoHome", "gotoEditor", "gotoDrafts", "gotoArchive", "gotoPurchaseOrder", "gotoAdmin", "gotoSettings", "toggleHelp"],
  },
  {
    title: "Archive",
    ids: ["archiveUp", "archiveDown", "archiveOpen"],
  },
];

// ShortcutsHelp is the ? overlay: a live view of the registry, so
// customized bindings always show their real keys. Esc or backdrop closes.
export function ShortcutsHelp({ onClose }: { onClose: () => void }) {
  const bindings = useMemo(() => loadBindings(), []);
  return (
    <div
      className="bg-ink-950/45 fixed inset-0 z-50 flex items-center justify-center p-5"
      onClick={onClose}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Keyboard shortcuts"
        className="max-h-[80vh] w-full max-w-xl overflow-y-auto rounded-xl border border-rule bg-white p-6 shadow-card"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between gap-3">
          <h2 className="text-title font-semibold text-ink-950">
            Keyboard shortcuts
          </h2>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>
        <div className="grid gap-5 sm:grid-cols-2">
          {GROUPS.map((group) => (
            <div key={group.title}>
              <h3 className="mb-2 text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
                {group.title}
              </h3>
              <dl className="grid gap-3">
                {group.ids.map((id) => {
                  const action = SHORTCUT_ACTIONS.find((a) => a.id === id);
                  if (!action) return null;
                  return (
                    <div key={id} className="flex items-baseline justify-between gap-3 text-small">
                      <dt className="text-ink-700">{action.label}</dt>
                      <dd className="tnum shrink-0 rounded-md border border-rule bg-paper-subtle px-2 py-0.5 font-mono text-caption text-ink-800">
                        {formatBinding(bindings[id])}
                      </dd>
                    </div>
                  );
                })}
              </dl>
            </div>
          ))}
        </div>
        <p className="mt-4 text-caption leading-relaxed text-ink-400">
          Grid keys: arrows move, Tab advances right, Enter advances down,
          Shift reverses either. Change any binding in Account settings →
          Keyboard.
        </p>
      </div>
    </div>
  );
}
