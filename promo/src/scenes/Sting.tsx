import {
  AbsoluteFill,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { CREAM, INK, PAPER } from "../script";
import { VeraMark } from "../VeraMark";

// 0-2.5s. Black void, cream line sweeps, the mark pops out of darkness.
export function Sting() {
  const frame = useCurrentFrame();
  const { width, height, fps } = useVideoConfig();
  const markW = Math.min(width * 0.42, 560);
  const reveal = interpolate(frame, [5, 45], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const sweepX = interpolate(frame, [0, 30], [-width * 0.2, width * 1.2], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const glow = interpolate(frame, [30, 45], [0.5, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const fadeOut = interpolate(frame, [65, 75], [1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const pop = spring({ frame: frame - 5, fps, config: { damping: 13 } });
  return (
    <AbsoluteFill
      style={{ backgroundColor: INK, opacity: fadeOut, overflow: "hidden" }}
    >
      <div
        style={{
          position: "absolute",
          top: 0,
          bottom: 0,
          left: sweepX,
          width: 3,
          backgroundColor: CREAM,
          opacity: 0.8,
          boxShadow: `0 0 120px 30px rgba(230,208,161,${0.35 + glow})`,
        }}
      />
      <div
        style={{
          position: "absolute",
          inset: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          transform: `scale(${0.9 + 0.1 * pop})`,
          filter: `blur(${interpolate(frame, [5, 40], [14, 0], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          })}px)`,
        }}
      >
        <VeraMark progress={reveal} width={markW} mono={PAPER} />
      </div>
      <div
        style={{
          position: "absolute",
          bottom: height * 0.1,
          width: "100%",
          textAlign: "center",
          color: CREAM,
          fontSize: Math.min(width * 0.028, 34),
          letterSpacing: "0.42em",
          opacity: interpolate(frame, [40, 60], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
          fontFamily: "system-ui, sans-serif",
        }}
      >
        VERA
      </div>
    </AbsoluteFill>
  );
}
