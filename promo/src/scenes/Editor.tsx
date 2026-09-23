import {
  AbsoluteFill,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { DEMO, INK, PAPER } from "../script";

// 5.5-10.5s. A quotation card assembles itself: line items slam in on the
// clicks, the grand total rolls up on the tally. Fictional demo data only.
export function Editor({ orient }: { orient: "landscape" | "vertical" }) {
  const frame = useCurrentFrame();
  const { width, fps } = useVideoConfig();
  const u = Math.min(width, 1600) / 100;
  const cardW = orient === "landscape" ? Math.min(width * 0.52, 880) : width * 0.86;

  const total = 322800000;
  const shown = Math.floor(
    interpolate(frame, [95, 130], [0, total], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    })
  );
  const shownStr =
    "Rp" + shown.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  const cardPop = spring({ frame, fps, config: { damping: 16 } });

  return (
    <AbsoluteFill
      style={{
        backgroundColor: PAPER,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexDirection: "column",
        gap: 3 * u,
      }}
    >
      <div
        style={{
          color: INK,
          fontSize: 4.2 * u,
          fontWeight: 700,
          letterSpacing: "-0.03em",
          opacity: interpolate(frame, [0, 15], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
          fontFamily: "system-ui, sans-serif",
        }}
      >
        Line items in seconds.
      </div>
      <div
        style={{
          width: cardW,
          backgroundColor: "#fff",
          borderRadius: 2 * u,
          boxShadow: "0 30px 80px rgba(43,42,39,0.14)",
          padding: 3.4 * u,
          transform: `translateY(${(1 - Math.min(1, Math.max(0, cardPop))) * 60}px)`,
        }}
      >
        <div
          style={{
            fontSize: 1.7 * u,
            letterSpacing: "0.22em",
            color: "#8a877e",
            marginBottom: 2 * u,
            fontFamily: "system-ui, sans-serif",
          }}
        >
          QUOTATION · DRAFT
        </div>
        {DEMO.lines.map((line, i) => {
          const start = 12 + i * 22;
          const pop = spring({
            frame: frame - start,
            fps,
            config: { damping: 15 },
          });
          const o = interpolate(frame, [start, start + 8], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          });
          const x = (1 - Math.min(1, Math.max(0, pop))) * 60;
          return (
            <div
              key={line.desc}
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "baseline",
                padding: `${1.5 * u}px 0`,
                borderBottom: "1px solid #efede7",
                opacity: o,
                transform: `translateX(${x}px)`,
                fontFamily: "system-ui, sans-serif",
              }}
            >
              <span style={{ fontSize: 2.3 * u, color: INK, fontWeight: 600 }}>
                {line.desc}
              </span>
              <span style={{ fontSize: 2.3 * u, color: INK, fontVariantNumeric: "tabular-nums" }}>
                {frame > start + 14 ? line.amount : "Rp···"}
              </span>
            </div>
          );
        })}
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "baseline",
            paddingTop: 2 * u,
            fontFamily: "system-ui, sans-serif",
          }}
        >
          <span
            style={{
              fontSize: 2 * u,
              letterSpacing: "0.18em",
              color: "#8a877e",
            }}
          >
            GRAND TOTAL
          </span>
          <span
            style={{
              fontSize: 3.6 * u,
              fontWeight: 800,
              color: INK,
              fontVariantNumeric: "tabular-nums",
            }}
          >
            {frame < 95 ? "Rp···" : shownStr}
          </span>
        </div>
      </div>
      <div
        style={{
          color: INK,
          fontSize: 2.6 * u,
          opacity: interpolate(frame, [118, 138], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
          fontFamily: "system-ui, sans-serif",
        }}
      >
        Margins stay inside. The client sees one number.
      </div>
    </AbsoluteFill>
  );
}
