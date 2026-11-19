#!/usr/bin/env python3
"""Render an ANSI text frame (truecolor SGR) to a PNG for visual review."""
import re, sys
from PIL import Image, ImageDraw, ImageFont

CW, CH = 9, 18
BG = (16, 16, 20)
FG = (220, 220, 220)

def load_font():
    for p in ["/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
              "/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf"]:
        try:
            return ImageFont.truetype(p, 15), ImageFont.truetype(p.replace("Mono.ttf", "Mono-Bold.ttf").replace("Regular", "Bold"), 15)
        except OSError:
            continue
    f = ImageFont.load_default()
    return f, f

SGR = re.compile(r"\x1b\[([0-9;]*)m")
OTHER = re.compile(r"\x1b\][^\x07]*\x07|\x1b\[[0-9;?]*[A-Za-z]")

def parse(text, cols, rows):
    grid = [[(" ", FG, None, False, False) for _ in range(cols)] for _ in range(rows)]
    fg, bg, bold, faint, ul = FG, None, False, False, False
    y = 0
    for line in text.split("\n"):
        if y >= rows: break
        x = 0
        pos = 0
        while pos < len(line):
            m = SGR.match(line, pos)
            if m:
                params = [int(p) if p else 0 for p in m.group(1).split(";")] if m.group(1) else [0]
                i = 0
                while i < len(params):
                    p = params[i]
                    if p == 0: fg, bg, bold, faint, ul = FG, None, False, False, False
                    elif p == 1: bold = True
                    elif p == 2: faint = True
                    elif p == 4: ul = True
                    elif p == 22: bold = faint = False
                    elif p == 24: ul = False
                    elif p == 39: fg = FG
                    elif p == 49: bg = None
                    elif p in (38, 48) and i + 4 < len(params) + 0 and params[i+1] == 2:
                        c = tuple(params[i+2:i+5])
                        if p == 38: fg = c
                        else: bg = c
                        i += 4
                    elif p in (38, 48) and params[i+1] == 5:
                        i += 2
                    i += 1
                pos = m.end(); continue
            m = OTHER.match(line, pos)
            if m:
                pos = m.end(); continue
            ch = line[pos]; pos += 1
            if x < cols:
                c = fg
                if faint: c = tuple(int(v * 0.55 + 20) for v in c)
                grid[y][x] = (ch, c, bg, bold, ul)
            x += 1
        y += 1
    return grid

def render(path, out, cols, rows):
    text = open(path, encoding="utf-8").read()
    grid = parse(text, cols, rows)
    img = Image.new("RGB", (cols * CW, rows * CH), BG)
    d = ImageDraw.Draw(img)
    font, bfont = load_font()
    for y, row in enumerate(grid):
        for x, (ch, fg, bg, bold, ul) in enumerate(row):
            x0, y0 = x * CW, y * CH
            if bg: d.rectangle([x0, y0, x0 + CW - 1, y0 + CH - 1], fill=bg)
            if ch == "▀":
                d.rectangle([x0, y0, x0 + CW - 1, y0 + CH // 2 - 1], fill=fg)
                continue
            if ch != " ":
                d.text((x0, y0 + 1), ch, font=bfont if bold else font, fill=fg)
            if ul:
                d.line([x0, y0 + CH - 2, x0 + CW - 1, y0 + CH - 2], fill=fg)
    img.save(out)

if __name__ == "__main__":
    render(sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4]))
