import {Composition} from "remotion";
import {registerRoot} from "remotion";
import {Promo} from "./Promo";
import {FPS, TOTAL_FRAMES} from "./script";

export const RemotionRoot: React.FC = () => <>
  <Composition id="PromoLandscape" component={() => <Promo />} durationInFrames={TOTAL_FRAMES} fps={FPS} width={1920} height={1080} />
  <Composition id="PromoVertical" component={() => <Promo vertical />} durationInFrames={TOTAL_FRAMES} fps={FPS} width={1080} height={1920} />
</>;

registerRoot(RemotionRoot);
