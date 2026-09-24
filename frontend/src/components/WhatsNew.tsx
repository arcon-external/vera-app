import { useCallback, useEffect, useState } from "react";
import { GetChangelog, Version } from "@backend/backend/UpdateAPI";
import { Button, Card, SectionHeading } from "./ui";

const SEEN_KEY = "vera-seen-version";

interface VersionNotes {
  version: string;
  sections: { title: string; items: string[] }[];
}

function parseSemver(value: string): [number, number, number] | null {
  const match = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(value.trim());
  if (!match) return null;
  return [Number(match[1]), Number(match[2]), Number(match[3])];
}

function compareSemver(a: string, b: string): number {
  const pa = parseSemver(a);
  const pb = parseSemver(b);
  if (!pa || !pb) return 0;
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] < pb[i] ? -1 : 1;
  }
  return 0;
}

/** Parse ## [x.y.z] blocks with ### sections of - bullets. Tolerant by design. */
function parseChangelog(raw: string): VersionNotes[] {
  const versions: VersionNotes[] = [];
  const blocks = raw.split(/^## \[/m).slice(1);
  for (const block of blocks) {
    const head = /^([^\]]+)\]/.exec(block);
    if (!head) continue;
    const version = head[1].trim();
    const sections: VersionNotes["sections"] = [];
    const parts = block.split(/^### /m).slice(1);
    for (const part of parts) {
      const lines = part.split("\n");
      const title = (lines[0] ?? "").trim();
      const items = lines
        .slice(1)
        .map((line) => line.trim())
        .filter((line) => line.startsWith("- "))
        .map((line) => line.slice(2).trim())
        .filter((line) => line !== "");
      if (title !== "" && items.length > 0) sections.push({ title, items });
    }
    versions.push({ version, sections });
  }
  return versions;
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
      if (cancelled || !parseSemver(current) || changelog.trim() === "") return;
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
          parseSemver(entry.version) !== null &&
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
      <div
        role="dialog"
        aria-modal="true"
        aria-label="What's new in Vera"
        className="max-h-[80vh] w-full max-w-xl overflow-y-auto rounded-xl border border-rule bg-white p-6 shadow-card"
        onClick={(event) => event.stopPropagation()}
      >
        <SectionHeading title="What's new" />
        <div className="grid gap-5">
          {notes.map((entry) => (
            <div key={entry.version}>
              <h3 className="tnum text-body font-semibold text-ink-950">
                v{entry.version.replace(/^v/, "")}
              </h3>
              <div className="mt-2 grid gap-3">
                {entry.sections.map((section) => (
                  <div key={section.title}>
                    <h4 className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
                      {section.title}
                    </h4>
                    <ul className="mt-1 grid list-disc gap-1 pl-5 text-small text-ink-700">
                      {section.items.map((item) => (
                        <li key={item}>{item}</li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
        <div className="mt-5">
          <Button variant="primary" onClick={onDismiss}>
            Got it
          </Button>
        </div>
      </div>
    </div>
  );
}
