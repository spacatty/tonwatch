"use client";

import { Check, ChevronDown, Hash, Loader2, Search, SlidersHorizontal, Sparkles, X, Zap } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { activeFilterCount, DEFAULT_FILTERS, DIGIT_MODES, LEN_MAX, LEN_MIN, type Filters, type View } from "@/lib/views";

type Key = "score" | "buyout" | "price" | "ending" | "length" | "digits" | "topic" | "kind" | "chars";

interface Props {
  view: View;
  f: Filters;
  set: <K extends keyof Filters>(k: K, v: Filters[K]) => void;
  patch: (p: Partial<Filters>) => void;
  categories: string[];
  categoryCounts: Record<string, number>;
  llmEnabled: boolean;
}

const LABELS: Record<Key, string> = {
  score: "AI score",
  buyout: "Buyout",
  price: "Price",
  ending: "Ends within",
  length: "Length",
  digits: "Digits",
  topic: "Topic",
  kind: "Kind",
  chars: "Characters",
};

/** Search, a chip for each filter in use (click to edit, × to clear), and a drawer with every filter. */
export function FilterBar({ view, f, set, patch, categories, categoryCounts, llmEnabled }: Props) {
  const auctionView = view.key === "ending" || view.key === "auction";
  const [open, setOpen] = useState<Key | null>(null);
  const [drawer, setDrawer] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (e.key === "/" && !["INPUT", "TEXTAREA"].includes(t.tagName)) {
        e.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, []);

  // value summary per chip; null = not set
  const value: Record<Key, string | null> = {
    score: f.minScore > 0 ? `≥ ${f.minScore}${f.unscored ? " or unrated" : ""}` : f.unscored ? null : "rated only",
    buyout: range(f.minBuyout, f.maxBuyout),
    price: range(f.minPrice, f.maxPrice),
    ending: f.endingIn ? { "900": "15 min", "3600": "1 hour", "21600": "6 hours", "86400": "24 hours" }[f.endingIn] ?? f.endingIn : null,
    length: f.len[0] > LEN_MIN || f.len[1] < LEN_MAX ? `${f.len[0]}–${f.len[1]}` : null,
    digits: f.digits ? `${f.digits} · ${DIGIT_MODES.find((m) => m.v === f.digitMode)?.label.toLowerCase()}` : null,
    topic: f.keywords.length ? (f.topic || f.keywords.slice(0, 2).join(", ")) + (f.keywords.length > 2 ? ` +${f.keywords.length - (f.topic ? 0 : 2)}` : "") : null,
    kind: kindSummary(f),
    chars: [f.noDigits && "no digits", f.noUnder && "no _", !f.hideJunk && "junk shown"].filter(Boolean).join(", ") || null,
  };
  const clear: Record<Key, () => void> = {
    score: () => patch({ minScore: 0, unscored: true }),
    buyout: () => patch({ minBuyout: "", maxBuyout: "" }),
    price: () => patch({ minPrice: "", maxPrice: "" }),
    ending: () => set("endingIn", ""),
    length: () => set("len", [LEN_MIN, LEN_MAX]),
    digits: () => set("digits", ""),
    topic: () => patch({ keywords: [], topic: "" }),
    kind: () => patch({ categories: [], notCategories: [] }),
    chars: () => patch({ noDigits: false, noUnder: false, hideJunk: true }),
  };

  const pinned: Key[] = ["score", "buyout", ...(auctionView ? (["ending"] as Key[]) : [])];
  const optional: Key[] = ["topic", "price", "length", "digits", "kind", "chars"];
  const shown = [...pinned, ...optional.filter((k) => value[k] != null)];
  const all: Key[] = ["score", "buyout", "price", ...(auctionView ? (["ending"] as Key[]) : []), "kind", "length", "digits", "chars", "topic"];
  const count = activeFilterCount(f);

  // focus the first field only in a single-filter popover, not in the drawer
  const editor = (k: Key, inDrawer = false) => {
    switch (k) {
      case "score":
        return (
          <>
            <Head title="Minimum AI score" />
            <Segmented
              value={String(f.minScore)}
              onChange={(v) => set("minScore", Number(v))}
              options={["0", "5", "6", "7", "8", "9"].map((v) => ({ v, label: v === "0" ? "Any" : `${v}+` }))}
            />
            <Toggle label="Include names not rated yet" checked={f.unscored} onChange={(v) => set("unscored", v)} />
            {f.digits && <Note>Off while a digit search is set: the AI rates digit names low by design.</Note>}
          </>
        );
      case "buyout":
        return (
          <>
            <Head title="Buyout price" sub="What it costs to own it right now: buy-now on an auction, the listing price otherwise." />
            <Range a={f.minBuyout} b={f.maxBuyout} onA={(v) => set("minBuyout", v)} onB={(v) => set("maxBuyout", v)} />
            <Presets values={["10", "25", "50", "100", "500", "1000"]} active={f.maxBuyout} onPick={(v) => set("maxBuyout", f.maxBuyout === v ? "" : v)} />
          </>
        );
      case "price":
        return (
          <>
            <Head title={view.key === "owned" ? "Last sale price" : "Price"} sub="Current bid on auctions, listing price on sales." />
            <Range a={f.minPrice} b={f.maxPrice} onA={(v) => set("minPrice", v)} onB={(v) => set("maxPrice", v)} />
          </>
        );
      case "ending":
        return (
          <>
            <Head title="Auction ends within" />
            <Segmented
              value={f.endingIn}
              onChange={(v) => set("endingIn", v)}
              options={[
                { v: "", label: "Any" },
                { v: "900", label: "15m" },
                { v: "3600", label: "1h" },
                { v: "21600", label: "6h" },
                { v: "86400", label: "24h" },
              ]}
            />
          </>
        );
      case "length":
        return (
          <>
            <Head title="Length" right={<span className="data text-sm">{f.len[0]}–{f.len[1]}</span>} />
            <Slider min={LEN_MIN} max={LEN_MAX} step={1} value={f.len} onValueChange={(v) => set("len", v as [number, number])} />
            <Presets
              values={["4–5", "4–6", "4–8", "6–12"]}
              active={`${f.len[0]}–${f.len[1]}`}
              onPick={(v) => {
                const [a, b] = v.split("–").map(Number);
                set("len", [a, b]);
              }}
            />
          </>
        );
      case "digits":
        return (
          <>
            <Head title="Digit combination" />
            <div className="relative">
              <Hash className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <input
                autoFocus={!inDrawer}
                inputMode="numeric"
                value={f.digits}
                onChange={(e) => set("digits", e.target.value.replace(/[^0-9]/g, "").slice(0, 8))}
                placeholder="552"
                className={cn(inputCls, "data pl-9 tracking-wider")}
              />
            </div>
            <Segmented className="mt-2" value={f.digitMode} onChange={(v) => set("digitMode", v as Filters["digitMode"])} options={DIGIT_MODES.map((m) => ({ v: m.v, label: m.label }))} />
            {f.digits && <Note>Names with {DIGIT_MODES.find((m) => m.v === f.digitMode)!.hint(f.digits)}.</Note>}
          </>
        );
      case "topic":
        return <TopicEditor f={f} patch={patch} enabled={llmEnabled} autoFocus={!inDrawer} />;
      case "kind":
        return (
          <>
            <Head title="Kind of name" sub="As judged by the AI. Click to cycle: only these ✓, hide ✕, any." />
            <div className="grid grid-cols-2 gap-0.5">
              {categories.map((c) => {
                const st = f.categories.includes(c) ? "on" : f.notCategories.includes(c) ? "off" : "any";
                const next = () => {
                  const cats = f.categories.filter((x) => x !== c);
                  const nots = f.notCategories.filter((x) => x !== c);
                  if (st === "any") patch({ categories: [...cats, c], notCategories: nots });
                  else if (st === "on") patch({ categories: cats, notCategories: [...nots, c] });
                  else patch({ categories: cats, notCategories: nots });
                };
                return (
                  <button
                    key={c}
                    onClick={next}
                    aria-label={`${c}: ${st === "on" ? "only these" : st === "off" ? "hidden" : "any"}`}
                    className={cn(
                      "flex h-8 items-center gap-2 rounded px-2 text-left text-sm transition-colors",
                      st === "on" ? "bg-accent text-accent-foreground" : st === "off" ? "text-muted-foreground hover:bg-muted" : "hover:bg-muted",
                    )}
                  >
                    <span
                      className={cn(
                        "flex size-4 shrink-0 items-center justify-center rounded-[3px] border",
                        st === "on" ? "border-primary bg-primary text-primary-foreground" : st === "off" ? "border-destructive bg-destructive text-background" : "border-input",
                      )}
                    >
                      {st === "on" && <Check className="size-3" strokeWidth={3} />}
                      {st === "off" && <X className="size-3" strokeWidth={3} />}
                    </span>
                    <span className={cn("truncate capitalize", st === "off" && "line-through decoration-destructive/70")}>{c}</span>
                    {categoryCounts[c] ? <span className="data ml-auto text-[11px] text-muted-foreground">{compact(categoryCounts[c])}</span> : null}
                  </button>
                );
              })}
            </div>
          </>
        );
      case "chars":
        return (
          <>
            <Head title="Characters" />
            <Toggle label="No digits" checked={f.noDigits} onChange={(v) => set("noDigits", v)} first />
            <Toggle label="No underscores" checked={f.noUnder} onChange={(v) => set("noUnder", v)} />
            <Toggle label="Hide obvious junk" checked={f.hideJunk} onChange={(v) => set("hideJunk", v)} />
          </>
        );
    }
  };

  return (
    <div className="space-y-2.5">
      <div className="relative">
        <Search className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground" />
        <input
          ref={searchRef}
          value={f.q}
          onChange={(e) => set("q", e.target.value)}
          placeholder="Search usernames and meanings"
          className="h-11 w-full rounded-lg border border-input bg-card pr-12 pl-10 text-[15px] outline-none transition-colors placeholder:text-muted-foreground hover:border-ring/40 focus:border-ring focus:ring-3 focus:ring-ring/15"
        />
        {f.q ? (
          <button onClick={() => set("q", "")} aria-label="Clear search" className="absolute top-1/2 right-3 -translate-y-1/2 rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
            <X className="size-4" />
          </button>
        ) : (
          <kbd className="data absolute top-1/2 right-3 -translate-y-1/2 rounded border border-border px-1.5 text-[11px] text-muted-foreground">/</kbd>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-1.5">
        {shown.map((k) => (
          <Popover key={k} open={open === k} onOpenChange={(o) => setOpen(o ? k : null)}>
            <Chip label={LABELS[k]} value={value[k]} icon={k === "topic" ? <Sparkles className="size-3.5" /> : undefined} onClear={value[k] != null ? clear[k] : undefined} />
            <PopoverContent align="start" className="w-80 p-4">
              {editor(k)}
            </PopoverContent>
          </Popover>
        ))}

        <button
          onClick={() => set("buyNow", !f.buyNow)}
          aria-pressed={f.buyNow}
          className={cn(
            "inline-flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-[13px] transition-colors",
            f.buyNow ? "border-positive/60 bg-pill-positive text-pill-positive-ink" : "border-border bg-card text-muted-foreground hover:border-input hover:text-foreground",
          )}
        >
          <Zap className={cn("size-3.5", f.buyNow && "fill-current")} />
          Instant buy
        </button>

        <button
          onClick={() => setDrawer(true)}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-input bg-card px-2.5 text-[13px] text-foreground transition-colors hover:border-ring"
        >
          <SlidersHorizontal className="size-3.5" />
          All filters
          {count > 0 && <span className="data rounded-sm bg-primary px-1 text-[10px] font-semibold text-primary-foreground">{count}</span>}
        </button>

        {JSON.stringify({ ...f, sorts: [] }) !== JSON.stringify({ ...DEFAULT_FILTERS, sorts: [] }) && (
          <button
            onClick={() => {
              patch({ ...DEFAULT_FILTERS, sorts: f.sorts });
            }}
            className="ml-1 h-8 px-1.5 text-[13px] text-muted-foreground hover:text-foreground"
          >
            Reset filters
          </button>
        )}
      </div>

      <Sheet open={drawer} onOpenChange={setDrawer}>
        <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-[400px]">
          <SheetHeader className="border-b border-border px-5 py-4">
            <SheetTitle>Filters</SheetTitle>
            <SheetDescription>Changes apply as you make them.</SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 divide-y divide-border overflow-y-auto">
            {all.map((k) => (
              <section key={k} className="px-5 py-4">
                {editor(k, true)}
              </section>
            ))}
          </div>
          <SheetFooter className="flex-row border-t border-border px-5 py-3">
            <Button variant="ghost" onClick={() => patch({ ...DEFAULT_FILTERS, q: f.q, sorts: f.sorts })}>
              Reset
            </Button>
            <Button className="ml-auto" onClick={() => setDrawer(false)}>
              Done
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  );
}

/** "crypto, no junk"; past two kinds, counts. */
function kindSummary(f: Filters) {
  const parts = [...f.categories, ...f.notCategories.map((c) => `no ${c}`)];
  if (!parts.length) return null;
  if (parts.length <= 2) return parts.join(", ");
  return [f.categories.length && `${f.categories.length} only`, f.notCategories.length && `${f.notCategories.length} hidden`].filter(Boolean).join(", ");
}

function range(a: string, b: string) {
  if (a && b) return `${a}–${b} TON`;
  if (b) return `≤ ${b} TON`;
  if (a) return `≥ ${a} TON`;
  return null;
}

function Chip({ label, value, icon, onClear }: { label: string; value: string | null; icon?: React.ReactNode; onClear?: () => void }) {
  const on = value != null;
  return (
    <span className={cn("inline-flex h-8 items-stretch overflow-hidden rounded-md border text-[13px] transition-colors", on ? "border-primary/50 bg-accent" : "border-border bg-card hover:border-input")}>
      <PopoverTrigger className={cn("inline-flex items-center gap-1.5 px-2.5", on ? "text-accent-foreground" : "text-muted-foreground hover:text-foreground")}>
        {icon}
        <span>{label}</span>
        {on ? <span className="data max-w-48 truncate font-medium text-foreground">{value}</span> : <ChevronDown className="size-3.5 opacity-60" />}
      </PopoverTrigger>
      {onClear && (
        <button onClick={onClear} aria-label={`Clear ${label}`} className="border-l border-primary/30 px-1.5 text-accent-foreground hover:bg-primary/15">
          <X className="size-3.5" />
        </button>
      )}
    </span>
  );
}

const inputCls =
  "h-9 w-full rounded-md border border-input bg-background px-3 text-sm outline-none transition-colors placeholder:text-muted-foreground hover:border-ring/40 focus:border-ring focus:ring-3 focus:ring-ring/15";

function Head({ title, sub, right }: { title: string; sub?: string; right?: React.ReactNode }) {
  return (
    <div className="mb-3">
      <div className="flex items-baseline justify-between gap-2">
        <h4 className="text-sm font-semibold">{title}</h4>
        {right}
      </div>
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </div>
  );
}

function Note({ children }: { children: React.ReactNode }) {
  return <p className="mt-2 text-xs text-muted-foreground">{children}</p>;
}

function NumberField({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder: string }) {
  return (
    <label className="flex h-9 min-w-0 flex-1 items-center rounded-md border border-input bg-background transition-colors hover:border-ring/40 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/15">
      <input
        type="number"
        min={0}
        inputMode="decimal"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="data h-full w-full min-w-0 bg-transparent pl-3 text-sm outline-none placeholder:text-muted-foreground [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none"
      />
      <span className="data pr-3 pl-1 text-[11px] text-muted-foreground">TON</span>
    </label>
  );
}

function Range({ a, b, onA, onB }: { a: string; b: string; onA: (v: string) => void; onB: (v: string) => void }) {
  return (
    <div className="flex items-center gap-2">
      <NumberField value={a} onChange={onA} placeholder="Min" />
      <span className="text-muted-foreground">–</span>
      <NumberField value={b} onChange={onB} placeholder="Max" />
    </div>
  );
}

function Presets({ values, active, onPick }: { values: string[]; active: string; onPick: (v: string) => void }) {
  return (
    <div className="mt-2.5 flex flex-wrap gap-1">
      {values.map((v) => (
        <button
          key={v}
          onClick={() => onPick(v)}
          className={cn(
            "data h-7 rounded px-2 text-xs transition-colors",
            active === v ? "bg-primary text-primary-foreground" : "bg-muted text-foreground hover:bg-accent hover:text-accent-foreground",
          )}
        >
          {/^[0-9]+$/.test(v) ? `≤${v}` : v}
        </button>
      ))}
    </div>
  );
}

function Toggle({ label, checked, onChange, first }: { label: string; checked: boolean; onChange: (v: boolean) => void; first?: boolean }) {
  return (
    <label className={cn("flex cursor-pointer items-center justify-between gap-3 text-sm", !first && "mt-3")}>
      {label}
      <Switch checked={checked} onCheckedChange={onChange} />
    </label>
  );
}

function Segmented({ value, onChange, options, className }: { value: string; onChange: (v: string) => void; options: { v: string; label: string }[]; className?: string }) {
  return (
    <div className={cn("flex gap-0.5 rounded-md bg-muted p-0.5", className)}>
      {options.map((o) => (
        <button
          key={o.v || "any"}
          onClick={() => onChange(o.v)}
          className={cn(
            "data h-7 flex-1 rounded-[4px] px-1.5 text-xs whitespace-nowrap transition-colors",
            value === o.v ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** AI topic: type a theme; the model expands it into stems that names or meanings must contain. */
function TopicEditor({ f, patch, enabled, autoFocus }: { f: Filters; patch: (p: Partial<Filters>) => void; enabled: boolean; autoFocus: boolean }) {
  const [busy, setBusy] = useState(false);
  const [draft, setDraft] = useState("");

  const expand = async () => {
    const topic = f.topic.trim();
    if (!topic) return;
    setBusy(true);
    try {
      const r = await api.expandTopic(topic);
      patch({ keywords: r.keywords });
    } catch (e) {
      toast.error(`Couldn't expand the topic: ${(e as Error).message}`);
    } finally {
      setBusy(false);
    }
  };
  const addKeyword = () => {
    const k = draft.trim().toLowerCase().replace(/[^a-z0-9_]/g, "");
    if (k && !f.keywords.includes(k)) patch({ keywords: [...f.keywords, k] });
    setDraft("");
  };

  return (
    <>
      <Head title="Topic" sub="Describe a theme. The AI turns it into words that names or meanings should contain." />
      <div className="flex gap-2">
        <input
          autoFocus={autoFocus}
          value={f.topic}
          onChange={(e) => patch({ topic: e.target.value })}
          onKeyDown={(e) => e.key === "Enter" && expand()}
          placeholder={enabled ? "vpn, coffee, travel…" : "Needs OPENROUTER_API_KEY"}
          disabled={!enabled}
          className={inputCls}
        />
        <Button className="h-9 shrink-0" onClick={expand} disabled={!enabled || busy || !f.topic.trim()}>
          {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Sparkles className="size-3.5" />}
          Find
        </Button>
      </div>
      {f.keywords.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1">
          {f.keywords.map((k) => (
            <button
              key={k}
              onClick={() => patch({ keywords: f.keywords.filter((x) => x !== k) })}
              className="data group inline-flex h-6 items-center gap-1 rounded bg-accent pr-1 pl-2 text-xs text-accent-foreground hover:bg-accent-track"
            >
              {k}
              <X className="size-3 opacity-50 group-hover:opacity-100" />
            </button>
          ))}
          <input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addKeyword()}
            onBlur={addKeyword}
            placeholder="+ word"
            className="data h-6 w-16 rounded bg-transparent px-2 text-xs outline-none placeholder:text-muted-foreground focus:bg-muted"
          />
        </div>
      )}
    </>
  );
}

function compact(n: number) {
  return n >= 10000 ? `${Math.round(n / 1000)}k` : n.toLocaleString();
}
