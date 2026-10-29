import { Clock3, EyeOff, Gavel, Tag, Store, ShoppingBag, HandCoins, Star, Layers, type LucideIcon } from "lucide-react";
import { getT } from "@/lib/i18n";
import { forQuery, type RuleNode } from "@/lib/rules";

// Labels are getters: they read the words for the current language when shown, not when this loads.

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

const views = () => getT().views.view;

export const VIEWS: View[] = ([
  { key: "ending", icon: Clock3, status: "auction", sort: "ending" },
  { key: "auction", icon: Gavel, status: "auction", sort: "score", count: (s) => s.auction },
  { key: "sale", icon: Tag, status: "sale", sort: "score", count: (s) => s.sale },
  { key: "market", icon: Store, status: "market", sort: "score", count: (s) => s.market },
  { key: "buyable", icon: ShoppingBag, status: "auction,sale,market", sort: "price", count: (s) => (s.auction ?? 0) + (s.sale ?? 0) + (s.market ?? 0) },
  { key: "owned", icon: HandCoins, status: "owned", sort: "score", count: (s) => s.owned },
  { key: "watch", icon: Star, sort: "ending", count: (_, w) => w },
  { key: "all", icon: Layers, sort: "score", count: (s) => Object.values(s).reduce((a, n) => a + n, 0) },
  { key: "hidden", icon: EyeOff, sort: "updated", count: (_, __, h) => h },
] satisfies Omit<View, "label" | "blurb">[] as Omit<View, "label" | "blurb">[]).map((v) => ({
  ...v,
  get label() {
    return views()[v.key].label;
  },
  get blurb() {
    return views()[v.key].blurb;
  },
}));

/** Status names, in the current language. */
export const STATUS_LABELS: Record<string, string> = lazyRecord(["auction", "sale", "market", "owned"], (k) => getT().views.status[k as "auction"]);

/** A record whose values are read from the dictionary each time, so they follow the language. */
function lazyRecord(keys: string[], get: (k: string) => string): Record<string, string> {
  const r: Record<string, string> = {};
  for (const k of keys) Object.defineProperty(r, k, { enumerable: true, get: () => get(k) });
  return r;
}

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
  buyableOnly: boolean; // views without a status of their own: only names on auction, for sale or listed
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
  quietDays: string; // no bids or price change for this many days ("" = off)
  rules: RuleNode | null; // AND / OR / NOT tree on top of the chips (lib/rules.ts); null = none
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
  buyableOnly: true,
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
  quietDays: "",
  rules: null,
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

/** A tracker's saved filters. Trackers saved before "buyable only" existed showed every status; they keep doing so. */
export const trackerFilters = (json: string) => normalizeFilters({ buyableOnly: false, ...JSON.parse(json) });

/** Statuses a name can be bought in right now. */
export const BUYABLE = "auction,sale,market";

/** Whether the "buyable only" switch means anything in this view: views with a status of their own already pick it. */
export const buyableApplies = (view: View) => !view.status && view.key !== "watch" && view.key !== "hidden";

export const sortParam = (sorts: SortKey[]) => sorts.map((k) => `${k.col}:${k.dir}`).join(",");

/** Sortable columns and the direction a first click sorts them in. */
export const SORTS: { value: string; label: string; natural: SortDir }[] = (
  [
    { value: "ending", natural: "asc" },
    { value: "score", natural: "desc" },
    { value: "price", natural: "asc" },
    { value: "buyout", natural: "asc" },
    { value: "value", natural: "asc" },
    { value: "len", natural: "asc" },
    { value: "name", natural: "asc" },
    { value: "status", natural: "asc" },
    { value: "recent", natural: "desc" },
  ] as const
).map((s) => ({
  ...s,
  get label() {
    return getT().views.sort[s.value];
  },
}));

export const naturalDir = (sort: string): SortDir => SORTS.find((s) => s.value === sort)?.natural ?? "asc";

export const DIGIT_MODES: { v: DigitMode; label: string; hint: (d: string) => string }[] = (["row", "any", "scattered"] as const).map((v) => ({
  v,
  get label() {
    return getT().views.digits[v].label;
  },
  hint: (d: string) => {
    const h = getT().views.digits;
    return v === "row" ? h.row.hint(d, `${d.slice(0, 1)}_${d.slice(1)}`) : v === "any" ? h.any.hint(unique(perm(d)).slice(0, 4).join(", ")) : h.scattered.hint(d);
  },
}));

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
  else if (f.buyableOnly && buyableApplies(view)) p.set("status", BUYABLE);
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
  if (Number(f.quietDays) > 0) p.set("quiet_days", f.quietDays);
  const rules = forQuery(f.rules);
  if (rules) p.set("rules", JSON.stringify(rules));
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
  if (f.quietDays) n++;
  if (forQuery(f.rules)) n++;
  return n;
}

/** Starter trackers, created once so the sidebar isn't empty. */
type Preset = { id: "instant" | "closing" | "short" | "crypto"; view: ViewKey; filters: Partial<Filters> };

export const PRESET_TRACKERS: { name: string; view: ViewKey; filters: Partial<Filters> }[] = (
  [
    { id: "instant", view: "buyable", filters: { buyNow: true, maxBuyout: "50", minScore: 7, unscored: false, sorts: [{ col: "score", dir: "desc" }, { col: "buyout", dir: "asc" }] } },
    { id: "closing", view: "ending", filters: { minScore: 8, unscored: false, endingIn: "3600" } },
    { id: "short", view: "all", filters: { len: [4, 6], minScore: 7, unscored: false, categories: ["word"] } },
    { id: "crypto", view: "buyable", filters: { categories: ["crypto"], minScore: 6, unscored: false, sorts: [{ col: "score", dir: "desc" }, { col: "buyout", dir: "asc" }] } },
  ] satisfies Preset[] as Preset[]
).map(({ id, view, filters }) => ({
  view,
  filters,
  get name() {
    return getT().views.presets[id];
  },
}));

/** What a kind of name suits, shown as inspiration. */
export const GOOD_FOR: Record<string, string> = lazyRecord(
  ["crypto", "brand", "word", "name", "phrase", "short", "geo", "slang", "foreign", "number", "pattern"],
  (k) => getT().views.goodFor[k as "crypto"],
);
