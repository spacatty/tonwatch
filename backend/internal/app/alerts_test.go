package app

import (
	"strings"
	"testing"

	"fragwatch/internal/i18n"
	"fragwatch/internal/store"
)

func TestNameCard(t *testing.T) {
	price, score, meaning, market := 19.5, 7, "Wizard of Oz <character>", "Getgems"
	u := store.Username{Name: "goodwitch", Status: "market", Buyout: &price, Score: &score, Meaning: &meaning, ChainMarket: &market}

	card := nameCard(i18n.EN, u, "⏳ closes in <b>5 min</b>")
	for _, want := range []string{
		`<blockquote><b><a href="https://fragment.com/username/goodwitch">@goodwitch</a></b>`,
		"💎 <b>19.5 TON</b>", "★★★★☆ 7/10", "🛒 Getgems",
		"<i>Wizard of Oz &lt;character&gt;</i>", // the AI's words are escaped
		"⏳ closes in <b>5 min</b></blockquote>",
	} {
		if !strings.Contains(card, want) {
			t.Errorf("card lacks %q:\n%s", want, card)
		}
	}
	if ru := nameCard(i18n.RU, u, ""); !strings.Contains(ru, "19,5 TON") {
		t.Errorf("Russian prices use a comma: %s", ru)
	}

	msg := alertMessage("🎯 <b>title</b>", "<i>sub</i>", []string{card, card}, "<i>+3 more</i>")
	if strings.Count(msg, "<blockquote>") != 2 || strings.Count(msg, "</blockquote>") != 2 {
		t.Fatalf("one quote per card:\n%s", msg)
	}
	if !strings.HasPrefix(msg, "🎯 <b>title</b>\n<i>sub</i>\n\n<blockquote>") {
		t.Fatalf("title, subtitle, a blank line, then the cards:\n%s", msg)
	}

	for v, want := range map[float64]string{19.5: "19.5", 10000: "10,000", 1234567.25: "1,234,567.25", 999: "999"} {
		if got := ton(i18n.EN, v); got != want {
			t.Errorf("ton(en, %v) = %q, want %q", v, got, want)
		}
	}
	if got := ton(i18n.RU, 10000.5); got != "10 000,5" {
		t.Errorf("ton(ru, 10000.5) = %q", got)
	}

	for s, want := range map[int]string{10: "★★★★★ 10/10", 6: "★★★☆☆ 6/10", 1: "★☆☆☆☆ 1/10", 0: "☆☆☆☆☆ 0/10"} {
		if got := scoreBar(s); got != want {
			t.Errorf("scoreBar(%d) = %q, want %q", s, got, want)
		}
	}
}
