package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// The AI filter: a request in plain words ("auctions nobody bid on for 5 days, under 30 TON,
// pretty or crypto") becomes the same rules the table's filter chips set. The model only picks
// values; the app checks every one before it is used, so a wrong guess can narrow a list but
// never break a query.
const filterPrompt = `You turn a request about Telegram usernames for sale into search rules for a database.
Every username has: status (auction | sale = fixed price on Fragment | market = listed on a TON marketplace | owned = not for sale),
an AI beauty score 0-10, a kind (word, name, brand, phrase, crypto, number, short, geo, slang, foreign, pattern = repeated letters like aaaa or abab, junk),
a length, a price in TON, a buyout (what buying it now costs) and an auction end time.

Return ONLY JSON with the keys that the request implies; leave out everything else:
{
 "view": "buyable" (anything you can buy now; the default) | "auction" | "ending" (auctions, soonest first) | "sale" | "market" | "owned" | "all",
 "min_score": 0-10 (beautiful/pretty/good ≈ 7, great/premium ≈ 8, any ≈ 0),
 "kinds": ["crypto", ...] (only kinds from the list; "or" between kinds means several),
 "not_kinds": [...],
 "max_buyout": TON (a budget: "under 30", "cheaper than 30", "≤ 30"),
 "min_buyout": TON,
 "max_price": TON (current bid/listed price, when they talk about the bid rather than the total cost),
 "min_len": 4-32, "max_len": 4-32 (short ≈ max 6),
 "no_digits": true, "no_underscores": true,
 "buy_now": true (only names purchasable instantly, no bidding),
 "ending_in": seconds (auctions ending within: 15 min=900, 1 hour=3600, 6 hours=21600, today/24h=86400),
 "quiet_days": days (no bids / no price change / nobody touched it for N days),
 "keywords": ["word stems the name or its meaning should contain", ...] (for themes: "looks like a bank" → bank, finance, pay...),
 "query": "a literal piece of the name" (only when they quote letters: starts with ^abc, ends with abc$, contains abc),
 "sort": "score" | "buyout" | "price" | "ending" | "len" | "recent" | "value" (value = cheap for its quality),
 "rules": a condition tree (see below), for anything the flat keys cannot say,
 "summary": "the rules restated in ≤ 12 words, in the user's language",
 "could_not": "the parts of the request no rule here can express, in ≤ 15 words, in the user's language; leave out when everything fits"
}

The flat keys above are all ANDed together. Whenever the request needs OR, NOT, nesting, or a comparison relative to
something else, put THOSE conditions in "rules" instead (and do not repeat them as flat keys). A rule is either
 a group: {"op":"and"|"or", "not":true?, "rules":[...]}  or
 a leaf:  {"field":..., "cmp":..., "value":..., "not":true?}
Leaf fields and comparisons:
 numbers (cmp lt|lte|gt|gte|eq|ne, or "between" with [a,b]):
   score, buyout (TON to own it now), price (current bid or listed price), len, last_sale (TON it last sold for),
   value_ratio (its price ÷ the typical price for names with the same score: 0.5 = half the usual, "cheap for its quality"),
   vs_last_sale (buyout ÷ last sale: < 1 = cheaper than it last sold), age_days (days since we first saw it: "new" ≈ lte 2),
   ends_in_hours (auctions only), quiet_days (days since its price last moved: "no bids for 5 days" = gte 5)
 sets (cmp "in" | "nin", value a list): status [auction, sale, market, owned, available], kind [word, name, brand,
   phrase, crypto, number, short, geo, slang, foreign, pattern, junk], market [getgems, marketapp, ...]
 text: name (cmp contains|starts|ends|eq, value a string), meaning (cmp contains), keywords (cmp "any", value a list of stems)
 yes/no (cmp "is", value true|false): has_digits, has_underscore, junk, buy_now, rated, watched
Example: "beautiful or crypto, no bids for 5 days, under 30, but nothing with digits" →
 {"max_buyout":30,"rules":{"op":"and","rules":[
   {"op":"or","rules":[{"field":"score","cmp":"gte","value":7},{"field":"kind","cmp":"in","value":["crypto"]}]},
   {"field":"quiet_days","cmp":"gte","value":5},
   {"field":"has_digits","cmp":"is","value":false}]}}
Example: "аукционы до 50 TON, красивые и короткие, без цифр" →
 {"view":"auction","max_buyout":50,"min_score":7,"max_len":6,"no_digits":true,"summary":"Красивые короткие аукционы до 50 TON без цифр"}
Example: "что-нибудь про кофе дешевле обычного, чтобы понравилось инвесторам" →
 {"keywords":["coffee","cafe","espresso","latte","brew","bean","kofe"],"sort":"value",
  "summary":"Имена про кофе, дешёвые для своего качества","could_not":"«Понравится инвесторам» — такого правила нет"}`

// FilterSpec is what the model understood. The app validates it before turning it into a filter.
type FilterSpec struct {
	View      string          `json:"view"`
	MinScore  *int            `json:"min_score"`
	Kinds     []string        `json:"kinds"`
	NotKinds  []string        `json:"not_kinds"`
	MaxBuyout *float64        `json:"max_buyout"`
	MinBuyout *float64        `json:"min_buyout"`
	MaxPrice  *float64        `json:"max_price"`
	MinLen    *int            `json:"min_len"`
	MaxLen    *int            `json:"max_len"`
	NoDigits  bool            `json:"no_digits"`
	NoUnder   bool            `json:"no_underscores"`
	BuyNow    bool            `json:"buy_now"`
	EndingIn  *int64          `json:"ending_in"`
	QuietDays *int            `json:"quiet_days"`
	Keywords  []string        `json:"keywords"`
	Query     string          `json:"query"`
	Sort      string          `json:"sort"`
	Rules     json.RawMessage `json:"rules"`
	Summary   string          `json:"summary"`
	CouldNot  string          `json:"could_not"` // what the rules leave out, said back to the user
}

// FilterPromptVersion changes whenever the filter prompt does, so cached answers to the old one
// are never served.
func FilterPromptVersion() string {
	h := sha256.Sum256([]byte(filterPrompt + ruRule))
	return hex.EncodeToString(h[:6])
}

// ParseFilter reads a request in plain words into search rules.
func (c *Client) ParseFilter(ctx context.Context, request string) (FilterSpec, Usage, error) {
	var s FilterSpec
	content, usage, err := c.Chat(ctx, InLang(ctx, filterPrompt), request, 0.1, 1500)
	if err != nil {
		return s, usage, err
	}
	if err := json.Unmarshal([]byte(content), &s); err != nil {
		return s, usage, fmt.Errorf("bad model JSON: %w", err)
	}
	s.Keywords = cleanList(s.Keywords, stemRe, 20)
	s.Summary = strings.TrimSpace(s.Summary)
	s.CouldNot = strings.TrimSpace(s.CouldNot)
	return s, usage, nil
}
