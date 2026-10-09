import { useCallback, useEffect, useState } from "react";
import { GetChangelog, Version } from "@backend/backend/UpdateAPI";
import { BrowserOpenURL } from "../../wailsjs/runtime";
import {
  compareSemver,
  isReleaseVersion,
  parseChangelog,
  parseInline,
} from "../notes";
import type { InlineNode, VersionNotes } from "../notes";
import { Button, SectionHeading } from "./ui";

const SEEN_KEY = "vera-seen-version";

/**
 * Renders the inline markdown subset as React elements.
 *
 * Nodes, never HTML strings: React escapes every text node, so a release note
 * cannot introduce markup. This is the reason the parser exists in notes.ts
 * rather than as a string transform next to dangerouslySetInnerHTML.
 */
function Inline({ nodes }: { nodes: InlineNode[] }) {
  return (
    <>
      {nodes.map((node, index) => {
        switch (node.kind) {
          case "text":
            return <span key={index}>{node.value}</span>;
          case "bold":
            return (
              <strong key={index} className="font-semibold text-ink-900">
                {node.value}
              </strong>
            );
          case "italic":
            return <em key={index}>{node.value}</em>;
          case "code":
            return (
              <code
                key={index}
                className="rounded bg-paper-muted px-1 py-0.5 font-mono text-[0.85em] text-ink-800"
              >
                {node.value}
              </code>
            );
          case "link":
            return (
              <button
                key={index}
                type="button"
                onClick={() => BrowserOpenURL(node.url)}
                className="rounded text-left underline underline-offset-2 transition-colors duration-150 hover:text-accent-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
              >
                {node.label.map((child, childIndex) => (
                  <Inline key={childIndex} nodes={[child]} />
                ))}
              </button>
            );
        }
      })}
    </>
  );
}

function RichText({ text }: { text: string }) {
  return <Inline nodes={parseInline(text)} />;
}

// useWhatsNew shows a once-per-upgrade modal. Fresh installs stay silent
// (nothing changed for them) as do dev builds (unparseable versions).
export function useWhatsNew(signedIn: boolean): {
  notes: VersionNotes[];
  dismiss: () => void;
} {
  const [notes, setNotes] = useState<VersionNotes[]>([]);

  useEffect(() => {
    if (!signedIn) {
      setNotes([]);
      return;
    }
    let cancelled = false;
    (async () => {
      let current = "";
      let changelog = "";
      try {
        const [version, notes] = await Promise.all([Version(), GetChangelog()]);
        current = String(version);
        changelog = String(notes);
      } catch {
        // Offline or unreachable: stay silent until the next launch.
        return;
      }
      if (cancelled || !isReleaseVersion(current) || changelog.trim() === "") return;
      let seen: string | null = null;
      try {
        seen = window.localStorage.getItem(SEEN_KEY);
      } catch {
        return;
      }
      if (!seen) {
        // First run on this device: adopt the version silently.
        try {
          window.localStorage.setItem(SEEN_KEY, current);
        } catch {
          // Best-effort only.
        }
        return;
      }
      if (compareSemver(seen, current) >= 0) return;
      const fresh = parseChangelog(changelog).filter(
        (entry) =>
          isReleaseVersion(entry.version) &&
          compareSemver(seen as string, entry.version) < 0 &&
          compareSemver(entry.version, current) <= 0
      );
      if (!cancelled && fresh.length > 0) setNotes(fresh);
    })();
    return () => {
      cancelled = true;
    };
  }, [signedIn]);

  const dismiss = useCallback(() => {
    setNotes([]);
    Version()
      .then((current) => {
        try {
          window.localStorage.setItem(SEEN_KEY, String(current));
        } catch {
          // Best-effort only.
        }
      })
      .catch(() => {
        // Staying silent beats breaking Settings over a version stamp.
      });
  }, []);

  return { notes, dismiss };
}

export function WhatsNewModal({
  notes,
  onDismiss,
}: {
  notes: VersionNotes[];
  onDismiss: () => void;
}) {
  if (notes.length === 0) return null;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-5"
      style={{ backgroundColor: "rgba(28, 27, 25, 0.45)" }}
      onClick={onDismiss}
      role="presentation"
    >
      {/* A scrolling body with the dismiss action pinned: an upgrade that
          spans several releases is a long read, and a "Got it" button that
          only exists at the very bottom of it is a "scroll forever" trap. */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label="What's new in Vera"
        className="flex max-h-[80vh] w-full max-w-xl flex-col rounded-xl border border-rule bg-white shadow-card"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="shrink-0 px-6 pt-6">
          <SectionHeading title="What's new" />
        </div>
        <div className="grid gap-6 overflow-y-auto scroll-thin px-6 py-5">
          {notes.map((entry) => (
            <section key={entry.version}>
              <h3 className="tnum text-lead font-semibold text-ink-950">
                v{entry.version.replace(/^v/, "")}
              </h3>
              <div className="mt-3 grid gap-4">
                {entry.sections.map((section) => (
                  <div key={section.title}>
                    <h4 className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
                      {section.title}
                    </h4>
                    <div className="mt-1.5 grid gap-1.5">
                      {section.body.blocks.map((block, index) =>
                        block.kind === "list" ? (
                          <ul
                            key={index}
                            className="grid list-disc gap-1.5 pl-5 text-small leading-relaxed text-ink-700"
                          >
                            {block.items.map((item, itemIndex) => (
                              <li key={itemIndex}>
                                <RichText text={item} />
                              </li>
                            ))}
                          </ul>
                        ) : (
                          <p
                            key={index}
                            className="text-small leading-relaxed text-ink-700"
                          >
                            <RichText text={block.text} />
                          </p>
                        )
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>
        <div className="flex shrink-0 items-center justify-end gap-3 border-t border-rule px-6 py-4">
          {notes.length > 1 ? (
            <p className="text-caption text-ink-400">
              {notes.length} versions
            </p>
          ) : null}
          <Button variant="primary" onClick={onDismiss}>
            Got it
          </Button>
        </div>
      </div>
    </div>
  );
}

