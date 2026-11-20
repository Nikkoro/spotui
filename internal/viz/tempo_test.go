package viz

import (
	"bufio"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"
)

// band makes seconds of a synthetic band in 30 fps frames: a kick on every
// beat under a loud, steady bass line (the case a plain bass threshold
// misses), a snare on beats 2 and 4, and hats on the eighths. It returns
// the frames and the frame each kick starts in.
func band(seconds, bpm float64, snare bool) ([][]float32, []int) {
	per := SampleRate / 30
	total := int(seconds * SampleRate)
	beat := 60 / bpm * SampleRate
	rng := rand.New(rand.NewPCG(1, 2))
	sig := make([]float64, total)
	var kicks []int
	for b := 0; float64(b)*beat < float64(total); b++ {
		start := int(float64(b) * beat)
		kicks = append(kicks, start/per)
		for i := 0; i < int(0.12*SampleRate) && start+i < total; i++ {
			env := math.Exp(-float64(i) / (0.04 * SampleRate))
			sig[start+i] += 0.6 * env * math.Sin(2*math.Pi*55*float64(i)/SampleRate)
		}
		if snare && b%2 == 1 {
			for i := 0; i < int(0.1*SampleRate) && start+i < total; i++ {
				env := math.Exp(-float64(i) / (0.03 * SampleRate))
				sig[start+i] += 0.3 * env * (rng.Float64()*2 - 1)
			}
		}
		for _, half := range []float64{0, 0.5} {
			hs := start + int(half*beat)
			for i := 0; i < int(0.03*SampleRate) && hs+i < total; i++ {
				env := math.Exp(-float64(i) / (0.008 * SampleRate))
				sig[hs+i] += 0.08 * env * (rng.Float64()*2 - 1)
			}
		}
	}
	for i := range sig {
		// The bass line: loud, steady, moving between two notes each beat.
		f := 82.0
		if int(float64(i)/beat)%2 == 1 {
			f = 98
		}
		sig[i] += 0.35 * math.Sin(2*math.Pi*f*float64(i)/SampleRate)
	}
	frames := make([][]float32, total/per)
	for f := range frames {
		fr := make([]float32, per)
		for i := range fr {
			fr[i] = float32(sig[f*per+i])
		}
		frames[f] = fr
	}
	return frames, kicks
}

func TestTempoTrackerFindsTheBeatUnderABassLine(t *testing.T) {
	for _, bpm := range []float64{148, 92, 124} {
		frames, kicks := band(14, bpm, true)
		tr := NewTempoTracker()
		var hits []int
		var last Tempo
		for f, fr := range frames {
			last = tr.Feed(fr, 1.0/30)
			if last.Hit && f > 9*30 {
				hits = append(hits, f)
			}
		}
		if math.Abs(last.BPM-bpm)/bpm > 0.03 {
			t.Errorf("%.0f bpm: found %.1f bpm", bpm, last.BPM)
			continue
		}
		// Every beat in the last seconds, each within two frames of a kick.
		want := int(5 * bpm / 60)
		if len(hits) < want-1 || len(hits) > want+1 {
			t.Errorf("%.0f bpm: %d beats in the last 5 s, want about %d", bpm, len(hits), want)
		}
		for _, h := range hits {
			near := 99
			for _, k := range kicks {
				near = min(near, abs(h-k))
			}
			if near > 2 {
				t.Errorf("%.0f bpm: beat at frame %d is %d frames from a kick", bpm, h, near)
			}
		}
		if last.Confidence < 0.15 {
			t.Errorf("%.0f bpm: confidence %.2f", bpm, last.Confidence)
		}
	}
}

func TestTempoTrackerSnares(t *testing.T) {
	frames, _ := band(10, 120, true)
	tr := NewTempoTracker()
	snares := 0
	for f, fr := range frames {
		if tr.Feed(fr, 1.0/30).Snare && f > 5*30 {
			snares++
		}
	}
	// 120 bpm, a snare every other beat: 5 in 5 s. Hats are quieter and
	// must not count, or there would be 20.
	if snares < 4 || snares > 7 {
		t.Fatalf("%d snares in 5 s, want about 5", snares)
	}
}

func TestTempoTrackerSilence(t *testing.T) {
	tr := NewTempoTracker()
	quiet := make([]float32, SampleRate/30)
	for i := 0; i < 300; i++ {
		if got := tr.Feed(quiet, 1.0/30); got.Hit || got.Snare || got.BPM != 0 {
			t.Fatalf("something in silence: %+v", got)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// TestTempoOnRecording replays a recording made with SPOTUI_TEMPO_DUMP
// through the tracker and prints the tempo it holds, second by second:
//
//	SPOTUI_TEMPO_RECORDING=track.csv SPOTUI_TEMPO_EXPECT=148 go test -run OnRecording -v ./internal/viz
//
// With SPOTUI_TEMPO_EXPECT it fails if the tempo is off for more than a
// tenth of the time after the first ten seconds.
func TestTempoOnRecording(t *testing.T) {
	path := os.Getenv("SPOTUI_TEMPO_RECORDING")
	if path == "" {
		t.Skip("set SPOTUI_TEMPO_RECORDING to a SPOTUI_TEMPO_DUMP file")
	}
	expect, _ := strconv.ParseFloat(os.Getenv("SPOTUI_TEMPO_EXPECT"), 64)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan() // header
	tr := NewTempoTracker()
	var nextPrint float64
	var beats, snares, off, judged int
	for sc.Scan() {
		var v [5]float64
		for i, p := range strings.Split(sc.Text(), ",") {
			v[i], _ = strconv.ParseFloat(p, 64)
		}
		got := tr.FeedEnergies(v[2], v[3], v[4], v[0])
		if got.Hit {
			beats++
		}
		if got.Snare {
			snares++
		}
		if v[1] >= nextPrint {
			t.Logf("%5.1fs  %6.1f bpm  sure %.2f  %d beats %d snares", v[1], got.BPM, got.Confidence, beats, snares)
			nextPrint = math.Floor(v[1]) + 1
			beats, snares = 0, 0
			if expect > 0 && v[1] > 10 {
				judged++
				if math.Abs(got.BPM-expect)/expect > 0.03 {
					off++
				}
			}
		}
	}
	if expect > 0 && off*10 > judged {
		t.Errorf("tempo off %.0f bpm for %d of %d seconds", expect, off, judged)
	}
}
