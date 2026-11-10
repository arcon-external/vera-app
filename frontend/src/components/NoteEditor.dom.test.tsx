// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LIST_INSIDE_BLOCK } from "../noteFixtures";
import { NoteEditor, type NoteEditorProps } from "./NoteEditor";

// The rich-text note editor (#535). The tests pin the external behaviour the
// staff and the stored source depend on: the toolbar runs the four supported
// commands, the shortcuts are scoped to the editor and accept Ctrl and Cmd,
// every edit serializes to the restricted Markdown source, the caret is never
// reset by a re-render, paste can only insert the allowlisted subset, and the
// 1000-rune limit refuses rather than truncates.

type Command = { command: string; value: string };

let commands: Command[];

beforeEach(() => {
  commands = [];
  Object.defineProperty(document, "execCommand", {
    configurable: true,
    writable: true,
    value: (command: string, _ui?: boolean, value?: string) => {
      commands.push({ command, value: value ?? "" });
      return true;
    },
  });
  Object.defineProperty(document, "queryCommandState", {
    configurable: true,
    writable: true,
    value: () => false,
  });
});

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

interface Mounted {
  editor: HTMLElement;
  onChange: ReturnType<typeof vi.fn>;
  rerender: (next?: Partial<NoteEditorProps>) => void;
}

function mount(props: Partial<NoteEditorProps> = {}): Mounted {
  const onChange = vi.fn();
  const base: NoteEditorProps = { value: "", onChange, ...props };
  const rerender = (next: Partial<NoteEditorProps> = {}) => {
    act(() => {
      root.render(<NoteEditor {...base} {...next} />);
    });
  };
  rerender();
  const editor = container.querySelector<HTMLElement>('[role="textbox"]');
  if (editor === null) throw new Error("note editor not rendered");
  return { editor, onChange, rerender };
}

function toolbarButton(label: string): HTMLButtonElement {
  const found = container.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no toolbar button labelled ${label}`);
  return found;
}

function press(editor: HTMLElement, init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent("keydown", {
    bubbles: true,
    cancelable: true,
    ...init,
  });
  act(() => {
    editor.dispatchEvent(event);
  });
  return event;
}

function typeInto(editor: HTMLElement, html: string): void {
  act(() => {
    editor.innerHTML = html;
    editor.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function paste(editor: HTMLElement, html: string | null, text: string): Event {
  const event = new Event("paste", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "clipboardData", {
    value: {
      getData: (kind: string) =>
        kind === "text/html" ? (html ?? "") : kind === "text/plain" ? text : "",
    },
  });
  act(() => {
    editor.dispatchEvent(event);
  });
  return event;
}

describe("NoteEditor toolbar", () => {
  it("renders the four supported commands with shortcuts and a tooltip", () => {
    mount({ value: "Hello" });
    for (const [label, shortcut] of [
      ["Bold", "Control+B Meta+B"],
      ["Italic", "Control+I Meta+I"],
      ["Bullet list", "Control+Shift+8 Meta+Shift+8"],
      ["Numbered list", "Control+Shift+7 Meta+Shift+7"],
    ]) {
      const button = toolbarButton(label);
      expect(button.getAttribute("aria-pressed")).toBe("false");
      expect(button.getAttribute("aria-keyshortcuts")).toBe(shortcut);
      expect(button.getAttribute("title")).toBeTruthy();
    }
  });

  it("runs the four commands from the toolbar", () => {
    mount({ value: "Hello" });
    act(() => toolbarButton("Bold").click());
    act(() => toolbarButton("Italic").click());
    act(() => toolbarButton("Bullet list").click());
    act(() => toolbarButton("Numbered list").click());
    expect(commands.map((entry) => entry.command)).toEqual([
      "bold",
      "italic",
      "insertUnorderedList",
      "insertOrderedList",
    ]);
  });

  it("keeps the selection by preventing the button mousedown default", () => {
    mount({ value: "Hello" });
    const event = new MouseEvent("mousedown", { bubbles: true, cancelable: true });
    act(() => {
      toolbarButton("Bold").dispatchEvent(event);
    });
    expect(event.defaultPrevented).toBe(true);
  });
});

describe("NoteEditor shortcuts", () => {
  it("runs bold and italic for Ctrl and for Cmd", () => {
    const { editor } = mount({ value: "Hello" });
    expect(press(editor, { key: "b", ctrlKey: true }).defaultPrevented).toBe(true);
    expect(press(editor, { key: "i", ctrlKey: true }).defaultPrevented).toBe(true);
    expect(press(editor, { key: "b", metaKey: true }).defaultPrevented).toBe(true);
    expect(press(editor, { key: "i", metaKey: true }).defaultPrevented).toBe(true);
    expect(commands.map((entry) => entry.command)).toEqual([
      "bold",
      "italic",
      "bold",
      "italic",
    ]);
  });

  it("runs the list commands for Ctrl/Cmd+Shift+8 and +7", () => {
    const { editor } = mount({ value: "Hello" });
    expect(
      press(editor, { key: "8", code: "Digit8", shiftKey: true, ctrlKey: true })
        .defaultPrevented
    ).toBe(true);
    expect(
      press(editor, { key: "7", code: "Digit7", shiftKey: true, metaKey: true })
        .defaultPrevented
    ).toBe(true);
    expect(commands.map((entry) => entry.command)).toEqual([
      "insertUnorderedList",
      "insertOrderedList",
    ]);
  });

  it("does not bind Mod+U, because underline is not supported", () => {
    const { editor } = mount({ value: "Hello" });
    const event = press(editor, { key: "u", ctrlKey: true });
    expect(event.defaultPrevented).toBe(false);
    expect(commands).toHaveLength(0);
  });
});

describe("NoteEditor source sync", () => {
  it("serializes every input and reports the Markdown source", () => {
    const { editor, onChange } = mount({ value: "" });
    typeInto(editor, "<p>Hello <strong>world</strong></p>");
    expect(onChange).toHaveBeenLastCalledWith("Hello **world**");
  });

  it("serializes a list nested in a toolbar block to the Markdown source", () => {
    // The same fixture the serializer tests use: the toolbar wraps its list in
    // a block, and the editor must report the list source, not flattened text.
    const { editor, onChange } = mount({ value: "" });
    typeInto(editor, LIST_INSIDE_BLOCK.html);
    expect(onChange).toHaveBeenLastCalledWith(LIST_INSIDE_BLOCK.source);
  });

  it("renders the external value into the editor on mount", () => {
    const { editor } = mount({ value: "**Bold** note" });
    expect(editor.innerHTML).toBe("<p><strong>Bold</strong> note</p>");
  });

  it("updates the DOM when the external value changes while unfocused", () => {
    const { editor, rerender } = mount({ value: "First" });
    expect(editor.innerHTML).toBe("<p>First</p>");
    rerender({ value: "Second" });
    expect(editor.innerHTML).toBe("<p>Second</p>");
  });

  it("never rewrites the DOM while the editor is focused", () => {
    const { editor, rerender } = mount({ value: "First" });
    act(() => editor.focus());
    rerender({ value: "Second" });
    expect(editor.innerHTML).toBe("<p>First</p>");
  });

  it("shows the rune counter and the shortcut hint", () => {
    mount({ value: "abc" });
    expect(container.textContent).toContain("3/1000");
    const hint = container.textContent ?? "";
    for (const shortcut of ["Ctrl+B", "Ctrl+I", "Ctrl+Shift+8", "Ctrl+Shift+7"]) {
      expect(hint).toContain(shortcut);
    }
  });

  it("honours a custom label and limit", () => {
    mount({ value: "abc", label: "Delivery note", maxLength: 20 });
    const editor = container.querySelector<HTMLElement>('[role="textbox"]');
    expect(editor?.getAttribute("aria-label")).toBe("Delivery note");
    expect(container.textContent).toContain("3/20");
  });
});

describe("NoteEditor paste", () => {
  it("sanitizes pasted HTML to the supported subset", () => {
    const { editor } = mount({ value: "" });
    const event = paste(
      editor,
      '<p>Hello <strong>bold</strong></p><script>alert("x")</script>',
      "Hello bold"
    );
    expect(event.defaultPrevented).toBe(true);
    const insert = commands.find((entry) => entry.command === "insertHTML");
    expect(insert).toBeDefined();
    expect(insert?.value).toContain("<strong>bold</strong>");
    expect(insert?.value).not.toContain("script");
    expect(insert?.value).not.toContain("alert");
  });

  it("falls back to plain text when the HTML carries nothing usable", () => {
    const { editor } = mount({ value: "" });
    paste(editor, "<script>alert('x')</script>", "Delivery is ex-works.");
    const insert = commands.find((entry) => entry.command === "insertHTML");
    expect(insert?.value).toBe("<p>Delivery is ex-works.</p>");
  });
});

describe("NoteEditor limit", () => {
  it("refuses an over-limit edit, reverts it and shows the limit", () => {
    const { editor, onChange } = mount({ value: "abc", maxLength: 5 });
    typeInto(editor, "<p>abcdef</p>");
    expect(onChange).not.toHaveBeenCalled();
    expect(editor.textContent).toBe("abc");
    expect(container.textContent).toContain("5");
    const alert = container.querySelector('[role="alert"]');
    expect(alert).not.toBeNull();
    expect(alert?.textContent).toContain("5");
  });

  it("accepts an edit exactly at the limit", () => {
    const { editor, onChange } = mount({ value: "abc", maxLength: 5 });
    typeInto(editor, "<p>abcde</p>");
    expect(onChange).toHaveBeenLastCalledWith("abcde");
  });
});

describe("NoteEditor disabled", () => {
  it("is not editable and its toolbar is disabled", () => {
    const { editor } = mount({ value: "Hello", disabled: true });
    expect(editor.getAttribute("contenteditable")).toBe("false");
    expect(toolbarButton("Bold").disabled).toBe(true);
  });
});
