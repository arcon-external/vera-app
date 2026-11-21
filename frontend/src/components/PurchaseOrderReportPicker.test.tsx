// @vitest-environment jsdom
import { act, createRef, StrictMode, useRef, useState, type RefObject } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  PurchaseOrderReportPicker,
  PurchaseOrderReportPickerView,
  type PurchaseOrderReportPickerProps,
  type ReportPickerItem,
} from "./PurchaseOrderReportPicker";

// Issue #642: the live month picker.
//
// The contract is deliberately unusual: item and All clicks commit immediately
// and never close, backdrop and Escape do not dismiss, and only the explicit X
// closes. This suite drives those decisions in a real DOM, through the public
// controlled props, and proves that a parent re-render while open keeps the
// same dialog, focus and scroll -- the failure mode a naive remount would hide.

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const MONTH_NAMES = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
];

function monthsOf(year: number): ReportPickerItem[] {
  return MONTH_NAMES.map((label, index) => ({
    key: `${year}-${String(index + 1).padStart(2, "0")}`,
    label,
  }));
}

const ITEMS = monthsOf(2026);
const ALL_2026 = ITEMS.map((item) => item.key);

let container: HTMLDivElement;
let root: Root;
let triggerRef: RefObject<HTMLButtonElement | null>;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  triggerRef = createRef<HTMLButtonElement>();
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

function baseProps(
  overrides: Partial<PurchaseOrderReportPickerProps> = {}
): PurchaseOrderReportPickerProps {
  return {
    title: "Choose months",
    open: true,
    items: ITEMS,
    selectedKeys: [],
    allLabel: "All months in 2026",
    allSelected: false,
    onSelectAll: vi.fn(),
    onToggle: vi.fn(),
    onClose: vi.fn(),
    returnFocusRef: triggerRef,
    pageLabel: "2026",
    onPreviousPage: vi.fn(),
    onNextPage: vi.fn(),
    ...overrides,
  };
}

function directTree(overrides: Partial<PurchaseOrderReportPickerProps> = {}) {
  return (
    <>
      <button ref={triggerRef} data-trigger>
        Choose months
      </button>
      <div data-background>
        <button data-background-action>Background action</button>
      </div>
      <PurchaseOrderReportPicker {...baseProps(overrides)} />
    </>
  );
}

function render(overrides: Partial<PurchaseOrderReportPickerProps> = {}) {
  act(() => root.render(directTree(overrides)));
}

function find<T extends Element = HTMLElement>(selector: string): T {
  const found = document.querySelector<T>(selector);
  if (found === null) throw new Error(`no element for ${selector}`);
  return found;
}

function maybe<T extends Element = HTMLElement>(selector: string): T | null {
  return document.querySelector<T>(selector);
}

function dialog(): HTMLElement {
  const surface = find("[data-report-picker]");
  expect(surface.getAttribute("role")).toBe("dialog");
  return surface;
}

function click(element: Element) {
  act(() => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
  });
}

function press(target: EventTarget, key: string, shiftKey = false) {
  const event = new KeyboardEvent("keydown", {
    key,
    shiftKey,
    bubbles: true,
    cancelable: true,
  });
  act(() => {
    target.dispatchEvent(event);
  });
  return event;
}

function ControlledPicker({
  onClose,
  onToggle,
  onSelectAll,
  initialSelected = [],
}: {
  onClose: () => void;
  onToggle: (key: string) => void;
  onSelectAll: () => void;
  initialSelected?: string[];
}) {
  const [open, setOpen] = useState(true);
  const [selected, setSelected] = useState<string[]>(initialSelected);
  const [triggerLabel, setTriggerLabel] = useState("Choose months");
  const localTriggerRef = useRef<HTMLButtonElement>(null);

  return (
    <>
      <button
        ref={localTriggerRef}
        data-trigger
        onClick={() => setOpen(true)}
      >
        {triggerLabel}
      </button>
      <button data-change-trigger onClick={() => setTriggerLabel("Selected months")}>
        Rename trigger
      </button>
      <div data-background>
        <button data-background-action>Background action</button>
      </div>
      <PurchaseOrderReportPicker
        title="Choose months"
        open={open}
        items={ITEMS}
        selectedKeys={selected}
        allLabel="All months in 2026"
        allSelected={selected.length === ITEMS.length}
        onSelectAll={() => {
          onSelectAll();
          setSelected([...ALL_2026]);
        }}
        onToggle={(key) => {
          onToggle(key);
          setSelected((current) =>
            current.includes(key)
              ? current.filter((selectedKey) => selectedKey !== key)
              : [...current, key]
          );
        }}
        onClose={() => {
          onClose();
          setOpen(false);
        }}
        returnFocusRef={localTriggerRef}
        pageLabel="2026"
        onPreviousPage={() => undefined}
        onNextPage={() => undefined}
      />
    </>
  );
}

describe("PurchaseOrderReportPicker", () => {
  it("renders nothing until the controlled open prop is true", () => {
    render({ open: false });
    expect(maybe("[data-report-picker]")).toBeNull();
    expect(maybe("[data-report-picker-layer]")).toBeNull();

    act(() => root.render(directTree({ open: true })));
    expect(dialog()).not.toBeNull();
    expect(find("[data-report-picker]").closest("[data-report-picker-layer]")).not.toBeNull();
  });

  it("opens an accessible modal naming the title and current page", () => {
    render();
    const surface = dialog();
    expect(surface.getAttribute("aria-modal")).toBe("true");
    expect(document.body.contains(surface)).toBe(true);
    expect(container.contains(surface)).toBe(false);
    const labelledBy = surface.getAttribute("aria-labelledby");
    expect(labelledBy).not.toBeNull();
    expect(document.getElementById(labelledBy as string)?.textContent).toBe(
      "Choose months"
    );
    expect(document.body.textContent).toContain("2026");
    expect(find("[data-picker-all]").getAttribute("aria-pressed")).toBe("false");
    expect(find("[data-picker-all]").textContent).toContain("All months in 2026");

    for (const item of ITEMS) {
      const option = find(`[data-picker-key="${item.key}"]`);
      expect(option.tagName).toBe("BUTTON");
      expect(option.textContent).toContain(item.label);
      expect(option.getAttribute("aria-pressed")).toBe("false");
      expect(option.querySelector("[data-picker-check]")).toBeNull();
    }

    const close = find("[data-picker-close]");
    expect(close.tagName).toBe("BUTTON");
    expect(close.getAttribute("aria-label")).toBe("Close picker");
    expect(close.hasAttribute("disabled")).toBe(false);
  });

  it("keeps every control at a usable touch size and at most three option columns", () => {
    render();
    for (const control of [
      find("[data-picker-close]"),
      find("[data-picker-all]"),
      find("[data-picker-key='2026-01']"),
      find("[aria-label='Previous page']"),
      find("[aria-label='Next page']"),
    ]) {
      expect(control.className).toContain("min-h-11");
    }
    const options = find("[data-picker-options]");
    expect(options.className).toContain("grid-cols-1");
    expect(options.className).toContain("sm:grid-cols-2");
    expect(options.className).toContain("md:grid-cols-3");
    expect(options.className).not.toContain("grid-cols-4");
    // The only scrolling region is the options grid, so the X stays visible
    // when a long month or category list is scrolled.
    expect(options.className).toContain("overflow-y-auto");
    expect(dialog().className).toContain("overflow-hidden");
    expect(dialog().className).not.toContain("overflow-y-auto");
    expect(find("[data-picker-close]").closest("[data-picker-options]")).toBeNull();
  });

  it("commits an item click immediately and leaves the picker open", () => {
    const onToggle = vi.fn();
    const onClose = vi.fn();
    render({ onToggle, onClose });

    click(find("[data-picker-key='2026-03']"));
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(onToggle).toHaveBeenCalledWith("2026-03");
    expect(onClose).not.toHaveBeenCalled();
    expect(maybe("[data-report-picker]")).not.toBeNull();
  });

  it("reflects a controlled selection update without closing or replacing nodes", () => {
    const onClose = vi.fn();
    render({ onClose, selectedKeys: ["2026-01"] });
    const january = find("[data-picker-key='2026-01']");
    const dialogNode = dialog();
    const options = find("[data-picker-options]");
    january.focus();
    options.scrollTop = 84;
    expect(january.getAttribute("aria-pressed")).toBe("true");
    expect(january.querySelector("[data-picker-check]")).not.toBeNull();

    act(() =>
      root.render(
        directTree({
          onClose,
          selectedKeys: ["2026-01", "2025-12"],
          items: ITEMS.map((item) => ({ ...item })),
          allSelected: false,
        })
      )
    );

    expect(dialog()).toBe(dialogNode);
    expect(find("[data-picker-key='2026-01']")).toBe(january);
    expect(find("[data-picker-options]")).toBe(options);
    expect(document.activeElement).toBe(january);
    expect(options.scrollTop).toBe(84);
    expect(find("[data-picker-count]").textContent).toBe("2 selected");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("selects the displayed year through All without closing", () => {
    const onSelectAll = vi.fn();
    const onClose = vi.fn();
    render({ onSelectAll, onClose, allSelected: true, selectedKeys: ALL_2026 });

    expect(find("[data-picker-all]").getAttribute("aria-pressed")).toBe("true");
    expect(find("[data-picker-count]").textContent).toBe("12 selected");
    for (const item of ITEMS) {
      expect(find(`[data-picker-key="${item.key}"]`).getAttribute("aria-pressed")).toBe(
        "true"
      );
    }

    click(find("[data-picker-all]"));
    expect(onSelectAll).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
    expect(maybe("[data-report-picker]")).not.toBeNull();
  });

  it("counts selections from every page and pages options without touching selection", () => {
    const onToggle = vi.fn();
    const onSelectAll = vi.fn();
    const previous = vi.fn();
    const next = vi.fn();
    render({
      onToggle,
      onSelectAll,
      onPreviousPage: previous,
      onNextPage: next,
      selectedKeys: ["2025-12", "2026-01"],
    });

    expect(find("[data-picker-count]").textContent).toBe("2 selected");
    expect(find("[data-picker-key='2026-01']").getAttribute("aria-pressed")).toBe(
      "true"
    );

    click(find("[aria-label='Previous page']"));
    click(find("[aria-label='Next page']"));
    expect(previous).toHaveBeenCalledTimes(1);
    expect(next).toHaveBeenCalledTimes(1);
    expect(onToggle).not.toHaveBeenCalled();
    expect(onSelectAll).not.toHaveBeenCalled();
    expect(maybe("[data-report-picker]")).not.toBeNull();

    act(() =>
      root.render(
        directTree({
          onToggle,
          onSelectAll,
          onPreviousPage: previous,
          onNextPage: next,
          selectedKeys: ["2025-12", "2026-01"],
          items: monthsOf(2025),
          pageLabel: "2025",
        })
      )
    );
    expect(find("[data-picker-count]").textContent).toBe("2 selected");
    expect(find("[data-picker-key='2025-12']").getAttribute("aria-pressed")).toBe(
      "true"
    );
    expect(maybe("[data-picker-key='2026-01']")).toBeNull();
    expect(maybe("[data-report-picker]")).not.toBeNull();
  });

  it("omits paging controls when the caller supplies no paging props", () => {
    render({
      pageLabel: undefined,
      onPreviousPage: undefined,
      onNextPage: undefined,
    });
    expect(maybe("[aria-label='Previous page']")).toBeNull();
    expect(maybe("[aria-label='Next page']")).toBeNull();
  });

  it("does not dismiss on a backdrop click", () => {
    const onClose = vi.fn();
    render({ onClose });
    const backdrop = find("[data-report-picker-backdrop]");
    click(backdrop);
    expect(onClose).not.toHaveBeenCalled();
    expect(maybe("[data-report-picker]")).not.toBeNull();
  });

  it("does not dismiss on Escape and keeps the key away from page shortcuts", () => {
    const onClose = vi.fn();
    render({ onClose });
    const close = find("[data-picker-close]");
    close.focus();
    const pageShortcut = vi.fn();
    window.addEventListener("keydown", pageShortcut);
    const event = press(close, "Escape");
    window.removeEventListener("keydown", pageShortcut);
    expect(event.defaultPrevented).toBe(true);
    expect(pageShortcut).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(maybe("[data-report-picker]")).not.toBeNull();
  });

  it("keeps other page-level shortcuts from acting behind the modal", () => {
    render();
    const close = find("[data-picker-close]");
    close.focus();
    const pageShortcut = vi.fn();
    window.addEventListener("keydown", pageShortcut);
    act(() => {
      close.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "1",
          altKey: true,
          bubbles: true,
          cancelable: true,
        })
      );
    });
    window.removeEventListener("keydown", pageShortcut);
    expect(pageShortcut).not.toHaveBeenCalled();
    expect(maybe("[data-report-picker]")).not.toBeNull();
  });

  it("closes only through X, preserves the live selection and restores the stable trigger", () => {
    const onClose = vi.fn();
    const onToggle = vi.fn();
    const onSelectAll = vi.fn();
    act(() =>
      root.render(
        <ControlledPicker
          onClose={onClose}
          onToggle={onToggle}
          onSelectAll={onSelectAll}
          initialSelected={["2026-03"]}
        />
      )
    );

    const trigger = find("[data-trigger]");
    click(find("[data-picker-key='2026-04']"));
    expect(onToggle).toHaveBeenCalledWith("2026-04");

    // The trigger's text may change while open; the DOM node is what returns.
    click(find("[data-change-trigger]"));
    expect(find("[data-trigger]")).toBe(trigger);
    expect(trigger.textContent).toBe("Selected months");

    click(find("[data-picker-close]"));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(maybe("[data-report-picker]")).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expect(find("[data-background]").hasAttribute("inert")).toBe(false);

    click(trigger);
    expect(find("[data-picker-key='2026-03']").getAttribute("aria-pressed")).toBe(
      "true"
    );
    expect(find("[data-picker-key='2026-04']").getAttribute("aria-pressed")).toBe(
      "true"
    );
    expect(find("[data-picker-count]").textContent).toBe("2 selected");
  });

  it("keeps focus inside the modal and the background inert while open", () => {
    render();
    const surface = dialog();
    expect(document.activeElement).toBe(surface);
    expect(find("[data-background]").closest("[data-report-picker-layer]")).toBeNull();
    // The test container holds the trigger and the background; it is the body
    // child the portal must leave inert for the modal to be real.
    expect(container.hasAttribute("inert")).toBe(true);
    expect(find("[data-report-picker-layer]").hasAttribute("inert")).toBe(false);

    const backgroundAction = find("[data-background-action]");
    backgroundAction.focus();
    expect(document.activeElement).toBe(surface);

    // Tab from the dialog goes to the first control, wraps at the end, and
    // Shift+Tab wraps back from the first.
    press(surface, "Tab");
    expect(document.activeElement).toBe(find("[data-picker-close]"));

    const last = find("[data-picker-key='2026-12']");
    last.focus();
    press(last, "Tab");
    expect(document.activeElement).toBe(find("[data-picker-close]"));

    press(find("[data-picker-close]"), "Tab", true);
    expect(document.activeElement).toBe(last);
  });

  it("survives the app's StrictMode double-effect with focus contained", () => {
    const onClose = vi.fn();
    act(() =>
      root.render(
        <StrictMode>
          <ControlledPicker
            onClose={onClose}
            onToggle={() => undefined}
            onSelectAll={() => undefined}
          />
        </StrictMode>
      )
    );

    const trigger = find("[data-trigger]");
    const surface = dialog();
    expect(document.activeElement).toBe(surface);
    expect(container.hasAttribute("inert")).toBe(true);

    click(find("[data-picker-close]"));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(trigger);
    expect(container.hasAttribute("inert")).toBe(false);
  });

  it("renders the same open controls through the static evidence view", () => {
    const html = renderToStaticMarkup(
      <PurchaseOrderReportPickerView
        title="Choose months"
        items={monthsOf(2025)}
        selectedKeys={["2025-02", "2025-12"]}
        allLabel="All months in 2025"
        allSelected={false}
        onSelectAll={() => undefined}
        onToggle={() => undefined}
        onClose={() => undefined}
        pageLabel="2025"
        onPreviousPage={() => undefined}
        onNextPage={() => undefined}
      />
    );

    expect(html).toContain('data-report-picker="true"');
    expect(html).toContain('role="dialog"');
    expect(html).toContain('aria-modal="true"');
    expect(html).toContain("Choose months");
    expect(html).toContain("All months in 2025");
    expect(html).toContain('data-picker-all="true"');
    expect(html).toContain('data-picker-close="true"');
    expect(html).toContain('aria-label="Close picker"');
    expect(html).toContain('data-picker-key="2025-02"');
    expect(html).toContain('data-picker-key="2025-12"');
    expect(html).toContain("2 selected");
    expect(html).toContain("2025");
  });
});
