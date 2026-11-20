package cli

import (
	"errors"
	"testing"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

func TestParseCoordinates(t *testing.T) {
	ok := []struct {
		args     []string
		lat, lon float64
	}{
		{[]string{"43.6045", "1.444"}, 43.6045, 1.444},
		{[]string{"43.6045,1.444"}, 43.6045, 1.444},
		{[]string{"43,6045", "1,444"}, 43.6045, 1.444},
		{[]string{"43,6045;1,444"}, 43.6045, 1.444},
		{[]string{"-33.86", "151.2"}, -33.86, 151.2},
		{[]string{"43.6045,-1.444"}, 43.6045, -1.444},
		{[]string{"90,180"}, 90, 180},
		{[]string{"-90,-180"}, -90, -180},
	}
	for _, c := range ok {
		lat, lon, err := ParseCoordinates(c.args)
		if err != nil || lat != c.lat || lon != c.lon {
			t.Errorf("%v: got %v %v %v, want %v %v", c.args, lat, lon, err, c.lat, c.lon)
		}
	}
	bad := [][]string{{"91", "0"}, {"0", "181"}, {"abc"}, {}, {"1", "2", "3"}, {"90.0001,0"}}
	for _, args := range bad {
		_, _, err := ParseCoordinates(args)
		var ce *clierr.Error
		if !errors.As(err, &ce) || ce.Code != "invalid_coordinates" {
			t.Errorf("%v: got %v, want invalid_coordinates", args, err)
		}
	}
}

func TestFormatPointArg(t *testing.T) {
	if got := formatPointArg(43.6045, -1.444, 0, false); got != "43.6045,-1.444" {
		t.Errorf("got %q", got)
	}
	if got := formatPointArg(43.6045, -1.444, 25, true); got != "43.6045,-1.444,25" {
		t.Errorf("got %q", got)
	}
}
