#!/usr/bin/env python3
"""Pixel-delta harness for theme changes (issue 335, second pass).

Compares two PDFs page by page by rasterizing with PyMuPDF and diffing the
RGB bytes. The numbers it prints are the reproducible form of the Judge's
one-off measurement: for the default theme the maximum per-channel delta
must stay within the fidelity bound (7), with zero pixels at delta >= 8;
for an accent swap the harness shows how much of the document a rebrand now
reaches.

Dependencies (pip, no apt needed):
    Python 3.8+, pymupdf, Pillow, numpy
    pip install pymupdf Pillow numpy

Usage:
    python3 scripts/pixel-delta.py old.pdf new.pdf [--dpi 150]

Output explains itself: per-channel share of changed pixels, the maximum
delta, pixels at delta >= 8, and changed-pixel counts per vertical third of
the page (top/middle/bottom), which is the "across all thirds" figure.
Exit status is 0 on success regardless of the delta; this is a measurement,
not a gate.
"""
import sys

def load_pages(path, dpi):
    import fitz
    doc = fitz.open(path)
    zoom = dpi / 72.0
    pages = []
    for i, page in enumerate(doc):
        pix = page.get_pixmap(matrix=fitz.Matrix(zoom, zoom), alpha=False)
        pages.append((pix.width, pix.height, bytes(pix.samples)))
    return pages

def main():
    import argparse
    ap = argparse.ArgumentParser(description="Pixel-delta harness for theme changes")
    ap.add_argument("old", help="first PDF")
    ap.add_argument("new", help="second PDF")
    ap.add_argument("--dpi", type=int, default=150)
    ns = ap.parse_args()
    old_pages = load_pages(ns.old, ns.dpi)
    new_pages = load_pages(ns.new, ns.dpi)
    if len(old_pages) != len(new_pages):
        print(f"page count differs: {ns.old} has {len(old_pages)}, {ns.new} has {len(new_pages)}")
    import numpy as np
    total_pixels = 0
    for ch in range(3):
        changed = 0
        ge8 = 0
        maxd = 0
        for (w0, h0, s0), (w1, h1, s1) in zip(old_pages, new_pages):
            if w0 != w1 or h0 != h1:
                print(f"page size differs ({w0}x{h0} vs {w1}x{h1}); skipping that page for channel stats")
                continue
            a0 = np.frombuffer(s0, dtype=np.uint8).reshape(h0, w0, 3)[:, :, ch].astype(np.int16)
            a1 = np.frombuffer(s1, dtype=np.uint8).reshape(h1, w1, 3)[:, :, ch].astype(np.int16)
            d = np.abs(a0 - a1)
            changed += int((d > 0).sum())
            ge8 += int((d >= 8).sum())
            m = int(d.max()) if d.size else 0
            if m > maxd:
                maxd = m
            total_pixels += d.size if ch == 0 else 0
        chname = "RBG"[ch] if ch < 3 else str(ch)
        # channel order in pixmap samples is RGB
        names = ["R", "G", "B"]
        pct = 100.0 * changed / total_pixels if total_pixels else 0
        print(f"{names[ch]}: {changed} px changed ({pct:.1f}%), max delta {maxd}, {ge8} px at delta >= 8")
    # vertical thirds across all pages (any channel changed)
    thirds = [0, 0, 0]
    for (w0, h0, s0), (w1, h1, s1) in zip(old_pages, new_pages):
        if w0 != w1 or h0 != h1:
            continue
        a0 = np.frombuffer(s0, dtype=np.uint8).reshape(h0, w0, 3).astype(np.int16)
        a1 = np.frombuffer(s1, dtype=np.uint8).reshape(h1, w1, 3).astype(np.int16)
        diff = (np.abs(a0 - a1).max(axis=2) > 0)
        h = diff.shape[0]
        for k in range(3):
            thirds[k] += int(diff[h*k//3:h*(k+1)//3, :].sum())
    print(f"vertical thirds (top/middle/bottom) changed px: {thirds[0]}/{thirds[1]}/{thirds[2]}")
    print(f"pages: {len(old_pages)}, dpi: {ns.dpi}, total pixels per channel: {total_pixels}")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
