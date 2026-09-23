"""Procedural music + SFX bed for the Vera promo, 30-second high-energy cut.
Zero licensed material: every sample is synthesized with numpy against the
second marks in promo/src/script.ts. Re-run after any timing change.

Output: promo/public/bed.wav (stereo 44.1kHz, 30s, peak-normalized).
"""
import numpy as np
import wave

SR = 44100
DUR = 30.0
N = int(SR * DUR)
rng = np.random.default_rng(21)

# Beat starts mirror script.ts BEATS.
BEAT_STARTS = [0.0, 2.5, 5.5, 10.5, 15.0, 21.0, 25.0, 28.0]


def at(t: float) -> int:
    return int(t * SR)


def place(track: np.ndarray, sig: np.ndarray, t: float, gain: float = 1.0):
    i = at(t)
    j = min(N, i + len(sig))
    if j > i:
        track[i:j] += sig[: j - i] * gain


def sine(freq: float, dur: float, decay: float = 0.0):
    n = int(dur * SR)
    t = np.arange(n) / SR
    env = np.exp(-t * decay) if decay > 0 else np.ones(n)
    return np.sin(2 * np.pi * freq * t) * env


def sweep(f0: float, f1: float, dur: float, decay: float = 0.0):
    n = int(dur * SR)
    t = np.arange(n) / SR
    k = (f1 - f0) / dur
    phase = 2 * np.pi * (f0 * t + 0.5 * k * t * t)
    env = np.exp(-t * decay) if decay > 0 else np.ones(n)
    return np.sin(phase) * env


def noise(dur: float):
    return rng.standard_normal(int(dur * SR))


def fft_filter(sig: np.ndarray, kind: str, cutoff: float):
    X = np.fft.rfft(sig)
    freqs = np.fft.rfftfreq(len(sig), 1 / SR)
    if kind == "low":
        H = 1 / (1 + (freqs / cutoff) ** 4)
    else:
        H = 1 / (1 + (cutoff / np.maximum(freqs, 1)) ** 4)
    return np.fft.irfft(X * H, len(sig))


def banpass_sweep(dur: float, f0: float, f1: float, width_oct: float = 1.0):
    chunk = 0.08
    n_chunks = max(int(dur / chunk), 1)
    out = np.zeros(int(dur * SR))
    src = noise(dur)
    for c in range(n_chunks):
        f = f0 * (f1 / f0) ** (c / max(n_chunks - 1, 1))
        s = src[int(c * chunk * SR): int((c + 1) * chunk * SR)]
        lo = f / (2 ** (width_oct / 2))
        hi = f * (2 ** (width_oct / 2))
        X = np.fft.rfft(s)
        freqs = np.fft.rfftfreq(len(s), 1 / SR)
        H = 1 / (1 + (freqs / hi) ** 6) / (1 + (lo / np.maximum(freqs, 1)) ** 6)
        out[int(c * chunk * SR): int((c + 1) * chunk * SR)] = np.fft.irfft(
            X * H, len(s)
        )
    return out


def chime(t: float, freqs, decay: float = 2.4, gain: float = 0.5, gap: float = 0.12):
    for i, f in enumerate(freqs):
        sig = sine(f, 4.0, decay) + 0.35 * sine(2 * f, 4.0, decay * 1.4)
        place(mix, sig, t + i * gap, gain)


def blip(t: float, freq: float, dur: float = 0.12, gain: float = 0.3):
    place(mix, sine(freq, dur, 20.0), t, gain)


def click(t: float, gain: float = 0.28):
    n = int(0.03 * SR)
    place(mix, noise(0.03) * np.exp(-np.arange(n) / SR * 320), t, gain)
    place(mix, sine(1700, 0.05, 95.0), t, gain)


def kick(t: float, gain: float = 0.5):
    place(mix, sweep(150, 42, 0.16, 22.0), t, gain)


def snare(t: float, gain: float = 0.2):
    n = int(0.12 * SR)
    place(mix, noise(0.12) * np.exp(-np.arange(n) / SR * 40), t, gain)
    place(mix, sine(190, 0.1, 45.0), t, gain)


def drop(t: float, gain: float = 0.85):
    place(mix, sweep(70, 30, 0.9, 4.0), t, gain)


def riser(t: float, dur: float = 1.0, gain: float = 0.22):
    sig = banpass_sweep(dur, 500, 5000)
    n = len(sig)
    sig = sig * np.linspace(0.05, 1, n) ** 2
    place(mix, sig, t, gain)


mix = np.zeros(N)

# ---- Sub drops + risers on every cut -----------------------------------------
for start in BEAT_STARTS:
    drop(start)
for start in BEAT_STARTS[1:]:
    riser(start - 1.0, 1.0)

# ---- Driving beat: four-floor kicks, backbeat snares, 16th hats ---------------
t = 2.5
while t < 28.2:
    kick(t, 0.42)
    t += 0.5
t = 3.0
while t < 28.2:
    snare(t, 0.16)
    t += 1.0
hat = fft_filter(noise(0.04), "high", 7000) * np.exp(-np.arange(int(0.04 * SR)) / SR * 140)
t = 5.5
while t < 28.2:
    place(mix, hat, t, 0.045)
    t += 0.125

# ---- Music: sub drone + double-time pad (3.75s chords) -------------------------
drone_t = np.arange(N) / SR
drone = np.sin(2 * np.pi * 55 * drone_t) * (0.5 + 0.5 * np.sin(2 * np.pi * 0.25 * drone_t))
mix += drone * 0.05

CHORDS = [
    [110.0, 130.81, 164.81],
    [87.31, 110.0, 130.81],
    [130.81, 164.81, 196.0],
    [98.0, 123.47, 146.83],
]
for ci in range(8):
    chord = CHORDS[ci % 4]
    start = ci * 3.75
    for f in chord:
        n = int(3.75 * SR)
        tt = np.arange(n) / SR
        env = np.minimum(1, tt / 1.0) * np.minimum(1, (3.75 - tt) / 1.0)
        place(mix, np.sin(2 * np.pi * f * tt) * env, start, 0.03)
    # Bass roots on 8ths under each chord
    b = chord[0] / 2
    for k in range(8):
        place(mix, sine(b, 0.3, 9.0), start + k * 0.45, 0.16)

# ---- Music: motif ----------------------------------------------------------------
for t, f in [
    (6.0, 440.0), (7.0, 523.25), (8.0, 587.33), (11.5, 659.25),
    (13.0, 523.25), (16.0, 587.33), (17.0, 659.25), (18.2, 783.99),
    (22.0, 440.0), (23.0, 392.0), (26.0, 523.25), (27.0, 587.33),
]:
    sig = sine(f, 1.6, 3.0) + 0.3 * sine(3 * f, 1.6, 4.0)
    place(mix, sig, t, 0.09)

# ---- SFX --------------------------------------------------------------------------
# Sting shimmer rides the opening drop
for f in [4000, 6000, 8000]:
    place(mix, sine(f, 0.8, 6.0), 0.05, 0.05)

# Kinetic type ticks on the word cuts
for t in [2.6, 3.3, 4.1, 4.8]:
    blip(t, 2300, 0.05, 0.3)

# Editor clicks + fast tally 8.5-9.5s
t = 6.0
while t < 8.2:
    click(t, 0.24)
    t += 0.5
for i, f in enumerate(np.linspace(900, 1900, 9)):
    blip(8.5 + i * 0.11, f, 0.07, 0.24)

# Export: click, paper thump, stamp
click(11.2, 0.38)
place(mix, fft_filter(noise(0.22), "low", 500), 12.7, 0.5)
place(mix, sine(90, 0.28, 17.0), 12.7, 0.5)
place(mix, sine(70, 0.35, 13.0), 13.7, 0.7)
place(mix, sine(3200, 0.05, 90.0), 13.7, 0.3)

# Verify: scan sweep + chime on the seal
place(mix, banpass_sweep(1.8, 400, 4500), 15.6, 0.35)
chime(18.0, [659.25, 880.0, 1318.5], decay=2.2, gain=0.42)

# Theme wash whoosh
wash = fft_filter(noise(3.6), "low", 8000)
n = len(wash)
place(mix, wash * (np.sin(np.pi * np.arange(n) / n) ** 2), 21.2, 0.22)

# Platform triple blips
for t, f in [(25.3, 1046.5), (26.0, 1174.7), (26.7, 1318.5)]:
    blip(t, f, 0.14, 0.32)

# Final chime + tail
chime(28.2, [261.63, 392.0, 1046.5], decay=1.8, gain=0.45, gap=0.1)

# ---- Master -------------------------------------------------------------------------
mix = np.tanh(mix * 1.15)
peak = np.max(np.abs(mix))
mix = mix / peak * 10 ** (-1 / 20)
fade_n = int(1.5 * SR)
mix[-fade_n:] *= np.linspace(1, 0, fade_n)
stereo = np.stack([mix, mix], axis=1)
pcm = (stereo * 32767).astype(np.int16)
with wave.open("public/bed.wav", "wb") as wav:
    wav.setnchannels(2)
    wav.setsampwidth(2)
    wav.setframerate(SR)
    wav.writeframes(pcm.tobytes())
print(f"bed.wav: {DUR}s stereo, peak {20*np.log10(np.max(np.abs(mix))):.1f} dBFS")
