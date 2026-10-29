package llm

import "fragwatch/internal/rating"

// Categories the dashboard can filter by: the rating kinds, "pattern" included.
var Categories = rating.Kinds

// FactsPrompt asks the model for facts about each name, not for a score: rating.Score turns the
// facts into the 0-10 rating, so how names are judged can change without asking the model again.
// After editing this prompt, POST /api/llm/reset to record facts again (scores stay meanwhile).
const FactsPrompt = `You record facts about Telegram usernames (sold as NFTs on Fragment). You do not score them.
Usernames contain only a-z, 0-9 and "_" and are 4-32 characters long. Read each one the way people would:
split joined words ("darkmoon" = "dark moon"), recognise words in any major language written in Latin letters,
first names, places, brands, abbreviations and leetspeak ("h4ck3r" = "hacker").
Names may be Chinese pinyin; read them as pinyin when that is the best reading (xinqun = "xin qun", new group).
Record every name you are given, whatever it means.

For each name give:
- words: the reading, words separated by single spaces ("goodnews" -> "good news", "pizza" -> "pizza"); "" when it reads as nothing.
- kind, exactly one of:
  word (a dictionary word in any language) · name (a person's first or last name, or nickname) · brand (an invented
  brandable word) · phrase (two or more words) · crypto (crypto, web3 and finance terms) · number (mostly digits) ·
  short (a letter combination or abbreviation of 5 characters or fewer) · geo (a city, country or place) ·
  slang (internet slang, memes, leetspeak) · foreign (meaningful in a language other than English) · junk (reads as nothing)
- lang: two-letter code of the reading's language ("en", "es", "ru", "zh", "de", ...); "" for junk.
- fame 0-5: how widely known the reading is, as a whole name:
  5 iconic: known in almost every country and language: love, music, pizza, taxi, paris
  4 very common: an everyday word of a major language, a top first name, a big city or country: coffee, garden, london, maria, brazil
  3 common: an ordinary dictionary word, a common name, a natural two-word compound: lantern, harbor, oliver, goodnews, darkmoon
  2 recognisable but niche: a rarer or technical word, a word from a smaller language, a clear but unusual compound: nimbus, quartz, fjord, bluefox
  1 weak: a pronounceable invented word, an obscure word, a name with a suffix, an awkward compound: zorvia, lunexo, petrikshop
  0 nothing: random letters, keyboard mash, digit noise, a long chain of unrelated syllables
  A first name followed by a birth year or random digits is a personal handle: fame 1 at most. Long phrases and
  sentences are less known as a whole than their words: rate the whole name.
- pull 0-2: how much a business, product or community would want it as its handle:
  2 a whole industry or a big business would: travel, bank, coffee, crypto, taxi
  1 some project, shop, community or person would: a hobby, a niche, a pleasant name: sunrise, oliver, nimbus
  0 nobody in particular: function words, personal handles, gibberish: then, maybe, alex1987
- clean 0-2: how naturally it is written:
  2 the natural spelling
  1 a small blemish: one underscore between words, a common variant spelling, a digit that belongs (web3)
  0 awkward: random digits, several underscores, leetspeak, misspelling, letters doubled for no reason
- gloss: only when fame >= 2, a note of at most 10 words that tells a buyer something the name alone does not:
  what it refers to, the language it comes from, who would use it. Never just split the name into words.
  Good: casa -> "Spanish: house; real estate, home goods", nimbus -> "rain cloud; weather or cloud-storage brand",
  harbor -> "sheltered port; shipping, logistics, cafes". Bad: "good news", "dark moon", "blue fox".
  For fame 0-1 use "".

Output ONLY JSON in exactly this form, one row per input name, same spelling, no extra keys:
{"r":[["name","words","kind","lang",fame,pull,clean,"gloss"], ...]}`
