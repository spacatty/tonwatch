"use client";

import { Pause, Play, RotateCcw, Star } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, type Username } from "@/lib/api";
import { getT, num, useT } from "@/lib/i18n";
import { LEN_MAX, LEN_MIN } from "@/lib/views";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { useAuth } from "./Auth";
import { Score, StatusPill, Ton, useNow } from "./bits";

interface Props {
  categories: string[];
  onOpen: (name: string) => void;
  onWatch: (name: string) => void;
}

const VENUES = ["auction", "sale", "getgems", "marketapp", "market", "owned"] as const;
type Venue = (typeof VENUES)[number];

/** The feed's own filters, kept apart from the table's and from the snipe settings. */
interface FreshFilters {
  venues: Venue[];
  categories: string[];
  len: [number, number];
  minPrice: string;
  maxPrice: string;
  maxBuyout: string;
  minScore: number;
  unscored: boolean;
  buyNow: boolean;
  hideJunk: boolean;
}

const DEFAULTS: FreshFilters = {
  venues: ["auction", "sale", "getgems", "marketapp", "market"],
  categories: [],
  len: [LEN_MIN, LEN_MAX],
  minPrice: "",
  maxPrice: "",
  maxBuyout: "",
  minScore: 0,
  unscored: true,
  buyNow: false,
  hideJunk: true,
};

const POLL_MS = 8000;
// Each poll reads this far back past the newest name it has: a name found a few minutes ago may
// only now have its AI score or price, and so only now match the filters.
const LOOKBACK = 15 * 60;
// what the page keeps; the same as the most rows a free account's search shows
const KEEP = 300;
const SCORES = [0, 5, 6, 7, 8];

const lsKey = (uid: number) => `tonwatch.fresh.v1.${uid}`;

function params(f: FreshFilters) {
  const p = new URLSearchParams();
  if (f.venues.length && f.venues.length < VENUES.length) p.set("venue", f.venues.join(","));
  if (f.categories.length) p.set("category", f.categories.join(","));
  if (f.len[0] > LEN_MIN) p.set("min_len", String(f.len[0]));
  if (f.len[1] < LEN_MAX) p.set("max_len", String(f.len[1]));
  if (f.minPrice) p.set("min_price", f.minPrice);
  if (f.maxPrice) p.set("max_price", f.maxPrice);
  if (f.maxBuyout) p.set("max_buyout", f.maxBuyout);
  if (f.minScore > 0) {
    p.set("min_score", String(f.minScore));
    if (f.unscored) p.set("unscored", "1");
  }
  if (f.buyNow) p.set("buy_now", "1");
  if (f.hideJunk) p.set("hide_junk", "1");
  return p;
}

/** A name in the feed, and when it popped in on this page (0: it was there on opening). */
type Row = Username & { arrived: number };

/** Fresh names: everything the collectors see for the first time, newest on top, as it lands. */
export function Fresh({ categories, onOpen, onWatch }: Props) {
  const t = useT().fresh;
  const { user } = useAuth();
  const [f, setF] = useState<FreshFilters>(DEFAULTS);
  const [hydrated, setHydrated] = useState(false);
  const [rows, setRows] = useState<Row[] | null>(null);
  const [error, setError] = useState("");
  const [paused, setPaused] = useState(false);
  const [checkedAt, setCheckedAt] = useState(0);
  const [arrivedCount, setArrivedCount] = useState(0);
  const [watched, setWatched] = useState<Record<string, boolean>>({});
  const rowsRef = useRef<Row[]>([]);
  const ready = useRef(false); // the baseline for this filter set is in
  const gen = useRef(0);

  useEffect(() => {
    try {
      const s = JSON.parse(localStorage.getItem(lsKey(user.id)) ?? "null");
      if (s) setF({ ...DEFAULTS, ...s });
    } catch {}
    setHydrated(true);
  }, [user.id]);
  useEffect(() => {
    if (!hydrated) return;
    try {
      localStorage.setItem(lsKey(user.id), JSON.stringify(f));
    } catch {}
  }, [f, hydrated, user.id]);

  const query = useMemo(() => params(f).toString(), [f]);

  const put = (r: Row[]) => {
    rowsRef.current = r;
    setRows(r);
  };

  // a new filter set starts the feed over: what is there now is the baseline, not news
  useEffect(() => {
    if (!hydrated) return;
    const g = ++gen.current;
    ready.current = false;
    rowsRef.current = [];
    setRows(null);
    setArrivedCount(0);
    setError("");
    api
      .fresh(new URLSearchParams(query))
      .then((r) => {
        if (g !== gen.current) return;
        put(r.items.map((u) => ({ ...u, arrived: 0 })));
        ready.current = true;
        setCheckedAt(Math.floor(Date.now() / 1000));
      })
      .catch((e) => {
        if (g !== gen.current) return;
        setError((e as Error).message);
        put([]);
        ready.current = true;
      });
  }, [query, hydrated]);

  const poll = useCallback(async () => {
    const g = gen.current;
    const cur = rowsRef.current;
    const p = new URLSearchParams(query);
    const newest = cur.reduce((m, u) => Math.max(m, u.first_seen), 0);
    if (newest) p.set("since", String(newest - LOOKBACK));
    try {
      const r = await api.fresh(p);
      if (g !== gen.current) return;
      const byName = new Map(cur.map((u) => [u.name, u]));
      const now = Date.now();
      let added = 0;
      for (const u of r.items) {
        const old = byName.get(u.name);
        if (!old) added++;
        byName.set(u.name, { ...u, arrived: old ? old.arrived : now });
      }
      const next = [...byName.values()].sort((a, b) => b.first_seen - a.first_seen || a.name.localeCompare(b.name)).slice(0, KEEP);
      put(next);
      if (added) setArrivedCount((n) => n + added);
      setCheckedAt(Math.floor(now / 1000));
      setError("");
    } catch (e) {
      if (g === gen.current) setError((e as Error).message);
    }
  }, [query]);

  useEffect(() => {
    if (paused || !hydrated) return;
    const id = setInterval(() => {
      if (!document.hidden && ready.current) poll();
    }, POLL_MS);
    const wake = () => !document.hidden && ready.current && poll();
    document.addEventListener("visibilitychange", wake);
    return () => {
      clearInterval(id);
      document.removeEventListener("visibilitychange", wake);
    };
  }, [paused, hydrated, poll]);

  const patch = (p: Partial<FreshFilters>) => setF((x) => ({ ...x, ...p }));
  const toggle = <T,>(xs: T[], x: T) => (xs.includes(x) ? xs.filter((y) => y !== x) : [...xs, x]);
  const watch = (name: string) => {
    onWatch(name);
    setWatched((w) => ({ ...w, [name]: true }));
  };
  const cat = (c: string) => getT().filterBar.categories[c] ?? c;
  const isDefault = JSON.stringify(f) === JSON.stringify(DEFAULTS);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <span className={cn("inline-flex items-center gap-2 text-[13px] font-semibold tracking-wide uppercase", paused ? "text-muted-foreground" : "text-positive")}>
          <span className="relative flex size-2">
            {!paused && <span className="absolute inset-0 animate-ping rounded-full bg-positive opacity-60" />}
            <span className={cn("relative size-2 rounded-full", paused ? "bg-muted-foreground" : "bg-positive")} />
          </span>
          {paused ? t.paused : t.live}
        </span>
        {checkedAt > 0 && <Checked ts={checkedAt} />}
        {arrivedCount > 0 && <span className="data text-[13px] text-foreground">{t.arrived(arrivedCount)}</span>}
        <Button variant="outline" size="sm" className="ml-auto h-8" onClick={() => setPaused(!paused)}>
          {paused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
          {paused ? t.resume : t.pause}
        </Button>
      </div>

      <section className="space-y-3 rounded-lg border border-border bg-card p-3 sm:p-4">
        <ChipRow label={t.venue}>
          {VENUES.map((v) => (
            <Chip key={v} on={f.venues.includes(v)} onClick={() => patch({ venues: toggle(f.venues, v) })}>
              {t.venues[v]}
            </Chip>
          ))}
        </ChipRow>
        <ChipRow label={t.kind}>
          <Chip on={f.categories.length === 0} onClick={() => patch({ categories: [] })}>
            {t.anyKind}
          </Chip>
          {categories
            .filter((c) => c !== "junk")
            .map((c) => (
              <Chip key={c} on={f.categories.includes(c)} onClick={() => patch({ categories: toggle(f.categories, c) })}>
                {cat(c)}
              </Chip>
            ))}
        </ChipRow>
        <ChipRow label={t.score}>
          {SCORES.map((s) => (
            <Chip key={s} on={f.minScore === s} onClick={() => patch({ minScore: s })}>
              {s === 0 ? t.anyScore : `${s}+`}
            </Chip>
          ))}
          {f.minScore > 0 && <Toggle label={t.unscored} checked={f.unscored} onChange={(v) => patch({ unscored: v })} />}
        </ChipRow>
        <div className="grid gap-x-6 gap-y-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,0.8fr)]">
          <Field label={`${t.length}: ${f.len[0]}–${f.len[1]}`}>
            <Slider min={LEN_MIN} max={LEN_MAX} step={1} value={f.len} onValueChange={(v) => patch({ len: v as [number, number] })} className="mt-2" />
          </Field>
          <Field label={t.price}>
            <div className="flex items-center gap-2">
              <Input inputMode="decimal" placeholder={t.from} value={f.minPrice} onChange={(e) => patch({ minPrice: e.target.value.replace(/[^0-9.]/g, "") })} className="h-8" />
              <span className="text-muted-foreground">–</span>
              <Input inputMode="decimal" placeholder={t.to} value={f.maxPrice} onChange={(e) => patch({ maxPrice: e.target.value.replace(/[^0-9.]/g, "") })} className="h-8" />
            </div>
          </Field>
          <Field label={t.buyout}>
            <Input inputMode="decimal" placeholder="—" value={f.maxBuyout} onChange={(e) => patch({ maxBuyout: e.target.value.replace(/[^0-9.]/g, "") })} className="h-8" />
          </Field>
        </div>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-border pt-3">
          <Toggle label={t.buyNow} title={t.buyNowHint} checked={f.buyNow} onChange={(v) => patch({ buyNow: v })} />
          <Toggle label={t.hideJunk} checked={f.hideJunk} onChange={(v) => patch({ hideJunk: v })} />
          {!isDefault && (
            <button onClick={() => setF(DEFAULTS)} className="ml-auto inline-flex items-center gap-1.5 text-[13px] text-muted-foreground hover:text-foreground">
              <RotateCcw className="size-3.5" />
              {t.reset}
            </button>
          )}
        </div>
      </section>

      {error && <p className="text-sm text-destructive">{t.loadFailed(error)}</p>}

      <section className="overflow-hidden rounded-lg border border-border bg-card">
        <div className="label grid grid-cols-[3.5rem_minmax(0,1fr)_6.5rem] items-center gap-3 border-b border-border px-3 py-2 sm:grid-cols-[3.5rem_minmax(0,1fr)_6rem_3rem_8rem_9rem_7.5rem_2rem]">
          <span>{t.cols.age}</span>
          <span>{t.cols.name}</span>
          <span className="hidden sm:block">{t.cols.kind}</span>
          <span className="hidden text-right sm:block">{t.cols.len}</span>
          <span className="hidden sm:block">{t.cols.score}</span>
          <span className="hidden sm:block">{t.cols.venue}</span>
          <span className="text-right">{t.cols.price}</span>
          <span className="hidden sm:block" />
        </div>
        {rows == null ? (
          <div className="space-y-2 p-3">
            {Array.from({ length: 8 }, (_, i) => (
              <Skeleton key={i} className="h-9" />
            ))}
          </div>
        ) : rows.length === 0 ? (
          <p className="px-4 py-10 text-center text-sm text-muted-foreground">{t.empty}</p>
        ) : (
          <ul className="divide-y divide-border">
            {rows.map((u) => (
              <FreshRow key={u.name} u={u} cat={cat} watched={u.watched || !!watched[u.name]} onOpen={onOpen} onWatch={watch} />
            ))}
          </ul>
        )}
      </section>
      {rows != null && rows.length >= KEEP && <p className="text-center text-[12.5px] text-muted-foreground">{t.cap(KEEP)}</p>}
    </div>
  );
}

function FreshRow({ u, cat, watched, onOpen, onWatch }: { u: Row; cat: (c: string) => string; watched: boolean; onOpen: (n: string) => void; onWatch: (n: string) => void }) {
  const t = useT().fresh;
  return (
    <li
      onClick={() => onOpen(u.name)}
      className={cn(
        "grid cursor-pointer grid-cols-[3.5rem_minmax(0,1fr)_6.5rem] items-center gap-3 px-3 py-2 transition-colors hover:bg-muted/60 sm:grid-cols-[3.5rem_minmax(0,1fr)_6rem_3rem_8rem_9rem_7.5rem_2rem]",
        u.arrived > 0 && "fresh-in",
      )}
    >
      <Age ts={u.first_seen} />
      <div className="min-w-0">
        <div className="truncate font-medium">@{u.name}</div>
        {u.meaning && <div className="truncate text-[12.5px] text-muted-foreground">{u.meaning}</div>}
        <div className="mt-0.5 flex items-center gap-2 text-[12px] text-muted-foreground sm:hidden">
          <StatusPill u={u} className="text-[12px]" />
          {u.score != null && <span className="data">★ {u.score}</span>}
        </div>
      </div>
      <span className="hidden truncate text-[13px] text-muted-foreground sm:block">{u.category ? cat(u.category) : "—"}</span>
      <span className="data hidden text-right text-[13px] sm:block">{u.len}</span>
      <Score u={u} className="hidden sm:inline-flex" />
      <StatusPill u={u} className="hidden sm:inline-flex" />
      <div className="text-right">
        {u.price != null ? <Ton v={u.price} className="text-[14px] font-semibold" /> : <span className="text-muted-foreground">—</span>}
        {u.buyout != null && u.buyout !== u.price && <div className="data text-[11.5px] text-positive">{t.buyNowAt(num(u.buyout))}</div>}
      </div>
      <button
        onClick={(e) => {
          e.stopPropagation();
          if (!watched) onWatch(u.name);
        }}
        aria-label={watched ? t.watching : t.watch(u.name)}
        title={watched ? t.watching : t.watch(u.name)}
        className="hidden size-8 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground sm:grid"
      >
        <Star className={cn("size-4", watched && "fill-primary text-primary")} />
      </button>
    </li>
  );
}

function ageLabel(s: number) {
  const a = getT().fresh.age;
  return s < 60 ? a.s(s) : s < 3600 ? a.m(Math.floor(s / 60)) : s < 86400 ? a.h(Math.floor(s / 3600)) : a.d(Math.floor(s / 86400));
}

/** How long ago a name was first seen, ticking; under a minute reads as just in. */
function Age({ ts }: { ts: number }) {
  const now = useNow();
  useT(); // re-render on a language switch
  const s = Math.max(0, now - ts);
  return (
    <span className={cn("data text-[13px]", s < 300 ? "font-semibold text-positive" : s < 3600 ? "text-foreground" : "text-muted-foreground")} title={new Date(ts * 1000).toLocaleString()}>
      {ageLabel(s)}
    </span>
  );
}

function Checked({ ts }: { ts: number }) {
  const now = useNow();
  const t = useT().fresh;
  return <span className="data text-[12.5px] text-muted-foreground">{t.checked(ageLabel(Math.max(0, now - ts)))}</span>;
}

function ChipRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="label mr-1 w-full sm:w-20">{label}</span>
      {children}
    </div>
  );
}

function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      aria-pressed={on}
      className={cn(
        "h-7 rounded-full border px-3 text-[13px] transition-colors",
        on ? "border-primary bg-accent text-accent-foreground" : "border-border text-muted-foreground hover:border-foreground/30 hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <div className="label">{label}</div>
      {children}
    </div>
  );
}

function Toggle({ label, title, checked, onChange }: { label: string; title?: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="inline-flex cursor-pointer items-center gap-2 text-[13px]" title={title}>
      <Switch checked={checked} onCheckedChange={onChange} />
      {label}
    </label>
  );
}
