// Package i18n holds the languages the product speaks (English and Russian) and the bot's words.
// The web app has its own dictionaries; this side covers what the server writes itself: bot
// messages, alerts, Stars invoices, and which language the AI answers in.
package i18n

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	EN = "en"
	RU = "ru"
)

// Langs are the languages with a full set of words, default first.
var Langs = []string{EN, RU}

// Norm turns a Telegram language_code or an X-Lang header into a supported language, or "".
func Norm(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	switch {
	case c == RU || strings.HasPrefix(c, "ru-"):
		return RU
	case c == EN || strings.HasPrefix(c, "en-"):
		return EN
	}
	return ""
}

// Or is Norm with English as the fallback.
func Or(code string) string {
	if l := Norm(code); l != "" {
		return l
	}
	return EN
}

type ctxKey struct{}

// With marks a request with the reader's language (the AI writes its prose in it).
func With(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, ctxKey{}, lang)
}

// From is the request's language; English when none was set.
func From(ctx context.Context) string {
	l, _ := ctx.Value(ctxKey{}).(string)
	return Or(l)
}

// T is the bot text for key in lang, formatted with args. A key missing in Russian falls back to
// English; a key missing everywhere comes back as itself, so a typo shows up instead of silence.
func T(lang, key string, args ...any) string {
	s, ok := texts[lang][key]
	if !ok {
		if s, ok = texts[EN][key]; !ok {
			return key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Plural picks the word form for n: English one/other, Russian one/few/many (1 имя, 2 имени, 5 имён).
func Plural(lang string, n int, forms ...string) string {
	if lang == RU && len(forms) == 3 {
		a := n % 100
		if a < 0 {
			a = -a
		}
		b := a % 10
		switch {
		case a > 10 && a < 20:
			return forms[2]
		case b == 1:
			return forms[0]
		case b >= 2 && b <= 4:
			return forms[1]
		}
		return forms[2]
	}
	if n == 1 {
		return forms[0]
	}
	return forms[len(forms)-1]
}

// N is the word form of key for the count n: the text holds the forms split by "|"
// (English "name|names", Russian "имя|имени|имён").
func N(lang, key string, n int) string {
	return Plural(lang, n, strings.Split(T(lang, key), "|")...)
}

// Num writes a whole number the way the language does: 20,000 in English, 20 000 in Russian.
func Num(lang string, n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + Num(lang, -n)
	}
	sep := ","
	if lang == RU {
		sep = " "
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + sep + s[i:]
	}
	return s
}
