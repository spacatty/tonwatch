package rating

import "testing"

func TestPattern(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"aaaa", "same"},
		{"zzzzz", "same"},
		{"777777", "same"},
		{"abab", "abab"},
		{"tutu", "abab"},
		{"lalala", "abab"},
		{"momomomo", "abab"},
		{"momomomomo", ""}, // abab only up to 8 characters
		{"anna", "abba"},
		{"otto", "abba"},
		{"abba", "abba"},
		{"abcba", ""},
		{"coffee", ""},
		{"aab", ""}, // too short to count
		{"aaab", ""},
		{"abcabc", ""},
	} {
		if got := Pattern(c.name); got != c.want {
			t.Errorf("Pattern(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLenBucket(t *testing.T) {
	for n, want := range map[int]string{3: "4", 4: "4", 5: "5", 6: "6-7", 7: "6-7", 8: "8-10", 10: "8-10", 11: "11+", 32: "11+"} {
		if got := LenBucket(n); got != want {
			t.Errorf("LenBucket(%d) = %q, want %q", n, got, want)
		}
	}
	// the table and the function agree, and the buckets cover every length without gaps
	prev := 0
	for _, b := range LenBuckets {
		if b.Min != prev && !(prev == 0 && b.Min == 0) {
			t.Errorf("bucket %s starts at %d, after %d", b.Name, b.Min, prev)
		}
		for n := max(4, b.Min); n <= min(b.Max, 32); n++ {
			if LenBucket(n) != b.Name {
				t.Errorf("length %d: LenBucket %q, table %q", n, LenBucket(n), b.Name)
			}
		}
		prev = b.Max + 1
	}
}

func TestScore(t *testing.T) {
	type F = Facts
	for _, c := range []struct {
		name  string
		f     F
		score int
		kind  string
	}{
		// readings of six letters and up: fame first, then length, words, pull and spelling
		{"coffee", F{"coffee", "word", "en", 4, 2, 2}, 9, "word"},
		{"harbor", F{"harbor", "word", "en", 3, 1, 2}, 7, "word"},
		{"goodnews", F{"good news", "phrase", "en", 3, 1, 2}, 6, "phrase"},
		{"goodmorningsun", F{"good morning sun", "phrase", "en", 3, 1, 2}, 3, "phrase"},
		{"lunexo", F{"lunexo", "brand", "", 1, 1, 2}, 2, "brand"},
		{"alex1987", F{"alex 1987", "name", "en", 1, 0, 0}, 0, "name"},
		{"qxzt7kp", F{"", "junk", "", 0, 0, 0}, 0, "junk"},
		{"garden_club", F{"garden club", "phrase", "en", 3, 1, 1}, 4, "phrase"},
		// five letters
		{"maria", F{"maria", "name", "es", 4, 1, 2}, 9, "name"},
		{"zzzzz", F{"", "junk", "", 0, 0, 2}, 10, "pattern"},
		// four letters: readings in 6-9, iconic words and letter combinations at the top
		{"kiwi", F{"kiwi", "word", "en", 3, 1, 2}, 8, "word"},
		{"cafe", F{"cafe", "word", "en", 4, 2, 2}, 9, "word"},
		{"fiji", F{"fiji", "geo", "en", 2, 0, 2}, 6, "geo"},
		{"love", F{"love", "word", "en", 5, 2, 2}, 10, "word"},
		{"zorv", F{"zorv", "brand", "", 1, 0, 2}, 10, "brand"},
		{"xqzt", F{"", "junk", "", 0, 0, 2}, 10, "short"},
		{"aaaa", F{"", "junk", "", 0, 0, 2}, 10, "pattern"},
		{"anna", F{"anna", "name", "en", 4, 1, 2}, 9, "pattern"},
		{"tutu", F{"tutu", "word", "en", 2, 1, 2}, 9, "pattern"},
		// six letters, same letter
		{"aaaaaa", F{"", "junk", "", 0, 0, 2}, 7, "pattern"},
		// the model's kind is checked, and its numbers clamped
		{"garden", F{"garden", "unicorn", "en", 4, 1, 2}, 8, "brand"},
		{"meadow", F{"meadow", "pattern", "en", 3, 1, 2}, 7, "brand"},
		{"travel", F{"travel", "word", "en", 9, 7, 5}, 10, "word"},
	} {
		score, kind := Score(c.name, c.f)
		if score != c.score || kind != c.kind {
			t.Errorf("Score(%q) = %d %s, want %d %s", c.name, score, kind, c.score, c.kind)
		}
	}
}

// Four letters sell for far more than five: a four-letter name never scores below a five-letter
// one with the same facts.
func TestFourLettersOutrankFive(t *testing.T) {
	for _, kind := range Kinds {
		for _, words := range []string{"", "nova", "blue fox"} {
			for fame := 0; fame <= 5; fame++ {
				for pull := 0; pull <= 2; pull++ {
					for clean := 0; clean <= 2; clean++ {
						f := Facts{words, kind, "en", fame, pull, clean}
						four, _ := Score("nova", f)
						five, _ := Score("novas", f)
						if four < five || four < 6 {
							t.Errorf("%+v: four letters %d, five %d", f, four, five)
						}
					}
				}
			}
		}
	}
}
