package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// the context window is 50 bytes either side of the difference; the lone "a" bytes shift the parity of the two-byte runes so that both window edges land in the middle of a character
func TestDiffRegionsValidUTF8(t *testing.T) {
	build := func(marker string) []byte {
		return []byte(strings.Repeat("é", 40) + "a" + marker + strings.Repeat("é", 5) + "a" + strings.Repeat("é", 40))
	}
	before, after := build("X"), build("Y")

	regions := diffRegions(before, after)
	if len(regions) != 1 {
		t.Fatalf("expected 1 region, got %d: %q", len(regions), regions)
	}
	for i, side := range []string{"before", "after"} {
		if !utf8.ValidString(regions[0][i]) {
			t.Errorf("%s snippet is not valid UTF-8 (window edge split a character): %q", side, regions[0][i])
		}
	}
}

// every offset pair over a string with 1-, 2-, 3-, and 4-byte characters must come back widened to whole characters
func TestSnapToRunes(t *testing.T) {
	s := []byte("aé€😀b")
	for lo := 0; lo <= len(s); lo++ {
		for hi := lo; hi <= len(s); hi++ {
			gotLo, gotHi := snapToRunes(s, lo, hi)
			if gotLo > lo || gotHi < hi {
				t.Fatalf("snapToRunes(%d, %d) = (%d, %d): must only widen the range", lo, hi, gotLo, gotHi)
			}
			if !utf8.Valid(s[gotLo:gotHi]) {
				t.Fatalf("snapToRunes(%d, %d) = (%d, %d): %q is not valid UTF-8", lo, hi, gotLo, gotHi, s[gotLo:gotHi])
			}
		}
	}
}

// a run of continuation bytes with no lead byte (not valid UTF-8) must not make either edge run away
func TestSnapToRunesInvalid(t *testing.T) {
	s := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}
	lo, hi := snapToRunes(s, 5, 6)
	if lo < 5-(utf8.UTFMax-1) || hi > 6+(utf8.UTFMax-1) {
		t.Fatalf("snapToRunes(5, 6) = (%d, %d): moved an edge more than %d bytes", lo, hi, utf8.UTFMax-1)
	}
}
