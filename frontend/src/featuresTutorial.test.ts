import { describe, expect, it } from "vitest";
import {
  EMPTY_FEATURES_TUTORIAL,
  clearFeaturesTutorial,
  featuresTutorialKey,
  markFeaturesTutorialComplete,
  readFeaturesTutorial,
} from "./featuresTutorial";

function memoryStorage() {
  const values = new Map<string, string>();
  return {
    values,
    getItem(key: string) {
      return values.get(key) ?? null;
    },
    setItem(key: string, value: string) {
      values.set(key, value);
    },
    removeItem(key: string) {
      values.delete(key);
    },
  };
}

describe("features tutorial persistence", () => {
  it("treats a missing store or account as not seen", () => {
    expect(readFeaturesTutorial(null, 7)).toEqual(EMPTY_FEATURES_TUTORIAL);
    expect(readFeaturesTutorial(memoryStorage(), null)).toEqual(
      EMPTY_FEATURES_TUTORIAL
    );
  });

  it("marks each workspace independently and reads the flags back", () => {
    const storage = memoryStorage();
    const first = markFeaturesTutorialComplete(storage, 7, "quotation");
    expect(first).toEqual({ quotation: true, "purchase-order": false });
    expect(readFeaturesTutorial(storage, 7)).toEqual({
      quotation: true,
      "purchase-order": false,
    });
    expect(readFeaturesTutorial(storage, 8)).toEqual(EMPTY_FEATURES_TUTORIAL);

    const second = markFeaturesTutorialComplete(
      storage,
      7,
      "purchase-order"
    );
    expect(second).toEqual({ quotation: true, "purchase-order": true });
  });

  it("keeps completion monotonic across a replay", () => {
    const storage = memoryStorage();
    markFeaturesTutorialComplete(storage, 7, "quotation");
    expect(
      markFeaturesTutorialComplete(storage, 7, "quotation")
    ).toEqual({ quotation: true, "purchase-order": false });
  });

  it("ignores a corrupt, wrong-version or wrong-shape record", () => {
    const storage = memoryStorage();
    storage.setItem(featuresTutorialKey(7), "{");
    expect(readFeaturesTutorial(storage, 7)).toEqual(EMPTY_FEATURES_TUTORIAL);

    storage.setItem(
      featuresTutorialKey(7),
      JSON.stringify({ version: 99, completed: { quotation: true } })
    );
    expect(readFeaturesTutorial(storage, 7)).toEqual(EMPTY_FEATURES_TUTORIAL);

    storage.setItem(
      featuresTutorialKey(7),
      JSON.stringify({ version: 1, completed: "yes" })
    );
    expect(readFeaturesTutorial(storage, 7)).toEqual(EMPTY_FEATURES_TUTORIAL);
  });

  it("clears one account without touching another", () => {
    const storage = memoryStorage();
    markFeaturesTutorialComplete(storage, 7, "quotation");
    markFeaturesTutorialComplete(storage, 8, "purchase-order");
    expect(clearFeaturesTutorial(storage, 7)).toEqual(EMPTY_FEATURES_TUTORIAL);
    expect(readFeaturesTutorial(storage, 7)).toEqual(EMPTY_FEATURES_TUTORIAL);
    expect(readFeaturesTutorial(storage, 8)).toEqual({
      quotation: false,
      "purchase-order": true,
    });
  });

  it("survives a storage that throws", () => {
    const throwing = {
      getItem() {
        throw new Error("blocked");
      },
      setItem() {
        throw new Error("blocked");
      },
      removeItem() {
        throw new Error("blocked");
      },
    };
    expect(readFeaturesTutorial(throwing, 7)).toEqual(EMPTY_FEATURES_TUTORIAL);
    expect(
      markFeaturesTutorialComplete(throwing, 7, "quotation")
    ).toEqual({ quotation: true, "purchase-order": false });
  });
});
