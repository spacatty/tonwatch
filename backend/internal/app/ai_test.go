package app

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSuggestReqClean(t *testing.T) {
	b := suggestReq{Brief: strings.Repeat("кофе ", 400), ExcludeNames: make([]string, 500)}
	b.clean()
	if len(b.Brief) > 1000 || !utf8.ValidString(b.Brief) {
		t.Errorf("brief: %d bytes, valid %v", len(b.Brief), utf8.ValidString(b.Brief))
	}
	if len(b.ExcludeNames) != 200 {
		t.Errorf("exclude: %d", len(b.ExcludeNames))
	}
}
