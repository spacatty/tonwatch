"use client";

import { ArrowRight, Bookmark, ExternalLink, Lightbulb, Loader2, ScanSearch, Settings2, Star, X, Zap } from "lucide-react";
import { motion } from "motion/react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, fragmentUrl, getgemsUrl, studio, type Angle, type Settings, type Tracker, type Username } from "@/lib/api";
import { toast } from "sonner";
import { type Dict, locale, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { GOOD_FOR, type Filters, type SortKey, type ViewKey } from "@/lib/views";
import { Skeleton } from "@/components/ui/skeleton";
import { Countdown, Deal, discount, Score, StatusPill, Ton, type Medians } from "./bits";

interface Props {
  settings: Settings;
  onSettings: () => void;
  trackers: Tracker[];
  medians: Medians;
  categories: string[];
  onOpen: (name: string) => void;
  onWatch: (name: string) => void;
  onTracker: (t: Tracker) => void;
  onBrowse: (view: ViewKey, patch: Partial<Filters>) => void;
}

const BEST: SortKey[] = [
  { col: "score", dir: "desc" },
  { col: "buyout", dir: "asc" },
];
const bestParam = "score:desc,buyout:asc";

type Lens = "deals" | "top" | "cheap";
const LENSES: Lens[] = ["deals", "top", "cheap"];

const HIDDEN_KEY = "fragwatch.hidden.v1";

/** Cards dismissed from the home screen: hidden here, and a soft skip for the taste. */
function useHidden() {
  const [hidden, setHidden] = useState<Set<string>>(new Set());
  useEffect(() => {
    try {
      setHidden(new Set(JSON.parse(localStorage.getItem(HIDDEN_KEY) ?? "[]")));
    } catch {}
  }, []);
  const dismiss = useCallback((name: string) => {
    setHidden((h) => {
      const n = new Set(h).add(name);
      try {
        localStorage.setItem(HIDDEN_KEY, JSON.stringify([...n].slice(-300)));
      } catch {}
      return n;
    });
    // archived on the server too: it leaves every list, and the taste learns from it
    api.hide(name, true).catch(() => {});
  }, []);
  return { hidden, dismiss };
}

function useList(params: string | null) {
  const [items, setItems] = useState<Username[] | null>(null);
  const [total, setTotal] = useState(0);
  useEffect(() => {
    if (params == null) return;
    let alive = true;
    setItems(null);
    api
      .list(new URLSearchParams(params))
      .then((r) => {
        if (!alive) return;
        setItems(r.items);
        setTotal(r.total);
      })
      .catch(() => alive && setItems([]));
    return () => {
      alive = false;
    };
  }, [params]);
  return { items, total };
}

/** Home: where the opportunities are, across the market and inside each tracker. */
export function Snipes({ settings, onSettings, trackers, medians, categories, onOpen, onWatch, onTracker, onBrowse }: Props) {
  const t = useT().snipes;
  const kind = useKind();
  const [lens, setLens] = useState<Lens>("deals");
  const [theme, setTheme] = useState(settings.kinds[0] ?? "crypto");
  const { hidden, dismiss } = useHidden();
  const [angles, setAngles] = useState<Record<string, Angle>>({});
  const addAngles = useCallback((a: Record<string, Angle>) => setAngles((x) => ({ ...x, ...a })), []);
  const grid = { medians, onOpen, onWatch, hidden, onDismiss: dismiss, angles, onAngles: addAngles };

  // only offers you can take right now: live auctions, fixed prices, market listings
  const LIVE = "status=auction,sale,market&live=1";
  const minScore = settings.min_score;
  const cap = settings.max_buyout ? `&max_buyout=${settings.max_buyout}` : "";
  const pool = useList(`${LIVE}&buy_now=1&min_score=${minScore}${cap}&sort=${bestParam}&limit=600`);
  const cheap8 = useList(`${LIVE}&buy_now=1&min_score=${Math.max(8, minScore)}${cap}&sort=buyout:asc&limit=12`);
  const closing = useList(`status=auction&live=1&ending_in=3600&min_score=${minScore}&limit=1`);
  const themed = useList(`${LIVE}&category=${theme}&min_score=${minScore}${cap}&hide_junk=1&sort=${bestParam}&limit=12`);
  const budget = settings.max_buyout ? `≤ ${settings.max_buyout.toLocaleString(locale())} TON` : t.anyPrice;
  const browseBase: Partial<Filters> = { buyNow: true, minScore, unscored: false, maxBuyout: settings.max_buyout ? String(settings.max_buyout) : "" };

  const hasMedians = Object.keys(medians).length > 0;
  const deals = useMemo(() => {
    if (!pool.items || !hasMedians) return null;
    return pool.items
      .map((u) => ({ u, d: discount(u, medians) ?? 0 }))
      .filter((x) => x.d >= 0.3)
      .sort((a, b) => (b.u.score ?? 0) - (a.u.score ?? 0) || b.d - a.d)
      .slice(0, 12)
      .map((x) => x.u);
  }, [pool.items, medians, hasMedians]);

  const shown = lens === "deals" ? deals : lens === "top" ? (pool.items?.slice(0, 12) ?? null) : cheap8.items;
  const best = deals?.[0];
  const rated8 = pool.items?.filter((u) => (u.score ?? 0) >= 8).length ?? 0;

  return (
    <div className="space-y-10">
      <div className="-mt-2 flex flex-wrap items-center gap-2 text-[13px] text-muted-foreground">
        <span>
          {t.withinSettings}
          <span className="data text-foreground">{budget}</span>
          {t.rated}
          <span className="data text-foreground">{minScore}+</span>
          {settings.kinds.length > 0 && (
            <>
              {t.leaning}
              <span className="text-foreground">{settings.kinds.map(kind).join(", ")}</span>
            </>
          )}
          {t.onlyOpen}
        </span>
        <button onClick={onSettings} className="inline-flex h-7 items-center gap-1 rounded border border-input px-2 text-[12px] hover:border-primary hover:text-primary">
          <Settings2 className="size-3.5" />
          {t.change}
        </button>
      </div>

      {/* the numbers that matter, as type */}
      <section className="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-border bg-border lg:grid-cols-4">
        <Kpi label={t.kpiInstant(minScore)} value={pool.total} sub={budget} onClick={() => onBrowse("buyable", { ...browseBase, sorts: BEST })} />
        <Kpi label={t.kpiRated(Math.max(8, minScore))} value={pool.items ? rated8 : null} sub={t.kpiRatedSub} onClick={() => onBrowse("buyable", { ...browseBase, minScore: Math.max(8, minScore), sorts: BEST })} />
        <Kpi label={t.kpiClosing(minScore)} value={closing.items ? closing.total : null} sub={t.kpiClosingSub} onClick={() => onBrowse("ending", { minScore, unscored: false, endingIn: "3600" })} />
        <Kpi
          label={t.kpiBest}
          value={best ? `@${best.name}` : deals ? "—" : null}
          sub={best ? t.kpiBestSub(Math.round((discount(best, medians) ?? 0) * 100), best.buyout ?? 0) : t.kpiBestNone}
          onClick={best ? () => onOpen(best.name) : undefined}
          small
        />
      </section>

      <section>
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-xl font-semibold tracking-tight">{t.bestDeals}</h2>
            <p className="text-sm text-muted-foreground">{t.lenses[lens].sub}.</p>
          </div>
          <div className="flex gap-0.5 rounded-md bg-muted p-0.5">
            {LENSES.map((l) => (
              <button
                key={l}
                onClick={() => setLens(l)}
                className={cn("h-8 rounded-[4px] px-3 text-[13px] transition-colors", lens === l ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground")}
              >
                {t.lenses[l].label}
              </button>
            ))}
          </div>
        </div>
        <CardGrid items={shown} {...grid} empty={t.emptyDeals} />
        <button
          onClick={() => onBrowse("buyable", lens === "cheap" ? { ...browseBase, minScore: Math.max(8, minScore), sorts: [{ col: "buyout", dir: "asc" }] } : lens === "deals" ? { ...browseBase, sorts: [{ col: "value", dir: "asc" }] } : { ...browseBase, sorts: BEST })}
          className="mt-3 inline-flex items-center gap-1 text-sm text-primary hover:underline"
        >
          {t.seeAll} <ArrowRight className="size-3.5" />
        </button>
      </section>

      <section>
        <div className="mb-4">
          <h2 className="text-xl font-semibold tracking-tight">{t.fromTrackers}</h2>
          <p className="text-sm text-muted-foreground">{t.fromTrackersSub}</p>
        </div>
        {trackers.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t.noTrackers}</p>
        ) : (
          <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3">
            {trackers.filter((tr) => tr.view !== "owned").slice(0, 9).map((tr) => (
              <TrackerPanel key={tr.id} t={tr} medians={medians} onOpen={onOpen} onTracker={onTracker} />
            ))}
          </div>
        )}
      </section>

      <section>
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-xl font-semibold tracking-tight">{t.inspiration}</h2>
            <p className="text-sm text-muted-foreground">{t.inspirationSub}</p>
          </div>
          <div className="flex flex-wrap gap-1">
            {(categories.length ? categories : Object.keys(GOOD_FOR))
              .filter((c) => c !== "junk")
              .map((c) => (
                <button
                  key={c}
                  onClick={() => setTheme(c)}
                  className={cn(
                    "h-7 rounded border px-2.5 text-[13px] transition-colors first-letter:uppercase",
                    theme === c ? "border-primary bg-accent text-accent-foreground" : "border-border text-muted-foreground hover:border-input hover:text-foreground",
                  )}
                >
                  {kind(c)}
                </button>
              ))}
          </div>
        </div>
        <p className="mb-3 text-sm">
          <span className="text-muted-foreground">{t.goodFor}</span>
          {GOOD_FOR[theme] ?? t.manyProjects}.
        </p>
        <CardGrid items={themed.items} {...grid} inspiration empty={t.emptyThemed(kind(theme), minScore, budget)} />
        <button onClick={() => onBrowse("buyable", { ...browseBase, buyNow: false, categories: [theme], sorts: BEST })} className="mt-3 inline-flex items-center gap-1 text-sm text-primary hover:underline">
          {t.browseEvery(kind(theme))} <ArrowRight className="size-3.5" />
        </button>
      </section>
    </div>
  );
}

/** A kind of name (crypto, word…) as the reader says it; unknown kinds show as they are. */
function useKind() {
  const cats = useT().filterBar.categories as Record<string, string>;
  return (c: string) => cats[c] ?? c;
}

function Kpi({ label, value, sub, onClick, small }: { label: string; value: number | string | null; sub: string; onClick?: () => void; small?: boolean }) {
  return (
    <button onClick={onClick} disabled={!onClick} className="group bg-card p-5 text-left transition-colors enabled:hover:bg-row-hover">
      <div className="label">{label}</div>
      <div className={cn("data mt-2 truncate font-semibold tracking-tight", small ? "text-xl text-primary" : "text-3xl")}>
        {value == null ? <Skeleton className="h-8 w-24" /> : typeof value === "number" ? value.toLocaleString(locale()) : value}
      </div>
      <div className="mt-1 truncate text-[13px] text-muted-foreground">{sub}</div>
    </button>
  );
}

interface GridProps {
  items: Username[] | null;
  medians: Medians;
  onOpen: (n: string) => void;
  onWatch: (n: string) => void;
  hidden: Set<string>;
  onDismiss: (n: string) => void;
  angles: Record<string, Angle>;
  onAngles: (a: Record<string, Angle>) => void;
  inspiration?: boolean;
  empty: string;
}

function CardGrid({ items, medians, onOpen, onWatch, hidden, onDismiss, angles, onAngles, inspiration, empty }: GridProps) {
  const t = useT().snipes;
  const [scanning, setScanning] = useState(false);
  if (items == null)
    return (
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
        {Array.from({ length: 8 }, (_, i) => (
          <Skeleton key={i} className="h-40 rounded-lg" />
        ))}
      </div>
    );
  const shown = items.filter((u) => !hidden.has(u.name));
  if (shown.length === 0) return <p className="panel px-5 py-8 text-sm text-muted-foreground">{empty}</p>;
  const unread = shown.filter((u) => !angles[u.name]).slice(0, 12);
  // one call for the whole grid: ~10x cheaper per name than the lightbulb on each card
  const scan = async () => {
    setScanning(true);
    try {
      const r = await studio.angles(unread.map((u) => u.name));
      onAngles(r.angles);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setScanning(false);
    }
  };
  return (
    <div>
      {unread.length > 1 && (
        <div className="mb-2 flex justify-end">
          <button
            onClick={scan}
            disabled={scanning}
            title={t.scanTitle(unread.length)}
            className="inline-flex h-7 items-center gap-1.5 rounded border border-input px-2 text-[12px] text-muted-foreground hover:border-primary hover:text-primary disabled:opacity-50"
          >
            {scanning ? <Loader2 className="size-3.5 animate-spin" /> : <ScanSearch className="size-3.5" />}
            {t.whyThese(unread.length)} <span className="data text-[10px]">5 {t.cr}</span>
          </button>
        </div>
      )}
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
        {shown.map((u, i) => (
          <SnipeCard key={u.name} u={u} i={i} medians={medians} onOpen={onOpen} onWatch={onWatch} onDismiss={onDismiss} preset={angles[u.name]} onAngle={(a) => onAngles({ [u.name]: a })} inspiration={inspiration} />
        ))}
      </div>
    </div>
  );
}

/** What the card's price corner says: the price you would pay and what kind of price it is. */
function priceOf(u: Username, t: Dict["snipes"]["price"]): { label: string; v: number | null; tone: "buy" | "bid" | "none" } {
  if (u.buyout != null) return { label: t.buyNow, v: u.buyout, tone: "buy" };
  if (u.status === "auction") return { label: u.price != null ? t.currentBid : t.auction, v: u.price, tone: "bid" };
  if (u.price != null) return { label: u.status === "market" ? (u.chain_market ?? t.listed) : t.price, v: u.price, tone: "bid" };
  return { label: u.status === "owned" ? t.notListed : t.noPrice, v: null, tone: "none" };
}

function SnipeCard({ u, i, medians, onOpen, onWatch, onDismiss, preset, onAngle, inspiration }: { u: Username; i: number; medians: Medians; onOpen: (n: string) => void; onWatch: (n: string) => void; onDismiss: (n: string) => void; preset?: Angle; onAngle: (a: Angle) => void; inspiration?: boolean }) {
  const t = useT().snipes;
  const kind = useKind();
  const [watched, setWatched] = useState(u.watched);
  const angle = preset ?? null;
  const [asking, setAsking] = useState(false);
  const ask = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (angle || asking) return;
    setAsking(true);
    try {
      onAngle((await studio.angle(u.name)).angle);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setAsking(false);
    }
  };
  const pr = priceOf(u, t.price);
  const href = u.status === "market" && u.nft_friendly ? getgemsUrl(u.nft_friendly) : fragmentUrl(u.name);
  const verb = pr.tone === "buy" ? t.verb.buy : u.status === "auction" ? t.verb.bid : u.status === "owned" ? t.verb.offer : t.verb.view;
  return (
    <motion.article
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.22, delay: i * 0.025 }}
      onClick={() => onOpen(u.name)}
      className="panel group relative flex cursor-pointer flex-col p-4 transition-colors hover:border-primary/60"
    >
      <button
        onClick={(e) => {
          e.stopPropagation();
          onDismiss(u.name);
        }}
        aria-label={t.notForMe(u.name)}
        title={t.neverShow}
        className="absolute -top-2 -right-2 hidden size-6 items-center justify-center rounded-full border border-border bg-background text-muted-foreground shadow-sm group-hover:flex hover:text-foreground"
      >
        <X className="size-3" />
      </button>
      {/* name on the left, price in the corner, like a product tile */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="data truncate text-[17px] leading-6 font-semibold">@{u.name}</div>
          <div className="mt-0.5 line-clamp-1 text-[13px] text-muted-foreground">{u.meaning || (u.category ? <span className="inline-block first-letter:uppercase">{kind(u.category)}</span> : " ")}</div>
        </div>
        <div className="shrink-0 text-right">
          <div className="label">{pr.label}</div>
          {pr.v != null ? (
            <div className={cn("mt-0.5 text-[19px] leading-6 font-semibold", pr.tone === "buy" ? "text-positive" : "text-foreground")}>
              <Ton v={pr.v} />
            </div>
          ) : (
            <div className="mt-0.5 text-[15px] leading-6 text-muted-foreground">—</div>
          )}
          <div className="mt-1 flex h-5 justify-end">
            <Deal u={u} medians={medians} />
          </div>
        </div>
      </div>

      <div className="mt-3 flex items-center justify-between gap-2">
        <Score u={u} />
        {u.category && u.meaning && <span className="rounded-sm border border-border px-1.5 text-[11px] text-muted-foreground">{kind(u.category)}</span>}
      </div>
      {inspiration && u.category && GOOD_FOR[u.category] && !angle && <p className="mt-2 text-[13px] leading-snug text-muted-foreground">{t.couldSuit(GOOD_FOR[u.category])}</p>}
      {angle && (
        <div className="mt-2.5 rounded-md border border-primary/30 bg-accent/50 px-2.5 py-2 text-[13px] leading-snug">
          <div className="flex items-start gap-1.5">
            <Lightbulb className="mt-0.5 size-3.5 shrink-0 text-primary" />
            <span>{angle.angle}</span>
          </div>
          <p className="mt-1 pl-5 text-muted-foreground">{angle.project}</p>
          <p className="data mt-1 pl-5 text-[11px] text-muted-foreground">{t.fit(angle.fit)}</p>
        </div>
      )}

      {/* action on the left, timing on the right */}
      <div className="min-h-4 flex-1" />
      <div className="flex items-center gap-2 border-t border-border/70 pt-3">
        <a
          href={href}
          target="_blank"
          rel="noreferrer"
          onClick={(e) => {
            e.stopPropagation();
            if (pr.tone === "buy") studio.signal(u.name, "buy");
          }}
          className={cn(
            "inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md px-3 text-[13px] font-semibold transition-colors",
            pr.tone === "buy" ? "bg-positive text-background hover:bg-positive/85" : "border border-input hover:border-primary hover:text-primary",
          )}
        >
          {pr.tone === "buy" && <Zap className="size-3.5 fill-current" />}
          {verb}
          <ExternalLink className="size-3 opacity-70" />
        </a>
        {!angle && (
          <button onClick={ask} disabled={asking} aria-label={t.whyAria(u.name)} title={t.whyTitle} className="inline-flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground/60 hover:bg-accent hover:text-primary">
            {asking ? <Loader2 className="size-4 animate-spin" /> : <Lightbulb className="size-4" />}
          </button>
        )}
        <button
          onClick={(e) => {
            e.stopPropagation();
            onWatch(u.name);
            setWatched(true);
          }}
          aria-label={t.trackAria(u.name)}
          className="inline-flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground/60 hover:bg-muted hover:text-warning"
        >
          <Star className={cn("size-4", watched && "fill-warning text-warning")} />
        </button>
        <div className="ml-auto min-w-0 text-right text-[12px] text-muted-foreground">
          {u.ends_at ? (
            <span>
              {u.ends_at * 1000 > Date.now() && t.leftBefore}
              <Countdown ts={u.ends_at} />
              {u.ends_at * 1000 > Date.now() && t.leftAfter}
            </span>
          ) : (
            <StatusPill u={u} className="text-[12px] text-muted-foreground" />
          )}
        </div>
      </div>
    </motion.article>
  );
}

function TrackerPanel({ t, medians, onOpen, onTracker }: { t: Tracker; medians: Medians; onOpen: (n: string) => void; onTracker: (t: Tracker) => void }) {
  const tt = useT().snipes;
  const params = useMemo(() => {
    const p = new URLSearchParams(t.params);
    p.set("status", "auction,sale,market");
    p.set("live", "1");
    p.set("sort", bestParam);
    p.set("limit", "4");
    return p.toString();
  }, [t.params]);
  const { items } = useList(params);
  return (
    <div className="panel flex flex-col">
      <button onClick={() => onTracker(t)} className="flex items-center gap-2 border-b border-border px-4 py-3 text-left hover:bg-row-hover">
        <Bookmark className="size-4 shrink-0 text-primary" />
        <span className="min-w-0 flex-1 truncate font-medium">{t.name}</span>
        {t.new > 0 && <span className="data rounded-sm bg-primary px-1.5 text-[11px] font-semibold text-primary-foreground">{tt.newCount(t.new)}</span>}
        <span className="data text-[13px] text-muted-foreground">{t.total.toLocaleString(locale())}</span>
        <ArrowRight className="size-3.5 text-muted-foreground" />
      </button>
      <ul className="flex-1">
        {items == null &&
          Array.from({ length: 3 }, (_, i) => (
            <li key={i} className="px-4 py-2.5">
              <Skeleton className="h-4 w-full" />
            </li>
          ))}
        {items?.length === 0 && <li className="px-4 py-4 text-[13px] text-muted-foreground">{tt.noMatches}</li>}
        {items?.map((u) => (
          <li key={u.name}>
            <button onClick={() => onOpen(u.name)} className="flex w-full items-center gap-3 px-4 py-2 text-left hover:bg-row-hover">
              <span className="data min-w-0 flex-1 truncate text-[14px]">@{u.name}</span>
              <Score u={u} className="shrink-0" />
              <span className="w-24 shrink-0 text-right">
                {u.buyout != null ? (
                  <span className="inline-flex items-center gap-1 text-[13px] font-semibold text-positive">
                    <Ton v={u.buyout} unit={false} />
                    <Deal u={u} medians={medians} className="ml-0.5" />
                  </span>
                ) : u.price != null ? (
                  <Ton v={u.price} unit={false} className="text-[13px] text-muted-foreground" />
                ) : (
                  <span className="text-[12px] text-muted-foreground">{tt.offer}</span>
                )}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
