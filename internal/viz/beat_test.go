package viz

import (
	"math"
	"testing"
)

// frames makes 30 fps frames of a synthetic signal: a 60 Hz "kick" for the
// first 80 ms of every beat, quiet noise otherwise.
func kickFrames(seconds, bpm, amp float64) [][]float32 {
	per := SampleRate / 30
	n := int(seconds * 30)
	beatEvery := 60 / bpm * SampleRate
	out := make([][]float32, n)
	t := 0
	for f := range out {
		fr := make([]float32, per)
		for i := range fr {
			inBeat := math.Mod(float64(t), beatEvery) < 0.08*SampleRate
			v := 0.002 * math.Sin(float64(t)*0.9)
			if inBeat {
				v += amp * math.Sin(2*math.Pi*60*float64(t)/SampleRate)
			}
			fr[i] = float32(v)
			t++
		}
		out[f] = fr
	}
	return out
}

func TestBeatDetectorFindsKicks(t *testing.T) {
	b := NewBeatDetector()
	hits := 0
	for _, fr := range kickFrames(8, 120, 0.6) {
		if b.Feed(fr, 1000.0/30).Hit {
			hits++
		}
	}
	// 120 bpm for 8 s is 16 beats; the first half second is warm-up.
	if hits < 12 || hits > 17 {
		t.Fatalf("found %d beats in 8 s at 120 bpm", hits)
	}
}

func TestBeatDetectorQuietOnSilence(t *testing.T) {
	b := NewBeatDetector()
	for i := 0; i < 120; i++ {
		if b.Feed(make([]float32, SampleRate/30), 1000.0/30).Hit {
			t.Fatal("a beat in silence")
		}
	}
}

func TestBeatDetectorDrop(t *testing.T) {
	b := NewBeatDetector()
	for _, fr := range kickFrames(6, 120, 0.4) { // the track runs
		b.Feed(fr, 1000.0/30)
	}
	for _, fr := range kickFrames(3, 120, 0.05) { // a breakdown
		b.Feed(fr, 1000.0/30)
	}
	drop := false
	for _, fr := range kickFrames(1, 120, 0.9) { // and it comes back in
		if b.Feed(fr, 1000.0/30).Drop {
			drop = true
		}
	}
	if !drop {
		t.Fatal("no drop when the kick came back after the breakdown")
	}
}
