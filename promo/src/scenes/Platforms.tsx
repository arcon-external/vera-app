import {
  AbsoluteFill,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { CREAM, INK } from "../script";

// 25-28s. Three stingers popping on the ascending blips.
export function Platforms({ orient }: { orient: "landscape" | "vertical" }) {
  const frame = useCurrentFrame();
  const { width, fps } = useVideoConfig();
  const u = Math.min(width, 1600) / 100;
  const items = [
    { top: "Windows", sub: "Intel · AMD · ARM64" },
    { top: "macOS", sub: "Universal build" },
    { top: "Auto-updates", sub: "Never miss a release" },
  ];
  return (
    <AbsoluteFill
      style={{
        backgroundColor: INK,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexDirection: "column",
        gap: 4 * u,
      }}
    >
      <div
        style={{
          color: CREAM,
          fontSize: 2 * u,
          letterSpacing: "0.34em",
          fontFamily: "system-ui, sans-serif",
        }}
      >
        EVERYWHERE YOUR TEAM WORKS
      </div>
      <div
        style={{
          display: "flex",
          flexDirection: orient === "landscape" ? "row" : "column",
          gap: 3 * u,
        }}
      >
        {items.map((item, i) => {
          const start = 6 + i * 20;
          const pop = spring({
            frame: frame - start,
            fps,
            config: { damping: 12 },
          });
          const o = interpolate(frame, [start, start + 8], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          });
          const clamped = Math.min(1.15, Math.max(0, pop));
          return (
            <div
              key={item.top}
              style={{
                width: orient === "landscape" ? 34 * u : 64 * u,
                border: `1px solid rgba(230,208,161,0.4)`,
                borderRadius: 1.8 * u,
                padding: `${3 * u}px ${2 * u}px`,
                textAlign: "center",
                opacity: o,
                transform: `scale(${0.6 + 0.4 * clamped})`,
                fontFamily: "system-ui, sans-serif",
              }}
            >
              <div style={{ color: "#fff", fontSize: 3.2 * u, fontWeight: 700 }}>
                {item.top}
              </div>
              <div style={{ color: CREAM, fontSize: 1.9 * u, marginTop: u }}>
                {item.sub}
              </div>
            </div>
          );
        })}
      </div>
    </AbsoluteFill>
  );
}
