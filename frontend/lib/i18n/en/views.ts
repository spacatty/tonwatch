// Built-in views, statuses, sort columns, digit modes, starter trackers (lib/views.ts).
const en = {
  view: {
    ending: { label: "Closing soon", blurb: "Fragment auctions, soonest first" },
    auction: { label: "Auctions", blurb: "Every Fragment auction we found" },
    sale: { label: "For sale", blurb: "Fixed price on Fragment, buy instantly" },
    market: { label: "Other markets", blurb: "Listed on Getgems and other TON markets" },
    buyable: { label: "All listings", blurb: "Everything you can buy or bid on right now" },
    owned: { label: "Make an offer", blurb: "Owned and not listed: offer the owner on Fragment" },
    watch: { label: "Favorites", blurb: "Names you starred" },
    all: { label: "Everything", blurb: "Every username in the database" },
    hidden: { label: "Hidden", blurb: "Names you archived; they stay out of every other list" },
  },
  status: {
    auction: "Auction",
    sale: "For sale",
    market: "Other market",
    owned: "Owned",
  },
  sort: {
    ending: "Ends",
    score: "AI score",
    price: "Price",
    buyout: "Buyout",
    value: "Best value",
    len: "Length",
    name: "Name",
    status: "Status",
    recent: "Recently found",
  },
  /** hints follow "Names with …" */
  digits: {
    row: { label: "Exactly", hint: (d: string, split: string) => `${d} in them; underscores are ignored, so ${split} counts too` },
    any: { label: "Any order", hint: (examples: string) => `these digits together in any order: ${examples}` },
    scattered: { label: "Spread out", hint: (d: string) => `the digits of ${d} anywhere, with other characters between them` },
  },
  presets: {
    instant: "Instant buy under 50 TON",
    closing: "Rated 8+, closing in 1 hour",
    short: "Short real words",
    crypto: "Crypto names for sale",
  },
  /** what a kind of name suits; follows "Could suit …" */
  goodFor: {
    crypto: "a crypto, DeFi or Web3 project",
    brand: "a startup or product brand",
    word: "a media channel or one-word brand",
    name: "a personal brand or creator",
    phrase: "a community, channel or campaign",
    short: "a premium short handle",
    geo: "a local business or city guide",
    slang: "a meme page or community",
    foreign: "a brand in that language",
    number: "a lucky-number or VIP handle",
    pattern: "a collector's handle or an easy-to-remember brand",
  },
};

export default en;
