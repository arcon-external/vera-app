import {
  AbsoluteFill,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { CREAM, DEMO, INK, PAPER } from "../script";

// 15-21s. A phone frame scans the QR (viewfinder sweep), then the
// verification page blooms open with the seal popping on the chime.
// Fictional data only.
export function Verify({ orient }: { orient: "landscape" | "vertical" }) {
  const frame = useCurrentFrame();
  const { width, fps } = useVideoConfig();
  const u = Math.min(width, 1600) / 100;

  const scan = interpolate(frame, [10, 60], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const bloom = interpolate(frame, [70, 95], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const sealPop = spring({
    frame: frame - 90,
    fps,
    config: { damping: 10, stiffness: 220 },
  });
  const seal = interpolate(frame, [90, 100], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const phoneW = orient === "landscape" ? Math.min(width * 0.24, 330) : width * 0.52;
  const phoneH = phoneW * 2.05;

  return (
    <AbsoluteFill
      style={{
        backgroundColor: PAPER,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexDirection: orient === "landscape" ? "row" : "column",
        gap: 5 * u,
      }}
    >
      <div
        style={{
          color: INK,
          fontSize: 4.6 * u,
          fontWeight: 700,
          letterSpacing: "-0.03em",
          maxWidth: orient === "landscape" ? 36 * u : "84%",
          textAlign: orient === "landscape" ? "right" : "center",
          fontFamily: "system-ui, sans-serif",
        }}
      >
        Scan the code.
        <br />
        <span style={{ color: "#B48A2E" }}>Trust the document.</span>
      </div>
      <div
        style={{
          width: phoneW,
          height: phoneH,
          backgroundColor: INK,
          borderRadius: phoneW * 0.12,
          padding: phoneW * 0.06,
          boxShadow: "0 30px 90px rgba(43,42,39,0.3)",
          position: "relative",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            width: "100%",
            height: "100%",
            backgroundColor: PAPER,
            borderRadius: phoneW * 0.07,
            position: "relative",
            overflow: "hidden",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexDirection: "column",
            opacity: 1 - bloom * 0.0,
          }}
        >
          <div
            style={{
              width: phoneW * 0.44,
              height: phoneW * 0.44,
              backgroundColor: INK,
              display: "grid",
              gridTemplateColumns: "repeat(6, 1fr)",
              padding: phoneW * 0.04,
              gap: phoneW * 0.015,
              opacity: 1 - bloom,
              transform: `scale(${1 - bloom * 0.3})`,
            }}
          >
            {Array.from({ length: 36 }).map((_, k) => (
              <div
                key={k}
                style={{
                  backgroundColor: (k * 11 + 5) % 7 < 3 ? PAPER : "transparent",
                }}
              />
            ))}
          </div>
          <div
            style={{
              position: "absolute",
              left: "8%",
              right: "8%",
              top: `${8 + scan * 80}%`,
              height: 3,
              backgroundColor: CREAM,
              boxShadow: "0 0 24px 6px rgba(230,208,161,0.9)",
              opacity: bloom > 0 ? 0 : 1,
            }}
          />
          <div
            style={{
              position: "absolute",
              inset: 0,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexDirection: "column",
              gap: phoneH * 0.02,
              opacity: bloom,
              transform: `scale(${0.85 + bloom * 0.15})`,
              backgroundColor: PAPER,
              padding: phoneW * 0.08,
              textAlign: "center",
              fontFamily: "system-ui, sans-serif",
            }}
          >
            <div
              style={{
                width: phoneW * 0.22,
                height: phoneW * 0.22,
                borderRadius: "50%",
                backgroundColor: "#1B7F3B",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                color: "#fff",
                fontSize: phoneW * 0.12,
                fontWeight: 800,
                transform: `scale(${seal === 0 ? 0.3 : Math.max(0.3, sealPop)})`,
                opacity: seal,
              }}
            >
              ✓
            </div>
            <div style={{ fontSize: phoneW * 0.062, fontWeight: 800, color: INK }}>
              VERIFIED
            </div>
            <div style={{ fontSize: phoneW * 0.045, color: "#67655d" }}>
              Quotation {DEMO.quoteNo}
              <br />
              {DEMO.grandTotal}
            </div>
          </div>
        </div>
      </div>
    </AbsoluteFill>
  );
}
