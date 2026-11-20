package spclient

import "testing"

func TestParseLyrics(t *testing.T) {
	// Invented text, shaped like Spotify's answer.
	body := []byte(`{"lyrics":{"syncType":"LINE_SYNCED","providerDisplayName":"Musixmatch","language":"en",
		"lines":[{"startTimeMs":"1200","words":"first made-up line"},{"startTimeMs":"4500","words":"second made-up line"},{"startTimeMs":"9000","words":"♪"}]}}`)
	l, err := parseLyrics(body)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Synced || l.Provider != "Musixmatch" || len(l.Lines) != 3 || l.Lines[1].StartMs != 4500 {
		t.Fatalf("%+v", l)
	}
	if _, err := parseLyrics([]byte(`{"lyrics":{"lines":[]}}`)); err != ErrNoLyrics {
		t.Fatalf("empty: %v", err)
	}
}
