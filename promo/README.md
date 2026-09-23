# Vera promo video (Remotion)

Apple-style 30s promo + 9:16 vertical cut. All visuals are code, all audio
is synthesized — zero licensed material, fully reproducible.

## Prerequisites

- Node 22+, ffmpeg with libx264, Python 3 + numpy

## Workflow

```bash
cd promo
npm install
python3 audio/synth.py        # regenerate public/bed.wav (seeded, deterministic)
npx remotion render PromoLandscape --output=out/vera-promo-30s.mp4
npx remotion render PromoVertical --output=out/vera-promo-vertical.mp4
```

## Timing

`src/script.ts` is the single source of truth for beat durations and copy.
`audio/synth.py` is synthesized against the same second marks — change a
duration in one, re-run the other, then re-render.
