"""SuperSonic's logos as vectors, and the rasters the app is built with.

The designs are source/*.png (left untouched). vector/ holds them redrawn as SVG:
the mark is fitted to source/logomark.png, and the wordmark is set as source/logotype-b.png
sets it, in Segoe UI Light at 64px with -3px tracking, as outlines (so nothing needs the font).
raster/ is vector/ rendered at the sizes the Android build takes (android/app/build.gradle.kts).

    python -m pip install fonttools      # once
    python branding/generate.py          # needs ImageMagick (magick) and C:/Windows/Fonts/segoeuil.ttf
"""
import os
import subprocess
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont

HERE = os.path.dirname(os.path.abspath(__file__))
FONT = "C:/Windows/Fonts/segoeuil.ttf"

PINK, GREY, WHITE, BLACK = "#FF0080", "#4C4C4C", "#FFFFFF", "#000000"

# ── the mark, in source/logomark.png's own 480px space ──────────────────────
# A wave and two stems in pink, round-capped strokes 33px wide, and three grey pills.
STROKE = 33
WAVE = ("M 124.75 178.75 C 146.5 157.75 168.75 152.5 189 152.5 "
        "C 228 152.5 249 203.5 285 203.5 C 306.75 203.5 326 201 345.5 178.25")
STEMS = "M 186 170 L 186 328 M 287 212 L 287 328"
PILLS = [(137, 254.5, 283.5, 29), (236.5, 237, 298, 34), (335.5, 255, 283, 30)]  # x, top, bottom, width
MARK_BOX = (108, 136, 362, 345)  # its extent: x0, y0, x1, y1


def mark(pink, grey):
    s = (f'<path d="{WAVE} {STEMS}" fill="none" stroke="{pink}" stroke-width="{STROKE}" '
         'stroke-linecap="round" stroke-linejoin="round"/>')
    for x, top, bottom, w in PILLS:
        s += f'<path d="M {x} {top} L {x} {bottom}" stroke="{grey}" stroke-width="{w}" stroke-linecap="round"/>'
    return s


def placed(pink, grey, x0, y0, width):
    """The mark scaled to `width` with its top-left at (x0, y0)."""
    k = width / (MARK_BOX[2] - MARK_BOX[0])
    return (f'<g transform="translate({x0:.3f} {y0:.3f}) scale({k:.5f}) '
            f'translate({-MARK_BOX[0]} {-MARK_BOX[1]})">{mark(pink, grey)}</g>')


# ── the wordmark ────────────────────────────────────────────────────────────
def wordmark(text, size, tracking, x, baseline, fill):
    """`text` as outlines, the first glyph's origin at (x, baseline)."""
    font = TTFont(FONT)
    glyphs, cmap, hmtx = font.getGlyphSet(), font.getBestCmap(), font["hmtx"]
    k = size / font["head"].unitsPerEm
    out = []
    for ch in text:
        name = cmap[ord(ch)]
        pen = SVGPathPen(glyphs)
        glyphs[name].draw(pen)
        out.append(f'<path transform="translate({x:.3f} {baseline:.3f}) scale({k:.6f} {-k:.6f})" d="{pen.getCommands()}"/>')
        x += hmtx[name][0] * k + tracking
    return f'<g fill="{fill}">{"".join(out)}</g>'


def svg(w, h, body):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">{body}</svg>\n'


# ── the designs ─────────────────────────────────────────────────────────────
def logomark():
    return svg(480, 480, mark(PINK, GREY))


def mono(ink):
    # source/mono-*.png: a 349px pink tile, corners rounded ~4.5px, the mark in one colour 184px wide, centred
    return svg(349, 349, f'<rect width="349" height="349" rx="4.5" fill="{PINK}"/>'
               + placed(ink, ink, 82, 99, 184))


def logotype_b(text_fill):
    # source/logotype-b.png: the mark 218px wide at (14, 0), "SuperSonic" beneath, the S's origin at x -4.4,
    # its baseline at 245.5; the canvas runs on past the source's 259px so the p's descender isn't cut
    return svg(266, 262, placed(PINK, GREY, 14, 0, 218)
               + wordmark("SuperSonic", 64, -3, -4.4, 245.5, text_fill))


VECTORS = {
    "logomark.svg": logomark(),
    "mono-white.svg": mono(WHITE),
    "mono-black.svg": mono(BLACK),
    "logotype-b-on-dark.svg": logotype_b(WHITE),   # white lettering, as source/logotype-b.png
    "logotype-b-on-light.svg": logotype_b(BLACK),  # black lettering, for light backgrounds
}

# raster/name: (vector, its size in px, the raster's size in px)
RASTERS = {
    "app-icon-1024.png": ("mono-white.svg", (349, 349), (1024, 1024)),  # the launcher icon (and its themed mask)
    "app-icon-256.png": ("mono-white.svg", (349, 349), (256, 256)),     # the app bar's mark
    "logotype-b-on-dark@4x.png": ("logotype-b-on-dark.svg", (266, 262), (1064, 1048)),    # the splash, dark theme
    "logotype-b-on-light@4x.png": ("logotype-b-on-light.svg", (266, 262), (1064, 1048)),  # the splash, light theme
}

if __name__ == "__main__":
    os.makedirs(os.path.join(HERE, "vector"), exist_ok=True)
    os.makedirs(os.path.join(HERE, "raster"), exist_ok=True)
    for name, text in VECTORS.items():
        with open(os.path.join(HERE, "vector", name), "w", newline="\n") as f:
            f.write(text)
    for name, (vector, (vw, vh), (w, h)) in RASTERS.items():
        # drawn at twice the size, then brought down to it exactly
        subprocess.run(["magick", "-background", "none", "-density", f"{96 * 2 * w / vw:.4f}",
                        os.path.join(HERE, "vector", vector), "-resize", f"{w}x{h}!", "-strip",
                        os.path.join(HERE, "raster", name)], check=True)
        print(name)
