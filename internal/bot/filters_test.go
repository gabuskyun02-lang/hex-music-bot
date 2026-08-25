package bot

import (
	"testing"
)

func f(v float64) *float64 { return &v }

func TestValidateTimescale(t *testing.T) {
	if err := validateTimescale(nil, nil, nil); err != nil {
		t.Fatalf("no values provided must pass, got %v", err)
	}
	if err := validateTimescale(f(0.5), f(0.25), f(4.0)); err != nil {
		t.Fatalf("exact bounds must pass (inclusive), got %v", err)
	}
	if err := validateTimescale(f(2.0), nil, nil); err != nil {
		t.Fatalf("speed upper bound inclusive, got %v", err)
	}

	for _, c := range []struct {
		name               string
		speed, pitch, rate *float64
	}{
		{"speed too low", f(0.49), nil, nil},
		{"speed too high", f(2.01), nil, nil},
		{"pitch too low", nil, f(0.24), nil},
		{"pitch too high", nil, f(4.01), nil},
		{"rate too low", nil, nil, f(0.24)},
		{"rate too high", nil, nil, f(4.1)},
	} {
		if err := validateTimescale(c.speed, c.pitch, c.rate); err == nil {
			t.Errorf("%s: out-of-range value must be rejected", c.name)
		}
	}
}
