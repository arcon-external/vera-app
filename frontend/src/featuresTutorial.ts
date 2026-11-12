import type { SettingsStorage } from "./workspaceSettings";

// Issue #595: the Features Tutorial is a per-account, device-local learning
// state. It is a preference, not company data: completion lives beside the
// workspace defaults, never in Postgres, and a corrupt record simply means the
// tutorial is offered again. The tour itself may never write a quotation or
// purchase order; this module only remembers which workspace tours were seen.

export type FeaturesTutorialWorkspace = "quotation" | "purchase-order";

export const FEATURES_TUTORIAL_VERSION = 1;

export type FeaturesTutorialCompletion = Record<
  FeaturesTutorialWorkspace,
  boolean
>;

export const EMPTY_FEATURES_TUTORIAL: FeaturesTutorialCompletion = {
  quotation: false,
  "purchase-order": false,
};

interface StoredFeaturesTutorial {
  version: number;
  completed: Partial<Record<FeaturesTutorialWorkspace, boolean>>;
}

export function featuresTutorialKey(userId: number): string {
  return `vera-features-tutorial:${userId}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function completionFromRecord(
  completed: Record<string, unknown> | undefined
): FeaturesTutorialCompletion {
  return {
    quotation: completed?.quotation === true,
    "purchase-order": completed?.["purchase-order"] === true,
  };
}

/** Reads one account's completion. A missing/corrupt store means "not seen". */
export function readFeaturesTutorial(
  storage: SettingsStorage | null,
  userId: number | null
): FeaturesTutorialCompletion {
  if (storage === null || userId === null) {
    return { ...EMPTY_FEATURES_TUTORIAL };
  }
  try {
    const raw = storage.getItem(featuresTutorialKey(userId));
    if (raw === null || raw.trim() === "") {
      return { ...EMPTY_FEATURES_TUTORIAL };
    }
    const parsed: unknown = JSON.parse(raw);
    if (
      !isRecord(parsed) ||
      parsed.version !== FEATURES_TUTORIAL_VERSION ||
      !isRecord(parsed.completed)
    ) {
      return { ...EMPTY_FEATURES_TUTORIAL };
    }
    return completionFromRecord(parsed.completed);
  } catch {
    return { ...EMPTY_FEATURES_TUTORIAL };
  }
}

/**
 * Marks one workspace tour as seen. Completion is monotonic: replaying a tour
 * never clears the flag, and a storage failure leaves the in-memory answer
 * truthful for this session (the next reload offers it again).
 */
export function markFeaturesTutorialComplete(
  storage: SettingsStorage | null,
  userId: number | null,
  workspace: FeaturesTutorialWorkspace
): FeaturesTutorialCompletion {
  const current = readFeaturesTutorial(storage, userId);
  const next: FeaturesTutorialCompletion = { ...current, [workspace]: true };
  if (storage === null || userId === null) return next;
  try {
    const record: StoredFeaturesTutorial = {
      version: FEATURES_TUTORIAL_VERSION,
      completed: {
        quotation: next.quotation,
        "purchase-order": next["purchase-order"],
      },
    };
    storage.setItem(featuresTutorialKey(userId), JSON.stringify(record));
  } catch {
    // A locked-down profile is the same as a missing store: the tour has been
    // seen for this session; a reload may offer it again.
  }
  return next;
}

/** Clears one account's learning state. Used by tests and future reset tooling. */
export function clearFeaturesTutorial(
  storage: SettingsStorage | null,
  userId: number | null
): FeaturesTutorialCompletion {
  if (storage !== null && userId !== null) {
    try {
      storage.removeItem(featuresTutorialKey(userId));
    } catch {
      // Same fallback as the read/write paths.
    }
  }
  return { ...EMPTY_FEATURES_TUTORIAL };
}
