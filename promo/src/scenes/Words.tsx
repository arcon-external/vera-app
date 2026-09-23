import {
  AbsoluteFill,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { CREAM, INK, PAPER } from "../script";

// 2.5-5.5s. One word per beat, alternating voids. Words slam in with
// overshoot; hard cuts on the ticks.
export function Words({ words }: { words: string[] }) {
  const frame = useCurrentFrame();
  const { width, fps } = useVideoConfig();
  const per = 90 / words.length;
  const idx = Math.min(words.length - 1, Math.floor(frame / per));
  const local = frame - idx * per;
  const pop = spring({ frame: local, fps, config: { damping: 14 } });
  const inY = (1 - pop) * 70;
  const inO = interpolate(local, [0, 8], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const dark = idx % 2 === 0;
  return (
    <AbsoluteFill
      style={{
        backgroundColor: dark ? INK : PAPER,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      <div
        style={{
          color: dark ? PAPER : INK,
          fontSize: Math.min(width * 0.11, 190),
          fontWeight: 700,
          letterSpacing: "-0.04em",
          transform: `translateY(${inY}px)`,
          opacity: inO,
          fontFamily: "system-ui, sans-serif",
        }}
      >
        {words[idx]}
        {idx === words.length - 1 ? (
          <span style={{ color: "#B48A2E" }}>.</span>
        ) : null}
      </div>
      {idx === words.length - 1 ? (
        <div
          style={{
            position: "absolute",
            bottom: "18%",
            width: Math.min(width * 0.3, 420),
            height: 6,
            backgroundColor: CREAM,
            transform: `scaleX(${interpolate(local, [10, 30], [0, 1], {
              extrapolateLeft: "clamp",
              extrapolateRight: "clamp",
            })})`,
          }}
        />
      ) : null}
    </AbsoluteFill>
  );
}
