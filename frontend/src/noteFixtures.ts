// The contenteditable DOM shapes the note toolbar really produces, shared by
// the serializer tests and the editor DOM tests.
//
// A browser's markup for one toolbar action is not stable across engines, and
// the toolbar's own commands add a block wrapper around a list. The serializer
// reads structure, not one browser's exact markup (#550), so every test that
// exercises a toolbar shape reads it from here instead of retyping the HTML.
// The expected `source` values are the restricted Markdown the note is stored
// as (#540), written out rather than computed: a serializer change cannot
// silently redefine what the fixtures mean.

export interface NoteDomFixture {
  /** The shape, named for the test that runs it. */
  readonly name: string;
  /** The contenteditable's innerHTML, exactly as the toolbar leaves it. */
  readonly html: string;
  /** The note source domToMarkdown must produce from `html`. */
  readonly source: string;
}

/** A bullet list the toolbar created at the top level of the editor. */
export const TOP_LEVEL_LIST: NoteDomFixture = {
  name: "a top-level bullet list",
  html: "<ul><li>First item</li><li>Second item</li></ul>",
  source: "- First item\n- Second item",
};

/**
 * The shape behind #550: the toolbar wraps its list in a block. A serializer
 * that treats every block as one paragraph flattens the items into plain
 * lines, so this fixture is what keeps the nested-list handling honest.
 */
export const LIST_INSIDE_BLOCK: NoteDomFixture = {
  name: "a bullet list inside a contenteditable block (#550)",
  html: "<div><ul><li>one</li><li>two</li></ul></div>",
  source: "- one\n- two",
};

/** Text typed before and after the toolbar's nested list stays in order. */
export const TEXT_AROUND_LIST: NoteDomFixture = {
  name: "text before and after a nested list",
  html: "<div>Intro<div><ul><li>one</li><li>two</li></ul></div>Tail</div>",
  source: "Intro\n\n- one\n- two\n\nTail",
};

/**
 * The empty list the toolbar leaves behind, including the filler <br> a
 * browser puts in an empty item. It carries no note text, so it serializes to
 * nothing rather than to a stray list marker.
 */
export const EMPTY_LIST: NoteDomFixture = {
  name: "an empty bullet list",
  html: "<div><ul><li><br></li></ul></div>",
  source: "",
};

/** Every shape above, in the order the toolbar can produce them. */
export const NOTE_DOM_FIXTURES: readonly NoteDomFixture[] = [
  TOP_LEVEL_LIST,
  LIST_INSIDE_BLOCK,
  TEXT_AROUND_LIST,
  EMPTY_LIST,
];
