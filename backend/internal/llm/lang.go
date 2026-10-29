package llm

import (
	"context"

	"fragwatch/internal/i18n"
)

const ruRule = `

Language: write every human-readable text value (pitches, angles, reasons, summaries, replies) in natural Russian,
the way a careful native editor would: plain words, correct grammar, no marketing filler, no calques from English.
Keep usernames, project brand names, JSON keys, keywords, numbers and enum values exactly as specified, in Latin letters.`

// InLang adds to a system prompt which language the reader-facing text must be in, taken from the
// request (i18n.AI: the account's AI language, else the interface's). English needs nothing added.
// Ratings and meanings are shared by every reader, so the rating job never calls this and stays
// English.
func InLang(ctx context.Context, system string) string {
	if i18n.AI(ctx) == i18n.RU {
		return system + ruRule
	}
	return system
}
