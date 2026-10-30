import { enPlural } from "../index";

// The live feed of names seen for the first time (Fresh.tsx).
const en = {
  live: "Live",
  paused: "Paused",
  pause: "Pause",
  resume: "Resume",
  checked: (ago: string) => `checked ${ago} ago`,
  arrived: (n: number) => `${n} new ${enPlural(n, "name", "names")} since you opened the page`,
  filters: "Filters",
  reset: "Reset",
  venue: "Where",
  venues: { auction: "Fragment auction", sale: "Fragment sale", getgems: "Getgems", marketapp: "Marketapp", market: "Other markets", owned: "Not on sale" },
  kind: "Kind",
  anyKind: "Any kind",
  length: "Length",
  price: "Price, TON",
  buyout: "Buyout up to, TON",
  from: "from",
  to: "to",
  score: "AI score",
  anyScore: "Any",
  unscored: "Include not yet rated",
  buyNow: "Only buyable now",
  buyNowHint: "Names with a price you can pay this minute: a listing, or an auction's buy-now",
  hideJunk: "Hide junk",
  cols: { age: "Seen", name: "Name", kind: "Kind", len: "Len", score: "Score", venue: "Where", price: "Price" },
  buyNowAt: (v: string) => `buy now ${v}`,
  watch: (name: string) => `Track @${name}`,
  watching: "Tracked",
  empty: "Nothing new matches these filters yet. New names show up here the moment the collectors find them.",
  loadFailed: (why: string) => `Could not load the feed: ${why}`,
  age: { s: (n: number) => `${n}s`, m: (n: number) => `${n}m`, h: (n: number) => `${n}h`, d: (n: number) => `${n}d` },
  cap: (n: number) => `The feed keeps the newest ${n} names.`,
};

export default en;
