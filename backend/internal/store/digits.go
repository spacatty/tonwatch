package store

import (
	"sort"
	"strings"
)

// digitCond builds the WHERE clause for a digit-combination search. Underscores are ignored,
// so "5_52" and "55_2" count as containing 552.
//
//	row:       the digits appear together in this order ("552")
//	any:       together, in any order ("552", "525", "255")
//	scattered: each digit appears anywhere, as many times as given (two 5s and a 2)
func digitCond(digits, mode string) (string, []any) {
	if digits == "" || len(digits) > 8 || strings.Trim(digits, "0123456789") != "" {
		return "", nil
	}
	switch mode {
	case "any":
		if len(digits) > 6 { // 7! permutations is too many LIKEs; "anywhere" is close enough
			return digitCond(digits, "scattered")
		}
		perms := permutations(digits)
		conds := make([]string, len(perms))
		args := make([]any, len(perms))
		for i, p := range perms {
			conds[i] = "replace(name, '_', '') LIKE ?"
			args[i] = "%" + p + "%"
		}
		return "(" + strings.Join(conds, " OR ") + ")", args
	case "scattered":
		counts := map[rune]int{}
		for _, r := range digits {
			counts[r]++
		}
		var conds []string
		var args []any
		for r, n := range counts {
			conds = append(conds, "(length(name) - length(replace(name, ?, ''))) >= ?")
			args = append(args, string(r), n)
		}
		return "(" + strings.Join(conds, " AND ") + ")", args
	default:
		return "replace(name, '_', '') LIKE ?", []any{"%" + digits + "%"}
	}
}

// permutations returns the distinct orderings of s, sorted.
func permutations(s string) []string {
	seen := map[string]bool{}
	var rec func(prefix string, rest []byte)
	rec = func(prefix string, rest []byte) {
		if len(rest) == 0 {
			seen[prefix] = true
			return
		}
		for i := range rest {
			next := append(append([]byte{}, rest[:i]...), rest[i+1:]...)
			rec(prefix+string(rest[i]), next)
		}
	}
	rec("", []byte(s))
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
