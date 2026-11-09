// The frontend mirror of backend/note (#540).
//
// The note is one signed string, stored as the restricted Markdown subset:
// bold (**text**), italic (*text* or _text_), bullet items (- text), numbered
// items (1. text), paragraphs separated by blank lines, and line breaks inside
// a paragraph. Everything else -- headings, links, images, tables, underline,
// strikethrough, code, raw HTML -- is text, escaped where it is rendered.
//
// One parser decides what the source means, exactly as the Go parser does, so
// the editor's contenteditable, the HTML card and the native PDF renderer
// cannot disagree about it. This module is pure except for the DOM helpers
// (domToMarkdown, sanitizePastedHTML), which take the nodes they read.

export const NOTE_MAX_LENGTH = 1000;

export interface NoteSpan {
  text: string;
  bold: boolean;
  italic: boolean;
}

export interface NoteBlock {
  kind: "paragraph" | "bullets" | "numbers";
  /** First source number for a numbered list; 1 for every other block. */
  start: number;
  /** item -> line -> spans. */
  items: NoteSpan[][][];
}

export interface NoteDocument {
  blocks: NoteBlock[];
}

/**
 * Parses a note source into its block structure. Outer blank lines and
 * whitespace-only lines are ignored; CRLF is normalized to LF. This is the
 * TypeScript port of backend/note.Parse, case for case.
 */
export function parseNote(source: string): NoteDocument {
  const normalized = source.replace(/\r\n/g, "\n").replace(/\r/g, "\n");

  const blocks: NoteBlock[] = [];
  let paragraph: NoteSpan[][] = [];
  let list: NoteBlock | null = null;

  const flushParagraph = (): void => {
    if (paragraph.length === 0) return;
    blocks.push({ kind: "paragraph", start: 1, items: [paragraph] });
    paragraph = [];
  };
  const flushList = (): void => {
    if (list !== null && list.items.length > 0) blocks.push(list);
    list = null;
  };

  for (const raw of normalized.split("\n")) {
    const line = raw.replace(/[ \t]+$/, "");
    if (line.trim() === "") {
      flushParagraph();
      flushList();
      continue;
    }
    const bullet = bulletText(line);
    if (bullet !== null) {
      flushParagraph();
      if (list === null || list.kind !== "bullets") {
        flushList();
        list = { kind: "bullets", start: 1, items: [] };
      }
      const spans = parseInline(bullet);
      if (spans.length > 0) list.items.push([spans]);
      continue;
    }
    const numbered = numberedText(line);
    if (numbered !== null) {
      flushParagraph();
      if (list === null || list.kind !== "numbers") {
        flushList();
        list = { kind: "numbers", start: numbered.number, items: [] };
      }
      const spans = parseInline(numbered.text);
      if (spans.length > 0) list.items.push([spans]);
      continue;
    }
    flushList();
    paragraph.push(parseInline(line.trim()));
  }
  flushParagraph();
  flushList();
  return { blocks };
}

/** Recognizes "- text", including an empty item written as "-". */
function bulletText(line: string): string | null {
  if (line === "-") return "";
  if (!line.startsWith("- ")) return null;
  return line.slice(2).trim();
}

/** Recognizes "1. text"; the number is kept as the list's start. */
function numberedText(line: string): { number: number; text: string } | null {
  let i = 0;
  while (i < line.length && line[i] >= "0" && line[i] <= "9") i++;
  if (i === 0 || i >= line.length || line[i] !== ".") return null;
  if (i + 1 >= line.length || (line[i + 1] !== " " && line[i + 1] !== "\t")) {
    return null;
  }
  const number = Number.parseInt(line.slice(0, i), 10);
  if (!Number.isFinite(number) || number < 1) return null;
  return { number, text: line.slice(i + 1).trim() };
}

/**
 * Turns one line into styled spans. Bold is checked before italic so
 * "**text**" is never read as two empty italic runs. An unmatched marker is
 * literal text, and an underscore only emphasises at a word boundary, so a
 * plain-text value such as snake_case_name survives.
 */
function parseInline(text: string): NoteSpan[] {
  const spans: NoteSpan[] = [];
  const add = (value: string, bold: boolean, italic: boolean): void => {
    if (value === "") return;
    const last = spans[spans.length - 1];
    if (last !== undefined && last.bold === bold && last.italic === italic) {
      last.text += value;
      return;
    }
    spans.push({ text: value, bold, italic });
  };

  let start = 0;
  let i = 0;
  while (i < text.length) {
    if (text.startsWith("**", i)) {
      const close = text.indexOf("**", i + 2);
      if (close > i + 2) {
        const content = text.slice(i + 2, close);
        if (emphasizable(content)) {
          add(text.slice(start, i), false, false);
          add(content, true, false);
          i = close + 2;
          start = i;
          continue;
        }
      }
    }
    const marker = text[i];
    if (marker === "*" || marker === "_") {
      const close = text.indexOf(marker, i + 1);
      if (close > i + 1) {
        const content = text.slice(i + 1, close);
        const boundaryOK =
          marker !== "_" ||
          (wordBoundaryBefore(text, i) && wordBoundaryAfter(text, close + 1));
        const notBoldMarker = text[i + 1] !== marker;
        if (boundaryOK && notBoldMarker && emphasizable(content)) {
          add(text.slice(start, i), false, false);
          add(content, false, true);
          i = close + 1;
          start = i;
          continue;
        }
      }
    }
    i++;
  }
  add(text.slice(start), false, false);
  return spans;
}

/**
 * A marker pair whose content starts or ends with whitespace is literal text,
 * which keeps "2 * 3 * 4" and "a ** b ** c" printing as typed.
 */
function emphasizable(content: string): boolean {
  return content !== "" && content === content.trim();
}

function wordBoundaryBefore(text: string, i: number): boolean {
  return i === 0 || !isWordChar(text[i - 1]);
}

function wordBoundaryAfter(text: string, i: number): boolean {
  return i >= text.length || !isWordChar(text[i]);
}

function isWordChar(value: string): boolean {
  return value === "_" || /[A-Za-z0-9]/.test(value);
}

const HTML_ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&#34;",
  "'": "&#39;",
};

/** Escapes text exactly as Go's html.EscapeString does. */
function escapeHTML(text: string): string {
  return text.replace(/[&<>"']/g, (character) => HTML_ESCAPES[character]);
}

/**
 * Renders a note source to the safe markup the quotation card prints. Every
 * text span is escaped first; the only tags in the output are the ones this
 * function writes. An empty document renders as an empty string, so a
 * whitespace-only note adds no card.
 */
export function noteToHTML(source: string): string {
  const document = parseNote(source);
  let html = "";
  for (const block of document.blocks) {
    if (block.kind === "paragraph") {
      const item = block.items[0];
      if (item === undefined) continue;
      html += "<p>";
      html += renderItem(item);
      html += "</p>";
      continue;
    }
    if (block.kind === "bullets") {
      html += "<ul>";
      for (const item of block.items) html += `<li>${renderItem(item)}</li>`;
      html += "</ul>";
      continue;
    }
    html += block.start === 1 ? "<ol>" : `<ol start="${block.start}">`;
    for (const item of block.items) html += `<li>${renderItem(item)}</li>`;
    html += "</ol>";
  }
  return html;
}

/** One item's lines, separated by <br>. */
function renderItem(item: NoteSpan[][]): string {
  return item
    .map((line) =>
      line
        .map((span) => {
          const text = escapeHTML(span.text);
          if (span.bold) return `<strong>${text}</strong>`;
          if (span.italic) return `<em>${text}</em>`;
          return text;
        })
        .join("")
    )
    .join("<br>");
}

const ELEMENT_NODE = 1;
const TEXT_NODE = 3;

interface InlineToken {
  text: string;
  bold: boolean;
  italic: boolean;
}

interface NoteStyle {
  bold: boolean;
  italic: boolean;
}

const PLAIN: NoteStyle = { bold: false, italic: false };

/** Block tags that start a new line inside a contenteditable block. */
const BLOCK_BOUNDARY = new Set([
  "p",
  "div",
  "li",
  "ul",
  "ol",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "blockquote",
  "pre",
  "section",
  "article",
  "header",
  "footer",
  "main",
  "aside",
  "nav",
  "table",
  "thead",
  "tbody",
  "tfoot",
  "tr",
  "td",
  "th",
  "figure",
  "figcaption",
]);

/** Elements whose content is never note text. */
const DROPPED_CONTENT = new Set([
  "script",
  "style",
  "noscript",
  "template",
  "iframe",
  "object",
  "embed",
  "svg",
  "math",
  "link",
  "meta",
  "title",
  "head",
  "base",
  "img",
  "input",
  "textarea",
  "select",
  "option",
  "button",
]);

/**
 * Collects a node's inline content into lines. A <br> or a block boundary
 * starts a new line; bold/italic wrappers set the style of everything inside
 * them. Trailing empty lines (the filler <br> a browser leaves in an empty
 * block) are dropped.
 */
function collectLines(node: Node, style: NoteStyle): InlineToken[][] {
  const lines: InlineToken[][] = [];
  let current: InlineToken[] = [];
  const flush = (): void => {
    lines.push(current);
    current = [];
  };

  const walk = (child: Node, currentStyle: NoteStyle): void => {
    if (child.nodeType === TEXT_NODE) {
      const text = child.nodeValue ?? "";
      if (text !== "") {
        current.push({ text, bold: currentStyle.bold, italic: currentStyle.italic });
      }
      return;
    }
    if (child.nodeType !== ELEMENT_NODE) return;
    const element = child as HTMLElement;
    const tag = element.tagName.toLowerCase();
    if (tag === "br") {
      flush();
      return;
    }
    if (tag === "strong" || tag === "b") {
      for (const grandchild of Array.from(element.childNodes)) {
        walk(grandchild, { bold: true, italic: currentStyle.italic });
      }
      return;
    }
    if (tag === "em" || tag === "i") {
      for (const grandchild of Array.from(element.childNodes)) {
        walk(grandchild, { bold: currentStyle.bold, italic: true });
      }
      return;
    }
    if (BLOCK_BOUNDARY.has(tag)) {
      flush();
      for (const grandchild of Array.from(element.childNodes)) {
        walk(grandchild, currentStyle);
      }
      flush();
      return;
    }
    for (const grandchild of Array.from(element.childNodes)) {
      walk(grandchild, currentStyle);
    }
  };

  for (const child of Array.from(node.childNodes)) walk(child, style);
  flush();
  return trimTrailingEmptyLines(lines);
}

function trimTrailingEmptyLines(lines: InlineToken[][]): InlineToken[][] {
  const kept = [...lines];
  while (kept.length > 0 && isEmptyLine(kept[kept.length - 1])) kept.pop();
  return kept;
}

function isEmptyLine(line: InlineToken[]): boolean {
  return line.every((token) => token.text.trim() === "");
}

/** Merges adjacent runs that share a style, then wraps each styled core. */
function renderLine(tokens: InlineToken[]): string {
  const merged: InlineToken[] = [];
  for (const token of tokens) {
    if (token.text === "") continue;
    const last = merged[merged.length - 1];
    if (last !== undefined && last.bold === token.bold && last.italic === token.italic) {
      last.text += token.text;
      continue;
    }
    merged.push({ ...token });
  }
  return merged
    .map((token) => {
      if (!token.bold && !token.italic) return token.text;
      const leading = /^\s*/.exec(token.text)?.[0] ?? "";
      const trailing = /\s*$/.exec(token.text)?.[0] ?? "";
      const core = token.text.slice(
        leading.length,
        token.text.length - trailing.length
      );
      // A marker pair whose content starts or ends with whitespace would be
      // literal text, so the spaces stay outside the markers.
      if (core === "") return token.text;
      const marker = token.bold ? "**" : "*";
      return `${leading}${marker}${core}${marker}${trailing}`;
    })
    .join("");
}

type SerializedBlock =
  | { kind: "paragraph"; lines: string[] }
  | { kind: "bullets"; items: string[] }
  | { kind: "numbers"; start: number; items: string[] };

/**
 * Serializes a contenteditable tree to the note source. Browsers produce a
 * handful of shapes for the same intent (<div> per line, <p> per block, <b>
 * or <strong> for bold), so the serializer reads structure, not one browser's
 * exact markup. It never emits a tag: the output is the restricted source.
 */
export function domToMarkdown(root: HTMLElement): string {
  const blocks: SerializedBlock[] = [];
  let inline: Node[] = [];
  const document = root.ownerDocument;

  const flushInline = (): void => {
    if (inline.length === 0) return;
    const wrapper = document.createElement("div");
    for (const node of inline) wrapper.appendChild(node.cloneNode(true));
    inline = [];
    pushParagraph(blocks, collectLines(wrapper, PLAIN));
  };

  for (const child of Array.from(root.childNodes)) {
    if (child.nodeType === ELEMENT_NODE) {
      const tag = (child as HTMLElement).tagName.toLowerCase();
      if (tag === "ul" || tag === "ol") {
        flushInline();
        const list = serializeList(child as HTMLElement);
        if (list !== null) blocks.push(list);
        continue;
      }
      if (BLOCK_BOUNDARY.has(tag)) {
        flushInline();
        pushParagraph(blocks, collectLines(child, PLAIN));
        continue;
      }
    }
    inline.push(child);
  }
  flushInline();

  return blocks
    .map((block) => {
      if (block.kind === "paragraph") return block.lines.join("\n");
      if (block.kind === "bullets") {
        return block.items.map((item) => `- ${item}`).join("\n");
      }
      return block.items
        .map((item, index) => `${block.start + index}. ${item}`)
        .join("\n");
    })
    .filter((block) => block.trim() !== "")
    .join("\n\n");
}

function pushParagraph(blocks: SerializedBlock[], lines: InlineToken[][]): void {
  if (lines.length === 0) return;
  blocks.push({ kind: "paragraph", lines: lines.map(renderLine) });
}

function serializeList(element: HTMLElement): SerializedBlock | null {
  const items: string[] = [];
  for (const child of Array.from(element.children)) {
    if (child.tagName.toLowerCase() !== "li") continue;
    const lines = collectLines(child, PLAIN);
    const text = lines.map(renderLine).join(" ").trim();
    if (text !== "") items.push(text);
  }
  if (items.length === 0) return null;
  if (element.tagName.toLowerCase() === "ul") {
    return { kind: "bullets", items };
  }
  const parsed = Number.parseInt(element.getAttribute("start") ?? "1", 10);
  const start = Number.isFinite(parsed) && parsed >= 1 ? parsed : 1;
  return { kind: "numbers", start, items };
}

/**
 * Turns plain clipboard text into note source. Line endings are normalized
 * and outer blank lines are dropped; internal line breaks are content and
 * stay. Plain prose is valid note source exactly as typed.
 */
export function plainTextToMarkdown(text: string): string {
  const lines = text
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .split("\n")
    .map((line) => line.replace(/[ \t]+$/, ""));
  while (lines.length > 0 && lines[0].trim() === "") lines.shift();
  while (lines.length > 0 && lines[lines.length - 1].trim() === "") lines.pop();
  return lines.join("\n");
}

const ALLOWED_TAGS = new Set([
  "p",
  "div",
  "br",
  "ul",
  "ol",
  "li",
  "strong",
  "em",
]);

/**
 * Sanitizes pasted HTML to the supported subset and returns note source, or
 * null when the paste carries no usable text. Scripts, styles, links, images
 * and every other tag are dropped (link labels and unknown inline text are
 * kept); only the allowlisted tags survive, so the result is source, never
 * raw HTML.
 */
export function sanitizePastedHTML(html: string): string | null {
  if (typeof DOMParser === "undefined") return null;
  let parsed: Document;
  try {
    parsed = new DOMParser().parseFromString(html, "text/html");
  } catch {
    return null;
  }
  if (!parsed.body) return null;
  const container = parsed.createElement("div");
  for (const child of Array.from(parsed.body.childNodes)) {
    copyPasteNode(child, container, parsed);
  }
  if ((container.textContent ?? "").trim() === "") return null;
  const markdown = domToMarkdown(container);
  return markdown.trim() === "" ? null : markdown;
}

function copyPasteNode(source: Node, target: Node, document: Document): void {
  if (source.nodeType === TEXT_NODE) {
    target.appendChild(document.createTextNode(source.nodeValue ?? ""));
    return;
  }
  if (source.nodeType !== ELEMENT_NODE) return;
  const element = source as HTMLElement;
  const tag = element.tagName.toLowerCase();
  if (DROPPED_CONTENT.has(tag)) return;

  const mapped = tag === "b" ? "strong" : tag === "i" ? "em" : tag;
  if (ALLOWED_TAGS.has(mapped)) {
    if (mapped === "br") {
      target.appendChild(document.createElement("br"));
      return;
    }
    const copy = document.createElement(mapped);
    if (mapped === "ol") {
      const start = element.getAttribute("start");
      if (start !== null) copy.setAttribute("start", start);
    }
    for (const child of Array.from(element.childNodes)) {
      copyPasteNode(child, copy, document);
    }
    target.appendChild(copy);
    return;
  }

  if (BLOCK_BOUNDARY.has(tag)) {
    // An unsupported block (a heading, a table cell) keeps its text but not
    // its identity: it becomes a plain block so the words do not run together.
    const copy = document.createElement("div");
    for (const child of Array.from(element.childNodes)) {
      copyPasteNode(child, copy, document);
    }
    target.appendChild(copy);
    return;
  }

  // Unknown inline markup (a link, a span, a font tag): keep the words.
  for (const child of Array.from(element.childNodes)) {
    copyPasteNode(child, target, document);
  }
}

/** Rune count, not UTF-16 units: the server counts code points too. */
export function noteLength(source: string): number {
  return Array.from(source).length;
}
