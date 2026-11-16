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
  onReveal,
  steps: tutorialSteps = steps,
}: {
  onClose: (reason: "finished" | "skipped") => void;
  onNavigate?: () => void;
  onReveal?: (reveal: string | null) => void;
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
        onReveal={onReveal}
        onClose={onClose}
      />
    </div>
  );
}

async function flush(ms = 320): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });
}

function motionPhase(): string | null {
  return (
    document.querySelector<HTMLElement>("[data-features-tutorial]")?.dataset
      .featuresTutorialPhase ?? null
  );
}

function nextButton(): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    'button[aria-label="Next step"]'
  );
  if (found === null) throw new Error("no Next step button");
  return found;
}

beforeEach(() => {
  Element.prototype.scrollIntoView = () => undefined;
  // Deterministic across real and fake timers: a frame is a timer, so a test
  // that advances timers also runs the measurement frame.
  window.requestAnimationFrame = ((callback: FrameRequestCallback) =>
    window.setTimeout(
      () => callback(performance.now()),
      16
    )) as typeof window.requestAnimationFrame;
  window.cancelAnimationFrame = ((id: number) =>
    window.clearTimeout(id)) as typeof window.cancelAnimationFrame;
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
    expect(document.body.textContent).toContain("Continue tutorial");
    expect(document.body.textContent).toContain("1/2");

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

  it("leaves the old step before navigating and enters only after measuring", async () => {
    vi.useFakeTimers();
    const onClose = vi.fn();
    const onNavigate = vi.fn();
    const pageSteps: FeaturesTutorialStep[] = [
      {
        id: "one",
        target: '[data-tutorial="first"]',
        title: "First",
        body: "First body",
      },
      {
        id: "two",
        target: '[data-tutorial="second"]',
        page: "drafts",
        title: "Second",
        body: "Second body",
        final: true,
      },
    ];
    act(() =>
      root.render(
        <Harness
          onClose={onClose}
          onNavigate={onNavigate}
          steps={pageSteps}
        />
      )
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(motionPhase()).toBe("idle");

    act(() => nextButton().click());
    // Leaving: the old step is still on screen and the page has not moved.
    expect(motionPhase()).toBe("leaving");
    expect(document.body.textContent).toContain("First body");
    expect(onNavigate).not.toHaveBeenCalled();
    expect(
      document.querySelector("[data-features-tutorial-veil]")
    ).toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100);
    });
    expect(motionPhase()).toBe("leaving");
    expect(onNavigate).not.toHaveBeenCalled();

    // The page change happens behind the veil, with the old step gone.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30);
    });
    expect(onNavigate).toHaveBeenCalledWith("drafts");
    expect(
      document.querySelector("[data-features-tutorial-veil]")
    ).not.toBeNull();
    expect(motionPhase()).toBe("leaving");

    // Entering waits for the target to be measured, not just for the timer.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(200);
    });
    expect(motionPhase()).toBe("entering");
    expect(
      document.querySelector("[data-tutorial-spotlight]")
    ).not.toBeNull();
    expect(
      document.querySelector("[data-features-tutorial-veil]")
    ).toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(motionPhase()).toBe("idle");
    expect(document.body.textContent).toContain("Second body");
  });

  it("puts a pointer-safe beacon outside an interactive target and none on a read-only step", async () => {
    const clickSteps: FeaturesTutorialStep[] = [
      {
        id: "click",
        target: '[data-tutorial="first"]',
        title: "Use the control",
        body: "Click the control.",
        advance: "click",
      },
      {
        id: "done",
        target: '[data-tutorial="second"]',
        title: "Done",
        body: "Read only.",
        final: true,
      },
    ];
    act(() => root.render(<Harness onClose={vi.fn()} steps={clickSteps} />));
    const target = document.querySelector<HTMLElement>(
      '[data-tutorial="first"]'
    );
    if (target === null) throw new Error("no first target");
    target.getBoundingClientRect = () =>
      ({
        top: 200,
        left: 300,
        width: 120,
        height: 40,
        bottom: 240,
        right: 420,
        x: 300,
        y: 200,
        toJSON: () => ({}),
      }) as DOMRect;
    await flush();

    const beacon = document.querySelector<HTMLElement>(
      "[data-tutorial-beacon]"
    );
    expect(beacon).not.toBeNull();
    expect(beacon?.textContent).toContain("Click");
    expect(beacon?.className).toContain("pointer-events-none");

    const ring = document.querySelector<HTMLElement>(
      "[data-tutorial-beacon-ring]"
    );
    expect(ring).not.toBeNull();
    // The ring wraps the control with its padding outside the rectangle.
    expect(Number.parseFloat(ring?.style.top ?? "NaN")).toBe(193);
    expect(Number.parseFloat(ring?.style.height ?? "NaN")).toBe(54);

    const pill = document.querySelector<HTMLElement>(
      "[data-tutorial-beacon-pill]"
    );
    expect(pill).not.toBeNull();
    // Above the target: 200 - 10 gap - 26 pill height, so it never covers it.
    expect(Number.parseFloat(pill?.style.top ?? "NaN")).toBe(164);

    // The next step is read-only: no cue, because there is nothing to press.
    act(() => target.click());
    await flush();
    expect(document.body.textContent).toContain("Read only.");
    expect(document.querySelector("[data-tutorial-beacon]")).toBeNull();
  });

  it("keeps the fade but drops movement under prefers-reduced-motion", async () => {
    const originalMatchMedia =
      typeof window.matchMedia === "function" ? window.matchMedia : undefined;
    window.matchMedia = ((query: string) => ({
      matches: query.includes("prefers-reduced-motion"),
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    })) as typeof window.matchMedia;
    try {
      const clickSteps: FeaturesTutorialStep[] = [
        {
          id: "click",
          target: '[data-tutorial="first"]',
          title: "Use the control",
          body: "Click the control.",
          advance: "click",
        },
        {
          id: "done",
          target: '[data-tutorial="second"]',
          title: "Done",
          body: "Read only.",
          final: true,
        },
      ];
      vi.useFakeTimers();
      act(() => root.render(<Harness onClose={vi.fn()} steps={clickSteps} />));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(300);
      });

      const wrapper = document.querySelector<HTMLElement>(
        "[data-features-tutorial]"
      );
      expect(wrapper?.dataset.featuresTutorialMotion).toBe("reduced");
      const tooltip = document.querySelector<HTMLElement>('[role="dialog"]');
      expect(tooltip?.style.transition).toBe("opacity 120ms linear");
      expect(tooltip?.style.transition).not.toContain("left");
      const ring = document.querySelector<HTMLElement>(
        "[data-tutorial-beacon-ring]"
      );
      expect(ring?.style.transition).toBe("opacity 120ms linear");
      expect(
        document.querySelector("[data-tutorial-beacon-pulse]")
      ).toBeNull();

      // The motion phases still run: fade out, swap, fade in. Separate timer
      // advances give React a chance to flush the step effect in between.
      act(() => nextButton().click());
      expect(motionPhase()).toBe("leaving");
      await act(async () => {
        await vi.advanceTimersByTimeAsync(130);
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(300);
      });
      expect(document.body.textContent).toContain("Read only.");
      expect(motionPhase()).not.toBe("leaving");
    } finally {
      if (originalMatchMedia === undefined) {
        delete (window as unknown as { matchMedia?: unknown }).matchMedia;
      } else {
        window.matchMedia = originalMatchMedia;
      }
    }
  });

  it("slides between two steps of the same page without a page veil", async () => {
    vi.useFakeTimers();
    const samePageSteps: FeaturesTutorialStep[] = [
      {
        id: "one",
        target: '[data-tutorial="first"]',
        page: "create",
        title: "First",
        body: "First body",
      },
      {
        id: "two",
        target: '[data-tutorial="second"]',
        page: "create",
        title: "Second",
        body: "Second body",
        final: true,
      },
    ];
    act(() => root.render(<Harness onClose={vi.fn()} steps={samePageSteps} />));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    act(() => nextButton().click());
    expect(motionPhase()).toBe("leaving");
    expect(document.querySelector("[data-features-tutorial-veil]")).toBeNull();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    // The spotlight moved in place; no paper flash between two steps that are
    // already on the same page.
    expect(document.querySelector("[data-features-tutorial-veil]")).toBeNull();
    expect(document.body.textContent).toContain("Second body");
    expect(motionPhase()).not.toBe("leaving");
  });

  it("names each beacon by the action the step expects", async () => {
    vi.useFakeTimers();
    const actionSteps: FeaturesTutorialStep[] = [
      {
        id: "focus",
        target: '[data-tutorial="first"]',
        title: "Press the field",
        body: "Focus it.",
        advance: "focus",
      },
      {
        id: "event",
        target: '[data-tutorial="second"]',
        title: "Choose a value",
        body: "Wait for the app.",
        advance: "event",
        advanceEvent: "chosen",
        final: true,
      },
    ];
    act(() => root.render(<Harness onClose={vi.fn()} steps={actionSteps} />));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(
      document.querySelector("[data-tutorial-beacon]")?.textContent
    ).toContain("Press");

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })
      );
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(
      document.querySelector("[data-tutorial-beacon]")?.textContent
    ).toContain("Select");
  });

  it("reports the active step's reveal key and clears it when the step ends", async () => {
    const reveals: (string | null)[] = [];
    const revealSteps: FeaturesTutorialStep[] = [
      {
        id: "reveal",
        target: '[data-tutorial="first"]',
        title: "Open the bar",
        body: "The bar is open.",
        reveal: "quote-bar",
      },
      {
        id: "after",
        target: '[data-tutorial="second"]',
        title: "After",
        body: "The bar closes.",
        final: true,
      },
    ];
    act(() =>
      root.render(
        <Harness
          onClose={vi.fn()}
          steps={revealSteps}
          onReveal={(key) => reveals.push(key)}
        />
      )
    );
    await flush();
    expect(reveals.at(-1)).toBe("quote-bar");

    act(() => nextButton().click());
    await flush();
    expect(reveals.at(-1)).toBeNull();
  });
});
