// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  FeaturesTutorial,
  emitFeaturesTutorialAdvance,
  type FeaturesTutorialStep,
} from "./FeaturesTutorial";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

const steps: FeaturesTutorialStep[] = [
  {
    id: "one",
    target: '[data-tutorial="first"]',
    title: "First",
    body: "First body",
  },
  {
    id: "two",
    target: '[data-tutorial="second"]',
    title: "Second",
    body: "Second body",
    final: true,
  },
];

function Harness({
  onClose,
  onNavigate = () => undefined,
  steps: tutorialSteps = steps,
}: {
  onClose: (reason: "finished" | "skipped") => void;
  onNavigate?: () => void;
  steps?: FeaturesTutorialStep[];
}) {
  return (
    <div>
      <button type="button" data-tutorial="first">
        First target
      </button>
      <button type="button" data-tutorial="second">
        Second target
      </button>
      <FeaturesTutorial
        workspace="quotation"
        steps={tutorialSteps}
        onNavigate={onNavigate}
        onClose={onClose}
      />
    </div>
  );
}

async function flush(ms = 80): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });
}

beforeEach(() => {
  Element.prototype.scrollIntoView = () => undefined;
  if (typeof window.requestAnimationFrame !== "function") {
    window.requestAnimationFrame = ((callback: FrameRequestCallback) =>
      window.setTimeout(() => callback(performance.now()), 0)) as typeof window.requestAnimationFrame;
    window.cancelAnimationFrame = ((id: number) =>
      window.clearTimeout(id)) as typeof window.cancelAnimationFrame;
  }
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.useRealTimers();
});

describe("FeaturesTutorial", () => {
  it("walks forward and back, then skips", async () => {
    const onClose = vi.fn();
    act(() => root.render(<Harness onClose={onClose} />));
    await flush();

    expect(document.body.textContent).toContain("First");
    expect(document.body.textContent).toContain("Step 1 of 2");
    act(() => {
      document
        .querySelector<HTMLButtonElement>('button[aria-label="Next step"]')
        ?.click();
    });
    await flush();
    expect(document.body.textContent).toContain("Second");
    expect(document.body.textContent).toContain("Step 2 of 2");

    act(() => {
      document
        .querySelector<HTMLButtonElement>('button[aria-label="Previous step"]')
        ?.click();
    });
    await flush();
    expect(document.body.textContent).toContain("First");

    act(() => {
      document
        .querySelector<HTMLButtonElement>('button[aria-label="Skip Features Tutorial"]')
        ?.click();
    });
    expect(onClose).toHaveBeenCalledWith("skipped");
  });

  it("finishes on the last step", async () => {
    const onClose = vi.fn();
    act(() => root.render(<Harness onClose={onClose} />));
    await flush();
    act(() => {
      document
        .querySelector<HTMLButtonElement>('button[aria-label="Next step"]')
        ?.click();
    });
    await flush();
    act(() => {
      document
        .querySelector<HTMLButtonElement>('button[aria-label="Finish Features Tutorial"]')
        ?.click();
    });
    expect(onClose).toHaveBeenCalledWith("finished");
  });

  it("advances when the highlighted control is clicked", async () => {
    const onClose = vi.fn();
    const clickSteps: FeaturesTutorialStep[] = [
      {
        id: "click",
        target: '[data-tutorial="first"]',
        title: "Click it",
        body: "Use the control.",
        advance: "click",
      },
      {
        id: "done",
        target: '[data-tutorial="second"]',
        title: "Done",
        body: "Next.",
        final: true,
      },
    ];
    act(() =>
      root.render(<Harness onClose={onClose} steps={clickSteps} />)
    );
    await flush();
    act(() => {
      document
        .querySelector<HTMLButtonElement>('[data-tutorial="first"]')
        ?.click();
    });
    await flush();
    expect(document.body.textContent).toContain("Done");
  });

  it("advances on the app's tutorial event", async () => {
    const onClose = vi.fn();
    const eventSteps: FeaturesTutorialStep[] = [
      {
        id: "event",
        target: '[data-tutorial="first"]',
        title: "Waiting",
        body: "Wait for the app.",
        advance: "event",
        advanceEvent: "linked",
      },
      {
        id: "done",
        target: '[data-tutorial="second"]',
        title: "Done",
        body: "Next.",
        final: true,
      },
    ];
    act(() =>
      root.render(<Harness onClose={onClose} steps={eventSteps} />)
    );
    await flush();
    act(() => emitFeaturesTutorialAdvance("linked"));
    await flush();
    expect(document.body.textContent).toContain("Done");
  });

  it("uses the keyboard: arrows move, Escape skips", async () => {
    const onClose = vi.fn();
    act(() => root.render(<Harness onClose={onClose} />));
    await flush();
    act(() => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    });
    await flush();
    expect(document.body.textContent).toContain("Second");
    act(() => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });
    expect(onClose).toHaveBeenCalledWith("skipped");
  });

  it("navigates to a step's page before locating its target", async () => {
    const onNavigate = vi.fn();
    const pageSteps: FeaturesTutorialStep[] = [
      {
        id: "page",
        target: '[data-tutorial="second"]',
        page: "drafts",
        title: "Drafts",
        body: "The list.",
        final: true,
      },
    ];
    act(() =>
      root.render(<Harness onClose={vi.fn()} onNavigate={onNavigate} steps={pageSteps} />)
    );
    await flush();
    expect(onNavigate).toHaveBeenCalledWith("drafts");
    expect(document.body.textContent).toContain("Drafts");
  });

  it("can be hidden so the highlighted control is usable, then shown again", async () => {
    const onClose = vi.fn();
    act(() => root.render(<Harness onClose={onClose} />));
    await flush();
    act(() => {
      document
        .querySelector<HTMLButtonElement>('button[aria-label="Hide Features Tutorial guide"]')
        ?.click();
    });
    await flush();
    expect(document.body.textContent).not.toContain("First body");
    expect(document.body.textContent).toContain("Features Tutorial · 1/2");

    act(() => {
      document
        .querySelector<HTMLButtonElement>('button[aria-label="Show Features Tutorial guide"]')
        ?.click();
    });
    await flush();
    expect(document.body.textContent).toContain("First body");
  });

  it("shows a centered tooltip when the target never appears", async () => {
    vi.useFakeTimers();
    const missing: FeaturesTutorialStep[] = [
      {
        id: "missing",
        target: '[data-tutorial="never"]',
        title: "Missing target",
        body: "The target is gone.",
        final: true,
      },
    ];
    act(() => root.render(<Harness onClose={vi.fn()} steps={missing} />));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2700);
    });
    expect(document.body.textContent).toContain("Missing target");
    expect(document.body.textContent).toContain("Finish");
  });
});
