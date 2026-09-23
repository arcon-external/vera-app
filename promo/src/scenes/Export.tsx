import {
  AbsoluteFill,
  interpolate,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { CREAM, INK, PAPER } from "../script";

// 10.5-15s. One click: the card compresses, PDF pages fan out, a QR block
// stamps into the corner with a press.
export function Export({ orient }: { orient: "landscape" | "vertical" }) {
  const frame = useCurrentFrame();
  const { width } = useVideoConfig();
  const u = Math.min(width, 1600) / 100;

  const press = interpolate(frame, [85, 95], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const cardScale = 1 - 0.12 * press;
  const fan = interpolate(frame, [30, 80], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <AbsoluteFill
      style={{
        backgroundColor: INK,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexDirection: orient === "landscape" ? "row" : "column",
        gap: 6 * u,
      }}
    >
      <div
        style={{
          color: PAPER,
          fontSize: 5 * u,
          fontWeight: 700,
          letterSpacing: "-0.03em",
          maxWidth: orient === "landscape" ? 34 * u : "80%",
          textAlign: orient === "landscape" ? "right" : "center",
          opacity: interpolate(frame, [0, 20], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
          fontFamily: "system-ui, sans-serif",
        }}
      >
        One click.
        <br />
        <span style={{ color: CREAM }}>A signed PDF.</span>
      </div>
      <div style={{ position: "relative", transform: `scale(${cardScale})` }}>
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            style={{
              position: i === 0 ? "relative" : "absolute",
              top: 0,
              left: 0,
              width: 30 * u,
              height: 40 * u,
              backgroundColor: i === 0 ? PAPER : "#fff",
              borderRadius: 1.2 * u,
              boxShadow: "0 20px 60px rgba(0,0,0,0.45)",
              transform: `rotate(${(i - 1) * 9 * fan}deg) translate(${
                (i - 1) * 4 * fan * u
              }px, ${Math.abs(i - 1) * -1.5 * fan * u}px)`,
              opacity: i === 0 ? 1 : fan,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexDirection: "column",
              gap: 1 * u,
            }}
          >
            {i === 0 ? (
              <>
                <div style={{ width: "70%", height: 1.4 * u, backgroundColor: INK }} />
                <div style={{ width: "70%", height: 0.8 * u, backgroundColor: "#c9c6bd" }} />
                <div style={{ width: "70%", height: 0.8 * u, backgroundColor: "#c9c6bd" }} />
                <div style={{ width: "70%", height: 0.8 * u, backgroundColor: "#c9c6bd" }} />
                <div
                  style={{
                    width: 7 * u,
                    height: 7 * u,
                    backgroundColor: INK,
                    marginTop: 1.5 * u,
                    transform: `scale(${0.4 + 0.6 * press})`,
                    opacity: press,
                    display: "grid",
                    gridTemplateColumns: "repeat(4, 1fr)",
                    padding: 0.8 * u,
                    gap: 0.4 * u,
                  }}
                >
                  {Array.from({ length: 16 }).map((_, k) => (
                    <div
                      key={k}
                      style={{
                        backgroundColor: (k * 7 + 3) % 5 < 2 ? PAPER : "transparent",
                      }}
                    />
                  ))}
                </div>
              </>
            ) : null}
          </div>
        ))}
      </div>
    </AbsoluteFill>
  );
}
