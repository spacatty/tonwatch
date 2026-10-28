"use client";

import { ArrowUp, BookmarkPlus, ExternalLink, Search, Sparkles, Star, Trash2, Zap } from "lucide-react";
import { motion } from "motion/react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { api, fragmentUrl, getgemsUrl, type SuggestResult, type Username } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { StatusPill, Ton } from "./bits";

interface Turn {
  id: string;
  brief: string;
  opts: { buyableOnly: boolean; maxPrice: string };
  result?: SuggestResult;
  error?: string;
}

interface Props {
  llmEnabled: boolean;
  onOpen: (name: string) => void;
  onWatch: (name: string) => void;
  onBrowse: (keywords: string[], topic: string) => void;
  onSaveTracker: (keywords: string[], topic: string) => void;
}

const EXAMPLES = [
  "I'm launching a VPN app and need a short, trustworthy name",
  "A specialty coffee shop in Berlin",
  "Crypto trading signals channel, under 50 TON",
  "Personal brand for a travel photographer",
  "AI assistant bot for lawyers",
];

const STEPS = ["Reading your request", "Searching every username we track", "Ranking what fits", "Checking fresh ideas on Fragment"];
const LS = "fragwatch.ai.v1";

export function AskAI({ llmEnabled, onOpen, onWatch, onBrowse, onSaveTracker }: Props) {
  const [turns, setTurns] = useState<Turn[]>([]);
  const [draft, setDraft] = useState("");
  const [buyableOnly, setBuyableOnly] = useState(false);
  const [maxPrice, setMaxPrice] = useState("");
  const [step, setStep] = useState(0);
  const busy = turns.some((t) => !t.result && !t.error);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    try {
      setTurns((JSON.parse(localStorage.getItem(LS) ?? "[]") as Turn[]).filter((t) => t.result || t.error));
    } catch {}
  }, []);
  useEffect(() => {
    try {
      localStorage.setItem(LS, JSON.stringify(turns.filter((t) => t.result || t.error).slice(-8)));
    } catch {}
  }, [turns]);

  useEffect(() => {
    if (!busy) return;
    setStep(0);
    const t = setInterval(() => setStep((s) => Math.min(s + 1, STEPS.length - 1)), 3500);
    return () => clearInterval(t);
  }, [busy]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [turns.length, busy]);

  const ask = async (brief: string, exclude: string[] = []) => {
    brief = brief.trim();
    if (!brief || busy) return;
    const turn: Turn = { id: Math.random().toString(36).slice(2), brief, opts: { buyableOnly, maxPrice } };
    setTurns((ts) => [...ts, turn]);
    setDraft("");
    try {
      const result = await api.suggest({
        brief,
        buyable_only: buyableOnly,
        max_price: maxPrice ? Number(maxPrice) : undefined,
        exclude,
      });
      setTurns((ts) => ts.map((t) => (t.id === turn.id ? { ...t, result } : t)));
    } catch (e) {
      setTurns((ts) => ts.map((t) => (t.id === turn.id ? { ...t, error: (e as Error).message } : t)));
    }
  };

  return (
    <div className="mx-auto flex max-w-[860px] flex-col">
      {turns.length === 0 && !busy && (
        <div className="py-6">
          <p className="max-w-[60ch] text-[15px] text-muted-foreground">
            Describe what the username is for. The AI turns it into search terms, looks through every name we track, ranks the ones that fit and checks a few fresh ideas live on
            Fragment.
          </p>
          <div className="mt-5 flex flex-wrap gap-2">
            {EXAMPLES.map((e) => (
              <button
                key={e}
                onClick={() => ask(e)}
                disabled={!llmEnabled}
                className="rounded-md border border-border bg-card px-3.5 py-2 text-left text-sm transition-colors hover:border-primary/60 hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
              >
                {e}
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="space-y-8">
        {turns.map((t) => (
          <div key={t.id} className="space-y-4">
            <div className="flex justify-end">
              <div className="max-w-[80%] rounded-2xl rounded-br-md bg-primary px-4 py-2.5 text-[15px] text-primary-foreground">
                {t.brief}
                {(t.opts.buyableOnly || t.opts.maxPrice) && (
                  <div className="mt-1 text-xs opacity-80">
                    {[t.opts.buyableOnly && "buyable now", t.opts.maxPrice && `buyout ≤ ${t.opts.maxPrice} TON`].filter(Boolean).join(" · ")}
                  </div>
                )}
              </div>
            </div>
            {!t.result && !t.error && <Thinking step={step} />}
            {t.error && <div className="rounded-lg bg-pill-negative px-4 py-3 text-sm text-pill-negative-ink">Couldn&apos;t get suggestions: {t.error}</div>}
            {t.result && (
              <Answer
                r={t.result}
                onOpen={onOpen}
                onWatch={onWatch}
                onBrowse={() => onBrowse(t.result!.keywords, t.brief)}
                onSave={() => onSaveTracker(t.result!.keywords, t.result!.summary || t.brief)}
                onMore={() => ask(t.brief, t.result!.picks.map((p) => p.item.name))}
                busy={busy}
              />
            )}
          </div>
        ))}
        <div ref={endRef} />
      </div>

      <div className="sticky bottom-0 mt-8 bg-background pt-2 pb-4">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            ask(draft);
          }}
          className="rounded-xl border border-input bg-card p-2 transition-colors focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/15"
        >
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                ask(draft);
              }
            }}
            rows={2}
            disabled={!llmEnabled}
            placeholder={llmEnabled ? "I need a username for…" : "Add OPENROUTER_API_KEY to .env to use Ask AI"}
            className="w-full resize-none bg-transparent px-2 py-1.5 text-[15px] outline-none placeholder:text-muted-foreground"
          />
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-1">
            <label className="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
              <Switch size="sm" checked={buyableOnly} onCheckedChange={setBuyableOnly} />
              Buyable now
            </label>
            <label className="flex items-center gap-1.5 text-sm text-muted-foreground">
              Buyout up to
              <input
                type="number"
                min={0}
                value={maxPrice}
                onChange={(e) => setMaxPrice(e.target.value)}
                placeholder="any"
                className="num h-7 w-16 rounded-md border border-input bg-background px-2 text-right text-foreground outline-none focus:border-ring"
              />
              TON
            </label>
            {turns.length > 0 && (
              <button type="button" onClick={() => setTurns([])} className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
                <Trash2 className="size-3.5" /> Clear chat
              </button>
            )}
            <Button type="submit" size="icon" className="ml-auto rounded-full" disabled={!llmEnabled || busy || !draft.trim()} aria-label="Ask">
              <ArrowUp className="size-4" />
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}

function Thinking({ step }: { step: number }) {
  return (
    <div className="flex items-center gap-3 text-sm text-muted-foreground">
      <Sparkles className="size-4 animate-pulse text-primary" />
      <motion.span key={step} initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }}>
        {STEPS[step]}…
      </motion.span>
    </div>
  );
}

function Answer({
  r,
  onOpen,
  onWatch,
  onBrowse,
  onSave,
  onMore,
  busy,
}: {
  r: SuggestResult;
  onOpen: (n: string) => void;
  onWatch: (n: string) => void;
  onBrowse: () => void;
  onSave: () => void;
  onMore: () => void;
  busy: boolean;
}) {
  const [watched, setWatched] = useState<Record<string, boolean>>({});
  const watch = (n: string) => {
    onWatch(n);
    setWatched((w) => ({ ...w, [n]: true }));
  };
  const listed = r.picks.filter((p) => p.item.status !== "owned");
  const owned = r.picks.filter((p) => p.item.status === "owned");
  const available = r.ideas.filter((i) => i.status !== "unknown");

  return (
    <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      <div>
        <p className="text-[15px]">
          {r.summary} I looked through <span className="num font-medium">{r.searched}</span> matching names and picked{" "}
          <span className="num font-medium">{r.picks.length}</span> that fit.
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs text-muted-foreground">Searched for</span>
          {r.keywords.slice(0, 18).map((k) => (
            <span key={k} className="data rounded-sm bg-muted px-1.5 py-0.5 text-xs">
              {k}
            </span>
          ))}
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={onBrowse}>
            <Search className="size-3.5" /> Browse all matches
          </Button>
          <Button size="sm" variant="outline" onClick={onSave}>
            <BookmarkPlus className="size-3.5" /> Save as tracker
          </Button>
          <Button size="sm" variant="ghost" onClick={onMore} disabled={busy}>
            <Sparkles className="size-3.5" /> More like this
          </Button>
        </div>
      </div>

      {listed.length > 0 && <PickList title="You can get these now" picks={listed} onOpen={onOpen} onWatch={watch} watched={watched} />}
      {owned.length > 0 && <PickList title="Owned: make the owner an offer" picks={owned} onOpen={onOpen} onWatch={watch} watched={watched} />}
      {r.picks.length === 0 && <p className="text-sm text-muted-foreground">Nothing in the database fits well yet. Try the fresh ideas below, or describe it differently.</p>}

      {available.length > 0 && (
        <section>
          <h3 className="mb-1 text-sm font-semibold">Fresh ideas, checked on Fragment</h3>
          <p className="mb-3 text-xs text-muted-foreground">Invented for you. “Taken” names can still get an offer.</p>
          <div className="flex flex-wrap gap-2">
            {available.map((i) => (
              <a
                key={i.name}
                href={fragmentUrl(i.name)}
                target="_blank"
                rel="noreferrer"
                className="group inline-flex items-center gap-2 rounded-lg bg-card py-1.5 pr-2.5 pl-3 text-sm ring-1 ring-border transition-colors hover:ring-primary/50"
              >
                <span className="data font-medium">@{i.name}</span>
                <IdeaStatus s={i.status} item={i.item} />
                <ExternalLink className="size-3 text-muted-foreground group-hover:text-foreground" />
              </a>
            ))}
          </div>
        </section>
      )}
    </motion.div>
  );
}

function IdeaStatus({ s, item }: { s: string; item?: Username }) {
  const map: Record<string, [string, string]> = {
    available: ["Available", "bg-pill-positive text-pill-positive-ink"],
    auction: ["Auction", "bg-pill-warning text-pill-warning-ink"],
    sale: ["For sale", "bg-pill-positive text-pill-positive-ink"],
    sold: ["Taken", "bg-secondary text-muted-foreground"],
    taken: ["Taken", "bg-secondary text-muted-foreground"],
  };
  const [label, cls] = map[s] ?? [s, "bg-secondary text-muted-foreground"];
  return (
    <span className={cn("rounded-full px-2 py-0.5 text-xs font-medium", cls)}>
      {label}
      {item?.price != null && (s === "auction" || s === "sale") ? ` · ${item.price} TON` : ""}
    </span>
  );
}

function PickList({
  title,
  picks,
  onOpen,
  onWatch,
  watched,
}: {
  title: string;
  picks: SuggestResult["picks"];
  onOpen: (n: string) => void;
  onWatch: (n: string) => void;
  watched: Record<string, boolean>;
}) {
  return (
    <section>
      <h3 className="mb-2 text-sm font-semibold">{title}</h3>
      <ul className="panel overflow-hidden">
        {picks.map(({ item: u, fit, reason }) => (
          <li key={u.name} onClick={() => onOpen(u.name)} className="group flex cursor-pointer items-center gap-3 border-b border-border/70 px-4 py-3 last:border-0 hover:bg-muted/60">
            <span
              className={cn(
                "num flex size-8 shrink-0 items-center justify-center rounded-md text-sm font-semibold",
                fit >= 8 ? "bg-primary text-primary-foreground" : "bg-accent text-accent-foreground",
              )}
              title="How well it fits your request"
            >
              {fit}
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="data truncate text-[15px] font-medium">@{u.name}</span>
                <StatusPill u={u} />
              </div>
              <div className="truncate text-[13px] text-muted-foreground">{reason}</div>
            </div>
            <div className="hidden text-right text-sm sm:block">
              {u.buyout != null ? (
                <span className="inline-flex items-center gap-1 font-medium">
                  <Zap className="size-3 fill-positive text-positive" />
                  <Ton v={u.buyout} />
                </span>
              ) : u.price != null ? (
                <span>
                  bid <Ton v={u.price} />
                </span>
              ) : (
                <span className="text-xs text-muted-foreground">{u.status === "market" ? `price on ${u.chain_market ?? "market"}` : "offer only"}</span>
              )}
            </div>
            <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
              <button onClick={() => onWatch(u.name)} aria-label={`Track @${u.name}`} className="rounded p-1.5 hover:bg-muted">
                <Star className={cn("size-4", watched[u.name] || u.watched ? "fill-warning text-warning" : "text-muted-foreground")} />
              </button>
              <a
                href={u.status === "market" && u.nft_friendly ? getgemsUrl(u.nft_friendly) : fragmentUrl(u.name)}
                target="_blank"
                rel="noreferrer"
                aria-label={`Open @${u.name}`}
                className="rounded p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
              >
                <ExternalLink className="size-4" />
              </a>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
