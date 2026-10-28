package store

import "strings"

// IsJunk is a cheap, conservative prefilter so obvious garbage never reaches the LLM.
// False negatives are fine (the LLM catches them); false positives hide good names, so stay strict.
func IsJunk(name string) bool {
	n := len(name)
	if n == 0 || n > 24 {
		return true
	}
	var digits, letters, unders, vowels, run, maxRun int
	var prev rune
	for _, r := range name {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '_':
			unders++
		default:
			letters++
			if strings.ContainsRune("aeiouy", r) {
				vowels++
			}
		}
		if r == prev {
			run++
		} else {
			run = 1
		}
		prev = r
		maxRun = max(maxRun, run)
	}
	switch {
	case maxRun >= 5 && n > maxRun+1: // "aaattttttt", "c8cccccccc" (but keep pure "aaaaa")
		return true
	case unders >= 3:
		return true
	case digits >= 4 && letters >= 1: // "vip8989d", "jinsanjiao1388"
		return true
	case letters >= 6 && vowels == 0:
		return true
	case n >= 18 && unders == 0 && digits > 0:
		return true
	}
	return false
}
