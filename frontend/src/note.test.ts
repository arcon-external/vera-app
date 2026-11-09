// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import {
  NOTE_MAX_LENGTH,
  domToMarkdown,
  noteLength,
  noteToHTML,
  parseNote,
  plainTextToMarkdown,
  sanitizePastedHTML,
} from "./note";

// The frontend mirror of backend/note (#540). What it pins is the same external
// behaviour the Go parser pins: the rendered HTML, the escaped text, the block
// and line structure, and the source a contenteditable serializes back to.
// These cases are deliberately the Go test's cases, so the editor and the
// renderer cannot disagree about what the note means.

function render(source: string): string {
  return noteToHTML(source);
}

describe("parseNote / noteToHTML grammar", () => {
  it("keeps a plain note's line breaks", () => {
    const doc = parseNote("Delivery is ex-works.\nInstallation by the customer.");
    expect(doc.blocks).toHaveLength(1);
    expect(doc.blocks[0].kind).toBe("paragraph");
    expect(doc.blocks[0].items[0]).toHaveLength(2);
    expect(render("Delivery is ex-works.\nInstallation by the customer.")).toBe(
      "<p>Delivery is ex-works.<br>Installation by the customer.</p>"
    );
  });

  it("separates paragraphs at a blank line", () => {
    expect(parseNote("First paragraph.\n\nSecond paragraph.").blocks).toHaveLength(2);
    expect(render("First paragraph.\n\nSecond paragraph.")).toBe(
      "<p>First paragraph.</p><p>Second paragraph.</p>"
    );
  });

  it("renders bold and both italic markers as semantic tags", () => {
    expect(render("Please **confirm** access and *arrive early*.")).toBe(
      "<p>Please <strong>confirm</strong> access and <em>arrive early</em>.</p>"
    );
    expect(render("Please _confirm_ access.")).toBe(
      "<p>Please <em>confirm</em> access.</p>"
    );
  });

  it("prints spaced markers literally", () => {
    for (const source of ["2 * 3 * 4", "a ** b ** c", "a _ b _ c"]) {
      expect(render(source)).toBe(`<p>${source}</p>`);
    }
  });

  it("prints unmatched markers literally", () => {
    for (const source of ["2 * 3 = 6", "unclosed **bold", "snake_case_name"]) {
      expect(render(source)).toBe(`<p>${source}</p>`);
    }
  });

  it("renders a bullet list with inline formatting", () => {
    expect(render("Before:\n- **First** item\n- Second item")).toBe(
      "<p>Before:</p><ul><li><strong>First</strong> item</li><li>Second item</li></ul>"
    );
  });

  it("renumbers a numbered list from its first item", () => {
    expect(render("1. Notify security\n1. Log the visit")).toBe(
      "<ol><li>Notify security</li><li>Log the visit</li></ol>"
    );
    const started = parseNote("3. Third\n4. Fourth");
    expect(started.blocks[0].start).toBe(3);
    expect(render("3. Third\n4. Fourth")).toBe(
      '<ol start="3"><li>Third</li><li>Fourth</li></ol>'
    );
  });

  it("escapes raw HTML as text", () => {
    const source = `Delivery <script>alert("x")</script> & <b>bold</b>.`;
    expect(render(source)).toBe(
      "<p>Delivery &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt; &amp; &lt;b&gt;bold&lt;/b&gt;.</p>"
    );
  });

  it("normalizes CRLF and outer blank lines", () => {
    const want = render("Line one\nLine two");
    for (const source of [
      "Line one\r\nLine two",
      "\nLine one\nLine two\n",
      "  \nLine one\nLine two\n\n",
    ]) {
      expect(render(source)).toBe(want);
    }
  });

  it("produces nothing for an empty source", () => {
    for (const source of ["", " ", "\n\t\n"]) {
      expect(parseNote(source).blocks).toHaveLength(0);
      expect(render(source)).toBe("");
    }
  });

  it("drops empty list items", () => {
    expect(render("- First\n- \n- Third")).toBe(
      "<ul><li>First</li><li>Third</li></ul>"
    );
  });
});

describe("domToMarkdown", () => {
  function fromHTML(html: string): string {
    const root = document.createElement("div");
    root.innerHTML = html;
    return domToMarkdown(root);
  }

  it("serializes bold and italic runs", () => {
    expect(
      fromHTML("<p>Please <strong>confirm</strong> access and <em>arrive early</em>.</p>")
    ).toBe("Please **confirm** access and *arrive early*.");
    expect(fromHTML("<p>Use <b>bold</b> and <i>italic</i>.</p>")).toBe(
      "Use **bold** and *italic*."
    );
  });

  it("serializes line breaks in a div and a paragraph", () => {
    expect(fromHTML("<div>Line one<br>Line two</div>")).toBe("Line one\nLine two");
    expect(fromHTML("<p>Line one<br>Line two</p>")).toBe("Line one\nLine two");
  });

  it("serializes bullet and numbered lists", () => {
    expect(fromHTML("<ul><li>First</li><li>Second</li></ul>")).toBe(
      "- First\n- Second"
    );
    expect(fromHTML("<ol><li>One</li><li>Two</li></ol>")).toBe("1. One\n2. Two");
    expect(fromHTML('<ol start="3"><li>Third</li><li>Fourth</li></ol>')).toBe(
      "3. Third\n4. Fourth"
    );
  });

  it("serializes one block per div, as a contenteditable produces", () => {
    expect(fromHTML("<div>First</div><div>Second</div>")).toBe("First\n\nSecond");
  });

  it("drops the filler br of an empty editor", () => {
    expect(fromHTML("<div><br></div>")).toBe("");
    expect(fromHTML("<br>")).toBe("");
  });

  it("serializes a run that carries both styles as bold, never ***", () => {
    // The subset has no bold-italic face: the Go Span prints one style. A
    // contenteditable can still nest the two wrappers, and "***x***" would
    // parse back as a literal asterisk plus partial emphasis.
    expect(fromHTML("<p><strong><em>both</em></strong></p>")).toBe("**both**");
    expect(fromHTML("<p><em><strong>both</strong></em></p>")).toBe("**both**");
    expect(fromHTML("<p><b><i>both</i></b></p>")).toBe("**both**");
    expect(fromHTML("<p><strong><em>both</em></strong></p>")).not.toContain("***");
  });
});

describe("round trip", () => {
  it("preserves a formatted note through HTML and back", () => {
    const source =
      "Delivery is ex-works.\n\n**Important:**\n- *First* item\n- Second item\n\n1. Notify security\n2. Log the visit";
    const root = document.createElement("div");
    root.innerHTML = noteToHTML(source);
    const serialized = domToMarkdown(root);
    expect(parseNote(serialized)).toEqual(parseNote(source));
  });

  it("returns the same source for a simple formatted line", () => {
    const root = document.createElement("div");
    root.innerHTML = noteToHTML("**Bold** and *italic*");
    expect(domToMarkdown(root)).toBe("**Bold** and *italic*");
  });
});

describe("plainTextToMarkdown", () => {
  it("normalizes line endings and keeps internal breaks", () => {
    expect(plainTextToMarkdown("Line one\r\nLine two")).toBe("Line one\nLine two");
    expect(plainTextToMarkdown("Line one\rLine two")).toBe("Line one\nLine two");
  });

  it("drops outer blank lines and trailing spaces", () => {
    expect(plainTextToMarkdown("\n  \nHello  \nWorld\t\n\n")).toBe("Hello\nWorld");
  });

  it("returns empty for whitespace only", () => {
    expect(plainTextToMarkdown("   \n\t\n")).toBe("");
  });
});

describe("sanitizePastedHTML", () => {
  it("keeps the allowed subset and drops everything else", () => {
    const source = sanitizePastedHTML(
      '<p>Hello <strong>bold</strong> and <em>italic</em> <a href="https://x">link</a></p>' +
        "<script>alert('x')</script><style>p{color:red}</style>" +
        '<img src="x" onerror="alert(1)"><ul><li>item</li></ul>'
    );
    expect(source).not.toBeNull();
    expect(source).toContain("**bold**");
    expect(source).toContain("*italic*");
    expect(source).toContain("link");
    expect(source).toContain("- item");
    expect(source).not.toContain("script");
    expect(source).not.toContain("alert");
    expect(source).not.toContain("<");
    expect(source).not.toContain("href");
  });

  it("maps b and i to the supported markers", () => {
    expect(sanitizePastedHTML("<div><b>bold</b> <i>italic</i></div>")).toBe(
      "**bold** *italic*"
    );
  });

  it("serializes a pasted bold-italic run as bold, never ***", () => {
    expect(sanitizePastedHTML("<b><i>both</i></b>")).toBe("**both**");
    expect(sanitizePastedHTML("<i><b>both</b></i>")).toBe("**both**");
  });

  it("returns null when the paste carries no usable text", () => {
    expect(sanitizePastedHTML("<script>alert('x')</script>")).toBeNull();
    expect(sanitizePastedHTML("<style>p{}</style>")).toBeNull();
    expect(sanitizePastedHTML("")).toBeNull();
  });
});

describe("noteLength", () => {
  it("counts runes, not UTF-16 units", () => {
    expect(noteLength("abc")).toBe(3);
    expect(noteLength("a😀")).toBe(2);
    expect(noteLength("**bold**")).toBe(8);
  });

  it("pins the shared limit", () => {
    expect(NOTE_MAX_LENGTH).toBe(1000);
  });
});
