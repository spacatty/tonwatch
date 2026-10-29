// Package rating turns the facts the LLM records about a username into its 0-10 score.
//
// The rating has two layers. The model records facts that do not drift with fashion: how the name
// reads, its language, how well known the reading is, how much a business would want it, how
// cleanly it is written. That costs one model call per name and is rarely redone. The score is
// arithmetic over those facts and the name's shape (length, patterns), calibrated on what names
// actually sold for. Changing how names are judged is an edit here, a bump of Version, and a
// rescore of the whole table in seconds (the server does it on start); never a new model pass.
package rating

import (
	"math"
	"slices"
	"strings"
)

// Version identifies the formula. Bump it on any change to Score: on start the server sees a
// version it has not applied yet and recomputes every score from the stored facts.
const Version = 2

// Kinds a name can be. The model picks one; "pattern" is only ever set by Score, from the shape.
var Kinds = []string{"word", "name", "brand", "phrase", "crypto", "number", "short", "geo", "slang", "foreign", "pattern", "junk"}

// Facts are what the model records about one name. See llm.FactsPrompt for their definitions.
type Facts struct {
	Words string // the reading, words separated by spaces ("dark moon"); "" when it reads as nothing
	Kind  string
	Lang  string // two-letter code of the reading's language
	Fame  int    // 0-5: how widely known the reading is
	Pull  int    // 0-2: how much a business, product or community would want it as a handle
	Clean int    // 0-2: how naturally it is written
}

// fameBase is the score a name of a given fame starts from, before length, shape and spelling.
var fameBase = [6]float64{0, 2, 4, 6, 7.5, 9}

// Score is a name's 0-10 rating and its final kind.
//
// What the market data says, and where each rule comes from (Fragment sales, Oct 2026):
//   - Below the top bands, length is what buyers pay for. Four-letter names are scarce and sold
//     for ~6k TON median even as plain words (see four for how they split); five letters and up
//     sell for nothing unless they mean something.
//   - Repeated letters are prized on their own: aaaa/aaaaa sold for 66-78k TON median, abab and
//     abba four-letter names for ~16k.
//   - Long compounds and phrases sell far below single words of the same fame: at 13+ letters the
//     median was 10 TON however good the words.
func Score(name string, f Facts) (int, string) {
	n := len(name)
	fame := min(5, max(0, f.Fame))
	pull := min(2, max(0, f.Pull))
	clean := min(2, max(0, f.Clean))
	kind := strings.ToLower(strings.TrimSpace(f.Kind))
	if !slices.Contains(Kinds, kind) || kind == "pattern" {
		kind = "brand"
	}

	s := fameBase[fame]
	if fame >= 2 {
		// a reading that means something gains from being short and loses from being long
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
		s += 0.75 * float64(pull-1) // commercial pull: "bank" over "then"
	}
	switch clean {
	case 0:
		s -= 2
	case 1:
		s -= 0.5
	}
	if n == 4 {
		s, kind = four(fame, kind, s)
	} else if fame == 0 || kind == "junk" {
		s = min(s, 1)
		kind = "junk"
	}

	switch p := Pattern(name); {
	case p == "same" && n <= 5:
		s, kind = 10, "pattern"
	case p == "same" && n == 6:
		s, kind = max(s, 7), "pattern"
	case (p == "abab" || p == "abba") && n == 4:
		s, kind = max(s, 9), "pattern"
	case p == "abab":
		s, kind = max(s, 7), "pattern"
	}
	return int(math.Round(min(10, max(0, s)))), kind
}

// four scores a four-letter name. s is the score the same facts get at five letters; a four-letter
// name always scores above it (or level at 9), since four letters sell for far more.
//
// Four-letter names are scarce (Telegram only issues them through Fragment), so none is junk to
// the market. Past that, sales split them by what kind of name they are more than by how well
// known the word is (300 sold four-letter names, Oct 2026; tuned on half, checked on the other):
//   - A plain reading (a word, a name, a place) sold for ~6k TON whatever its fame from 2 to 4.
//     Fame, pull and spelling still order the readings, one step above their five-letter score,
//     within 6-9. Ranking them all level at 9 fitted the tuning half better but not the held-out
//     half, so the ordering stays. Only iconic words (fame 5) went far higher, ~50k: 10.
//   - A letter combination the model cannot read as a word (an abbreviation, a ticker, an invented
//     brand) sold for ~65k, like same-letter names: buyers of four letters pay for the letters. 10.
func four(fame int, kind string, s float64) (float64, string) {
	switch {
	case fame < 2 || kind == "junk" || kind == "short" || kind == "brand":
		if kind == "junk" {
			kind = "short"
		}
		return 10, kind
	case fame == 5:
		return 10, kind
	default:
		return min(9, max(6, s+1)), kind
	}
}

// Pattern names a shape the market pays for by itself, whatever the name means: "same" (aaaa),
// "abab" (abab, ababab), "abba", or "" for none.
func Pattern(name string) string {
	n := len(name)
	if n < 4 {
		return ""
	}
	if strings.Count(name, name[:1]) == n {
		return "same"
	}
	if n%2 == 0 && n <= 8 && strings.Repeat(name[:2], n/2) == name {
		return "abab"
	}
	if n == 4 && name[0] == name[3] && name[1] == name[2] {
		return "abba"
	}
	return ""
}

// LenBucket groups lengths that sell alike, for typical prices: "4", "5", "6-7", "8-10", "11+".
func LenBucket(n int) string {
	switch {
	case n <= 4:
		return "4"
	case n == 5:
		return "5"
	case n <= 7:
		return "6-7"
	case n <= 10:
		return "8-10"
	default:
		return "11+"
	}
}

// LenBuckets lists the buckets with their length ranges, in order.
var LenBuckets = []struct {
	Name     string
	Min, Max int
}{{"4", 0, 4}, {"5", 5, 5}, {"6-7", 6, 7}, {"8-10", 8, 10}, {"11+", 11, 64}}
