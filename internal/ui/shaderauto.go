package ui

import (
	"math"
	"math/rand/v2"
	"slices"

	"github.com/Nikkoro/spotui/internal/engine"
)

// "auto" picks the shader's effect by itself, the way Winamp moves through
// MilkDrop presets: it moves on at a drop (quickly), when a louder part
// comes in, on a new track, or after about 32 bars on the first beat of a
// bar, and fades from one effect to the next. Which effect comes next
// fits the mood with a little chance: calm music leans to the aurora and
// the rain, loud and fast music to the tunnel, the warp, synthwave and
// chains, the plasma fits any.
const shaderAuto = "auto"

// autoStyles are the effects auto chooses from (indices in shaderStyles),
// and how well each fits calm (0) and intense (1) music.
var autoStyles = []struct {
	style      int
	calm, wild float64
}{
	{0, 0.6, 0.6},   // plasma
	{1, 0.1, 1.0},   // tunnel
	{2, 0.2, 1.0},   // warp
	{3, 1.0, 0.1},   // aurora
	{4, 0.15, 0.95}, // synthwave
	{5, 0.1, 0.9},   // chains
	{6, 0.9, 0.15},  // rain
}

const (
	autoFade     = 2.5  // seconds a change fades over
	autoDropFade = 0.6  // …and at a drop
	autoMinGap   = 12.0 // seconds at least between changes, but at a drop
	autoDropGap  = 6.0  // …and at a drop
	autoBars     = 32   // bars before moving on anyway
	autoRise     = 0.3  // energy over its low of the last few seconds that marks a louder part
	autoChance   = 0.25 // every effect's share, whatever the mood
	autoDropMood = 0.85 // the mood a drop counts as, at least
)

// shaderAutoState is auto's: what shows, what it fades from, since when.
type shaderAutoState struct {
	cur, prev int     // styles (indices in shaderStyles)
	blend     float64 // 0 showing prev … 1 showing cur
	fade      float64 // seconds the current fade takes
	since     float64 // seconds since the last change
	beats     int     // beats since the last change
	lowEnergy float64 // energy's recent low, rising back slowly
	track     string  // the track it last changed for
	started   bool
	rng       *rand.Rand
}

// mood is how intense the music is now, 0 calm … 1 intense: the energy of
// this part of the track, the tempo and the bass.
func (s *shaderFx) mood() float64 {
	tempo := 0.5
	if s.beatSec > 0 {
		tempo = math.Max(0, math.Min(1, (60/s.beatSec-80)/80)) // 80 bpm calm … 160 intense
	}
	return math.Max(0, math.Min(1, 0.55*s.energy+0.25*tempo+0.2*s.bass))
}

// autoNames are the effects auto can use, by name, in autoStyles' order.
var autoNames = func() []string {
	n := make([]string, len(autoStyles))
	for i, c := range autoStyles {
		n[i] = shaderStyles[c.style]
	}
	return n
}()

// autoChoice are the effects auto moves between (wt_shader_auto), as
// styles; nil means all of them. Names it doesn't know are left out (the
// config warns about them at start).
func autoChoice(names []string) []int {
	var out []int
	for _, c := range autoStyles {
		if slices.Contains(names, shaderStyles[c.style]) {
			out = append(out, c.style)
		}
	}
	return out
}

// unknownAutoNames are the names in wt_shader_auto that are no effect
// auto can use.
func unknownAutoNames(names []string) []string {
	var bad []string
	for _, n := range names {
		if !slices.Contains(autoNames, n) {
			bad = append(bad, n)
		}
	}
	return bad
}

// autoAllows reports whether style is among the chosen (nil: all).
func autoAllows(chosen []int, style int) bool {
	return len(chosen) == 0 || slices.Contains(chosen, style)
}

// pick chooses the next effect for the mood among the chosen ones (nil:
// all), never the one showing while there is another.
func (a *shaderAutoState) pick(mood float64, chosen []int) int {
	if a.rng == nil {
		a.rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	others := 0
	for _, c := range autoStyles {
		if autoAllows(chosen, c.style) && !(a.started && c.style == a.cur) {
			others++
		}
	}
	weights := make([]float64, len(autoStyles))
	var total float64
	for i, c := range autoStyles {
		if !autoAllows(chosen, c.style) || (others > 0 && a.started && c.style == a.cur) {
			continue
		}
		weights[i] = autoChance + c.calm*(1-mood) + c.wild*mood
		total += weights[i]
	}
	r := a.rng.Float64() * total
	for i, w := range weights {
		if r < w {
			return autoStyles[i].style
		}
		r -= w
	}
	for i := len(autoStyles) - 1; i >= 0; i-- { // rounding: the last one that counts
		if weights[i] > 0 {
			return autoStyles[i].style
		}
	}
	return autoStyles[0].style
}

// stepAuto moves auto on by dt seconds and changes the effect when the
// music asks for it. beat and drop are this frame's; track is the one
// playing; chosen are the effects it may use (nil: all).
func (s *shaderFx) stepAuto(dt float64, beat, drop bool, track string, chosen []int, eng *engine.Engine) {
	a := &s.auto
	a.since += dt
	if a.fade > 0 {
		a.blend = math.Min(1, a.blend+dt/a.fade)
	}
	if beat {
		a.beats++
	}
	// The energy's recent low: follows it down at once, back up slowly.
	if !a.started || s.energy < a.lowEnergy {
		a.lowEnergy = s.energy
	} else {
		a.lowEnergy += (s.energy - a.lowEnergy) * math.Min(1, dt/4)
	}

	// A single effect chosen: it just shows, no fades, no moving on.
	if len(chosen) == 1 {
		if !a.started || a.cur != chosen[0] || a.blend < 1 {
			a.cur, a.prev, a.blend, a.fade = chosen[0], chosen[0], 1, 0
		}
		a.since, a.beats, a.track, a.started = 0, 0, track, true
		return
	}

	why := ""
	switch {
	case !a.started:
		why = "start"
	case !autoAllows(chosen, a.cur):
		why = "taken out of auto"
	case track != a.track:
		why = "new track"
	case drop && a.since > autoDropGap:
		why = "drop"
	case a.since > autoMinGap && s.energy-a.lowEnergy > autoRise:
		why = "louder part"
	case a.beats >= autoBars*4 && beat && s.count%4 == 1:
		why = "32 bars"
	case a.since > 120:
		why = "two minutes" // no beat found to count bars by
	}
	if why == "" {
		return
	}
	mood := s.mood()
	if why == "drop" {
		// The mood comes from the last few seconds, which before a drop
		// are the quiet build-up; the drop itself is anything but calm.
		mood = math.Max(mood, autoDropMood)
	}
	next := a.pick(mood, chosen)
	from := "start"
	if a.started {
		from = shaderStyles[a.cur]
	}
	eng.Debugf("ui: shader auto: %s → %s (%s, mood %.2f)", from, shaderStyles[next], why, mood)
	if !a.started {
		a.cur, a.prev, a.blend = next, next, 1
	} else {
		a.prev, a.cur, a.blend = a.cur, next, 0
	}
	a.fade = autoFade
	if why == "drop" {
		a.fade = autoDropFade
	}
	a.since, a.beats, a.track, a.started = 0, 0, track, true
	a.lowEnergy = s.energy
}
