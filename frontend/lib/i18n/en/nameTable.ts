import { enPlural } from "../index";

// The names table (NameTable.tsx).
const en = {
  /** the word after the count; the count itself is shown apart */
  names: (n: number) => enPlural(n, "name", "names"),
  /** the order when no column is picked: "by ai score" */
  byDefault: (col: string) => `by ${col.toLowerCase()}`,
  by: "by",
  flip: "Flip direction",
  stopSorting: (col: string) => `Stop sorting by ${col}`,
  bestValueHint: "Cheapest for its AI score: price compared with what names rated the same usually go for",
  bestValue: "Best value",
  colUsername: "Username",
  colScore: "Score",
  colEnds: "Ends",
  colPrice: "Buyout",
  colStatus: "Status",
  bid: (price: string) => `bid ${price} TON`,
  bidUsd: (usd: string) => `bid ≈ $${usd}`,
  lastSale: "last sale",
  verb: { buy: "Buy", offer: "Offer", buyNow: "Buy now", bid: "Bid", open: "Open" },
  /** the price button's tooltip: "Buy on Getgems" */
  on: (verb: string, market: string) => `${verb} on ${market}`,
  theMarketplace: "the marketplace",
  tracked: "Tracked",
  looksJunk: "looks like junk",
  notRated: "not rated yet",
  scoreOf: (score: number) => `AI score ${score} of 10`,
  track: "Track",
  stopTracking: "Stop tracking",
  likeHint: "Like: shapes your taste",
  like: "Like",
  imagineHint: "Imagine a project for it · 2 credits",
  imagine: "Imagine a project",
  bringBack: "Bring it back",
  neverShow: "Never show again",
  under: (pct: number, score: number | null) => `${pct}% under what names rated ${score} typically sell for`,
  sold: (price: string) => `sold ${price}`,
  more: "More",
  recheck: "Re-check on Fragment",
  copied: (name: string) => `@${name} copied`,
  copy: (name: string) => `Copy @${name}`,
  inPortfolio: (name: string) => `@${name} is in your portfolio`,
  iOwnIt: "I own it",
  openGetgems: "Open on Getgems",
  sortNext: { ascending: "Sort ascending", descending: "Sort descending", off: "Sort off" },
  showOnly: "Show only",
  showAllStatuses: "Show all statuses",
};

export default en;
