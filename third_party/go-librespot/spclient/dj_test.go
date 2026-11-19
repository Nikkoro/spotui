package spclient

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestDjAskMarshal(t *testing.T) {
	b := (&DjAskRequest{Prompt: "90s hip-hop", SessionId: "s", DeviceId: "d", CurrentTrackUri: "spotify:track:x", Input: DjPromptSuggested}).marshal()
	got := map[protowire.Number]any{}
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		b = b[n:]
		switch typ {
		case protowire.BytesType:
			s, m := protowire.ConsumeString(b)
			got[num] = s
			b = b[m:]
		case protowire.VarintType:
			v, m := protowire.ConsumeVarint(b)
			got[num] = v
			b = b[m:]
		}
	}
	if got[1] != "90s hip-hop" || got[2] != "s" || got[3] != "d" || got[7] != "spotify:track:x" || got[8] != uint64(3) {
		t.Fatalf("got %v", got)
	}
	if _, ok := got[4]; ok {
		t.Fatal("empty segment must be omitted")
	}
}

func TestParseStrings(t *testing.T) {
	var b []byte
	b = appendString(b, 1, "OK")
	b = protowire.AppendTag(b, 9, protowire.VarintType)
	b = protowire.AppendVarint(b, 5)
	b = appendString(b, 2, "fine")
	s1, s2, err := parseStrings12(b)
	if err != nil || s1 != "OK" || s2 != "fine" {
		t.Fatalf("%q %q %v", s1, s2, err)
	}
}
