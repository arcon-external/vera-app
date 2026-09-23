"""Soft, rhythmic 30s sound bed: felt pulse, glass clicks, air and chimes."""
import wave
import numpy as np

SR = 44100
DURATION = 30
N = SR * DURATION
rng = np.random.default_rng(42)
mix = np.zeros(N)

def put(signal, seconds, gain=1.0):
    start = int(seconds * SR)
    end = min(N, start + len(signal))
    if end > start:
        mix[start:end] += signal[:end-start] * gain

def tone(freq, seconds, decay=5):
    n = int(seconds * SR)
    t = np.arange(n) / SR
    return np.sin(2*np.pi*freq*t) * np.exp(-decay*t)

def noise(seconds):
    return rng.normal(0, 1, int(seconds * SR))

def whoosh(seconds=0.8):
    n = int(seconds * SR)
    t = np.arange(n) / SR
    raw = noise(seconds)
    spectrum = np.fft.rfft(raw)
    freqs = np.fft.rfftfreq(n, 1/SR)
    mask = 1 / (1 + (freqs / (900 + 4200*t.mean())) ** 4)
    shaped = np.fft.irfft(spectrum * mask, n)
    return shaped * np.sin(np.pi*t/seconds)**2

def soft_kick():
    n = int(.18 * SR)
    t = np.arange(n) / SR
    return np.sin(2*np.pi*(100-55*t/.18)*t) * np.exp(-28*t)

def silk_click():
    n = int(.08 * SR)
    t = np.arange(n) / SR
    return (tone(1800, .08, 42) + .18*noise(.08)) * np.exp(-18*t)

# Rounded pulse and delicate 16th-note detail.
for beat in np.arange(1.0, 29.0, .5):
    put(soft_kick(), beat, .16 if int(beat*2)%2 else .24)
for beat in np.arange(1.25, 29.0, .25):
    put(silk_click(), beat, .035)

# Air into every scene cut.
for t in [0, 2.5, 5.5, 10.5, 15, 21, 25, 28]:
    put(whoosh(.55), max(0, t-.42), .13)

# Warm motif, then a soft signature chime.
for t, freq in [(5.8, 440), (6.5, 523.25), (7.2, 659.25), (10.8, 783.99),
                (15.2, 523.25), (16.0, 659.25), (18.0, 880),
                (25.2, 659.25), (26.0, 783.99), (28.0, 1046.5)]:
    put(tone(freq, .75, 2.8) + .25*tone(freq*2, .75, 4), t, .12)
for i, freq in enumerate([659.25, 830.61, 1046.5]):
    put(tone(freq, 2.5, 1.8), 17.8 + i*.1, .24)
put(noise(.2) * np.exp(-np.arange(int(.2*SR))/SR*30), 12.7, .13)

# Master: warm saturation, -1 dBFS peak, short tail.
mix = np.tanh(mix * 1.3)
mix *= 10 ** (-1/20) / max(np.max(np.abs(mix)), 1e-9)
mix[-int(1.2*SR):] *= np.linspace(1, 0, int(1.2*SR))
stereo = np.stack([mix, mix], axis=1)
pcm = (stereo * 32767).astype(np.int16)
with wave.open("public/bed.wav", "wb") as out:
    out.setnchannels(2)
    out.setsampwidth(2)
    out.setframerate(SR)
    out.writeframes(pcm.tobytes())
print("wrote public/bed.wav", DURATION, "seconds")
