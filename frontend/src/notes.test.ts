import { readFileSync } from "node:fs";
import path from "node:path";
import {
  blockToText,
  compareSemver,
  isReleaseVersion,
  parseChangelog,
  parseInline,
} from "./notes";

const kinds = (nodes: ReturnType<typeof parseInline>) => nodes.map((n) => n.kind);
const allText = (nodes: ReturnType<typeof parseInline>) =>
  nodes
    .map((n) =>
      n.kind === "link"
        ? n.label.map((c) => (c.kind === "link" ? "" : c.value)).join("")
        : n.value
    )
    .join("");

describe("parseInline", () => {
  it("keeps plain text as a single text node", () => {
    expect(kinds(parseInline("just a sentence"))).toEqual(["text"]);
    expect(allText(parseInline("just a sentence"))).toBe("just a sentence");
  });

  it("renders bold, italic and code", () => {
    expect(parseInline("a **bold** b")).toMatchObject([
      { kind: "text", value: "a " },
      { kind: "bold", value: "bold" },
      { kind: "text", value: " b" },
    ]);
    expect(parseInline("an *italic* word")).toMatchObject([
      { kind: "text", value: "an " },
      { kind: "italic", value: "italic" },
      { kind: "text", value: " word" },
    ]);
    expect(parseInline("a `code` span")).toMatchObject([
      { kind: "text", value: "a " },
      { kind: "code", value: "code" },
      { kind: "text", value: " span" },
    ]);
  });

  it("does not parse inside a code span", () => {
    // Backticks in a note show a literal command; ** inside one stays literal.
    expect(parseInline("`**not bold**`")).toEqual([
      { kind: "code", value: "**not bold**" },
    ]);
  });

  it("makes an https or mailto link a link", () => {
    expect(parseInline("see [the docs](https://example.com/a) now")).toMatchObject([
      { kind: "text", value: "see " },
      { kind: "link", url: "https://example.com/a" },
      { kind: "text", value: " now" },
    ]);
    expect(parseInline("[mail](mailto:a@b.co)")[0]).toMatchObject({
      kind: "link",
      url: "mailto:a@b.co",
    });
  });

  it("refuses to link a dangerous scheme and shows the source instead", () => {
    // The notes render inside a privileged WebView. A hand-edited release
    // note must not be able to introduce a javascript: or data: URL.
    for (const raw of [
      "[click](javascript:alert)",
      "[click](data:text/html,boom)",
      "[click](file:///etc/passwd)",
      "[click](vbscript:msgbox)",
    ]) {
      const nodes = parseInline(raw);
      expect(nodes.some((n) => n.kind === "link")).toBe(false);
      expect(nodes.every((n) => n.kind === "text")).toBe(true);
      expect(allText(nodes)).toBe(raw);
    }
  });

  it("refuses a link carrying control characters", () => {
    // A control character inside a URL can smuggle a scheme past a parser
    // downstream, so it disqualifies the link outright.
    const nodes = parseInline("[x](https://a.co/" + String.fromCharCode(0) + "evil)");
    expect(nodes.some((n) => n.kind === "link")).toBe(false);
  });

  it("parses formatting inside a link label", () => {
    const node = parseInline("[**bold** link](https://a.co)")[0];
    expect(node.kind).toBe("link");
    if (node.kind === "link") {
      expect(node.label).toMatchObject([
        { kind: "bold", value: "bold" },
        { kind: "text", value: " link" },
      ]);
    }
  });

  it("keeps the tail after recursing into a link label", () => {
    // The label is parsed by this same function. If the scan position were
    // shared between the nested and outer call, the trailing text would be
    // swallowed -- this is the regression test for exactly that.
    const nodes = parseInline("[**b** c](https://a.co) and then more");
    expect(allText(nodes)).toContain("and then more");
    expect(nodes[nodes.length - 1]).toEqual({ kind: "text", value: " and then more" });
  });

  it("degrades unknown syntax to readable text", () => {
    for (const raw of ["a ** b", "***", "[unclosed", "](#anchor)", "| a | b |"]) {
      const nodes = parseInline(raw);
      expect(nodes.every((n) => ["text", "bold", "italic"].includes(n.kind))).toBe(true);
      expect(allText(nodes)).toBe(raw);
    }
  });

  it("never produces markup as a link or element", () => {
    // The point of emitting nodes: React escapes these text nodes, so an HTML
    // string in a release note is displayed, not interpreted.
    const nodes = parseInline("<img src=x onerror=alert(1)> **bold**");
    expect(allText(nodes)).toContain("<img src=x onerror=alert(1)>");
    expect(kinds(nodes)).toContain("bold");
  });
});

describe("parseChangelog blocks", () => {
  const raw = [
    "## [0.6.0] - 2026-10-09",
    "",
    "### Added",
    "",
    "- A bullet that is written across",
    "  three source lines and must read as one.",
    "- A second bullet.",
    "",
    "A free paragraph that is also",
    "wrapped across two lines.",
    "",
    "### Fixed",
    "",
    "- Something.",
    "",
  ].join("\n");

  const notes = parseChangelog(raw);

  it("finds the version and its sections", () => {
    expect(notes).toHaveLength(1);
    expect(notes[0].version).toBe("0.6.0");
    expect(notes[0].sections.map((s) => s.title)).toEqual(["Added", "Fixed"]);
  });

  it("joins a wrapped bullet into one item", () => {
    expect(notes[0].sections[0].body.blocks[0]).toEqual({
      kind: "list",
      items: [
        "A bullet that is written across three source lines and must read as one.",
        "A second bullet.",
      ],
    });
  });

  it("keeps a free paragraph as one paragraph, also unwrapped", () => {
    expect(notes[0].sections[0].body.blocks[1]).toEqual({
      kind: "para",
      text: "A free paragraph that is also wrapped across two lines.",
    });
  });

  it("skips a malformed block rather than throwing", () => {
    expect(parseChangelog("## [not-a-version]\n\n### Added\n\n- x\n")).toHaveLength(1);
    expect(parseChangelog("garbage")).toEqual([]);
    expect(parseChangelog("")).toEqual([]);
  });
});

describe("the real changelog", () => {
  const raw = readFileSync(path.resolve(__dirname, "..", "..", "CHANGELOG.md"), "utf8");
  const notes = parseChangelog(raw);

  it("parses every version block", () => {
    expect(notes.length).toBeGreaterThan(0);
    expect(notes.every((n) => isReleaseVersion(n.version))).toBe(true);
  });

  it("loses no bullet from any entry", () => {
    // A release note that silently disappeared is worse than one that reads
    // as preformatted text.
    const sourceBullets = (raw.match(/^\s*[-*] /gm) ?? []).length;
    const parsedBullets = notes.reduce(
      (total, entry) =>
        total +
        entry.sections.reduce(
          (sectionTotal, section) =>
            sectionTotal +
            section.body.blocks.reduce(
              (blockTotal, block) =>
                blockTotal + (block.kind === "list" ? block.items.length : 0),
              0
            ),
          0
        ),
      0
    );
    expect(parsedBullets).toBe(sourceBullets);
  });

  it("renders every block as non-empty text", () => {
    for (const entry of notes) {
      for (const section of entry.sections) {
        expect(section.title.trim()).not.toBe("");
        for (const block of section.body.blocks) {
          expect(blockToText(block).trim()).not.toBe("");
        }
      }
    }
  });
});

describe("versions", () => {
  it("orders releases", () => {
    expect(compareSemver("v0.5.1", "v0.5.0")).toBe(1);
    expect(compareSemver("v0.5.0", "v0.6.0")).toBe(-1);
    expect(compareSemver("0.6.0", "v0.6.0")).toBe(0);
    expect(compareSemver("dev", "v0.6.0")).toBe(0);
  });

  it("recognises only real release versions", () => {
    expect(isReleaseVersion("v0.5.1")).toBe(true);
    expect(isReleaseVersion("0.5.1")).toBe(true);
    expect(isReleaseVersion("v0.5.1-rc1")).toBe(false);
    expect(isReleaseVersion("dev")).toBe(false);
  });
});