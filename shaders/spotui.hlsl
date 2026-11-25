// spotui.hlsl: a Windows Terminal pixel shader that draws behind spotui's
// lyrics, moving to the music. Write it out with `spotui shader`.
//
// A pixel shader only sees the rendered window, so spotui sends the music
// as colour: with `wt_shader = true` under [lyrics] it paints sixteen cells
// at the start of its top row (see internal/ui/wtshader.go):
//
//   cell 0     signature (13, 37, 241)
//   cell 1     R hit (how hard the last beat was, the first of four the
//              hardest), G bass, B loudness
//   cell 2     the accent colour
//   cell 3     R colour phase in the track's tempo, G drop (1 for half a
//              second; debug style: the frame count), B energy (this part
//              of the track against the last half minute)
//   cell 4     R style (0 plasma, 1 tunnel, 2 warp, 3 aurora, 4 synthwave, 5 chains, 6 debug), G B travel (16 bits, 0…1):
//              the distance the music has pushed the effect, on top of
//              its own steady pace (debug style: spotui's clock)
//   cell 5…8   the spectrum, 12 bands low to high, three a cell
//   cell 9     R beat phase (0 on a beat, 1 at the next by the tempo),
//              G which beat of four (0, 85, 170, 255), B snare (a hit in
//              the high bands, fading fast)
//   cell 10    the first plasma's values, as they were: R kick, G bass
//              against its last second, B loudness
//   cell 11    bass, mid, treble as MilkDrop hears them: each against its
//              own recent average, 1 = as usual, divided by 3
//   cell 12    the same, smoothed (MilkDrop's bass_att and so on)
//   cell 13    R how much the camera moves with them (wt_shader_motion),
//              G B its turn (16 bits, 0…1 a whole turn): a slow spin
//              that the treble speeds up
//   cell 14    R the style faded from (auto), G how far the fade has got
//              (0 showing that one … 255 showing the style in cell 4),
//              B wt_shader_quality (0 low, 1 medium, 2 high: how much
//              the heavy effects, chains, may draw)
//   cell 15    signature again
//
// This shader finds them, reads them, hides them, and adds the effect
// wherever the window shows its background, kept dim so the text on top
// stays readable. Without the cells (another page, another program) it
// changes nothing. The style is switched from spotui, with no reload.
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
static const int STRIP_CELLS = 16;
static const int BANDS = 12;
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

// Music is what the strip carries.
struct Music
{
    float hit, bass, level, hue, drop, energy, travel;
    float phase, snare;
    bool first; // the first beat of four
    // pulse: the last beat fading over the beat, as hard as it was;
    // swell: rising just before the next one.
    float pulse, swell;
    float ck, cb, cl; // the first plasma's kick, bass, loudness
    // MilkDrop's bass, mid, treb (1 = as usual) and their smoothed _att
    float bass3, mid3, treb3, bassAtt, midAtt, trebAtt;
    float motion; // how much the camera moves with them, 0…1
    float spin;   // its turn, 0…1 a whole circle
    int prevStyle; // the style faded from (auto)…
    float blend;   // …and how far: 0 that one, 1 style
    int quality;   // wt_shader_quality: 0 low, 1 medium, 2 high
    float rowH;    // a row of text, as a share of the window's height
    float3 accent;
    int style;
    float bands[BANDS];
};

Music readMusic(int2 origin, int2 cell)
{
    Music m;
    float3 d1 = cellColour(origin, cell, 1);
    float3 d3 = cellColour(origin, cell, 3);
    float3 d4 = round(cellColour(origin, cell, 4) * 255.0);
    m.hit = d1.r;
    m.bass = d1.g;
    m.level = d1.b;
    m.accent = cellColour(origin, cell, 2);
    m.hue = d3.r;
    m.drop = d3.g;
    m.energy = d3.b;
    m.style = int(d4.r);
    m.travel = (d4.g * 256.0 + d4.b) / 65536.0;
    float3 d9 = cellColour(origin, cell, 9);
    m.phase = d9.r;
    m.first = d9.g < 0.1;
    m.snare = d9.b;
    float3 d11 = cellColour(origin, cell, 11) * 3.0;
    float3 d12 = cellColour(origin, cell, 12) * 3.0;
    m.bass3 = d11.r;
    m.mid3 = d11.g;
    m.treb3 = d11.b;
    m.bassAtt = d12.r;
    m.midAtt = d12.g;
    m.trebAtt = d12.b;
    float3 d13 = round(cellColour(origin, cell, 13) * 255.0);
    m.motion = d13.r / 255.0;
    m.spin = (d13.g * 256.0 + d13.b) / 65536.0;
    float3 d14 = round(cellColour(origin, cell, 14) * 255.0);
    m.prevStyle = int(d14.r);
    m.blend = d14.g / 255.0;
    m.quality = int(d14.b);
    m.rowH = cell.y / Resolution.y;
    float3 d10 = cellColour(origin, cell, 10);
    m.ck = d10.r;
    m.cb = d10.g;
    m.cl = d10.b;
    m.pulse = m.hit * (1.0 - m.phase) * (1.0 - m.phase);
    m.swell = pow(m.phase, 6.0) * (m.hit > 0.0 ? 1.0 : 0.0);
    [unroll] for (int i = 0; i < BANDS / 3; i++)
    {
        float3 b = cellColour(origin, cell, 5 + i);
        m.bands[i * 3] = b.r;
        m.bands[i * 3 + 1] = b.g;
        m.bands[i * 3 + 2] = b.b;
    }
    return m;
}

// --- effects: each returns the colour to add over the background.
//
// spotui's values arrive 60 times a second, not quite evenly. A slow,
// steady motion shows that as judder, because the eye follows it, so the
// steady flow of each effect is timed here by Time. Fast moves set off by
// the beat (a ring shot away, a zoom punch, the bass bending the waves)
// come from spotui's values: they are over before judder can show.

// camera moves the view the way MilkDrop presets do, p being centred,
// aspect-corrected coordinates: it zooms in with the bass, keeps turning
// (faster with the treble, added up by spotui), and ripples with the mid
// (vocals, melody). MilkDrop's small per-frame moves pile up through its
// feedback; there is none here, so the moves are bigger. Zoom follows
// the smoothed bass, so it glides; the ripple's size follows the mid.
// Each effect takes as much of each move as suits it (zoomK, turnK,
// rippleK; 1 = the plasma's).
float2 camera(float2 p, Music m, float zoomK, float turnK, float rippleK)
{
    if (m.motion <= 0.0)
        return p;
    float zoom = 1.0 + zoomK * 0.25 * clamp(m.bassAtt - 1.0, -0.4, 1.0);
    p /= zoom;
    float turn = turnK * 6.2832 * m.spin;
    float c = cos(turn), s = sin(turn);
    p = float2(c * p.x - s * p.y, s * p.x + c * p.y);
    float ripple = rippleK * 0.04 * clamp(0.5 * (m.mid3 + m.midAtt) - 0.7, 0.0, 1.5);
    p += ripple * float2(sin(p.y * 9.0 + Time * 1.7), cos(p.x * 8.0 + Time * 1.3));
    return p;
}

// plasma: the first effect, kept as it was because it followed the music
// best: sums of waves in the cover's colours, the bass pushing the rings
// out, a ring running from the middle on each beat. It reads its own
// values (cell 10), which jump with the music and fade. With
// wt_shader_motion its view moves with the music too (camera).
float3 plasma(float2 tex, Music m)
{
    float aspect = Resolution.x / Resolution.y;
    float2 q = camera((tex - 0.5) * float2(aspect, 1.0), m, 1.0, 1.0, 1.0) * 3.0;
    float t = Time * 0.5;
    float r = length(q);
    float v = sin(q.x * 1.7 + t) + sin(q.y * 2.3 - t * 1.3) +
              sin((q.x + q.y) * 1.3 + t * 0.7) + sin(r * 3.0 - t * 2.0 - m.ck * 2.0 - m.cb);
    v *= 0.25;

    float3 c = lerp(m.accent, m.accent.gbr, 0.5 + 0.5 * sin(v * 3.1416 + m.hue * 6.2832));
    c = lerp(c, m.accent.brg, 0.25 + 0.25 * sin(v * 6.2832 - t));

    // A ring running out from the middle on each beat.
    float ring = m.ck * exp(-pow((r - (1.0 - m.ck) * 2.5) * 3.0, 2.0));

    // Kept dim so the lyrics stay readable; louder music and beats lift it.
    float strength = 0.06 + 0.07 * m.cb + 0.04 * m.cl + 0.10 * m.ck + 0.12 * m.drop;
    return c * (strength * (0.6 + 0.4 * v) + ring * 0.18);
}

// tunnel: a flight down a tunnel of tiles: a slow steady drift timed here,
// smooth at the terminal's frame rate, plus what spotui adds up (travel):
// a pace in the track's tempo and the push of the bass, beats and drops;
// the floor is lit by
// the low bands and the ceiling by the high ones; each beat lights up the
// rings flying past, lunges the flight forward and sends a bright ring
// from the far end that reaches you by the next beat. Everything moves
// the way you fly, never back: a ring sent away, or the view pulled back
// on the beat, read as the tunnel jumping backwards. Snares twist it and
// make the tiles sparkle.
float3 tunnel(float2 tex, Music m)
{
    float aspect = Resolution.x / Resolution.y;
    // The camera turns and ripples, but does not zoom: zooming out reads
    // as the tunnel jumping backwards.
    float2 q = camera((tex - 0.5) * float2(aspect, 1.0), m, 0.0, 1.0, 0.7);
    // The far end drifts a little, so the flight curves.
    q -= 0.06 * float2(sin(Time * 0.37), cos(Time * 0.29));
    float r = max(length(q), 1e-3);
    float depth = 0.3 / r; // further down the tunnel towards the middle
    // Rings repeat a whole number of times per unit of travel, so the
    // wrap of travel from 1 to 0 is seamless.
    float v = depth + m.travel + Time * 0.1;
    float rings = 0.5 + 0.5 * cos(6.2832 * v * 3.0);

    // Around the tunnel: 0 at the floor, 1 at the ceiling, mirrored.
    float around = abs(atan2(q.x, q.y)) / 3.1416;
    float spin = Time * 0.03 + 0.06 * m.snare;
    float segs = 0.5 + 0.5 * cos(6.2832 * (atan2(q.y, q.x) / 6.2832 * 8.0 + spin));
    float tile = smoothstep(0.3, 0.9, rings * 0.6 + segs * 0.4) * (1.0 + 0.6 * m.snare * segs);

    int band = clamp(int(around * BANDS), 0, BANDS - 1);
    float lit = 0.3 + 0.7 * m.bands[band];

    // Every unit of travel one ring is a light; the beat turns them up,
    // the near ones most.
    float flash = m.pulse * pow(0.5 + 0.5 * cos(6.2832 * v), 8.0) * saturate(1.5 - depth * 0.3);
    // and one ring coming at you from the far end over the beat.
    float shot = m.hit * (0.4 + 0.6 * m.phase) * exp(-pow((depth - lerp(4.0, 0.45, m.phase)) * 2.0, 2.0));

    // Dark far away in the middle, so it reads as depth.
    float fog = saturate((r - 0.02) * 4.0);
    float3 c = lerp(m.accent, m.accent.gbr, 0.5 + 0.5 * cos(6.2832 * (v + m.hue)));

    float strength = 0.07 + 0.06 * m.bass + 0.03 * m.level + 0.10 * m.pulse + 0.04 * m.swell + 0.12 * m.drop;
    float amount = 0.55 + 0.9 * m.energy; // the verse calmer, the chorus wilder
    return c * fog * amount * (strength * (0.2 + 0.8 * tile * lit) + flash * 0.18 + shot * 0.28);
}

// hash1 is a repeatable pseudo-random number in 0…1 for n.
float hash1(float n)
{
    return frac(sin(n * 12.9898) * 43758.5453);
}

// warp: a field of stars flying at you at a steady pace (timed here, plus
// the music's travel). The core reacts at once, like the plasma: the kick
// and the bass stretch the stars into streaks and light the nebula in the
// middle, a drop throws it into hyperspace (long streaks in the cover's
// colour). On top: the first beat of four lights the nebula harder,
// snares make some stars twinkle, and a verse is calmer than a chorus.
float3 warp(float2 tex, Music m)
{
    float aspect = Resolution.x / Resolution.y;
    // The star field turns and zooms a little with the bass; a ripple
    // would only smear the stars.
    float2 q = camera((tex - 0.5) * float2(aspect, 1.0), m, 0.5, 1.0, 0.0);
    float r = max(length(q), 1e-3);
    float a = atan2(q.y, q.x) / 6.2832 + 0.5; // 0…1 around the middle

    // Each star sits in a sector around the middle and comes closer as
    // its depth z runs 0…1; at depth z it shows at radius 0.04 / (1 - z).
    // travel wraps from 1 to 0, and so does z, so the wrap does not show.
    float T = Time * 0.12 + m.travel;
    float zPix = 1.0 - 0.04 / r; // the depth this pixel's radius shows
    float streak = 0.03 + 0.10 * m.cb + 0.15 * m.ck + 0.45 * m.drop;

    float light = 0.0;
    [unroll] for (int layer = 0; layer < 3; layer++)
    {
        float sectors = 48.0 + 40.0 * layer;
        float id = floor(a * sectors);
        float seed = id * 7.13 + layer * 101.7;
        float h1 = hash1(seed), h2 = hash1(seed + 3.1), h3 = hash1(seed + 5.7);
        float centre = (id + 0.2 + 0.6 * h1) / sectors;
        float across = abs(a - centre) * 6.2832 * r; // distance from its line
        float head = frac(h2 + T * (0.6 + 0.4 * h3));
        // How far behind the star's head this pixel is, along its streak.
        float behind = frac(head - zPix);
        float along = 1.0 - saturate(behind / streak);
        float width = 0.0015 + 0.004 * saturate(zPix);
        float s = along * smoothstep(width, 0.0, across) * smoothstep(0.0, 0.3, zPix);
        s *= step(0.35, h3);                                  // some sectors are empty
        s *= 0.3 + 0.7 * saturate(zPix);                      // nearer, brighter
        s *= 1.0 + 2.0 * m.snare * step(0.75, h1);            // twinkle on snares
        light += s;
    }

    float punch = m.ck * (m.first ? 1.25 : 1.0);
    float glow = exp(-r * 5.0) * (0.2 + 0.8 * punch + 0.3 * m.cb);
    float3 star = lerp(float3(0.9, 0.9, 0.95), m.accent, 0.3 + 0.5 * m.drop);
    float3 nebula = lerp(m.accent, m.accent.gbr, 0.5 + 0.5 * sin(Time * 0.2 + m.hue * 6.2832));

    float amount = 0.6 + 0.8 * m.energy; // the verse calmer, the chorus wilder
    float3 fx = (star * light * 0.35 + nebula * glow * 0.25) * amount;
    return fx / (1.0 + 1.5 * fx); // a soft ceiling, so nothing glares
}

// aurora: three curtains of light across the upper sky, swaying slowly
// (timed here) with rays running up them. A slow effect, so its shape
// follows only smooth values (the bass as it is now swings the curtains
// wider, the highs make their edges ripple gently) and the jumpy ones
// only light it: the kick brightens the curtains at once and fades. The
// tempo: a soft glow sweeps along them from left to right once a beat,
// and they swell just before the next one. Snares make the rays stand
// out a little, the first beat of four is brighter, and a verse is
// calmer than a chorus.
float3 aurora(float2 tex, Music m)
{
    float aspect = Resolution.x / Resolution.y;
    // Only a gentle ripple with the vocals: a turn would tilt the sky and a
    // zoom would rush what should be calm.
    float2 cam = camera((tex - 0.5) * float2(aspect, 1.0), m, 0.0, 0.0, 0.5);
    float x = cam.x + 0.5 * aspect;
    float y = cam.y + 0.5;
    float t = Time;
    float high = (m.bands[8] + m.bands[9] + m.bands[10] + m.bands[11]) * 0.25;
    float sway = 1.0 + 0.6 * m.bass;

    float3 sum = float3(0.0, 0.0, 0.0);
    [unroll] for (int i = 0; i < 3; i++)
    {
        float fi = i;
        // The curtain's lower edge, waving across the screen.
        float edge = 0.20 + 0.13 * fi +
                     sway * (0.06 * sin(x * (1.3 + 0.4 * fi) + t * (0.25 + 0.07 * fi) + fi * 2.1) +
                             0.03 * sin(x * (3.1 + fi) - t * (0.4 + 0.1 * fi)));
        edge += high * 0.006 * sin(x * 28.0 + t * 1.2 + fi * 5.0); // the highs' ripple
        float d = y - edge;
        // Fading slowly upwards from the edge, cut off sharply below it.
        float shape = d < 0.0 ? exp(d * (7.0 - fi)) : exp(-d * 45.0);
        float rays = 0.55 + 0.45 * sin(x * (60.0 + 15.0 * fi) + 2.0 * sin(x * 7.0 + t * 0.6 + fi) + t * 0.3);
        rays = pow(rays, 1.0 + 1.2 * m.snare);
        float3 c = i == 0 ? m.accent : (i == 1 ? m.accent.gbr : m.accent.brg);
        c = lerp(c, float3(0.9, 0.95, 1.0), 0.25 * exp(-abs(d) * 60.0)); // a pale bright edge
        sum += c * shape * rays * (1.0 - 0.2 * fi);
    }

    // The beat's glow, sweeping across over the beat, as hard as it was.
    float sweep = m.hit * (1.0 - 0.5 * m.phase) * exp(-pow((tex.x - m.phase) * 3.0, 2.0));
    float punch = m.ck * (m.first ? 1.25 : 1.0);
    float strength = 0.11 + 0.05 * m.bass + 0.03 * m.level + 0.08 * punch + 0.06 * m.swell + 0.10 * sweep + 0.08 * m.drop;
    float amount = 0.6 + 0.8 * m.energy; // the verse calmer, the chorus wilder
    float3 fx = sum * strength * amount;
    return fx / (1.0 + 1.5 * fx); // a soft ceiling, so nothing glares
}

// synthwave: a striped sun setting behind mountains over a grid that
// flies at you. The core reacts at once, like the plasma: the sun swells
// and glows with the kick, the grid flashes on it and lunges forward with
// the bass (travel, added up by spotui in the track's tempo). On top: the
// mountains are the spectrum (bass on the flanks, highs in the middle),
// the first beat of four is brighter, snares flash along the horizon, and
// a verse is calmer than a chorus. No camera moves: a horizon that tilts
// or breathes looks wrong.
float3 synthwave(float2 tex, Music m)
{
    const float horizon = 0.58;
    float aspect = Resolution.x / Resolution.y;
    float x = (tex.x - 0.5) * aspect; // 0 in the middle
    float y = tex.y;                  // 0 at the top
    float punch = m.ck * (m.first ? 1.25 : 1.0);
    float3 hot = m.accent;
    float3 warm = m.accent.gbr;

    // The grid, worked out everywhere: its lines need screen derivatives,
    // which are not allowed inside a branch.
    float dz = max(y - horizon, 1e-3);
    float depth = 0.25 / dz;
    float gx = x * depth * 1.2;
    // Two lines per unit of depth and four per unit of travel, so the wrap
    // of travel from 1 to 0 is seamless.
    float gz = (depth + m.travel * 2.0 + Time * 0.3) * 2.0;
    float lx = abs(frac(gx + 0.5) - 0.5) / max(fwidth(gx), 1e-4);
    float lz = abs(frac(gz + 0.5) - 0.5) / max(fwidth(gz), 1e-4);
    float grid = saturate(1.0 - 0.6 * min(lx, lz));

    float3 c;
    if (y >= horizon)
    {
        float closeness = saturate(dz * 6.0); // dim, and less aliased, far off
        float glow = (0.35 + 0.65 * punch + 0.3 * m.drop) * closeness;
        c = hot * 0.04 + lerp(hot, warm, 0.3) * grid * glow;
        c += hot * 0.3 * exp(-dz * 18.0) * (0.5 + 0.5 * m.cb); // haze over the horizon
    }
    else
    {
        // Sky: dark above, glowing down to the horizon.
        float up = (horizon - y) / horizon;
        c = lerp(hot * 0.35, hot * 0.04, saturate(up * 1.6));

        // The sun, swelling with the kick, striped in its lower half, the
        // stripes sliding down.
        float2 sc = float2(0.0, horizon - 0.16);
        float r = 0.17 * (1.0 + 0.06 * punch + 0.04 * m.cb);
        float d = length(float2(x, y) - sc);
        float t = saturate((y - (sc.y - r)) / (2.0 * r)); // 0 its top … 1 its bottom
        float stripes = step(0.3 + 0.6 * t * t, frac((y - sc.y) * 22.0 - Time * 0.4));
        float3 sun = lerp(warm, hot, t) * 0.8;
        c = lerp(c, sun, (t > 0.45 ? stripes : 1.0) * smoothstep(r, r - 0.004, d));
        c += hot * 0.25 * exp(-max(d - r, 0.0) * 12.0) * (0.6 + 0.6 * punch);

        // Mountains: the spectrum, bass on the flanks, highs in the middle.
        float side = saturate(abs(x) / (0.5 * aspect));
        float bi = (1.0 - side) * (BANDS - 1);
        int b0 = int(bi);
        float level = lerp(m.bands[b0], m.bands[min(b0 + 1, BANDS - 1)], frac(bi));
        float ridge = horizon - (0.03 + 0.12 * level) * (0.7 + 0.3 * sin(x * 13.0 + 1.7) * sin(x * 5.3));
        if (y > ridge)
            c = hot * 0.05 + hot * 0.9 * exp(-(y - ridge) * 220.0);
    }

    c += warm * 0.4 * m.snare * exp(-abs(y - horizon) * 60.0); // snares along the horizon
    float amount = 0.6 + 0.8 * m.energy; // the verse calmer, the chorus wilder
    float3 fx = c * amount * 0.5;
    return fx / (1.0 + 1.5 * fx); // a soft ceiling, so nothing glares
}

// chains: glossy chrome chains crossing the screen at different angles
// and depths, a few near, big and bright, more far, thin and dim. The
// camera stays still; the chains move: their links slide along them like
// a snake, each its own way and speed, and they sway. Each comes in from
// off screen and leaves again, then comes back as a new chain (angle,
// depth, size), so the picture never repeats. Raymarched signed distance
// fields: a ray only marches against the chains it passes near, from
// where it comes within reach of one, so most pixels cost one or two
// short marches. The core reacts at once: the kick runs a light across
// the metal and lifts the glow around the links, the bass sways the
// chains deeper. On top: the first beat of four flashes harder, snares
// glint, a drop jerks them and slides them faster (spotui's travel), and
// the louder the part of the track, the more chains and the brighter.
// Dimmed hard behind the lyrics and under the first and last rows.

static const float CH_R1 = 0.16;  // a link's ring radius
static const float CH_R2 = 0.055; // its wire radius
static const float CH_LE = 0.10;  // half its straight part
static const float CH_PITCH = 2.0 * (CH_LE + CH_R1 - CH_R2); // link to link
static const float CH_PERIOD = 2.0 * CH_PITCH; // the pattern repeats every two links
static const float CH_FAR = 16.0;  // nothing is marched further than this
static const float CH_LENS = 1.4;  // the camera's focal length (-1…1 up the screen)

float sq(float x)
{
    return x * x;
}

// chainLinks is the distance to an endless chain of unit size along x, q
// in its frame: x along the chain, y and z across. Links alternate by 90
// degrees; the two nearest are tested so they interlock.
float chainLinks(float3 q)
{
    float best = 1e3;
    [unroll] for (int o = 0; o < 2; o++)
    {
        float idx = floor(q.x / CH_PITCH) + o;
        float zl = q.x - idx * CH_PITCH;
        bool odd = frac(idx * 0.5) > 0.25;
        float a = odd ? q.y : q.z;
        float b = odd ? q.z : q.y;
        float qz = max(abs(zl) - CH_LE, 0.0);
        best = min(best, length(float2(length(float2(qz, a)) - CH_R1, b)) - CH_R2);
    }
    return best;
}

// Chain is one chain as this frame has it.
struct Chain
{
    float3 c;          // a point on its axis
    float3 e1, e2, e3; // along it, and two across (rolled)
    float3 side;       // across it in the screen's plane: the way it sways
    float scale;       // its size, near ones bigger
    float slide;       // how far its links have slid along it, in link units
    float wave, waveK;  // its sway: how far, how tight
    float2 wavePh;      // where its two waves are
    float radius;      // nothing of it is further than this from its axis
    float weight;      // how much of it shows, 0…1
    float tint;        // which of the cover's colours its glow leans to
};

// hs is the j-th repeatable random number, 0…1, of a chain's seed.
float hs(float seed, float j)
{
    return hash1(seed * 1.37 + j * 7.91);
}

// chainAt is chain slot i now. Each slot lives 20…40 s; over a life its
// chain drifts across, at right angles to itself, from off screen to off
// screen (and fades in and out at the ends, in case a far end of a chain
// tilted into the depth still shows), then the slot comes back as a new
// chain. slots is how many there are; the louder this part of the track,
// the more of them show.
Chain chainAt(int i, Music m, float aspect, int slots)
{
    Chain ch;
    float fi = i;
    float lifeSec = 20.0 + 20.0 * hash1(fi * 3.71 + 0.5);
    float u = Time / lifeSec + hash1(fi * 5.13 + 1.3);
    float f = frac(u);
    float seed = floor(u) + fi * 17.0;

    // Any angle across the screen, a little into the depth, rolled. It
    // turns slowly as it crosses, so it never glides by like a ruler.
    float ang = hs(seed, 1.0) * 3.1416 + (f - 0.5) * (hs(seed, 13.0) - 0.5) * 1.2;
    float tilt = (hs(seed, 2.0) - 0.5) * 0.6;
    ch.e1 = float3(cos(ang) * cos(tilt), sin(ang) * cos(tilt), sin(tilt));
    ch.side = float3(-sin(ang), cos(ang), 0.0);
    float3 up = cross(ch.e1, ch.side);
    float roll = hs(seed, 3.0) * 6.2832;
    ch.e2 = ch.side * cos(roll) + up * sin(roll);
    ch.e3 = up * cos(roll) - ch.side * sin(roll);

    // More far chains than near ones; near ones bigger.
    float distant = sqrt(hs(seed, 4.0));
    float z = lerp(2.5, 10.0, distant);
    ch.scale = lerp(1.6, 0.7, distant) * (0.85 + 0.3 * hs(seed, 5.0));

    // Off screen: the line clear of the view at its depth, with room for
    // its links and the depth it tilts into.
    float2 n = ch.side.xy;
    float halfH = z / CH_LENS;
    float off = (halfH * aspect * abs(n.x) + halfH * abs(n.y)) * (1.1 + 1.5 * abs(tilt)) + ch.scale * 0.6;
    float way = hs(seed, 6.0) < 0.5 ? -1.0 : 1.0;
    ch.c = float3(n * (way * lerp(-off, off, f)), z);
    // A drop jerks it.
    ch.c += ch.side * (0.06 * ch.scale * m.drop * sin(Time * 31.0 + fi * 1.7));

    // The links slide along it: a steady pace timed here, plus spotui's
    // travel (the tempo, the bass, beats and drops). travel wraps from 1
    // to 0, so each slot takes a whole number of patterns per unit of it.
    float dir = hs(seed, 7.0) < 0.5 ? -1.0 : 1.0;
    float speed = 0.5 + 0.8 * hs(seed, 8.0); // link units a second
    float k = hs(seed, 9.0) < 0.6 ? 1.0 : 2.0;
    ch.slide = dir * (frac(Time * speed / CH_PERIOD) + m.travel * k) * CH_PERIOD;

    // The sway: two slow waves, a few links long, drifting opposite ways,
    // so the bends change shape slowly rather than run along it like a
    // ripple on water. Their length goes with the chain's size, so near
    // chains bend too. The bass deepens them, smoothed: the shape following
    // every kick looked nervous.
    ch.waveK = (1.3 + 0.6 * hs(seed, 10.0)) / ch.scale;
    float sway = 0.3 + 0.3 * hs(seed, 11.0); // radians a second
    ch.wavePh = float2(Time * sway, -Time * sway * 0.7) + fi * float2(2.3, 4.1);
    ch.wave = ch.scale * (0.06 + 0.06 * saturate(m.bassAtt - 0.8));
    ch.radius = ch.scale * (CH_R1 + CH_R2 + 0.12 * 1.35 + 0.06) + 0.12; // + room for the glow

    float ends = smoothstep(0.0, 0.08, f) * smoothstep(1.0, 0.92, f);
    float present = saturate((0.45 + 0.75 * m.energy) * slots + 1.0 - fi);
    ch.weight = ends * present;
    ch.tint = hs(seed, 12.0);
    return ch;
}

// chainDist is the distance from p to the chain.
float chainDist(Chain ch, float3 p)
{
    float3 d = p - ch.c;
    float x = dot(d, ch.e1);
    // The bend w(x), across in the screen's plane, and its slope.
    float a1 = x * ch.waveK + ch.wavePh.x;
    float a2 = x * ch.waveK * 2.3 + ch.wavePh.y;
    float w = ch.wave * (sin(a1) + 0.35 * sin(a2));
    float slope = ch.wave * ch.waveK * (cos(a1) + 0.805 * cos(a2));
    // Into the bent chain's own frame, so the links turn along the bend
    // rather than only shift sideways: along its tangent and across it.
    float side = dot(d, ch.side);
    float u = side - w;
    float l = rsqrt(1.0 + slope * slope);
    x += u * slope * l;
    d += ch.side * (u * l - side);
    float3 q = float3(x / ch.scale + ch.slide, dot(d, ch.e2) / ch.scale, dot(d, ch.e3) / ch.scale);
    return chainLinks(q) * ch.scale;
}

// nearAxis is where the ray from the camera along rd is within the
// chain's radius of its axis: from x to y (x > y: nowhere).
float2 nearAxis(float3 rd, Chain ch)
{
    float3 w = -ch.c;
    float3 rp = rd - ch.e1 * dot(rd, ch.e1);
    float3 wp = w - ch.e1 * dot(w, ch.e1);
    float a = dot(rp, rp);
    float b = dot(wp, rp);
    float c = dot(wp, wp) - ch.radius * ch.radius;
    float disc = b * b - a * c;
    if (a < 1e-6 || disc <= 0.0)
        return float2(1.0, 0.0);
    float s = sqrt(disc);
    return float2((-b - s) / a, (-b + s) / a);
}

// chainShade is the chrome at pos: a dark studio tinted by the cover,
// reflected, with a few sharp white lights in it.
float3 chainShade(Chain ch, float3 pos, float3 rd, float t, Music m)
{
    // The normal from four samples (a tetrahedron) rather than six.
    float h = 0.0015 * t;
    float2 k = float2(1.0, -1.0);
    float3 n = normalize(k.xyy * chainDist(ch, pos + k.xyy * h) + k.yyx * chainDist(ch, pos + k.yyx * h) +
                         k.yxy * chainDist(ch, pos + k.yxy * h) + k.xxx * chainDist(ch, pos + k.xxx * h));
    float3 r = reflect(rd, n);

    float3 white = lerp(float3(1.0, 1.0, 1.0), m.accent, 0.2);
    // Dark below, a grey-blue room above in the cover's colours, a bright
    // horizon between.
    float3 sky = lerp(m.accent * 0.10 + 0.04, m.accent.gbr * 0.30 + 0.30, smoothstep(-0.6, 0.8, r.y));
    sky += white * 0.5 * exp(-sq(r.y / 0.06));
    float lights = 1.6 * exp(-sq((r.y - 0.55) / 0.10)) + // a softbox above
                   1.1 * exp(-sq((r.x + 0.6) / 0.08)) +  // a strip on the left
                   0.5 * exp(-sq((r.x - 0.7) / 0.12));   // a dimmer one on the right
    float spec = pow(saturate(dot(r, normalize(float3(0.5, 0.6, -0.6)))), 120.0);
    // The kick: a light running across the metal as it fades, the first
    // beat of four harder.
    float run = lerp(-0.9, 0.9, 1.0 - m.ck);
    float flash = m.ck * (m.first ? 1.6 : 1.0) * exp(-sq((dot(r, normalize(float3(0.8, 0.4, -0.3))) - run) / 0.12));
    float glint = m.snare * pow(saturate(dot(r, normalize(float3(-0.3, 0.6, -0.75)))), 300.0) * 4.0;

    float fresnel = pow(1.0 - saturate(-dot(rd, n)), 3.0);
    float3 c = (sky + white * (0.9 * lights + 1.5 * spec + 1.5 * flash + glint)) * lerp(0.6, 1.0, fresnel);
    c += m.accent * 0.04 * saturate(dot(n, normalize(float3(-0.4, 0.7, -0.6))));
    return c;
}

float3 chains(float2 tex, Music m)
{
    float aspect = Resolution.x / Resolution.y;

    // Keep the lyrics readable: dim hard in an ellipse around the middle
    // and under the first and last rows.
    float clear = lerp(0.1, 1.0, smoothstep(0.7, 1.2, length((tex - 0.5) / float2(0.24, 0.36))));
    float edge = min(tex.y, 1.0 - tex.y);
    clear *= lerp(0.15, 1.0, smoothstep(2.0 * m.rowH, 3.0 * m.rowH, edge));

    // wt_shader_quality: low, medium, high.
    int slots = m.quality == 0 ? 5 : (m.quality == 2 ? 9 : 7);
    int steps = m.quality == 0 ? 24 : (m.quality == 2 ? 48 : 32);
    steps = max(8, int(steps * lerp(0.4, 1.0, clear))); // rays end early where it is dimmed

    float2 p = float2((tex.x - 0.5) * aspect, 0.5 - tex.y) * 2.0;
    float3 rd = normalize(float3(p, CH_LENS));
    float pix = 2.0 / (Resolution.y * CH_LENS); // a pixel's width at distance 1

    Chain best = (Chain)0;
    float bestCov = 0.0, bestT = CH_FAR, bestLit = 0.0;
    float3 glow = float3(0.0, 0.0, 0.0);
    [loop] for (int i = 0; i < slots; i++)
    {
        Chain ch = chainAt(i, m, aspect, slots);
        if (ch.weight < 0.01)
            continue;
        float2 span = nearAxis(rd, ch);
        float t = max(span.x, 0.3);
        float tEnd = min(span.y, CH_FAR);
        if (t >= tEnd)
            continue;
        // March along the stretch near its axis; keep how close the ray
        // came (as an angle) to smooth the chain's edges.
        float closest = 1e3, tClosest = t, g = 0.0;
        [loop] for (int s = 0; s < steps && t < tEnd; s++)
        {
            float d = chainDist(ch, rd * t);
            g += exp(-d * 12.0);
            if (d / t < closest)
            {
                closest = d / t;
                tClosest = t;
            }
            if (d < 0.3 * pix * t)
                break;
            t += d * 0.75; // under 1: the sway bends the distances a little
        }
        float fog = exp(-max(tClosest - 2.5, 0.0) * 0.10);
        float3 tint = lerp(m.accent, m.accent.gbr, 0.6 * ch.tint);
        glow += tint * g * fog * ch.weight;

        // How much of the pixel it covers: all where the ray hit, a part on
        // its edge. The nearest chain that covers most of it is shaded.
        float cov = 1.0 - smoothstep(0.3, 1.3, closest / pix);
        bool better = (cov > 0.5 && (bestCov <= 0.5 || tClosest < bestT)) || (bestCov <= 0.5 && cov > bestCov);
        if (better)
        {
            best = ch;
            bestCov = cov;
            bestT = tClosest;
            bestLit = ch.weight * fog; // far ones dim
        }
    }

    float3 c = glow * 0.012 * (1.0 + 2.5 * m.ck);
    if (bestCov > 0.0)
        c += chainShade(best, rd * bestT, rd, bestT, m) * bestCov * bestLit;

    float amount = 0.8 + 0.4 * m.energy; // the verse calmer, the chorus wilder
    float3 fx = c * clear * amount;
    // A soft ceiling, gentler than the others': chrome needs its whites.
    return fx / (1.0 + 0.7 * fx);
}

// debugView shows how evenly things move, top to bottom:
//   1. a white bar timed by this shader: how smooth the terminal draws
//   2. a yellow bar timed by spotui (its clock, sent each frame) at the
//      same speed: if it judders where the white one does not, frames from
//      spotui arrive unevenly
//   3. the spectrum and, on the right: bass (blue), pulse (red), snare
//      (green), energy (white)
// and a square in the top right corner that flips on every frame spotui
// sends: an even flicker means an even frame rate.
float3 debugView(float2 tex, Music m)
{
    const float bar = 0.004;
    float3 c = float3(0.0, 0.0, 0.0);
    if (tex.y < 0.33)
    {
        c = abs(tex.x - frac(Time / 2.0)) < bar ? float3(1.0, 1.0, 1.0) : float3(0.04, 0.04, 0.04);
    }
    else if (tex.y < 0.66)
    {
        c = abs(tex.x - m.travel) < bar ? float3(1.0, 0.8, 0.2) : float3(0.05, 0.04, 0.0);
    }
    else
    {
        float h = (1.0 - tex.y) / 0.34;
        if (tex.x < 0.92)
            c = h < m.bands[clamp(int(tex.x / 0.92 * BANDS), 0, BANDS - 1)] ? m.accent * 0.6 : float3(0.0, 0.0, 0.0);
        else if (tex.x < 0.94)
            c = h < m.bass ? float3(0.2, 0.4, 1.0) : float3(0.0, 0.0, 0.0);
        else if (tex.x < 0.96)
            c = h < m.pulse ? float3(0.9, 0.15, 0.15) : float3(0.0, 0.0, 0.0);
        else if (tex.x < 0.98)
            c = h < m.snare ? float3(0.2, 0.9, 0.3) : float3(0.0, 0.0, 0.0);
        else
            c = h < m.energy ? float3(0.9, 0.9, 0.9) : float3(0.0, 0.0, 0.0);
    }
    if (tex.x > 0.96 && tex.y < 0.06)
    {
        float frame = round(m.drop * 255.0);
        c = fmod(frame, 2.0) < 0.5 ? float3(0.0, 0.0, 0.0) : float3(1.0, 1.0, 1.0);
    }
    return c * 0.6;
}

// effect draws the given style.
float3 effect(int style, float2 tex, Music m)
{
    if (style == 6)
        return debugView(tex, m);
    if (style == 5)
        return chains(tex, m);
    if (style == 4)
        return synthwave(tex, m);
    if (style == 3)
        return aurora(tex, m);
    if (style == 2)
        return warp(tex, m);
    if (style == 1)
        return tunnel(tex, m);
    return plasma(tex, m);
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
    // With a transparent background this is exactly what a premultiplied
    // "over" asks for: the effect shows through by what the letter does not
    // cover, so its soft edges blend into the effect. Cutting the effect off
    // faster than that left a dark rim around every letter on bright
    // effects (the synthwave's sky). Opaque backgrounds have no coverage to
    // go by: there the distance from the background colour stands in for it.
    float mask = 1.0;
    if (!inStrip)
        mask = clearBg ? saturate(1.0 - colour.a)
                       : 1.0 - smoothstep(0.02, 0.35, distance(colour.rgb, Background.rgb));
    if (mask <= 0.0)
        return colour;

    Music m = readMusic(origin, cell);
    float3 fx = effect(m.style, tex, m);
    // auto fading from one effect to the next: both, mixed.
    if (m.blend < 1.0 && m.prevStyle != m.style)
        fx = lerp(effect(m.prevStyle, tex, m), fx, smoothstep(0.0, 1.0, m.blend));

    // Premultiplied: adding colour with the alpha left alone lays the
    // effect over whatever is drawn underneath, background or acrylic.
    return float4(saturate(base.rgb + fx * mask), base.a);
}
