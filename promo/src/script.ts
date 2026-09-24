export const FPS = 30;
export const INK = "#242321";
export const PAPER = "#fbfaf8";
export const CREAM = "#E6D0A1";

export const BEATS = [
  {id: "open", seconds: 2.5},
  {id: "promise", seconds: 3},
  {id: "build", seconds: 5},
  {id: "send", seconds: 4.5},
  {id: "trust", seconds: 6},
  {id: "brand", seconds: 4},
  {id: "close", seconds: 5},
] as const;

export const TOTAL_SECONDS = BEATS.reduce((sum, beat) => sum + beat.seconds, 0);
export const TOTAL_FRAMES = TOTAL_SECONDS * FPS;

export const DEMO = {
  lines: ["Harbour Crane Overhaul", "Control Panel Set", "Load Test & Commissioning"],
  total: "Rp322.800.000",
  quote: "Quotation 1502260042",
};
