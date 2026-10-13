import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Button } from "./ui";

// Pin for issue #365: Button must emit type="button" by default, so no call
// site can submit a form by accident.
//
// The suite runs in node with no DOM (see vitest.config.ts), so Button is
// rendered to a string and asserted on as markup -- the same house pattern as
// CompanyGeneralPanel.test.tsx. That is a behavioural pin, not a source scan:
// renderToString prints the attributes React will actually set on the node,
// so `type` in the output is the button's real type. The default used to be
// HTML's own (submit), which is exactly the trap #338 and #342 fell into.

/** The first `<button ...>` opening tag in the markup. Throws when the
 *  component rendered no button, so a component that stops rendering one
 *  fails loudly rather than matching nothing. */
function buttonTag(html: string): string {
  const match = /<button\b[^>]*>/.exec(html);
  if (!match) throw new Error(`no <button> in rendered markup: ${html}`);
  return match[0];
}

describe("Button's type default (issue #365)", () => {
  it("renders type=button when no type is given", () => {
    const tag = buttonTag(renderToString(<Button onClick={() => {}}>Save draft</Button>));
    expect(tag).toContain('type="button"');
  });

  it("keeps type=button inside a form, where the HTML default would submit it", () => {
    // The two bites from #342/#338 were both a typeless Button inside a form.
    const html = renderToString(
      <form onSubmit={() => {}}>
        <Button variant="ghost">Cancel</Button>
      </form>,
    );
    expect(buttonTag(html)).toContain('type="button"');
  });

  it("passes an explicit type=submit through untouched", () => {
    const tag = buttonTag(renderToString(<Button type="submit">Create client</Button>));
    expect(tag).toContain('type="submit"');
    expect(tag).not.toContain('type="button"');
  });

  it("passes an explicit type=reset through untouched", () => {
    const tag = buttonTag(renderToString(<Button type="reset">Reset</Button>));
    expect(tag).toContain('type="reset"');
    expect(tag).not.toContain('type="button"');
  });

  it("still spreads the remaining button props, so the default is not a wall", () => {
    const tag = buttonTag(
      renderToString(
        <Button disabled aria-label="Dismiss" name="dismiss">
          Dismiss
        </Button>,
      ),
    );
    expect(tag).toContain('type="button"');
    expect(tag).toContain("disabled=");
    expect(tag).toContain('aria-label="Dismiss"');
    expect(tag).toContain('name="dismiss"');
  });
});
