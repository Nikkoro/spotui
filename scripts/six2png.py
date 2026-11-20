import re, sys
from PIL import Image
s = open(sys.argv[1], encoding="latin-1").read()
body = s[s.index("q")+1 : s.rindex("\x1b\\")]
m = re.match(r'"(\d+);(\d+);(\d+);(\d+)', body)
w, h = int(m.group(3)), int(m.group(4))
body = body[m.end():]
img = Image.new("RGB", (w, h), (0, 0, 0))
pal = {}
x = y = 0; cur = 0; i = 0
while i < len(body):
    c = body[i]
    if c == "#":
        j = i + 1
        while j < len(body) and (body[j].isdigit() or body[j] == ";"): j += 1
        parts = body[i+1:j].split(";")
        if len(parts) == 5:
            pal[int(parts[0])] = tuple(int(int(v)*255/100) for v in parts[2:5])
        cur = int(parts[0]); i = j; continue
    if c == "$": x = 0; i += 1; continue
    if c == "-": x = 0; y += 6; i += 1; continue
    n = 1
    if c == "!":
        j = i + 1
        while body[j].isdigit(): j += 1
        n = int(body[i+1:j]); c = body[j]; i = j
    bits = ord(c) - 63
    for k in range(n):
        for d in range(6):
            if bits & (1 << d) and y + d < h and x < w:
                img.putpixel((x, y + d), pal[cur])
        x += 1
    i += 1
img.save(sys.argv[2])
print(w, h, len(pal))
