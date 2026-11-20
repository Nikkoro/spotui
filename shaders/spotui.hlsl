// spotui.hlsl: a Windows Terminal pixel shader that draws a plasma behind
// spotui's lyrics, moving to the music.
//
// A pixel shader only sees the rendered window, so spotui sends the music
// as colour: with `wt_shader = true` under [lyrics] it paints five cells at
// the start of its top row (see internal/ui/wtshader.go):
//
//   cell 0  signature (13, 37, 241)
//   cell 1  R kick (1 on a beat, fading), G bass, B loudness
//   cell 2  the accent colour
//   cell 3  R colour phase in the track's tempo, G drop, B zoom
//   cell 4  signature again
//
// This shader finds them, reads them, hides them, and adds the effect
// wherever the window shows its background, kept dim so the text on top
// stays readable.
// Without the cells (another page, another program) it changes nothing.
//
// Set it in your Windows Terminal profile, with no padding (the shader
// does not reach the padding, which would show as a plain frame):
//   "experimental.pixelShaderPath": "C:\\path\\to\\spotui.hlsl",
//   "padding": "0"
// After editing this file, touch settings.json or open a new tab.

Texture2D shaderTexture;
SamplerState samplerState;

cbuffer PixelShaderSettings
{
    float Time;        // seconds since the shader was loaded
    float Scale;       // UI scale
    float2 Resolution; // of shaderTexture
    float4 Background; // the profile's background colour
};

static const float3 SIG = float3(13.0, 37.0, 241.0) / 255.0;
static const int STRIP_CELLS = 5;
static const int SEARCH = 64; // pixels searched for the strip from the corner

float3 px(int2 p)
{
    return shaderTexture.Load(int3(p, 0)).rgb;
}

bool isSig(int2 p)
{
    return all(abs(px(p) - SIG) < 3.0 / 255.0);
}

// findStrip finds cell 0: its top-left pixel and the cell size.
bool findStrip(out int2 origin, out int2 cell)
{
    origin = int2(0, 0);
    cell = int2(0, 0);

    // Down the diagonal from the corner, past the window's padding.
    int2 hit = int2(-1, -1);
    [loop] for (int i = 0; i < SEARCH; i++)
    {
        if (isSig(int2(i, i)))
        {
            hit = int2(i, i);
            break;
        }
    }
    if (hit.x < 0)
        return false;

    // Back to the cell's left and top edges.
    int2 o = hit;
    [loop] for (int l = 0; l < SEARCH && o.x > 0 && isSig(int2(o.x - 1, o.y)); l++)
        o.x--;
    [loop] for (int u = 0; u < SEARCH && o.y > 0 && isSig(int2(o.x, o.y - 1)); u++)
        o.y--;

    // Its width and height.
    int w = 1;
    [loop] for (; w < SEARCH && isSig(int2(o.x + w, o.y)); w++) {}
    int h = 1;
    [loop] for (; h < 2 * SEARCH && isSig(int2(o.x, o.y + h)); h++) {}

    origin = o;
    cell = int2(w, h);
    // The strip ends with the signature again.
    return isSig(o + int2(cell.x * (STRIP_CELLS - 1) + cell.x / 2, cell.y / 2));
}

float3 cellColour(int2 origin, int2 cell, int k)
{
    return px(origin + int2(cell.x * k + cell.x / 2, cell.y / 2));
}

// Windows Terminal hands the shader a premultiplied texture: where only
// the background shows, pixels are transparent black, and the background
// colour (and acrylic) goes underneath afterwards. Some setups draw the
// background into the texture instead, opaque; both are handled.

float4 main(float4 pos : SV_POSITION, float2 tex : TEXCOORD) : SV_TARGET
{
    float4 colour = shaderTexture.Sample(samplerState, tex);

    int2 origin, cell;
    if (!findStrip(origin, cell))
        return colour;

    float3 d1 = cellColour(origin, cell, 1);
    float3 accent = cellColour(origin, cell, 2);
    float3 d3 = cellColour(origin, cell, 3);
    float kick = d1.r, bass = d1.g, level = d1.b;
    float hue = d3.r, drop = d3.g;

    // Is the background left transparent in the texture? The blank cell
    // right after the strip tells.
    bool clearBg = shaderTexture.Load(int3(origin + int2(cell.x * STRIP_CELLS + cell.x / 2, cell.y / 2), 0)).a < 0.5;
    float4 blank = clearBg ? float4(0.0, 0.0, 0.0, 0.0) : float4(Background.rgb, 1.0);

    // How much of this pixel is background: 1 on it, 0 on text, in between
    // on the soft edges of letters. The strip itself becomes background.
    int2 p = int2(tex * Resolution);
    bool inStrip = p.x >= origin.x && p.x < origin.x + cell.x * STRIP_CELLS &&
                   p.y >= origin.y && p.y < origin.y + cell.y;
    float4 base = inStrip ? blank : colour;
    float mask = 1.0;
    if (!inStrip)
        mask = clearBg ? 1.0 - smoothstep(0.05, 0.6, colour.a)
                       : 1.0 - smoothstep(0.02, 0.10, distance(colour.rgb, Background.rgb));
    if (mask <= 0.0)
        return colour;

    // Plasma: sums of waves, centred, the bass pushing the rings out.
    float aspect = Resolution.x / Resolution.y;
    float2 q = (tex - 0.5) * float2(aspect, 1.0) * 3.0;
    float t = Time * 0.5;
    float r = length(q);
    float v = sin(q.x * 1.7 + t) + sin(q.y * 2.3 - t * 1.3) +
              sin((q.x + q.y) * 1.3 + t * 0.7) + sin(r * 3.0 - t * 2.0 - kick * 2.0 - bass);
    v *= 0.25;

    float3 c = lerp(accent, accent.gbr, 0.5 + 0.5 * sin(v * 3.1416 + hue * 6.2832));
    c = lerp(c, accent.brg, 0.25 + 0.25 * sin(v * 6.2832 - t));

    // A ring running out from the middle on each beat.
    float ring = kick * exp(-pow((r - (1.0 - kick) * 2.5) * 3.0, 2.0));

    // Kept dim so the lyrics stay readable; louder music and beats lift it.
    float strength = 0.06 + 0.07 * bass + 0.04 * level + 0.10 * kick + 0.12 * drop;
    float3 fx = c * (strength * (0.6 + 0.4 * v) + ring * 0.18) * mask;

    // Premultiplied: adding colour with the alpha left alone lays the
    // effect over whatever is drawn underneath, background or acrylic.
    return float4(saturate(base.rgb + fx), base.a);
}
