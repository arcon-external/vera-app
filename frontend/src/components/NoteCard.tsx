import { Fragment } from "react";
import type { JSX, ReactNode } from "react";
import { parseNote, type NoteSpan } from "../note";
import { Card } from "./ui";

/**
 * The customer-facing note card (#535). The source is the restricted Markdown
 * the revision stored; it is parsed and rendered as React elements, so React
 * escapes every character and no raw markup from the note can become DOM.
 * An empty or whitespace-only note renders nothing at all, which is how most
 * quotations keep their pre-note shape.
 */
export function NoteCard({
  source,
  className = "",
}: {
  source: string;
  className?: string;
}): JSX.Element | null {
  const document = parseNote(typeof source === "string" ? source : "");
  if (document.blocks.length === 0) return null;

  return (
    <Card className={className}>
      <div className="border-l-4 border-accent-400 px-5 py-4">
        <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
          Note
        </p>
        <div className="mt-2 space-y-2 text-body leading-relaxed text-ink-800">
          {document.blocks.map((block, blockIndex) => {
            if (block.kind === "paragraph") {
              const item = block.items[0];
              if (item === undefined) return null;
              return <p key={blockIndex}>{renderLines(item)}</p>;
            }
            if (block.kind === "bullets") {
              return (
                <ul key={blockIndex} className="list-disc space-y-1 pl-5">
                  {block.items.map((item, itemIndex) => (
                    <li key={itemIndex}>{renderLines(item)}</li>
                  ))}
                </ul>
              );
            }
            return (
              <ol
                key={blockIndex}
                start={block.start === 1 ? undefined : block.start}
                className="list-decimal space-y-1 pl-5"
              >
                {block.items.map((item, itemIndex) => (
                  <li key={itemIndex}>{renderLines(item)}</li>
                ))}
              </ol>
            );
          })}
        </div>
      </div>
    </Card>
  );
}

/** One item's lines, separated by <br>, with bold/italic runs as elements. */
function renderLines(lines: NoteSpan[][]): ReactNode {
  return lines.map((line, lineIndex) => (
    <Fragment key={lineIndex}>
      {lineIndex > 0 ? <br /> : null}
      {line.map((span, spanIndex) => {
        if (span.bold) return <strong key={spanIndex}>{span.text}</strong>;
        if (span.italic) return <em key={spanIndex}>{span.text}</em>;
        return <Fragment key={spanIndex}>{span.text}</Fragment>;
      })}
    </Fragment>
  ));
}
