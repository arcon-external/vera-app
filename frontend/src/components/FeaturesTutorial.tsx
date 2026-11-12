import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import type { ReactNode, WheelEvent } from "react";
import type { AppPage } from "../poLeaveGuard";
import type { FeaturesTutorialWorkspace } from "../featuresTutorial";

// Issue #595: the Features Tutorial engine. It is deliberately a coach-mark
// layer, not a modal: the spotlight leaves the real target clickable, while
// every other pointer target is dimmed. The step content and navigation are
// supplied by the workspace scripts; this component owns only geometry,
// keyboard/focus, and the Back/Next/Skip state machine.

/** Window event used by a step that waits for the app to finish an action. */
export const FEATURES_TUTORIAL_ADVANCE_EVENT = "vera:features-tutorial-advance";

export function emitFeaturesTutorialAdvance(key: string): void {
  window.dispatchEvent(
    new CustomEvent(FEATURES_TUTORIAL_ADVANCE_EVENT, { detail: { key } })
  );
}

export interface FeaturesTutorialAction {
  label: string;
  action: string;
}

export interface FeaturesTutorialStep {
  id: string;
  /** Full CSS selector, normally `[data-tutorial="..."]`. */
  target: string;
  title: string;
  body: ReactNode;
  /** Navigate before locating the target. */
  page?: AppPage;
  placement?: "auto" | "top" | "bottom" | "left" | "right";
  /**
   * `next` is a read-only explanation. `click` and `focus` advance when the
   * user actually uses the highlighted control. `event` waits for the app to
   * emit FEATURES_TUTORIAL_ADVANCE_EVENT with the matching key.
   */
  advance?: "next" | "click" | "focus" | "event";
  advanceEvent?: string;
  /** Optional action shown in the tooltip, e.g. reopening the practice selector. */
  action?: FeaturesTutorialAction;
  /**
   * A modal owns its own focus and backdrop. `none` leaves it fully usable
   * and docks the guide at the bottom-left instead of drawing a box around it.
   */
  spotlight?: "target" | "none";
  /** Action to run when the step is left with Back or Next. */
  onLeave?: string;
  /** The last step's primary button is Finish. */
  final?: boolean;
}

export interface FeaturesTutorialProps {
  workspace: FeaturesTutorialWorkspace;
  steps: FeaturesTutorialStep[];
  onNavigate: (page: AppPage) => void;
  onClose: (reason: "finished" | "skipped") => void;
  onAction?: (action: string) => void;
}

interface TooltipPosition {
  left: number;
  top: number;
  width: number;
  height: number;
  placement: "top" | "bottom" | "left" | "right" | "center" | "sheet" | "dock";
}

const TOOLTIP_MARGIN = 12;
const TOOLTIP_GAP = 14;
const TARGET_PADDING = 6;
const TARGET_TIMEOUT_MS = 2500;

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return false;
    }
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  });
  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReduced(query.matches);
    query.addEventListener?.("change", update);
    return () => query.removeEventListener?.("change", update);
  }, []);
  return reduced;
}

/** Avoids stealing arrow keys from a field the user is typing in. */
function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName.toLowerCase();
  return (
    tag === "input" ||
    tag === "textarea" ||
    tag === "select" ||
    target.isContentEditable
  );
}

export function FeaturesTutorial({
  workspace,
  steps,
  onNavigate,
  onClose,
  onAction,
}: FeaturesTutorialProps) {
  const safeSteps = steps.length > 0 ? steps : [];
  const [index, setIndex] = useState(0);
  const [rect, setRect] = useState<DOMRect | null>(null);
  const [targetMissing, setTargetMissing] = useState(false);
  const [position, setPosition] = useState<TooltipPosition | null>(null);
  // A guide that covers the control it is explaining is worse than no guide.
  // Hide collapses the overlay so the user can use the selector or form; the
  // next step re-opens it with the next explanation.
  const [collapsed, setCollapsed] = useState(false);
  const tooltipRef = useRef<HTMLDivElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const navigateRef = useRef(onNavigate);
  navigateRef.current = onNavigate;
  const reducedMotion = useReducedMotion();
  const step = safeSteps[Math.min(index, Math.max(0, safeSteps.length - 1))];
  const finalStep = step?.final === true || index === safeSteps.length - 1;

  useEffect(() => {
    setCollapsed(false);
  }, [index]);

  const leave = useCallback(() => {
    if (step?.onLeave !== undefined) onAction?.(step.onLeave);
  }, [step?.onLeave, onAction]);

  const next = useCallback(() => {
    leave();
    if (index >= safeSteps.length - 1) {
      onClose("finished");
      return;
    }
    setIndex((current) => Math.min(current + 1, safeSteps.length - 1));
  }, [index, leave, onClose, safeSteps.length]);

  const back = useCallback(() => {
    leave();
    setIndex((current) => Math.max(0, current - 1));
  }, [leave]);

  const skip = useCallback(() => onClose("skipped"), [onClose]);

  useEffect(() => {
    previousFocusRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    return () => {
      previousFocusRef.current?.focus?.();
    };
  }, []);

  // Enter a step: navigate, wait for the target, measure it, and bring it into
  // view. A missing target is a graceful state, never a crash or a dead tour.
  useEffect(() => {
    if (step === undefined) return;
    setTargetMissing(false);
    setRect(null);
    if (step.page !== undefined) navigateRef.current(step.page);
    let cancelled = false;
    let timer: number | undefined;
    const started = performance.now();
    const find = () => {
      if (cancelled) return;
      const element = document.querySelector<HTMLElement>(step.target);
      if (element !== null) {
        if (typeof element.scrollIntoView === "function") {
          // The spotlight is measured immediately after this call. A smooth
          // scroll would leave the first measurement at the old position and
          // briefly cover the target; an instant scroll keeps the guide honest.
          element.scrollIntoView({
            block: "center",
            inline: "nearest",
            behavior: "auto",
          });
        }
        const measure = () => {
          if (cancelled) return;
          setRect(element.getBoundingClientRect());
        };
        if (typeof window.requestAnimationFrame === "function") {
          window.requestAnimationFrame(measure);
        } else {
          window.setTimeout(measure, 0);
        }
        return;
      }
      if (performance.now() - started >= TARGET_TIMEOUT_MS) {
        setTargetMissing(true);
        return;
      }
      timer = window.setTimeout(find, 80);
    };
    find();
    const focusTimer = window.setTimeout(() => {
      headingRef.current?.focus({ preventScroll: true });
    }, 30);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
      window.clearTimeout(focusTimer);
    };
  }, [step, reducedMotion]);

  // Keep the hole and the tooltip attached while the page or a modal scrolls.
  useEffect(() => {
    if (step === undefined) return;
    const update = () => {
      const element = document.querySelector<HTMLElement>(step.target);
      if (element !== null) {
        setRect(element.getBoundingClientRect());
      }
    };
    const onScroll = () => update();
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onScroll);
    const interval = window.setInterval(update, 300);
    return () => {
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onScroll);
      window.clearInterval(interval);
    };
  }, [step]);

  // Recompute the tooltip after its real height is known.
  const placeTooltip = useCallback(() => {
    const tip = tooltipRef.current;
    if (tip === null) return;
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    const width = tip.offsetWidth;
    const height = tip.offsetHeight;
    if (viewportWidth < 640) {
      setPosition({
        left: TOOLTIP_MARGIN,
        top: Math.max(
          TOOLTIP_MARGIN,
          viewportHeight - height - TOOLTIP_MARGIN
        ),
        width,
        height,
        placement: "sheet",
      });
      return;
    }
    if (rect === null || step?.spotlight === "none") {
      // A modal already centers itself. Docking the guide in the lower-left
      // corner keeps the form and the modal's own actions uncovered.
      if (step?.spotlight === "none") {
        setPosition({
          left: TOOLTIP_MARGIN + 4,
          top: Math.max(TOOLTIP_MARGIN, viewportHeight - height - TOOLTIP_MARGIN - 4),
          width,
          height,
          placement: "dock",
        });
        return;
      }
      setPosition({
        left: Math.max(TOOLTIP_MARGIN, (viewportWidth - width) / 2),
        top: Math.max(TOOLTIP_MARGIN, (viewportHeight - height) / 2),
        width,
        height,
        placement: "center",
      });
      return;
    }
    const requested = step?.placement ?? "auto";
    const largeTarget =
      rect.width > viewportWidth * 0.55 &&
      rect.height > viewportHeight * 0.5;
    let placement: TooltipPosition["placement"];
    if (requested !== "auto") {
      placement = requested;
    } else if (
      largeTarget &&
      rect.left - TOOLTIP_GAP - width >= TOOLTIP_MARGIN
    ) {
      // A modal or full-page target leaves no room above/below. Put the guide
      // in the side margin instead of covering its content.
      placement = "left";
    } else if (
      largeTarget &&
      viewportWidth - rect.right - TOOLTIP_GAP - width >= TOOLTIP_MARGIN
    ) {
      placement = "right";
    } else if (
      rect.bottom + TOOLTIP_GAP + height <=
      viewportHeight - TOOLTIP_MARGIN
    ) {
      placement = "bottom";
    } else if (rect.top - TOOLTIP_GAP - height >= TOOLTIP_MARGIN) {
      placement = "top";
    } else if (
      rect.right + TOOLTIP_GAP + width <=
      viewportWidth - TOOLTIP_MARGIN
    ) {
      placement = "right";
    } else if (rect.left - TOOLTIP_GAP - width >= TOOLTIP_MARGIN) {
      placement = "left";
    } else {
      placement = "center";
    }
    let left = 0;
    let top = 0;
    if (placement === "bottom") {
      left = rect.left + rect.width / 2 - width / 2;
      top = rect.bottom + TOOLTIP_GAP;
    } else if (placement === "top") {
      left = rect.left + rect.width / 2 - width / 2;
      top = rect.top - TOOLTIP_GAP - height;
    } else if (placement === "right") {
      left = rect.right + TOOLTIP_GAP;
      top = rect.top + rect.height / 2 - height / 2;
    } else if (placement === "left") {
      left = rect.left - TOOLTIP_GAP - width;
      top = rect.top + rect.height / 2 - height / 2;
    } else {
      left = (viewportWidth - width) / 2;
      top = (viewportHeight - height) / 2;
    }
    setPosition({
      left: Math.min(
        Math.max(TOOLTIP_MARGIN, left),
        Math.max(TOOLTIP_MARGIN, viewportWidth - width - TOOLTIP_MARGIN)
      ),
      top: Math.min(
        Math.max(TOOLTIP_MARGIN, top),
        Math.max(TOOLTIP_MARGIN, viewportHeight - height - TOOLTIP_MARGIN)
      ),
      width,
      height,
      placement,
    });
  }, [rect, step?.placement, step?.spotlight]);

  useLayoutEffect(() => {
    placeTooltip();
  }, [placeTooltip, index, targetMissing]);

  // A click or focus on the highlighted control is the step's action.
  useEffect(() => {
    if (step === undefined || step.advance === undefined || step.advance === "next") {
      return;
    }
    const target = () => document.querySelector<HTMLElement>(step.target);
    if (step.advance === "click") {
      const onClick = (event: MouseEvent) => {
        const element = target();
        if (element !== null && event.target instanceof Node && element.contains(event.target)) {
          next();
        }
      };
      document.addEventListener("click", onClick, true);
      return () => document.removeEventListener("click", onClick, true);
    }
    if (step.advance === "focus") {
      const onFocus = (event: FocusEvent) => {
        const element = target();
        if (element !== null && event.target instanceof Node && element.contains(event.target)) {
          next();
        }
      };
      document.addEventListener("focusin", onFocus, true);
      return () => document.removeEventListener("focusin", onFocus, true);
    }
    if (step.advance === "event" && step.advanceEvent !== undefined) {
      const onAdvance = (event: Event) => {
        const detail = (event as CustomEvent<{ key?: string }>).detail;
        if (detail?.key === step.advanceEvent) next();
      };
      window.addEventListener(FEATURES_TUTORIAL_ADVANCE_EVENT, onAdvance);
      return () =>
        window.removeEventListener(FEATURES_TUTORIAL_ADVANCE_EVENT, onAdvance);
    }
    return;
  }, [next, step]);

  // Keyboard navigation. Escape means Skip, matching the visible button.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        skip();
        return;
      }
      if (isTypingTarget(event.target)) return;
      if (event.key === "ArrowRight") {
        event.preventDefault();
        next();
      } else if (event.key === "ArrowLeft") {
        event.preventDefault();
        back();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [back, next, skip]);

  if (step === undefined) return null;

  if (collapsed) {
    return (
      <div data-features-tutorial={workspace}>
        <button
          type="button"
          aria-label="Show Features Tutorial guide"
          onClick={() => setCollapsed(false)}
          className="fixed bottom-20 right-4 z-[82] inline-flex min-h-10 items-center gap-2 rounded-full border border-rule-strong bg-white/95 px-4 text-[12px] font-medium text-ink-800 shadow-pop backdrop-blur hover:border-accent-300 hover:text-accent-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50"
        >
          <span
            aria-hidden="true"
            className="size-1.5 rounded-full bg-accent-600"
          />
          Continue tutorial
          <span className="tnum text-ink-400">
            {index + 1}/{safeSteps.length}
          </span>
        </button>
      </div>
    );
  }

  const panelStyle = {
    position: "fixed" as const,
    zIndex: 80,
    background: "rgba(28, 27, 25, 0.34)",
  };
  const scrollPanel = (event: WheelEvent) => {
    window.scrollBy(0, event.deltaY);
  };
  const compactTooltip =
    rect !== null && rect.width > window.innerWidth * 0.55;
  const showHole =
    step.spotlight !== "none" && rect !== null && !targetMissing;
  // Four panels, each covering one side of the hole and none overlapping the
  // others. Overlapping semi-transparent panels made the target's row or
  // column visibly darker than the rest of the dimmed page.
  const holeTop = showHole ? Math.max(0, rect.top - TARGET_PADDING) : 0;
  const holeLeft = showHole ? Math.max(0, rect.left - TARGET_PADDING) : 0;
  const holeRight = showHole
    ? Math.min(window.innerWidth, rect.right + TARGET_PADDING)
    : 0;
  const holeBottom = showHole
    ? Math.min(window.innerHeight, rect.bottom + TARGET_PADDING)
    : 0;
  const sideHeight = Math.max(0, holeBottom - holeTop);
  const ring = showHole
    ? {
        top: holeTop,
        left: holeLeft,
        width: Math.max(0, holeRight - holeLeft),
        height: Math.max(0, holeBottom - holeTop),
      }
    : null;

  // The arrow follows the target, clamped to the card so it never escapes a
  // rounded corner. It is omitted for docked/centered positions.
  const arrow =
    position !== null &&
    rect !== null &&
    (position.placement === "top" ||
      position.placement === "bottom" ||
      position.placement === "left" ||
      position.placement === "right")
      ? position.placement === "top" || position.placement === "bottom"
        ? {
            left: Math.min(
              Math.max(18, rect.left + rect.width / 2 - position.left),
              position.width - 18
            ),
            top: position.placement === "bottom" ? -5 : undefined,
            bottom: position.placement === "top" ? -5 : undefined,
          }
        : {
            top: Math.min(
              Math.max(18, rect.top + rect.height / 2 - position.top),
              position.height - 18
            ),
            left: position.placement === "right" ? -5 : undefined,
            right: position.placement === "left" ? -5 : undefined,
          }
      : null;

  const interactionHint =
    step.advance === "click"
      ? "Click the highlighted control"
      : step.advance === "focus"
        ? "Focus the highlighted field"
        : step.advance === "event"
          ? "Use the highlighted control"
          : null;

  return (
    <div data-features-tutorial={workspace}>
      {showHole ? (
        <>
          <div
            aria-hidden="true"
            style={{ ...panelStyle, top: 0, left: 0, right: 0, height: holeTop }}
            onWheel={scrollPanel}
          />
          <div
            aria-hidden="true"
            style={{
              ...panelStyle,
              top: holeTop,
              left: 0,
              width: holeLeft,
              height: sideHeight,
            }}
            onWheel={scrollPanel}
          />
          <div
            aria-hidden="true"
            style={{
              ...panelStyle,
              top: holeTop,
              left: holeRight,
              right: 0,
              height: sideHeight,
            }}
            onWheel={scrollPanel}
          />
          <div
            aria-hidden="true"
            style={{
              ...panelStyle,
              top: holeBottom,
              left: 0,
              right: 0,
              height: Math.max(0, window.innerHeight - holeBottom),
            }}
            onWheel={scrollPanel}
          />
          {ring !== null ? (
            <div
              aria-hidden="true"
              className="pointer-events-none fixed rounded-[10px] border-[1.5px] border-accent-400 shadow-[0_0_0_4px_rgba(255,255,255,0.16),0_0_22px_rgba(47,107,79,0.22)]"
              style={{
                zIndex: 81,
                top: ring.top,
                left: ring.left,
                width: ring.width,
                height: ring.height,
              }}
            />
          ) : null}
        </>
      ) : step.spotlight === "none" ? null : (
        <div
          aria-hidden="true"
          style={{ ...panelStyle, inset: 0 }}
          onWheel={scrollPanel}
        />
      )}

      <div
        ref={tooltipRef}
        role="dialog"
        aria-modal="false"
        aria-labelledby="features-tutorial-title"
        aria-describedby="features-tutorial-body"
        style={{
          position: "fixed",
          zIndex: 82,
          left: position?.left ?? 0,
          top: position?.top ?? 0,
          width:
            position?.placement === "sheet"
              ? "calc(100vw - 24px)"
              : position?.placement === "dock"
                ? "min(360px, calc(100vw - 24px))"
                : compactTooltip
                  ? "min(320px, calc(100vw - 24px))"
                  : "min(380px, calc(100vw - 24px))",
          opacity: position === null ? 0 : 1,
          transition: reducedMotion ? "none" : "opacity 120ms ease-out",
        }}
      >
        {arrow !== null ? (
          <span
            aria-hidden="true"
            className="absolute z-0 size-2.5 rotate-45 border border-rule-strong bg-white"
            style={arrow}
          />
        ) : null}
        <div className="relative z-10 overflow-hidden rounded-2xl border border-rule-strong bg-white text-ink-900 shadow-[0_22px_60px_-18px_rgba(28,27,25,0.42)]">
          <div className="h-[3px] w-full bg-paper-muted" aria-hidden="true">
            <div
              className="h-full bg-accent-600 transition-[width] duration-300 ease-out-quart"
              style={{
                width: `${((index + 1) / safeSteps.length) * 100}%`,
                transition: reducedMotion ? "none" : undefined,
              }}
            />
          </div>

          <div className="px-4 pb-3 pt-3">
            <div className="flex items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <span
                  aria-hidden="true"
                  className="size-1.5 shrink-0 rounded-full bg-accent-600"
                />
                <span className="truncate text-[10px] font-semibold uppercase tracking-[0.16em] text-accent-700">
                  Features Tutorial
                </span>
                <span className="tnum shrink-0 text-[10px] text-ink-400">
                  {index + 1} / {safeSteps.length}
                </span>
              </div>
              <div className="flex shrink-0 items-center gap-0.5">
                <button
                  type="button"
                  aria-label="Hide Features Tutorial guide"
                  onClick={() => setCollapsed(true)}
                  className="cursor-pointer rounded-md px-1.5 py-1 text-[11px] font-medium text-ink-400 hover:bg-paper-muted hover:text-ink-800 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                >
                  Hide
                </button>
                <button
                  type="button"
                  aria-label="Skip Features Tutorial"
                  onClick={skip}
                  className="cursor-pointer rounded-md px-1.5 py-1 text-[11px] font-medium text-ink-400 hover:bg-paper-muted hover:text-ink-800 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                >
                  Skip
                </button>
              </div>
            </div>

            <h2
              ref={headingRef}
              id="features-tutorial-title"
              tabIndex={-1}
              className="mt-1.5 text-[17px] font-semibold leading-[1.3] tracking-[-0.015em] text-ink-950 focus-visible:outline-hidden"
            >
              {step.title}
            </h2>

            <div
              id="features-tutorial-body"
              className="mt-2 max-h-[34vh] overflow-y-auto text-[13px] leading-[1.55] text-ink-600"
            >
              {step.body}
            </div>

            {interactionHint !== null ? (
              <span className="mt-3 inline-flex items-center gap-1.5 rounded-full bg-accent-50 px-2.5 py-1 text-[11px] font-medium text-accent-800 ring-1 ring-accent-200/70">
                <span
                  aria-hidden="true"
                  className="size-1.5 rounded-full bg-accent-500"
                />
                {interactionHint}
              </span>
            ) : null}

            {step.action !== undefined ? (
              <button
                type="button"
                onClick={() => onAction?.(step.action?.action ?? "")}
                className="mt-2.5 block cursor-pointer text-[12px] font-medium text-accent-700 underline underline-offset-2 hover:text-accent-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
              >
                {step.action.label}
              </button>
            ) : null}
          </div>

          <div className="flex items-center justify-between gap-3 border-t border-rule/70 bg-paper-subtle/40 px-3 py-2">
            <button
              type="button"
              aria-label="Previous step"
              disabled={index === 0}
              onClick={back}
              className="cursor-pointer rounded-full px-3 py-1.5 text-[12px] font-medium text-ink-500 hover:bg-white hover:text-ink-900 disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
            >
              Back
            </button>
            {step.advance === "event" ? (
              <span className="pr-1 text-[11px] text-ink-400">Waiting for you</span>
            ) : (
              <button
                type="button"
                aria-label={finalStep ? "Finish Features Tutorial" : "Next step"}
                onClick={finalStep ? () => onClose("finished") : next}
                className="rounded-full bg-accent-800 px-4 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50"
              >
                {finalStep ? "Finish" : "Next"}
              </button>
            )}
          </div>
        </div>
      </div>

      <p aria-live="polite" className="sr-only">
        Step {index + 1} of {safeSteps.length}: {step.title}
      </p>
    </div>
  );
}
