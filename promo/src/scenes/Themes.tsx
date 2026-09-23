import {
  AbsoluteFill,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { INK, PAPER } from "../script";
import { VeraMark } from "../VeraMark";

// 21-25s. The quotation washes through brand palettes like ink in water,
// then the mark settles into the header.
export function Themes({ orient }: { orient: "landscape" | "vertical" }) {
  const frame = useCurrentFrame();
  const { width, fps } = useVideoConfig();
  const u = Math.min(width, 1600) / 100;

  // Wash: paper -> cream tint -> deep ink -> back to paper.
  const wash = interpolate(frame, [10, 100], [0, 3], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const bg =
    wash < 1
      ? `rgb(${251 - wash * 21}, ${250 - wash * 24}, ${248 - wash * 53})`
      : wash < 2
        ? `rgb(${230 - (wash - 1) * 203}, ${208 - (wash - 1) * 173}, ${161 - (wash - 1) * 122})`
        : `rgb(${27 + (wash - 2) * 224}, ${26 + (wash - 2) * 224}, ${39 + (wash - 2) * 209})`;
  const dark = wash > 1 && wash < 2.4;
  const fg = dark ? PAPER : INK;

  const settle = spring({
    frame: frame - 70,
    fps,
    config: { damping: 14 },
  });
  const settleO = interpolate(frame, [70, 88], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <AbsoluteFill
      style={{
        backgroundColor: bg,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexDirection: "column",
        gap: 3 * u,
      }}
    >
      <div
        style={{
          color: fg,
          fontSize: 4.4 * u,
          fontWeight: 700,
          letterSpacing: "-0.03em",
          textAlign: "center",
          fontFamily: "system-ui, sans-serif",
        }}
      >
        Your company,
        <br />
        your colors.
      </div>
      <div
        style={{
          width: orient === "landscape" ? Math.min(width * 0.4, 640) : width * 0.8,
          backgroundColor: "rgba(255,255,255,0.92)",
          borderRadius: 2 * u,
          padding: `${2.4 * u}px ${3 * u}px`,
          display: "flex",
          alignItems: "center",
          gap: 2.4 * u,
          boxShadow: "0 24px 70px rgba(43,42,39,0.18)",
          opacity: settleO,
          transform: `translateY(${(1 - Math.min(1, Math.max(0, settle))) * 40}px)`,
        }}
      >
        <VeraMark progress={1} width={9 * u} />
        <div style={{ fontFamily: "system-ui, sans-serif" }}>
          <div style={{ fontSize: 2.4 * u, fontWeight: 700, color: INK }}>
            PT Arcon Penta Persada
          </div>
          <div style={{ fontSize: 1.8 * u, color: "#67655d" }}>
            Every quotation, on brand.
          </div>
        </div>
      </div>
    </AbsoluteFill>
  );
}
