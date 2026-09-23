import { Composition } from "remotion";
import { TOTAL_FRAMES, FPS } from "./script";
import { PromoLandscape, PromoVertical } from "./Compositions";

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition
        id="PromoLandscape"
        component={PromoLandscape}
        durationInFrames={TOTAL_FRAMES}
        fps={FPS}
        width={1920}
        height={1080}
      />
      <Composition
        id="PromoVertical"
        component={PromoVertical}
        durationInFrames={TOTAL_FRAMES}
        fps={FPS}
        width={1080}
        height={1920}
      />
    </>
  );
};
