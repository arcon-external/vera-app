import type { FeaturesTutorialWorkspace } from "../featuresTutorial";
import { Button, Card, SectionHeading } from "./ui";

// Issue #595: the replay control on each workspace settings page. It is a
// per-account preference, so the card is available to staff and admins alike;
// it never changes the workspace defaults beside it.

const COPY: Record<
  FeaturesTutorialWorkspace,
  { title: string; body: string }
> = {
  quotation: {
    title: "Features Tutorial",
    body: "A guided tour of the Quotation workspace: details, line pricing, totals, drafts, the archive and the actions that issue a revision.",
  },
  "purchase-order": {
    title: "Features Tutorial",
    body: "A guided tour of the Purchase Order workspace: linking a quotation, inline values, quotation ghosts, totals and the final-save confirmation.",
  },
};

export function FeaturesTutorialCard({
  workspace,
  onReplay,
}: {
  workspace: FeaturesTutorialWorkspace;
  onReplay: () => void;
}) {
  const copy = COPY[workspace];
  return (
    <Card
      className="p-5"
      data-tutorial={`${workspace === "quotation" ? "quotation" : "po"}-settings-tutorial`}
    >
      <SectionHeading
        title={copy.title}
        hint="Replay the workspace walkthrough"
      />
      <p className="mt-2 max-w-3xl text-small leading-relaxed text-ink-600">
        {copy.body}
      </p>
      <p className="mt-1 text-caption text-ink-500">
        The tutorial opens automatically the first time this account enters the
        workspace. Replaying it always starts from the first step.
      </p>
      <div className="mt-4">
        <Button aria-label="Replay Features Tutorial" onClick={onReplay}>
          Replay tutorial
        </Button>
      </div>
    </Card>
  );
}
