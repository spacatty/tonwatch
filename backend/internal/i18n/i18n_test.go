package i18n

import (
	"regexp"
	"strings"
	"testing"
)

var verb = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)

// Every English text has a Russian twin taking the same arguments, and the reverse.
func TestCatalogsMatch(t *testing.T) {
	for k, en := range texts[EN] {
		ru, ok := texts[RU][k]
		if !ok {
			t.Errorf("%s: missing in ru", k)
			continue
		}
		if a, b := len(verb.FindAllString(en, -1)), len(verb.FindAllString(ru, -1)); a != b {
			t.Errorf("%s: en has %d verbs, ru has %d", k, a, b)
		}
		if strings.HasPrefix(k, "w_") && strings.Count(ru, "|") != 2 {
			t.Errorf("%s: ru needs three forms", k)
		}
	}
	for k := range texts[RU] {
		if _, ok := texts[EN][k]; !ok {
			t.Errorf("%s: missing in en", k)
		}
	}
	if n := len([]rune(texts[RU]["about"])); n > 120 {
		t.Errorf("ru about is %d chars, Telegram allows 120", n)
	}
	if n := len([]rune(texts[EN]["about"])); n > 120 {
		t.Errorf("en about is %d chars, Telegram allows 120", n)
	}
	for _, l := range Langs {
		if n := len([]rune(texts[l]["description"])); n > 512 {
			t.Errorf("%s description is %d chars, Telegram allows 512", l, n)
		}
	}
}

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{1: "день", 2: "дня", 4: "дня", 5: "дней", 11: "дней", 14: "дней", 21: "день", 22: "дня", 30: "дней", 101: "день", 111: "дней"} {
		if got := N(RU, "w_days", n); got != want {
			t.Errorf("ru %d: %s, want %s", n, got, want)
		}
	}
	if got := N(EN, "w_days", 1) + " " + N(EN, "w_days", 30); got != "day days" {
		t.Errorf("en: %s", got)
	}
}

func TestNorm(t *testing.T) {
	for in, want := range map[string]string{"ru": RU, "ru-RU": RU, "RU": RU, "en-US": EN, "uk": "", "": ""} {
		if got := Norm(in); got != want {
			t.Errorf("Norm(%q) = %q, want %q", in, got, want)
		}
	}
	if Num(EN, 20000) != "20,000" || Num(RU, 1000) != "1 000" || Num(RU, 999) != "999" {
		t.Errorf("Num: %q %q", Num(EN, 20000), Num(RU, 1000))
	}
}
