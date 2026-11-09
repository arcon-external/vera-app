import { useCallback, useEffect, useRef, useState } from "react";
import type {
  ClipboardEvent,
  FormEvent,
  JSX,
  KeyboardEvent,
} from "react";
import {
  NOTE_MAX_LENGTH,
  domToMarkdown,
  noteLength,
  noteToHTML,
  plainTextToMarkdown,
  sanitizePastedHTML,
} from "../note";
import { Button } from "./ui";

export interface NoteEditorProps {
  value: string;
  onChange: (value: string) => void;
  /** Default "Note"; also the textbox's accessible name. */
  label?: string;
  /** Default NOTE_MAX_LENGTH. Enforced on the serialized source. */
  maxLength?: number;
  disabled?: boolean;
  /** Optional standing explanation, e.g. where the note prints. */
  hint?: string;
}

type Command = "bold" | "italic" | "insertUnorderedList" | "insertOrderedList";

interface FormatState {
  bold: boolean;
  italic: boolean;
  bullets: boolean;
  numbers: boolean;
}

const NO_FORMAT: FormatState = {
  bold: false,
  italic: false,
  bullets: false,
  numbers: false,
};

/**
 * The quotation note editor (#535).
 *
 * The DOM is deliberately uncontrolled: state never rewrites the
 * contenteditable on a keystroke, because that is what resets the caret. The
 * external `value` is rendered only when it differs from what the DOM already
 * serializes to, and only while the editor is unfocused. Every edit is
 * serialized to the restricted Markdown source (#540) and handed to
 * `onChange`; the stored, signed and printed note is that source.
 *
 * Formatting is the browser's own execCommand over the allowlisted subset:
 * bold, italic, bullet list and numbered list. Paste never inserts raw HTML:
 * it is sanitized to the subset first, with plain text as the fallback.
 */
export function NoteEditor({
  value,
  onChange,
  label = "Note",
  maxLength = NOTE_MAX_LENGTH,
  disabled = false,
  hint,
}: NoteEditorProps): JSX.Element {
  const rootRef = useRef<HTMLDivElement>(null);
  const focusedRef = useRef(false);
  const lastGoodHTML = useRef("");
  const [focused, setFocused] = useState(false);
  const [length, setLength] = useState(() => noteLength(value));
  const [overLimit, setOverLimit] = useState(false);
  const [active, setActive] = useState<FormatState>(NO_FORMAT);

  // Render the external value only when the DOM means something else. While
  // focused the DOM is the user's, so state never touches it.
  useEffect(() => {
    const root = rootRef.current;
    if (root === null || focused) return;
    if (domToMarkdown(root) === value) {
      lastGoodHTML.current = root.innerHTML;
      setLength(noteLength(value));
      return;
    }
    const html = noteToHTML(value);
    root.innerHTML = html;
    lastGoodHTML.current = html;
    setLength(noteLength(value));
    setOverLimit(false);
  }, [value, focused]);

  const refreshFormatState = useCallback(() => {
    const query = (command: string): boolean => {
      try {
        return (
          typeof document.queryCommandState === "function" &&
          document.queryCommandState(command)
        );
      } catch {
        return false;
      }
    };
    const next: FormatState = {
      bold: query("bold"),
      italic: query("italic"),
      bullets: query("insertUnorderedList"),
      numbers: query("insertOrderedList"),
    };
    setActive((current) =>
      current.bold === next.bold &&
      current.italic === next.italic &&
      current.bullets === next.bullets &&
      current.numbers === next.numbers
        ? current
        : next
    );
  }, []);

  // The pressed state follows the caret, not just the last command.
  useEffect(() => {
    const onSelectionChange = (): void => {
      if (focusedRef.current) refreshFormatState();
    };
    document.addEventListener("selectionchange", onSelectionChange);
    return () => document.removeEventListener("selectionchange", onSelectionChange);
  }, [refreshFormatState]);

  /**
   * Reads the DOM back to source and reports it. An edit that pushes the
   * source past the limit is undone (the previous DOM is restored) and the
   * limit is shown; the note is never silently truncated.
   */
  const commitDom = useCallback(
    (previousHTML: string) => {
      const root = rootRef.current;
      if (root === null) return;
      const source = domToMarkdown(root);
      if (noteLength(source) > maxLength) {
        root.innerHTML = previousHTML;
        setOverLimit(true);
        setLength(noteLength(domToMarkdown(root)));
        return;
      }
      lastGoodHTML.current = root.innerHTML;
      setLength(noteLength(source));
      setOverLimit(false);
      onChange(source);
    },
    [maxLength, onChange]
  );

  const runCommand = useCallback(
    (command: Command) => {
      const root = rootRef.current;
      if (root === null || disabled) return;
      const previousHTML = root.innerHTML;
      root.focus();
      if (typeof document.execCommand === "function") {
        document.execCommand(command);
      }
      commitDom(previousHTML);
      refreshFormatState();
    },
    [commitDom, disabled, refreshFormatState]
  );

  const handleInput = (): void => {
    if (disabled) return;
    commitDom(lastGoodHTML.current);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>): void => {
    if (disabled) return;
    const mod = event.ctrlKey || event.metaKey;
    if (!mod || event.altKey) return;
    const key = event.key.toLowerCase();
    let command: Command | null = null;
    if (!event.shiftKey && key === "b") {
      command = "bold";
    } else if (!event.shiftKey && key === "i") {
      command = "italic";
    } else if (
      event.shiftKey &&
      (event.code === "Digit8" || key === "8" || key === "*")
    ) {
      command = "insertUnorderedList";
    } else if (
      event.shiftKey &&
      (event.code === "Digit7" || key === "7" || key === "&")
    ) {
      command = "insertOrderedList";
    }
    if (command === null) return;
    // The app's global shortcut handler must never see an editor shortcut.
    event.preventDefault();
    event.stopPropagation();
    runCommand(command);
  };

  const handlePaste = (event: ClipboardEvent<HTMLDivElement>): void => {
    if (disabled) return;
    // Always stop the browser's own paste: it can insert raw HTML.
    event.preventDefault();
    event.stopPropagation();
    const clipboard = event.clipboardData;
    if (!clipboard) return;
    const root = rootRef.current;
    if (root === null) return;

    const html = clipboard.getData("text/html");
    const text = clipboard.getData("text/plain");
    const source =
      (html !== "" ? sanitizePastedHTML(html) : null) ??
      plainTextToMarkdown(text);
    if (source.trim() === "") return;

    const previousHTML = root.innerHTML;
    root.focus();
    // noteToHTML is escaped and allowlisted, so this is the only markup that
    // can ever enter the contenteditable.
    if (typeof document.execCommand === "function") {
      document.execCommand("insertHTML", false, noteToHTML(source));
    }
    commitDom(previousHTML);
  };

  const handleFocus = (): void => {
    focusedRef.current = true;
    setFocused(true);
    refreshFormatState();
  };

  const handleBlur = (): void => {
    focusedRef.current = false;
    setFocused(false);
  };

  const button = (
    command: Command,
    buttonLabel: string,
    shortcut: string,
    tooltip: string,
    pressed: boolean,
    content: JSX.Element
  ): JSX.Element => (
    <Button
      size="xs"
      variant={pressed ? "secondary" : "ghost"}
      aria-label={buttonLabel}
      aria-pressed={pressed}
      aria-keyshortcuts={shortcut}
      title={tooltip}
      disabled={disabled}
      // The selection lives in the contenteditable; moving focus to the
      // button would collapse it before the command runs.
      onMouseDown={(event) => event.preventDefault()}
      onClick={() => runCommand(command)}
    >
      {content}
    </Button>
  );

  return (
    <div className="flex flex-col gap-1.5">
      <span className="flex items-baseline justify-between gap-2">
        <span className="text-caption font-medium text-ink-600">{label}</span>
        <span className="tnum text-caption text-ink-400">
          {length}/{maxLength}
        </span>
      </span>

      <div
        className={`rounded-md border bg-white transition duration-150 ${
          overLimit
            ? "border-danger-300 ring-2 ring-danger-500/20"
            : "border-rule focus-within:border-accent-400 focus-within:ring-2 focus-within:ring-accent-500/20"
        }`}
      >
        <div
          role="toolbar"
          aria-label="Note formatting"
          className="flex flex-wrap items-center gap-1 border-b border-rule px-2 py-1.5"
        >
          {button(
            "bold",
            "Bold",
            "Control+B Meta+B",
            "Bold (Ctrl+B or Cmd+B)",
            active.bold,
            <strong aria-hidden="true">B</strong>
          )}
          {button(
            "italic",
            "Italic",
            "Control+I Meta+I",
            "Italic (Ctrl+I or Cmd+I)",
            active.italic,
            <em aria-hidden="true">I</em>
          )}
          {button(
            "insertUnorderedList",
            "Bullet list",
            "Control+Shift+8 Meta+Shift+8",
            "Bullet list (Ctrl+Shift+8 or Cmd+Shift+8)",
            active.bullets,
            <span aria-hidden="true">• List</span>
          )}
          {button(
            "insertOrderedList",
            "Numbered list",
            "Control+Shift+7 Meta+Shift+7",
            "Numbered list (Ctrl+Shift+7 or Cmd+Shift+7)",
            active.numbers,
            <span aria-hidden="true">1. List</span>
          )}
        </div>

        <div
          ref={rootRef}
          role="textbox"
          aria-label={label}
          aria-multiline="true"
          aria-keyshortcuts="Control+B Meta+B Control+I Meta+I Control+Shift+8 Meta+Shift+8 Control+Shift+7 Meta+Shift+7"
          contentEditable={!disabled}
          tabIndex={disabled ? -1 : 0}
          onInput={handleInput}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          onFocus={handleFocus}
          onBlur={handleBlur}
          className="min-h-24 w-full px-3 py-2 text-body leading-relaxed text-ink-900 outline-hidden [&_em]:italic [&_ol]:my-1 [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-1 [&_strong]:font-semibold [&_ul]:my-1 [&_ul]:list-disc [&_ul]:pl-5"
        />
      </div>

      {overLimit ? (
        <p role="alert" className="text-caption leading-relaxed text-danger-700">
          The note is limited to {maxLength} characters. The last edit was
          undone.
        </p>
      ) : null}

      {hint !== undefined && hint !== "" ? (
        <p className="text-caption leading-relaxed text-ink-400">{hint}</p>
      ) : null}

      <p className="text-caption leading-relaxed text-ink-400">
        Bold Ctrl+B · Italic Ctrl+I · Bullet list Ctrl+Shift+8 · Numbered list
        Ctrl+Shift+7 (Cmd on macOS)
      </p>
    </div>
  );
}
