package daemon

import (
	"testing"

	"google.golang.org/protobuf/proto"

	metadatapb "github.com/devgianlu/go-librespot/proto/spotify/metadata"
)

func TestReleaseDate(t *testing.T) {
	for _, c := range []struct {
		d    *metadatapb.Date
		want string
	}{
		{nil, ""},
		{&metadatapb.Date{Year: proto.Int32(2011)}, "2011"},
		{&metadatapb.Date{Year: proto.Int32(2011), Month: proto.Int32(10)}, "2011-10"},
		{&metadatapb.Date{Year: proto.Int32(2011), Month: proto.Int32(10), Day: proto.Int32(18)}, "2011-10-18"},
	} {
		if got := releaseDate(c.d); got != c.want {
			t.Errorf("%v: got %q, want %q", c.d, got, c.want)
		}
	}
}
