/**
 * Release notes rendering.
 *
 * The changelog is markdown written by hand in CHANGELOG.md and embedded at
 * compile time. It used to render as one preformatted blob, so a sentence
 * wrapped at 80 columns in the file read as a line break on screen and the
 * file's own backticks and asterisks showed as literal characters. This
 * module handles the subset of markdown the notes actually use -- paragraphs,
 * bullets, bold, italic, inline code and links -- and nothing else.
 *
 * Everything is produced as React elements rather than HTML strings. That is
 * the security property, not a style preference: the notes render inside a
 * WebView that can reach the filesystem and the Go bridge, so a hand-written
 * release note must never be able to introduce markup. React escapes every
 * text node it is given, so there is no escaping step to get wrong and no
 * `dangerouslySetInnerHTML` anywhere in the path.
 */

export interface NoteParagraph {
  /** Bulleted items and free paragraphs, in source order. */
  blocks: NoteBlock[];
}

export type NoteBlock =
  | { kind: "list"; items: string[] }
  | { kind: "para"; text: string };

export interface VersionNotes {
  version: string;
  sections: { title: string; body: NoteParagraph }[];
}

function parseSemver(value: string): [number, number, number] | null {
  const match = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(value.trim());
  if (!match) return null;
  return [Number(match[1]), Number(match[2]), Number(match[3])];
}

/** Orders two version strings. Unparseable versions compare equal. */
export function compareSemver(a: string, b: string): number {
  const pa = parseSemver(a);
  const pb = parseSemver(b);
  if (!pa || !pb) return 0;
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] < pb[i] ? -1 : 1;
  }
  return 0;
}

export function isReleaseVersion(value: string): boolean {
  return parseSemver(value) !== null;
}

/**
 * Turns one section's body into an ordered list of blocks.
 *
 * A `- ` starts a list item and its continuation lines join it, so a bullet
 * wrapped across three lines in the file is still one bullet on screen. Lines
 * that are not bullets, and not continuations, accumulate into a paragraph --
 * a blank line ends the paragraph.
 */
function parseBlocks(body: string): NoteBlock[] {
  const blocks: NoteBlock[] = [];
  let list: string[] = [];
  let paragraph: string[] = [];

  const flushList = () => {
    if (list.length > 0) blocks.push({ kind: "list", items: list });
    list = [];
  };
  const flushParagraph = () => {
    if (paragraph.length > 0) blocks.push({ kind: "para", text: paragraph.join(" ") });
    paragraph = [];
  };

  for (const raw of body.split("\n")) {
    const line = raw.trim();
    if (line === "") {
      flushParagraph();
      flushList();
      continue;
    }
    const bullet = /^[-*]\s+(.*)$/.exec(line);
    if (bullet) {
      flushParagraph();
      list.push(bullet[1].trim());
      continue;
    }
    // A continuation only extends a bullet; with no bullet open it opens a
    // paragraph.
    if (list.length > 0 && /^\s/.test(raw)) {
      list[list.length - 1] += " " + line;
      continue;
    }
    flushList();
    paragraph.push(line);
  }
  flushParagraph();
  flushList();
  return blocks;
}

/**
 * Parse `## [x.y.z]` blocks with `###` sections. Tolerant by design: a
 * malformed entry is skipped rather than throwing, because a broken note must
 * still be readable and the modal must still open.
 */
export function parseChangelog(raw: string): VersionNotes[] {
  const versions: VersionNotes[] = [];
  const blocks = raw.split(/^## \[/m).slice(1);
  for (const block of blocks) {
    const head = /^([^\]]+)\]/.exec(block);
    if (!head) continue;
    const version = head[1].trim();
    const sections: VersionNotes["sections"] = [];
    for (const part of block.split(/^### /m).slice(1)) {
      const newline = part.indexOf("\n");
      const title = (newline < 0 ? part : part.slice(0, newline)).trim();
      const body = newline < 0 ? "" : part.slice(newline + 1);
      // The body runs to the next `###` or the end of the block, both of
      // which the split above has already removed.
      if (title !== "") sections.push({ title, body: { blocks: parseBlocks(body) } });
    }
    versions.push({ version, sections });
  }
  return versions;
}

export type InlineNode =
  | { kind: "text"; value: string }
  | { kind: "bold" | "italic" | "code"; value: string }
  | { kind: "link"; url: string; label: InlineNode[] };

/**
 * Only https and mailto become links. A `javascript:` or `data:` URL in a
 * release note must stay inert text -- this is a privileged context and the
 * notes are hand-edited text that ships inside the binary.
 */
function safeUrl(raw: string): string | null {
  const url = raw.trim();
  if (!/^https:\/\//i.test(url) && !/^mailto:/i.test(url)) return null;
  // Control characters can smuggle a scheme past a prefix check in some URL
  // parsers, so they disqualify the link outright.
  if (/[\u0000-\u001f\u007f]/.test(url)) return null;
  return url;
}

/**
 * Built fresh per call. A module-level /g regex carries a lastIndex, and the
 * link-label case recurses -- so a shared one would have the inner call reset
 * the outer's position and skip or repeat text. A new regex per call costs
 * nothing at these sizes.
 */
function inlinePattern(): RegExp {
  return /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*\n]+\*)|(\[[^\]\n]+\]\([^)\s]+\))/g;
}

/**
 * Parses the inline subset into nodes. Anything not matched comes through as
 * a text node, so unknown syntax degrades to readable text and never to an
 * error -- and, because it is a text node, it is escaped.
 */
export function parseInline(text: string): InlineNode[] {
  const nodes: InlineNode[] = [];
  let last = 0;
  const pattern = inlinePattern();
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(text)) !== null) {
    if (match.index > last) {
      nodes.push({ kind: "text", value: text.slice(last, match.index) });
    }
    last = match.index + match[0].length;
    const token = match[0];

    if (token.startsWith("`")) {
      // Code spans are literal: no nested parsing, and no bold inside them.
      nodes.push({ kind: "code", value: token.slice(1, -1) });
    } else if (token.startsWith("**")) {
      nodes.push({ kind: "bold", value: token.slice(2, -2) });
    } else if (token.startsWith("*")) {
      nodes.push({ kind: "italic", value: token.slice(1, -1) });
    } else {
      const link = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(token);
      const url = link ? safeUrl(link[2]) : null;
      if (link && url) {
        nodes.push({ kind: "link", url, label: parseInline(link[1]) });
      } else {
        // Not a link we are willing to make. Show the source, not a guess.
        nodes.push({ kind: "text", value: token });
      }
    }
  }
  if (last < text.length) nodes.push({ kind: "text", value: text.slice(last) });
  return nodes;
}

export function inlineToText(nodes: InlineNode[]): string {
  return nodes
    .map((node) => {
      switch (node.kind) {
        case "text":
        case "bold":
        case "italic":
        case "code":
          return node.value;
        case "link":
          return inlineToText(node.label);
      }
    })
    .join("");
}

/** Flattens a section to plain text, for tests and for aria labels. */
export function blockToText(block: NoteBlock): string {
  return block.kind === "list"
    ? block.items.map((item) => inlineToText(parseInline(item))).join(" ")
    : inlineToText(parseInline(block.text));
}

