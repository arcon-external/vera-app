// Vera brand mark, inlined from build/vera-logo.svg. Progress animates the
// reveal: fills fade in path by path. Pass mono to render every path in a
// single light color for use on dark voids (the ink stroke would vanish).
export function VeraMark({
  progress,
  width,
  mono,
}: {
  progress: number;
  width: number;
  mono?: string;
}) {
  const clamped = Math.min(1, Math.max(0, progress));
  const fade = (start: number, span: number) =>
    Math.min(1, Math.max(0, (clamped - start) / span));
  const height = (width * 300) / 409;
  const fills = mono
    ? [mono, mono, mono, mono]
    : ["#2B2A27", "#E6D0A1", "#E6D0A1", "#E6D0A1"];
  const strokes = mono ? [mono, mono, mono, mono] : ["#2B2A27", "#2B2A27", "#2B2A27", "#2B2A27"];
  return (
    <svg
      width={width}
      height={height}
      viewBox="0 0 409 300"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M0.865967 1L95.1316 1L194.366 194L179.366 222L78.366 27H46.6316L172.783 245.5L300.377 24.5H331.132L172.783 298.768L0.865967 1Z"
        fill={fills[0]}
        fillOpacity={fade(0.25, 0.5)}
        stroke={strokes[0]}
      />
      <path
        d="M300.366 24.5L314.222 0.5H407.866L235.866 298.413L207.366 250L223.366 222.946L235.866 244.751L363.028 24.5H300.366Z"
        fill={fills[1]}
        fillOpacity={fade(0.35, 0.5)}
        stroke={strokes[1]}
      />
      <path
        d="M154.866 0.760742H199.366C199.366 0.760742 199.366 22.2607 199.366 43.7607C199.366 65.2607 171.866 81.7607 171.866 81.7607L161.366 68.8548C163.483 70.3431 187.818 43.7607 176.366 43.7607C175.192 43.7607 154.866 43.7607 154.866 43.7607V0.760742Z"
        fill={fills[2]}
        fillOpacity={fade(0.45, 0.5)}
        stroke={strokes[2]}
      />
      <path
        d="M214.366 0.5H258.866C258.866 0.5 258.866 22 258.866 43.5C258.866 65 231.366 81.5 231.366 81.5L220.866 68.594C222.983 70.0823 247.318 43.5 235.866 43.5C234.692 43.5 214.366 43.5 214.366 43.5V0.5Z"
        fill={fills[3]}
        fillOpacity={fade(0.55, 0.45)}
        stroke={strokes[3]}
      />
    </svg>
  );
}
