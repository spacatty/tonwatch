"use client";

import { ArrowUp, ArrowUpRight, BookmarkPlus, Coins, ExternalLink, Search, Sparkles, Star, Trash2, Zap } from "lucide-react";
import { motion } from "motion/react";
import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { api, creditsChanged, fragmentUrl, getgemsUrl, type AITurn, type SuggestResult, type Username } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { StatusPill, Ton } from "./bits";

interface Turn {
  id: number;
  brief: string;
  opts: { buyableOnly: boolean; maxPrice: string };
  result?: SuggestResult;
  error?: string;
}

const toTurn = (t: AITurn): Turn => ({
  id: t.id,
  brief: t.brief,
  opts: { buyableOnly: !!t.opts.buyable_only, maxPrice: t.opts.max_price != null ? String(t.opts.max_price) : "" },
  result: t.status === "done" ? t.result : undefined,
  error: t.status === "error" ? t.error || "failed" : undefined,
});

interface Props {
  llmEnabled: boolean;
  onOpen: (name: string) => void;
  onWatch: (name: string) => void;
  onBrowse: (keywords: string[], topic: string) => void;
  onSaveTracker: (keywords: string[], topic: string) => void;
}

const STEPS = 4; // loading messages, t.askAI.steps

export function AskAI({ llmEnabled, onOpen, onWatch, onBrowse, onSaveTracker }: Props) {
  const t = useT().askAI;
  const [turns, setTurns] = useState<Turn[]>([]);
  const [draft, setDraft] = useState("");
  const [buyableOnly, setBuyableOnly] = useState(false);
  const [maxPrice, setMaxPrice] = useState("");
  const [step, setStep] = useState(0);
  const busy = turns.some((t) => !t.result && !t.error);
  const endRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // the conversation lives on the server: load it, and while an answer is on its way, check back
  const load = useCallback(
    () =>
      api
        .aiChat()
        .then((r) => setTurns(r.turns.map(toTurn)))
        .catch(() => {}),
    [],
  );
  useEffect(() => {
    load();
  }, [load]);
  useEffect(() => {
    if (!busy) return;
    const id = setInterval(load, 1500);
    return () => {
      clearInterval(id);
      creditsChanged.dispatchEvent(new Event("change")); // the answer has been charged
    };
  }, [busy, load]);

  useEffect(() => {
    if (!busy) return;
    setStep(0);
    const id = setInterval(() => setStep((s) => Math.min(s + 1, STEPS - 1)), 3500);
    return () => clearInterval(id);
  }, [busy]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [turns.length, busy]);

  const ask = async (brief: string, exclude: string[] = []) => {
    brief = brief.trim();
    if (!brief || busy) return;
    // shown at once; replaced by the server's copy (with its real id) as soon as it is saved
    setTurns((ts) => [...ts, { id: -Date.now(), brief, opts: { buyableOnly, maxPrice } }]);
    setDraft("");
    try {
      const r = await api.aiAsk({ brief, buyable_only: buyableOnly, max_price: maxPrice ? Number(maxPrice) : undefined, exclude });
      setTurns(r.turns.map(toTurn));
    } catch (e) {
      setDraft(brief);
      setTurns((ts) => ts.filter((t) => t.id > 0));
      toast.error((e as Error).message);
    }
  };
  const clear = () => {
    api
      .aiClear()
      .then(() => setTurns([]))
      .catch((e) => toast.error(e.message));
  };

  const grow = (el: HTMLTextAreaElement | null) => {
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  };
  useEffect(() => grow(inputRef.current), [draft]);

  return (
    <div className="mx-auto flex min-h-[calc(100dvh-170px)] max-w-[820px] flex-col">
      {turns.length === 0 && !busy && (
        <div className="flex flex-1 flex-col items-center justify-center py-10 text-center">
          <AIMark className="size-12 rounded-2xl" icon="size-6" />
          <h2 className="mt-5 text-2xl font-semibold tracking-tight">{t.hello}</h2>
          <p className="mt-2 max-w-[56ch] text-[15px] text-muted-foreground">{t.intro}</p>
          <div className="mt-8 grid w-full gap-2.5 sm:grid-cols-2">
            {t.examples.map((e) => (
              <button
                key={e}
                onClick={() => ask(e)}
                disabled={!llmEnabled}
                className="group flex items-start gap-3 rounded-xl border border-border bg-card p-3.5 text-left text-[14px] transition-all hover:-translate-y-0.5 hover:border-primary/50 hover:shadow-md disabled:opacity-50"
              >
                <Sparkles className="mt-0.5 size-4 shrink-0 text-primary/70 group-hover:text-primary" />
                <span className="flex-1">{e}</span>
                <ArrowUpRight className="size-4 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="flex-1 space-y-10 pt-2">
        {turns.map((tn) => (
          <div key={tn.id} className="space-y-5">
            <div className="flex justify-end">
              <div className="max-w-[80%] rounded-2xl rounded-br-md bg-primary px-4 py-2.5 text-[15px] text-primary-foreground shadow-sm">
                {tn.brief}
                {(tn.opts.buyableOnly || tn.opts.maxPrice) && (
                  <div className="mt-1 text-xs opacity-80">
                    {[tn.opts.buyableOnly && t.buyableNowTag, tn.opts.maxPrice && t.buyoutTag(tn.opts.maxPrice)].filter(Boolean).join(" · ")}
                  </div>
                )}
              </div>
            </div>
            <div className="flex gap-3">
              <AIMark className="mt-0.5 size-8 shrink-0 rounded-lg" icon="size-4" />
              <div className="min-w-0 flex-1">
                {!tn.result && !tn.error && <Thinking step={step} />}
                {tn.error && <div className="rounded-lg bg-pill-negative px-4 py-3 text-sm text-pill-negative-ink">{t.failed(tn.error)}</div>}
                {tn.result && (
                  <Answer
                    r={tn.result}
                    onOpen={onOpen}
                    onWatch={onWatch}
                    onBrowse={() => onBrowse(tn.result!.keywords, tn.brief)}
                    onSave={() => onSaveTracker(tn.result!.keywords, tn.result!.summary || tn.brief)}
                    onMore={() => ask(tn.brief, tn.result!.picks.map((p) => p.item.name))}
                    busy={busy}
                  />
                )}
              </div>
            </div>
          </div>
        ))}
        <div ref={endRef} />
      </div>

      <div className="sticky bottom-0 z-10 mt-6 bg-gradient-to-t from-background from-70% to-transparent pt-8 pb-4">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            ask(draft);
          }}
          className="rounded-2xl border border-input bg-card shadow-lg shadow-black/5 transition-colors focus-within:border-ring/70 focus-within:ring-4 focus-within:ring-ring/10"
        >
          <textarea
            ref={inputRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                ask(draft);
              }
            }}
            rows={1}
            disabled={!llmEnabled}
            placeholder={llmEnabled ? t.placeholder : t.placeholderOff}
            className="block max-h-[200px] w-full resize-none bg-transparent px-4 pt-3.5 pb-2 text-[15px] leading-relaxed outline-none placeholder:text-muted-foreground"
          />
          <div className="flex items-center gap-1.5 px-2.5 pb-2.5">
            <button
              type="button"
              onClick={() => setBuyableOnly(!buyableOnly)}
              aria-pressed={buyableOnly}
              className={cn(
                "inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-[13px] transition-colors",
                buyableOnly ? "border-positive/50 bg-pill-positive text-pill-positive-ink" : "border-border text-muted-foreground hover:border-input hover:text-foreground",
              )}
            >
              <Zap className={cn("size-3.5", buyableOnly && "fill-current")} />
              {t.buyableNow}
            </button>
            <label
              className={cn(
                "inline-flex h-8 items-center gap-1 rounded-full border px-3 text-[13px] transition-colors focus-within:border-ring",
                maxPrice ? "border-primary/40 bg-accent text-accent-foreground" : "border-border text-muted-foreground hover:border-input",
              )}
            >
              <Coins className="size-3.5" />
              <span>{t.upTo}</span>
              <input
                type="number"
                min={0}
                value={maxPrice}
                onChange={(e) => setMaxPrice(e.target.value)}
                placeholder={t.any}
                className="num w-12 bg-transparent text-right text-foreground outline-none placeholder:text-muted-foreground [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none"
              />
              <span>TON</span>
            </label>
            <div className="ml-auto flex items-center gap-1">
              {turns.length > 0 && (
                <button
                  type="button"
                  onClick={clear}
                  aria-label={t.clearChat}
                  title={t.clearChat}
                  className="grid size-8 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <Trash2 className="size-4" />
                </button>
              )}
              <Button type="submit" size="icon" className="size-9 rounded-full" disabled={!llmEnabled || busy || !draft.trim()} aria-label={t.ask}>
                <ArrowUp className="size-4" />
              </Button>
            </div>
          </div>
        </form>
        <p className="mt-2 hidden text-center text-[11.5px] text-muted-foreground sm:block">{t.enterHint}</p>
      </div>
    </div>
  );
}

/** The AI's face in the chat: a sparkle on a soft gradient tile. */
function AIMark({ className, icon }: { className?: string; icon?: string }) {
  return (
    <span className={cn("grid place-items-center bg-gradient-to-br from-primary/25 to-primary/5 text-primary ring-1 ring-primary/20", className)}>
      <Sparkles className={icon} />
    </span>
  );
}

function Thinking({ step }: { step: number }) {
  const t = useT().askAI;
  return (
    <div className="flex h-8 items-center gap-2.5 text-sm text-muted-foreground">
      <span className="flex gap-1">
        {[0, 1, 2].map((i) => (
          <span key={i} className="size-1.5 animate-bounce rounded-full bg-primary/70" style={{ animationDelay: `${i * 150}ms` }} />
        ))}
      </span>
      <motion.span key={step} initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }}>
        {t.steps[step]}…
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
  const t = useT().askAI;
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
          {r.summary}
          {t.summary(r.searched, r.picks.length, (v) => (
            <span className="num font-medium">{v}</span>
          )).map((part, i) => (
            <Fragment key={i}>{part}</Fragment>
          ))}
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs text-muted-foreground">{t.searchedFor}</span>
          {r.keywords.slice(0, 18).map((k) => (
            <span key={k} className="data rounded-sm bg-muted px-1.5 py-0.5 text-xs">
              {k}
            </span>
          ))}
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={onBrowse}>
            <Search className="size-3.5" /> {t.browseAll}
          </Button>
          <Button size="sm" variant="outline" onClick={onSave}>
            <BookmarkPlus className="size-3.5" /> {t.saveTracker}
          </Button>
          <Button size="sm" variant="ghost" onClick={onMore} disabled={busy}>
            <Sparkles className="size-3.5" /> {t.moreLike}
          </Button>
        </div>
      </div>

      {listed.length > 0 && <PickList title={t.canGetNow} picks={listed} onOpen={onOpen} onWatch={watch} watched={watched} />}
      {owned.length > 0 && <PickList title={t.ownedOffer} picks={owned} onOpen={onOpen} onWatch={watch} watched={watched} />}
      {r.picks.length === 0 && <p className="text-sm text-muted-foreground">{t.nothingFits}</p>}

      {available.length > 0 && (
        <section>
          <h3 className="mb-1 text-sm font-semibold">{t.freshIdeas}</h3>
          <p className="mb-3 text-xs text-muted-foreground">{t.freshNote}</p>
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
  const t = useT().askAI;
  const map: Record<string, string> = {
    available: "bg-pill-positive text-pill-positive-ink",
    auction: "bg-pill-warning text-pill-warning-ink",
    sale: "bg-pill-positive text-pill-positive-ink",
    sold: "bg-secondary text-muted-foreground",
    taken: "bg-secondary text-muted-foreground",
  };
  const label = t.status[s] ?? s;
  const cls = map[s] ?? "bg-secondary text-muted-foreground";
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
  const t = useT().askAI;
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
              title={t.fitTitle}
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
                  {t.bid} <Ton v={u.price} />
                </span>
              ) : (
                <span className="text-xs text-muted-foreground">{u.status === "market" ? t.priceOn(u.chain_market) : t.offerOnly}</span>
              )}
            </div>
            <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
              <button onClick={() => onWatch(u.name)} aria-label={t.track(u.name)} className="rounded p-1.5 hover:bg-muted">
                <Star className={cn("size-4", watched[u.name] || u.watched ? "fill-warning text-warning" : "text-muted-foreground")} />
              </button>
              <a
                href={u.status === "market" && u.nft_friendly ? getgemsUrl(u.nft_friendly) : fragmentUrl(u.name)}
                target="_blank"
                rel="noreferrer"
                aria-label={t.open(u.name)}
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
