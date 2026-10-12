import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Guard for issue #342: a <Button> (or plain <button>) with no `type` inside
// a <form> is an implicit submit button. Two Cancel buttons relied on that
// default and created the record on the way out.
//
// The suite runs in node with no DOM (see vitest.config.ts), so this cannot
// be a rendered-interaction test. It scans the component sources instead:
// every <Button>/<button> lexically inside a <form>...</form> region must
// carry an explicit `type`.
//
// The legitimate submit case is deliberately strict: a button that means to
// submit must say type="submit" explicitly. Nothing inside a form may rely
// on the default — that is the whole point.

const SRC_DIR = dirname(fileURLToPath(import.meta.url));

interface FormRegion {
  file: string;
  openLine: number;
  start: number;
  end: number;
}

interface Violation {
  file: string;
  line: number;
  tag: string;
  formLine: number;
}

/** All .tsx sources, found by walking the tree — never a hand-written list,
 *  so a new file is visible to this test. Test files are excluded: they
 *  build markup inside string literals (e.g. split("<button")), which is
 *  not UI and must not be flagged. */
function collectSources(dir: string): string[] {
  const found: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      found.push(...collectSources(path));
    } else if (entry.isFile() && entry.name.endsWith(".tsx") && !entry.name.endsWith(".test.tsx")) {
      found.push(path);
    }
  }
  return found.sort();
}

/** Spans of block comments, so a token inside one is not mistaken for code.
 *  (Quoted strings need no equivalent treatment: no non-test source carries
 *  a <form>/<Button>/<button> token inside a string literal — verified by
 *  grep while writing this test.) */
function blockCommentSpans(src: string): Array<[number, number]> {
  const spans: Array<[number, number]> = [];
  const re = /\/\*[\s\S]*?\*\//g;
  let match: RegExpExecArray | null;
  while ((match = re.exec(src)) !== null) {
    spans.push([match.index, match.index + match[0].length]);
  }
  return spans;
}

function inSpans(spans: Array<[number, number]>, pos: number): boolean {
  return spans.some(([start, end]) => start <= pos && pos < end);
}

/** True when the token at pos sits after a // comment opener on its line.
 *  The opener counts only when the line holds no double quote before it, so
 *  a URL in code (https://…) is never read as a comment. */
function isLineCommented(src: string, pos: number): boolean {
  const lineStart = src.lastIndexOf("\n", pos - 1) + 1;
  const before = src.slice(lineStart, pos);
  const opener = before.indexOf("//");
  return opener !== -1 && !before.slice(0, opener).includes('"');
}

function lineOf(src: string, pos: number): number {
  return src.slice(0, pos).split("\n").length;
}

/** The full opening tag starting at pos, scanned quote- and brace-aware so
 *  that a `>` inside an expression container (e.g. onClick={() => void x()})
 *  never ends the tag early. Braces inside strings are ignored, so a `}`
 *  in a label cannot unbalance the depth. */
function openingTag(src: string, pos: number): string {
  let depth = 0;
  let quote: string | null = null;
  for (let i = pos; i < src.length; i++) {
    const char = src[i];
    if (quote !== null) {
      if (char === quote) quote = null;
      continue;
    }
    if (char === '"' || char === "'" || char === "`") {
      quote = char;
    } else if (char === "{") {
      depth++;
    } else if (char === "}") {
      depth--;
    } else if (char === ">" && depth === 0) {
      return src.slice(pos, i + 1);
    }
  }
  throw new Error(`unterminated tag at offset ${pos}`);
}

/** True when the opening tag carries an explicit type at its own level —
 *  type="button", type="submit", or type={…}. An attribute named `type`
 *  nested inside an expression container does not count. */
function hasExplicitType(tag: string): boolean {
  let depth = 0;
  let quote: string | null = null;
  for (let i = 0; i < tag.length; i++) {
    const char = tag[i];
    if (quote !== null) {
      if (char === quote) quote = null;
      continue;
    }
    if (char === '"' || char === "'" || char === "`") {
      quote = char;
    } else if (char === "{") {
      depth++;
    } else if (char === "}") {
      depth--;
    } else if (depth === 0 && tag.startsWith("type", i)) {
      const before = tag[i - 1];
      const after = tag.slice(i + 4);
      if ((before === undefined || /[\s</]/.test(before)) && /^[\s]*=/.test(after)) {
        return true;
      }
    }
  }
  return false;
}

/** Exported so the edge-case block below can pin the scanner directly. */
export function scanFile(file: string, src: string): { forms: FormRegion[]; violations: Violation[] } {
  const comments = blockCommentSpans(src);
  const token = /<form(?=[\s>])|<\/form>|<Button(?=[\s>/])|<button(?=[\s>/])/g;
  const forms: FormRegion[] = [];
  const violations: Violation[] = [];
  let open: { start: number; line: number } | null = null;
  let match: RegExpExecArray | null;
  while ((match = token.exec(src)) !== null) {
    if (inSpans(comments, match.index) || isLineCommented(src, match.index)) continue;
    const kind = match[0];
    if (kind === "<form") {
      if (open !== null) {
        throw new Error(
          `${file}:${lineOf(src, match.index)}: nested <form> inside the form opened at line ${open.line} — ` +
            "this scanner pairs each <form> with the next </form> and cannot see through nesting"
        );
      }
      open = { start: match.index, line: lineOf(src, match.index) };
    } else if (kind === "</form>") {
      if (open === null) {
        throw new Error(`${file}:${lineOf(src, match.index)}: </form> with no open <form>`);
      }
      forms.push({ file, openLine: open.line, start: open.start, end: match.index });
      open = null;
    } else if (open !== null) {
      const tag = openingTag(src, match.index);
      if (!hasExplicitType(tag)) {
        violations.push({
          file,
          line: lineOf(src, match.index),
          tag: tag.replace(/\s+/g, " ").slice(0, 160),
          formLine: open.line,
        });
      }
    }
  }
  if (open !== null) {
    throw new Error(`${file}:${open.line}: <form> never closed`);
  }
  return { forms, violations };
}

describe("the form-button scanner itself", () => {
  it("counts a type on a later line of a multi-line opening as typed", () => {
    const src = [
      "<form onSubmit={submit}>",
      "  <Button",
      '    variant="ghost"',
      '    type="button"',
      "    onClick={onCancel}>",
      "    Cancel",
      "  </Button>",
      "</form>",
      "",
    ].join("\n");
    expect(scanFile("multi.tsx", src).violations).toEqual([]);
  });

  it("does not end the tag at a > inside an expression container", () => {
    const src = [
      "<form onSubmit={submit}>",
      "  <Button variant={busy ? a : b} onClick={() => void x()}",
      '    type="submit">',
      "    Save",
      "  </Button>",
      "</form>",
      "",
    ].join("\n");
    expect(scanFile("arrow.tsx", src).violations).toEqual([]);
  });

  it("flags a bare button with its line and reports the form it sits in", () => {
    const src = [
      "<form onSubmit={submit}>",
      "  <Button",
      '    variant="ghost"',
      "    onClick={onCancel}>",
      "    Cancel",
      "  </Button>",
      "</form>",
      "",
    ].join("\n");
    const result = scanFile("bare.tsx", src);
    expect(result.forms.length).toBe(1);
    expect(result.violations.length).toBe(1);
    expect(result.violations[0].line).toBe(2);
    expect(result.violations[0].formLine).toBe(1);
  });

  it("ignores a <form> token inside a line comment", () => {
    const src = [
      "// One <form> for both states: not a form.",
      "<form onSubmit={save}>",
      '  <Button type="submit">Save</Button>',
      "</form>",
      "",
    ].join("\n");
    const result = scanFile("comment.tsx", src);
    expect(result.forms.length).toBe(1);
    expect(result.forms[0].openLine).toBe(2);
    expect(result.violations).toEqual([]);
  });
});

describe("no bare button inside a form (issue #342)", () => {
  it("requires an explicit type on every Button/button lexically inside a <form>", () => {
    const files = collectSources(SRC_DIR);
    expect(files.length, "expected the source walk to find component files").toBeGreaterThan(0);
    const forms: FormRegion[] = [];
    const violations: Violation[] = [];
    let buttonsChecked = 0;
    for (const file of files) {
      const src = readFileSync(file, "utf8");
      const result = scanFile(file, src);
      forms.push(...result.forms);
      violations.push(...result.violations);
      for (const form of result.forms) {
        buttonsChecked += src.slice(form.start, form.end).split(/<Button(?=[\s>/])|<button(?=[\s>/])/).length - 1;
      }
    }
    // The scan must actually see forms and buttons: an empty scan passing
    // would prove nothing (a guard that cannot fail is not a guard).
    expect(forms.length, "expected the scan to find at least one <form>").toBeGreaterThan(0);
    expect(buttonsChecked, "expected the scan to find buttons inside forms").toBeGreaterThan(0);
    expect(
      violations.map((v) => `${v.file}:${v.line}: bare button in form opened at line ${v.formLine} — ${v.tag}`),
      "every Button/button inside a <form> must say type=\"button\" or type=\"submit\" explicitly; nothing may rely on the implicit submit default"
    ).toEqual([]);
  });
});
