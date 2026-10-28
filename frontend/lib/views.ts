import { Clock3, EyeOff, Gavel, Tag, Store, ShoppingBag, HandCoins, Star, Layers, type LucideIcon } from "lucide-react";

export type ViewKey = "ending" | "auction" | "sale" | "market" | "buyable" | "owned" | "watch" | "all" | "hidden";

export interface View {
  key: ViewKey;
  label: string;
  icon: LucideIcon;
  status?: string;
  sort: string;
  blurb: string;
  count?: (byStatus: Record<string, number>, watched: number, hidden: number) => number | undefined;
}

export const VIEWS: View[] = [
  { key: "ending", label: "Closing soon", icon: Clock3, status: "auction", sort: "ending", blurb: "Fragment auctions, soonest first" },
  { key: "auction", label: "Auctions", icon: Gavel, status: "auction", sort: "score", blurb: "Every Fragment auction we found", count: (s) => s.auction },
  { key: "sale", label: "For sale", icon: Tag, status: "sale", sort: "score", blurb: "Fixed price on Fragment, buy instantly", count: (s) => s.sale },
  { key: "market", label: "Other markets", icon: Store, status: "market", sort: "score", blurb: "Listed on Getgems and other TON markets", count: (s) => s.market },
  { key: "buyable", label: "All listings", icon: ShoppingBag, status: "auction,sale,market", sort: "price", blurb: "Everything you can buy or bid on right now", count: (s) => (s.auction ?? 0) + (s.sale ?? 0) + (s.market ?? 0) },
  { key: "owned", label: "Make an offer", icon: HandCoins, status: "owned", sort: "score", blurb: "Owned and not listed: offer the owner on Fragment", count: (s) => s.owned },
  { key: "watch", label: "Watchlist", icon: Star, sort: "ending", blurb: "Names you track", count: (_, w) => w },
  { key: "all", label: "Everything", icon: Layers, sort: "score", blurb: "Every username in the database" },
  { key: "hidden", label: "Hidden", icon: EyeOff, sort: "updated", blurb: "Names you archived; they stay out of every other list", count: (_, __, h) => h },
];

export const STATUS_LABELS: Record<string, string> = {
  auction: "Auction",
  sale: "For sale",
  market: "Other market",
  owned: "Owned",
};

/** Statuses the table's status filter can narrow to, inside a view. */
export function viewStatuses(view: View): string[] {
  return view.status ? view.status.split(",") : Object.keys(STATUS_LABELS);
}

export type DigitMode = "row" | "any" | "scattered";
export type SortDir = "asc" | "desc";
/** One sort column; a list of them sorts by the first, then the next on ties. */
export interface SortKey {
  col: string;
  dir: SortDir;
}

export interface Filters {
  q: string;
  topic: string; // what the user typed for the AI topic filter
  keywords: string[]; // stems the AI expanded the topic into (editable)
  digits: string;
  digitMode: DigitMode;
  minPrice: string;
  maxPrice: string;
  minBuyout: string;
  maxBuyout: string;
  buyNow: boolean;
  len: [number, number];
  minScore: number;
  unscored: boolean;
  hideJunk: boolean;
  noDigits: boolean;
  noUnder: boolean;
  categories: string[];
  notCategories: string[]; // kinds to leave out
  statuses: string[]; // narrows the view's statuses; empty = all of them
  endingIn: string;
  sorts: SortKey[]; // empty = the view's default order
}

export const LEN_MIN = 4;
export const LEN_MAX = 32;

export const DEFAULT_FILTERS: Filters = {
  q: "",
  topic: "",
  keywords: [],
  digits: "",
  digitMode: "row",
  minPrice: "",
  maxPrice: "",
  minBuyout: "",
  maxBuyout: "",
  buyNow: false,
  len: [LEN_MIN, LEN_MAX],
  minScore: 5,
  unscored: true,
  hideJunk: true,
  noDigits: false,
  noUnder: false,
  categories: [],
  notCategories: [],
  statuses: [],
  endingIn: "",
  sorts: [],
};

/** Fill in defaults and convert the single sort that older saved state used. */
export function normalizeFilters(raw: Record<string, unknown> | null | undefined): Filters {
  const r = { ...(raw ?? {}) } as Record<string, unknown>;
  const f = { ...DEFAULT_FILTERS, ...r } as Filters & { sort?: string; dir?: string };
  if (!Array.isArray(r.sorts)) {
    const legacy: Record<string, SortKey> = { price_asc: { col: "price", dir: "asc" }, price_desc: { col: "price", dir: "desc" } };
    const s = typeof r.sort === "string" ? r.sort : "";
    f.sorts = s ? [legacy[s] ?? { col: s, dir: (r.dir as SortDir) || naturalDir(s) }] : [];
  }
  delete f.sort;
  delete f.dir;
  return f;
}

export const sortParam = (sorts: SortKey[]) => sorts.map((k) => `${k.col}:${k.dir}`).join(",");

/** Sortable columns and the direction a first click sorts them in. */
export const SORTS: { value: string; label: string; natural: SortDir }[] = [
  { value: "ending", label: "Ends", natural: "asc" },
  { value: "score", label: "AI score", natural: "desc" },
  { value: "price", label: "Price", natural: "asc" },
  { value: "buyout", label: "Buyout", natural: "asc" },
  { value: "value", label: "Best value", natural: "asc" },
  { value: "len", label: "Length", natural: "asc" },
  { value: "name", label: "Name", natural: "asc" },
  { value: "status", label: "Status", natural: "asc" },
  { value: "recent", label: "Recently found", natural: "desc" },
];

export const naturalDir = (sort: string): SortDir => SORTS.find((s) => s.value === sort)?.natural ?? "asc";

export const DIGIT_MODES: { v: DigitMode; label: string; hint: (d: string) => string }[] = [
  { v: "row", label: "Exactly", hint: (d) => `${d} in them; underscores are ignored, so ${d.slice(0, 1)}_${d.slice(1)} counts too` },
  { v: "any", label: "Any order", hint: (d) => `these digits together in any order: ${unique(perm(d)).slice(0, 4).join(", ")}` },
  { v: "scattered", label: "Spread out", hint: (d) => `the digits of ${d} anywhere, with other characters between them` },
];

function perm(s: string): string[] {
  if (s.length <= 1) return [s];
  return [...s].flatMap((c, i) => perm(s.slice(0, i) + s.slice(i + 1)).map((p) => c + p));
}

const unique = (xs: string[]) => [...new Set(xs)];

const isDigits = (d: string) => /^[0-9]{1,8}$/.test(d);

/** Same search, with only the user's own criteria: no view, no quality filters. */
export function searchOnlyParams(f: Filters) {
  const p = new URLSearchParams();
  if (f.q.trim()) p.set("q", f.q.trim());
  if (f.keywords.length) p.set("keywords", f.keywords.join(","));
  if (isDigits(f.digits)) {
    p.set("digits", f.digits);
    p.set("digit_mode", f.digitMode);
  }
  return p;
}

export function buildParams(view: View, f: Filters, extra: Record<string, string> = {}) {
  const p = new URLSearchParams();
  const allowed = viewStatuses(view);
  const narrowed = f.statuses.filter((s) => allowed.includes(s));
  if (narrowed.length) p.set("status", narrowed.join(","));
  else if (view.status) p.set("status", view.status);
  if (view.key === "watch") p.set("watched", "1");
  if (view.key === "hidden") p.set("hidden", "1");
  if (view.key === "ending") p.set("live", "1");
  p.set("sort", f.sorts.length ? sortParam(f.sorts) : view.sort);
  if (f.q.trim()) p.set("q", f.q.trim());
  if (f.keywords.length) p.set("keywords", f.keywords.join(","));
  // The AI scores digit-heavy names low on purpose, so a digit search skips the quality filters.
  const digitSearch = isDigits(f.digits);
  if (digitSearch) {
    p.set("digits", f.digits);
    p.set("digit_mode", f.digitMode);
  }
  if (f.maxPrice) p.set("max_price", f.maxPrice);
  if (f.minPrice) p.set("min_price", f.minPrice);
  if (f.maxBuyout) p.set("max_buyout", f.maxBuyout);
  if (f.minBuyout) p.set("min_buyout", f.minBuyout);
  if (f.buyNow) p.set("buy_now", "1");
  if (f.len[0] > LEN_MIN) p.set("min_len", String(f.len[0]));
  if (f.len[1] < LEN_MAX) p.set("max_len", String(f.len[1]));
  if (view.key !== "watch" && !digitSearch) {
    if (f.minScore > 0) p.set("min_score", String(f.minScore));
    if (f.unscored) p.set("unscored", "1");
    if (f.hideJunk) p.set("hide_junk", "1");
  }
  if (f.noDigits && !digitSearch) p.set("no_digits", "1");
  if (f.noUnder) p.set("no_under", "1");
  if (f.categories.length) p.set("category", f.categories.join(","));
  if (f.notCategories.length) p.set("not_category", f.notCategories.join(","));
  if (f.endingIn && (view.key === "ending" || view.key === "auction")) p.set("ending_in", f.endingIn);
  for (const [k, v] of Object.entries(extra)) p.set(k, v);
  return p;
}

/** How many filters differ from the defaults, for the Filters button badge. */
export function activeFilterCount(f: Filters) {
  let n = 0;
  if (f.q) n++;
  if (f.keywords.length) n++;
  if (f.digits) n++;
  if (f.minPrice || f.maxPrice) n++;
  if (f.minBuyout || f.maxBuyout || f.buyNow) n++;
  if (f.len[0] > LEN_MIN || f.len[1] < LEN_MAX) n++;
  if (f.minScore !== DEFAULT_FILTERS.minScore || !f.unscored) n++;
  if (!f.hideJunk || f.noDigits || f.noUnder) n++;
  if (f.categories.length || f.notCategories.length) n++;
  if (f.statuses.length) n++;
  if (f.endingIn) n++;
  return n;
}

/** Starter trackers, created once so the sidebar isn't empty. */
export const PRESET_TRACKERS: { name: string; view: ViewKey; filters: Partial<Filters> }[] = [
  { name: "Instant buy under 50 TON", view: "buyable", filters: { buyNow: true, maxBuyout: "50", minScore: 7, unscored: false, sorts: [{ col: "score", dir: "desc" }, { col: "buyout", dir: "asc" }] } },
  { name: "Rated 8+, closing in 1 hour", view: "ending", filters: { minScore: 8, unscored: false, endingIn: "3600" } },
  { name: "Short real words", view: "all", filters: { len: [4, 6], minScore: 7, unscored: false, categories: ["word"] } },
  { name: "Names with 552", view: "all", filters: { digits: "552", digitMode: "row", sorts: [{ col: "len", dir: "asc" }] } },
  { name: "Crypto names for sale", view: "buyable", filters: { categories: ["crypto"], minScore: 6, unscored: false, sorts: [{ col: "score", dir: "desc" }, { col: "buyout", dir: "asc" }] } },
];

/** What a kind of name suits, shown as inspiration. */
export const GOOD_FOR: Record<string, string> = {
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
};
