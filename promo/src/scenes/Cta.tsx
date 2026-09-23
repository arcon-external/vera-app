import {
  AbsoluteFill,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { CREAM, INK, PAPER } from "../script";
import { VeraMark } from "../VeraMark";

// 28-30s. Mark pops, wordmark, CTA button slams in. Holds into the tail.
export function Cta() {
  const frame = useCurrentFrame();
  const { width, fps } = useVideoConfig();
  const u = Math.min(width, 1600) / 100;
  const markW = Math.min(width * 0.24, 300);
  const rise = spring({ frame, fps, config: { damping: 13 } });
  const ctaPop = spring({
    frame: frame - 26,
    fps,
    config: { damping: 11 },
  });
  const cta = interpolate(frame, [26, 36], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  return (
    <AbsoluteFill
      style={{
        backgroundColor: PAPER,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexDirection: "column",
        gap: 1.5 * u,
      }}
    >
      <div
        style={{
          opacity: Math.min(1, Math.max(0, rise)),
          transform: `translateY(${(1 - Math.min(1, Math.max(0, rise))) * 40}px) scale(${0.85 + 0.15 * Math.min(1.1, Math.max(0, rise))})`,
        }}
      >
        <VeraMark progress={1} width={markW} />
      </div>
      <div
        style={{
          color: INK,
          fontSize: Math.min(width * 0.075, 110),
          fontWeight: 800,
          letterSpacing: "-0.04em",
          opacity: interpolate(frame, [8, 22], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
          fontFamily: "system-ui, sans-serif",
        }}
      >
        Vera
      </div>
      <div
        style={{
          color: "#67655d",
          fontSize: Math.min(width * 0.032, 40),
          opacity: interpolate(frame, [16, 30], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
          fontFamily: "system-ui, sans-serif",
        }}
      >
        Quotations, perfected.
      </div>
      <div
        style={{
          marginTop: width * 0.012,
          backgroundColor: INK,
          color: PAPER,
          borderRadius: 999,
          padding: `${Math.min(width * 0.012, 16)}px ${Math.min(width * 0.035, 48)}px`,
          fontSize: Math.min(width * 0.026, 32),
          fontWeight: 600,
          opacity: cta,
          transform: `scale(${0.7 + 0.3 * Math.min(1.12, Math.max(0, ctaPop))})`,
          fontFamily: "system-ui, sans-serif",
        }}
      >
        Download for Windows &amp; Mac
      </div>
      <div
        style={{
          position: "absolute",
          bottom: 0,
          left: 0,
          right: 0,
          height: 10,
          backgroundColor: CREAM,
          transform: `scaleX(${cta})`,
          transformOrigin: "left",
        }}
      />
    </AbsoluteFill>
  );
}
