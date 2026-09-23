// Single source of truth for script, timing, and copy. The audio bed
// (promo/audio/synth.py) is synthesized against these exact second marks —
// change a duration here and re-run the synth before rendering.
export const FPS = 30;

export interface Beat {
  id: string;
  seconds: number;
  title: string;
  copy: string[];
}

// 30 seconds exactly. Durations sum to 30. Punchy Apple-cut pacing: no
// beat overstays, every cut lands on a sound.
export const BEATS: Beat[] = [
  { id: "sting", seconds: 2.5, title: "Logo sting", copy: [] },
  {
    id: "words",
    seconds: 3,
    title: "Kinetic type",
    copy: ["Quotations,", "perfected."],
  },
  {
    id: "editor",
    seconds: 5,
    title: "Editor build-up",
    copy: ["Line items in seconds", "Margins stay inside"],
  },
  {
    id: "export",
    seconds: 4.5,
    title: "Export",
    copy: ["One click.", "A signed PDF."],
  },
  {
    id: "verify",
    seconds: 6,
    title: "Scan and verify",
    copy: ["Scan the code.", "Trust the document."],
  },
  {
    id: "themes",
    seconds: 4,
    title: "Brand wash",
    copy: ["Your company,", "your colors."],
  },
  {
    id: "platforms",
    seconds: 3,
    title: "Platforms",
    copy: ["Everywhere your team works."],
  },
  {
    id: "cta",
    seconds: 2,
    title: "Call to action",
    copy: ["Vera", "Quotations, perfected."],
  },
];

export const TOTAL_SECONDS = BEATS.reduce((sum, beat) => sum + beat.seconds, 0);
export const TOTAL_FRAMES = TOTAL_SECONDS * FPS;

// Fictional demo data only — never real client names or figures.
export const DEMO = {
  lines: [
    { desc: "Harbour Crane Overhaul", qty: "1 Lot", amount: "Rp184.500.000" },
    { desc: "Control Panel Set", qty: "2 Set", amount: "Rp96.200.000" },
    { desc: "Load Test & Commissioning", qty: "1 Lot", amount: "Rp42.800.000" },
  ],
  grandTotal: "Rp322.800.000",
  quoteNo: "1502260042",
  verifyCode: "vera-a3f9-02",
};

export const INK = "#2B2A27";
export const CREAM = "#E6D0A1";
export const PAPER = "#fbfaf8";
