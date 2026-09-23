import { Audio, Series, staticFile } from "remotion";
import { BEATS, FPS } from "./script";
import { Sting } from "./scenes/Sting";
import { Words } from "./scenes/Words";
import { Editor } from "./scenes/Editor";
import { Export } from "./scenes/Export";
import { Verify } from "./scenes/Verify";
import { Themes } from "./scenes/Themes";
import { Platforms } from "./scenes/Platforms";
import { Cta } from "./scenes/Cta";

function LandscapeBeat({ id, orient }: { id: string; orient: "landscape" | "vertical" }) {
  switch (id) {
    case "sting":
      return <Sting />;
    case "words":
      return <Words words={BEATS[1].copy} />;
    case "editor":
      return <Editor orient={orient} />;
    case "export":
      return <Export orient={orient} />;
    case "verify":
      return <Verify orient={orient} />;
    case "themes":
      return <Themes orient={orient} />;
    case "platforms":
      return <Platforms orient={orient} />;
    default:
      return <Cta />;
  }
}

export function PromoLandscape() {
  return (
    <>
      <Audio src={staticFile("bed.wav")} />
      <Series>
        {BEATS.map((beat) => (
          <Series.Sequence
            key={beat.id}
            durationInFrames={Math.round(beat.seconds * FPS)}
          >
            <LandscapeBeat id={beat.id} orient="landscape" />
          </Series.Sequence>
        ))}
      </Series>
    </>
  );
}

export function PromoVertical() {
  return (
    <>
      <Audio src={staticFile("bed.wav")} />
      <Series>
        {BEATS.map((beat) => (
          <Series.Sequence
            key={beat.id}
            durationInFrames={Math.round(beat.seconds * FPS)}
          >
            <LandscapeBeat id={beat.id} orient="vertical" />
          </Series.Sequence>
        ))}
      </Series>
    </>
  );
}
