package helpers

import (
	"math/big"
	"testing"
)

func bitmapWords(set map[int16][]int) func(int16) (*big.Int, error) {
	return func(wp int16) (*big.Int, error) {
		w := new(big.Int)
		for _, b := range set[wp] {
			w.SetBit(w, b, 1)
		}
		return w, nil
	}
}

func TestV4FindActiveRange(t *testing.T) {
	cases := []struct {
		name               string
		words              map[int16][]int
		tick, spacing      int32
		lower, upper       int32
		hasLower, hasUpper bool
		wordPos            int16
	}{
		{"same word", map[int16][]int{0: {0, 5}}, 100, 60, 0, 300, true, true, 0},
		{"tick on initialized tick", map[int16][]int{0: {0, 5}}, 300, 60, 300, 0, true, false, 0},
		{"negative tick crosses word boundary", map[int16][]int{-1: {250}, 0: {3}}, -5, 10, -60, 30, true, true, -1},
		{"adjacent words both sides", map[int16][]int{-1: {255}, 1: {0}}, 50, 1, -1, 256, true, true, 0},
		{"empty bitmap", map[int16][]int{}, 1000, 10, 0, 0, false, false, 0},
	}
	for _, c := range cases {
		lower, upper, hasLower, hasUpper, wp, _, err := v4FindActiveRange(bitmapWords(c.words), c.tick, c.spacing)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if hasLower != c.hasLower || hasUpper != c.hasUpper || wp != c.wordPos ||
			(hasLower && lower != c.lower) || (hasUpper && upper != c.upper) {
			t.Errorf("%s: got lower=%d(%v) upper=%d(%v) wp=%d, want lower=%d(%v) upper=%d(%v) wp=%d",
				c.name, lower, hasLower, upper, hasUpper, wp, c.lower, c.hasLower, c.upper, c.hasUpper, c.wordPos)
		}
	}
}
