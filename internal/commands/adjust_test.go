package commands

import "testing"

func TestClampSeek(t *testing.T) {
	cases := []struct {
		name     string
		pos, dur int64
		want     int64
	}{
		{"negative floors to zero", -5, 100, 0},
		{"in-range unchanged", 50, 100, 50},
		{"at duration clamps below it", 100, 100, 99},
		{"past duration clamps below it", 150, 100, 99},
		{"non-positive duration yields zero", 50, 0, 0},
		{"zero position stays zero", 0, 100, 0},
	}
	for _, c := range cases {
		if got := clampSeek(c.pos, c.dur); got != c.want {
			t.Errorf("%s: clampSeek(%d,%d) = %d, want %d", c.name, c.pos, c.dur, got, c.want)
		}
	}
}
