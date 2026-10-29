package main

import (
	"math"
	"slices"
	"strings"

	"fragwatch/internal/rating"
)

// prevVersion and prevScore freeze the formula before the current rating.Version, so -eval can
// show what a retune gained. Replace both with the outgoing rating.Score when the version is bumped.
const prevVersion = 1

var prevFameBase = [6]float64{0, 2, 4, 6, 7.5, 9}

func prevScore(name string, f rating.Facts) int {
	n := len(name)
	fame := min(5, max(0, f.Fame))
	pull := min(2, max(0, f.Pull))
	clean := min(2, max(0, f.Clean))
	kind := strings.ToLower(strings.TrimSpace(f.Kind))
	if !slices.Contains(rating.Kinds, kind) || kind == "pattern" {
		kind = "brand"
	}
	s := prevFameBase[fame]
	if fame >= 2 {
		switch {
		case n <= 5:
			s += 1
		case n <= 7:
			s += 0.5
		case n <= 10:
		case n <= 13:
			s -= 1
		default:
			s -= 1.5
		}
		switch words := len(strings.Fields(f.Words)); {
		case words == 2:
			s -= 0.5
		case words >= 3:
			s -= 1.5
		}
		s += 0.75 * float64(pull-1)
	}
	switch clean {
	case 0:
		s -= 2
	case 1:
		s -= 0.5
	}
	if fame == 0 || kind == "junk" {
		s = min(s, 1)
	}
	switch p := rating.Pattern(name); {
	case p == "same" && n <= 5:
		s = 10
	case p == "same" && n == 6:
		s = max(s, 7)
	case (p == "abab" || p == "abba") && n == 4:
		s = max(s, 9)
	case p == "abab":
		s = max(s, 7)
	}
	if n == 4 {
		s = max(s, 6)
	}
	return int(math.Round(min(10, max(0, s))))
}
