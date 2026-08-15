package semver

import "testing"

// Probe: caret range ^0.0.x must pin to exact patch.
// ^0.0.3 means >=0.0.3 <0.0.4 per SemVer caret semantics.

func TestCaretZeroZeroPatchUpperBound(t *testing.T) {
	cases := []struct {
		ver  string
		rng  string
		want bool
	}{
		{"0.0.3", "^0.0.3", true},
		{"0.0.4", "^0.0.3", false},
		{"0.0.0", "^0.0.0", true},
		{"0.0.1", "^0.0.0", false},
		{"0.0.5", "^0.0.5", true},
		{"0.0.6", "^0.0.5", false},
	}
	for _, c := range cases {
		v, err := Parse(c.ver)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.ver, err)
		}
		got, err := SatisfiesRange(v, c.rng)
		if err != nil {
			t.Fatalf("SatisfiesRange(%q, %q): %v", c.ver, c.rng, err)
		}
		if got != c.want {
			t.Errorf("SatisfiesRange(%q, %q) = %v, want %v", c.ver, c.rng, got, c.want)
		}
	}
}
