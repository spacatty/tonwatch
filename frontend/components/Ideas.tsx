"use client";

import { ChevronDown, ExternalLink, Heart, Lightbulb, MessageSquare, Sparkles, ThumbsDown, Loader2, RefreshCw, Settings2, Shuffle, Trash2, X, Zap } from "lucide-react";
import { motion } from "motion/react";
import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { fragmentUrl, getgemsUrl, studio, type Concept, type IdeasResult, type Settings, type Taste, type Username } from "@/lib/api";
import { locale, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ago, Score, Ton } from "./bits";
import { ConceptChat, type ChatTarget } from "./ConceptChat";

interface Props {
  llmEnabled: boolean;
  settings: Settings;
  onSettings: () => void;
  onOpen: (name: string) => void;
}

/** Ideas: train a taste, then let the model invent projects around names on sale within the budget. */
export function Ideas({ llmEnabled, settings, onSettings, onOpen }: Props) {
  const tAll = useT();
  const t = tAll.ideas;
  const [taste, setTaste] = useState<Taste | null>(null);
  const [sample, setSample] = useState<Username[] | null>(null);
  const [res, setRes] = useState<IdeasResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [rotating, setRotating] = useState<number | null>(null);
  const [refining, setRefining] = useState<number | null>(null); // which concept has the "I'd say" box open
  const [say, setSay] = useState("");
  const [history, setHistory] = useState<Concept[] | null>(null);
  const [tab, setTab] = useState<"studio" | "liked" | "history">("studio");
  const [chatFor, setChatFor] = useState<ChatTarget | null>(null);
  const [more, setMore] = useState<number | null>(null); // concept id a "more like this" is running for
  // the taste trainer stays folded unless there is nothing to go on yet
  const [tasteOpen, setTasteOpen] = useState(false);
  const [histQ, setHistQ] = useState("");
  const loadHistory = useCallback(() => {
    studio.history().then((r) => setHistory(r.concepts)).catch(() => setHistory([]));
  }, []);

  const loadSample = useCallback(() => {
    setSample(null);
    studio
      .tasteSample()
      .then((r) => setSample(r.items))
      .catch(() => setSample([]));
  }, []);

  useEffect(() => {
    studio
      .taste()
      .then((t) => {
        setTaste(t);
        if (t.likes.length < 3 && t.soft_likes.length === 0) setTasteOpen(true);
      })
      .catch(() => {});
    studio.ideas().then(setRes).catch(() => {});
    loadHistory();
  }, [loadHistory]);

  // names to judge are only fetched once the trainer is opened
  useEffect(() => {
    if (tasteOpen && sample == null) loadSample();
  }, [tasteOpen, sample, loadSample]);

  const judge = async (u: Username, verdict: "like" | "skip") => {
    setSample((s) => s?.filter((x) => x.name !== u.name) ?? null);
    try {
      setTaste(await studio.judge(u.name, verdict));
    } catch (e) {
      toast.error((e as Error).message);
    }
    if ((sample?.length ?? 0) <= 4) loadSample();
  };
  const unlike = async (name: string) => {
    try {
      setTaste(await studio.judge(name, "clear"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const generate = async () => {
    setBusy(true);
    try {
      setRes(await studio.generateIdeas());
      loadHistory();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const rotate = async (i: number, note?: string) => {
    setRotating(i);
    try {
      setRes(await studio.rotateIdea(i, note));
      loadHistory();
      if (note) {
        setRefining(null);
        setSay("");
      }
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setRotating(null);
    }
  };

  const likes = taste?.likes ?? [];
  const soft = taste?.soft_likes ?? [];
  const budget = settings.max_buyout ? `≤ ${settings.max_buyout.toLocaleString(locale())} TON` : t.anyPrice;

  const byId = new Map((history ?? []).map((c) => [c.id, c]));
  const likedN = (history ?? []).filter((c) => c.liked === 1).length;
  const patchConcept = (id: number, p: Partial<Concept>) => setHistory((h) => h?.map((c) => (c.id === id ? { ...c, ...p } : c)) ?? null);

  const verdict = async (id: number, v: 1 | -1) => {
    if (!id) return;
    const cur = byId.get(id)?.liked ?? 0;
    const next = cur === v ? 0 : v;
    patchConcept(id, { liked: next });
    try {
      await studio.verdict(id, next === 1 ? "like" : next === -1 ? "dislike" : "clear");
    } catch (e) {
      patchConcept(id, { liked: cur });
      toast.error((e as Error).message);
    }
  };

  // "more like this": one new project in the spirit of a liked concept, on its first name
  const moreLike = async (c: Concept) => {
    const name = c.names[0]?.name;
    if (!name) return;
    setMore(c.id);
    try {
      const r = await studio.imagine(name, `in the spirit of "${c.project}" (${c.pitch}), but a different project`);
      setHistory((h) => [{ ...r.concept, chat: 0, liked: 0 }, ...(h ?? [])]);
      toast.success(t.newConcept(r.concept.project));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setMore(null);
    }
  };

  const openChat = (c: { id: number; project: string; pitch: string; names: string[] }) => {
    if (!c.id) {
      toast(t.predatesChat);
      return;
    }
    setChatFor(c);
  };

  const forget = async (id: number) => {
    try {
      await studio.forget(id);
      setHistory((h) => h?.filter((c) => c.id !== id) ?? null);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  return (
    <div className="space-y-8">
      {/* two tabs: the studio, and everything it ever made */}
      <div className="-mt-2 flex items-center gap-1 border-b border-border">
        {(
          [
            ["studio", t.tabStudio],
            ["liked", t.tabLiked(likedN)],
            ["history", t.tabHistory(history?.length ?? 0)],
          ] as const
        ).map(([k, label]) => (
          <button
            key={k}
            onClick={() => setTab(k)}
            className={cn(
              "-mb-px h-10 border-b-2 px-3 text-[14px] transition-colors",
              tab === k ? "border-primary text-foreground" : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {(tab === "history" || tab === "liked") && (
        <HistoryTab
          key={tab}
          liked={tab === "liked"}
          items={history}
          q={histQ}
          onQ={(v) => setHistQ(v.toLowerCase().replace(/^@/, ""))}
          onOpen={onOpen}
          onForget={forget}
          onStudio={() => setTab("studio")}
          onVerdict={verdict}
          onChat={(c) => openChat({ id: c.id, project: c.project, pitch: c.pitch, names: c.names.map((n) => n.name) })}
          onMore={moreLike}
          more={more}
        />
      )}

      <ConceptChat target={chatFor} onClose={() => setChatFor(null)} onCount={(id, n) => patchConcept(id, { chat: n })} />

      {tab === "studio" && (
      <div className="space-y-10">
      {/* 1. taste: a one-line summary, opened to train */}
      <section className="panel">
        <button onClick={() => setTasteOpen((o) => !o)} aria-expanded={tasteOpen} className="flex w-full items-center gap-3 px-4 py-3 text-left">
          <Heart className={cn("size-4 shrink-0", likes.length ? "fill-primary text-primary" : "text-muted-foreground")} />
          <span className="text-[15px] font-semibold tracking-tight">{t.yourTaste}</span>
          <span className="min-w-0 flex-1 truncate text-[13px] text-muted-foreground">
            {taste == null
              ? "…"
              : likes.length === 0 && soft.length === 0
                ? t.notTrained
                : [
                    t.likes(likes.length),
                    taste.skips.length ? t.skipped(taste.skips.length) : "",
                    soft.length ? t.soft(soft.length) : "",
                    taste.profile ? t.profileBuilt : likes.length < 3 ? t.moreToProfile(3 - likes.length) : t.profileNextRun,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
          </span>
          <span className="shrink-0 text-[13px] text-muted-foreground">{tasteOpen ? t.done : t.train}</span>
          <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", tasteOpen && "rotate-180")} />
        </button>

        {tasteOpen && (
        <div className="border-t border-border px-4 pt-4 pb-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-muted-foreground">
            {t.trainHint}
          </p>
          <button onClick={loadSample} className="inline-flex h-8 items-center gap-1.5 rounded-md border border-input px-2.5 text-[13px] text-muted-foreground hover:text-foreground">
            <RefreshCw className="size-3.5" />
            {t.otherNames}
          </button>
        </div>

        {likes.length > 0 && (
          <div className="mb-3 flex flex-wrap items-center gap-1.5">
            <Heart className="size-3.5 fill-primary text-primary" />
            {likes.map((n) => (
              <span key={n} className="data inline-flex h-6 items-center gap-1 rounded bg-accent pr-1 pl-2 text-xs text-accent-foreground">
                @{n}
                <button onClick={() => unlike(n)} aria-label={t.unlikeAria(n)} className="rounded p-0.5 opacity-50 hover:opacity-100">
                  <X className="size-3" />
                </button>
              </span>
            ))}
            {taste?.profile && <span className="ml-1 text-xs text-muted-foreground">{t.profileBuiltCap}</span>}
          </div>
        )}
        {soft.length > 0 && (
          <p className="mb-3 text-xs text-muted-foreground" title={soft.map((n) => "@" + n).join(", ")}>
            {t.alsoLearning(soft.slice(0, 6).map((n) => "@" + n).join(", "), soft.length - 6)}
          </p>
        )}

        {sample == null ? (
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {Array.from({ length: 8 }, (_, i) => (
              <Skeleton key={i} className="h-[72px] rounded-lg" />
            ))}
          </div>
        ) : sample.length === 0 ? (
          <p className="panel px-5 py-6 text-sm text-muted-foreground">{t.nothingToJudge(budget, settings.min_score)}</p>
        ) : (
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {sample.slice(0, 12).map((u, i) => (
              <motion.div
                key={u.name}
                layout
                initial={{ opacity: 0, y: 4 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.18, delay: i * 0.02 }}
                className="panel flex items-center gap-3 p-3"
              >
                <button onClick={() => onOpen(u.name)} className="min-w-0 flex-1 text-left">
                  <div className="data truncate text-[15px] font-semibold">@{u.name}</div>
                  <div className="truncate text-[12px] text-muted-foreground">
                    {u.meaning || (u.category && ((tAll.filterBar.categories as Record<string, string>)[u.category] ?? u.category))} · {u.buyout != null ? <Ton v={u.buyout} /> : "—"}
                  </div>
                </button>
                <button onClick={() => judge(u, "skip")} aria-label={t.skipAria(u.name)} className="rounded-md p-2 text-muted-foreground/60 hover:bg-muted hover:text-foreground">
                  <X className="size-4" />
                </button>
                <button onClick={() => judge(u, "like")} aria-label={t.likeAria(u.name)} className="rounded-md p-2 text-muted-foreground/60 hover:bg-accent hover:text-primary">
                  <Heart className="size-4" />
                </button>
              </motion.div>
            ))}
          </div>
        )}
        </div>
        )}
      </section>

      {/* 2. ideas */}
      <section>
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-xl font-semibold tracking-tight">{t.projectsTitle}</h2>
            <p className="text-sm text-muted-foreground">
              {t.projectsSub(budget, settings.min_score)}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={onSettings} className="inline-flex h-9 items-center gap-1.5 rounded-md border border-input px-3 text-[13px] text-muted-foreground hover:text-foreground">
              <Settings2 className="size-4" />
              {t.settings}
            </button>
            <Button onClick={generate} disabled={!llmEnabled || busy} className="h-9">
              {busy ? <Loader2 className="size-4 animate-spin" /> : <Lightbulb className="size-4" />}
              {res?.ideas.length ? t.newSet : t.generate}
              <span className="data ml-1 rounded-sm bg-primary-foreground/15 px-1 text-[10px]">10 {t.cr}</span>
            </Button>
          </div>
        </div>

        {!llmEnabled && <p className="panel px-5 py-6 text-sm text-warning">{t.aiOff}</p>}

        {busy && (
          <div className="grid gap-3 md:grid-cols-2">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-44 rounded-lg" />
            ))}
          </div>
        )}

        {!busy && res && res.ideas.length > 0 && (
          <>
            <div className="grid gap-3 md:grid-cols-2">
              {res.ideas.map((idea, i) => (
                <motion.article
                  key={idea.project + i}
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ duration: 0.22, delay: i * 0.05 }}
                  className={cn("panel relative flex flex-col p-5", rotating === i && "opacity-50")}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="label">{t.concept(i + 1)}</div>
                    <div className="ml-auto flex items-center gap-0.5">
                      <VerdictBtns liked={byId.get(idea.id)?.liked ?? 0} onVerdict={(v) => verdict(idea.id, v)} />
                    </div>
                    {/* the chat leads: it is how a concept becomes a plan */}
                    <button
                      onClick={() => openChat({ id: idea.id, project: idea.project, pitch: idea.pitch, names: idea.items.map((x) => x.item.name) })}
                      title={t.chatTitle}
                      className="inline-flex h-7 items-center gap-1.5 rounded border border-primary/40 bg-primary/10 px-2 text-[12px] font-medium text-primary transition-colors hover:bg-primary/20"
                    >
                      <MessageSquare className="size-3.5" />
                      {byId.get(idea.id)?.chat ? t.chatCount(byId.get(idea.id)!.chat / 2) : t.ask}
                      <span className="data text-[10px] font-normal opacity-70">2 {t.cr}</span>
                    </button>
                    <button
                      onClick={() => rotate(i)}
                      disabled={rotating != null}
                      title={t.rotateTitle}
                      className="inline-flex h-7 items-center gap-1 rounded border border-input px-2 text-[12px] text-muted-foreground hover:border-primary hover:text-primary disabled:opacity-50"
                    >
                      {rotating === i ? <Loader2 className="size-3.5 animate-spin" /> : <Shuffle className="size-3.5" />}
                      {t.rotate} <span className="data text-[10px]">3 {t.cr}</span>
                    </button>
                  </div>
                  <h3 className="mt-1 text-lg font-semibold tracking-tight">{idea.project}</h3>
                  <p className="mt-1 text-[14px] leading-snug">{idea.pitch}</p>
                  {idea.built_with && (
                    <p className="mt-2 text-[13px] text-muted-foreground">
                      <span className="text-foreground/80">{t.builtWith}</span> {idea.built_with}
                    </p>
                  )}
                  {idea.first_move && (
                    <p className="mt-2 text-[13px] text-muted-foreground">
                      <span className="text-foreground/80">{t.firstMove}</span> {idea.first_move}
                    </p>
                  )}
                  {refining === i ? (
                    <form
                      onSubmit={(e) => {
                        e.preventDefault();
                        if (say.trim()) rotate(i, say.trim());
                      }}
                      className="mt-3 flex gap-1.5"
                    >
                      <input
                        autoFocus
                        value={say}
                        onChange={(e) => setSay(e.target.value.slice(0, 300))}
                        placeholder={t.sayPlaceholder}
                        className="data h-8 min-w-0 flex-1 rounded-md border border-input bg-transparent px-2.5 text-[13px] outline-none focus:border-primary"
                      />
                      <Button type="submit" size="sm" disabled={!say.trim() || rotating != null} className="h-8">
                        {t.makeItMine}
                      </Button>
                      <Button type="button" size="sm" variant="ghost" onClick={() => setRefining(null)} className="h-8">
                        {tAll.common.cancel}
                      </Button>
                    </form>
                  ) : (
                    <button
                      onClick={() => {
                        setRefining(i);
                        setSay("");
                      }}
                      disabled={rotating != null}
                      className="mt-2 self-start text-[12px] text-muted-foreground hover:text-primary"
                    >
                      {t.sayOpen} <span className="opacity-70">{t.sayOpenHint}</span>
                    </button>
                  )}
                  <ul className="mt-4 space-y-2 border-t border-border/70 pt-3">
                    {idea.items.map(({ item: u, angle }) => (
                      <li key={u.name} className="flex items-center gap-3">
                        <button onClick={() => onOpen(u.name)} className="min-w-0 flex-1 text-left">
                          <div className="flex items-center gap-2">
                            <span className="data truncate text-[15px] font-semibold">@{u.name}</span>
                            <Score u={u} className="hidden sm:inline-flex" />
                          </div>
                          <div className="truncate text-[12px] text-muted-foreground">{angle}</div>
                        </button>
                        {u.buyout != null && (
                          <a
                            href={u.status === "market" && u.nft_friendly ? getgemsUrl(u.nft_friendly) : fragmentUrl(u.name)}
                            target="_blank"
                            rel="noreferrer"
                            className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md bg-positive px-2.5 text-[13px] font-semibold text-background hover:bg-positive/85"
                          >
                            <Zap className="size-3.5 fill-current" />
                            <Ton v={u.buyout} />
                            <ExternalLink className="size-3 opacity-70" />
                          </a>
                        )}
                      </li>
                    ))}
                  </ul>
                </motion.article>
              ))}
            </div>
            <p className="mt-3 text-xs text-muted-foreground">
              {t.generated(ago(res.generated_at), res.searched, res.usage.prompt_tokens + res.usage.completion_tokens)}
              {res.usage.cost ? ` · $${res.usage.cost.toFixed(4)}` : ""}
              {res.profile ? t.steered : t.noProfile}
            </p>
          </>
        )}

        {!busy && res && res.ideas.length === 0 && llmEnabled && (
          <p className={cn("panel px-5 py-8 text-sm text-muted-foreground")}>{t.noIdeas}</p>
        )}
      </section>

      </div>
      )}
    </div>
  );
}

/** The archive: every concept ever generated, as cards, newest first, filterable by name or title. */
type HFilter = "all" | "liked" | "disliked" | "chat";

function VerdictBtns({ liked, onVerdict }: { liked: number; onVerdict: (v: 1 | -1) => void }) {
  const t = useT().ideas;
  return (
    <>
      <button onClick={() => onVerdict(1)} aria-label={t.likeConceptAria} title={t.likeConceptTitle} className={cn("inline-flex size-7 items-center justify-center rounded hover:bg-muted", liked === 1 ? "text-primary" : "text-muted-foreground/50 hover:text-primary")}>
        <Heart className={cn("size-4", liked === 1 && "fill-current")} />
      </button>
      <button onClick={() => onVerdict(-1)} aria-label={t.dislikeConceptAria} title={t.dislikeConceptTitle} className={cn("inline-flex size-7 items-center justify-center rounded hover:bg-muted", liked === -1 ? "text-foreground" : "text-muted-foreground/50 hover:text-foreground")}>
        <ThumbsDown className={cn("size-4", liked === -1 && "fill-current")} />
      </button>
    </>
  );
}

function HistoryTab({
  liked,
  items,
  q,
  onQ,
  onOpen,
  onForget,
  onStudio,
  onVerdict,
  onChat,
  onMore,
  more,
}: {
  liked?: boolean;
  items: Concept[] | null;
  q: string;
  onQ: (v: string) => void;
  onOpen: (n: string) => void;
  onForget: (id: number) => void;
  onStudio: () => void;
  onVerdict: (id: number, v: 1 | -1) => void;
  onChat: (c: Concept) => void;
  onMore: (c: Concept) => void;
  more: number | null;
}) {
  const t = useT().ideas;
  const [filter, setFilter] = useState<HFilter>("all");
  if (items == null)
    return (
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {Array.from({ length: 6 }, (_, i) => (
          <Skeleton key={i} className="h-36 rounded-lg" />
        ))}
      </div>
    );
  if (items.length === 0)
    return (
      <div className="panel flex flex-col items-start gap-3 px-6 py-10">
        <p className="text-sm text-muted-foreground">{t.historyEmpty}</p>
        <Button variant="outline" onClick={onStudio} className="h-8">
          {t.goStudio}
        </Button>
      </div>
    );
  const f: HFilter = liked ? "liked" : filter;
  const shown = items
    .filter((c) => f === "all" || (f === "liked" ? c.liked === 1 : f === "disliked" ? c.liked === -1 : c.chat > 0))
    .filter((c) => !q || c.project.toLowerCase().includes(q) || c.names.some((n) => n.name.includes(q)));
  const count = (k: HFilter) => (k === "all" ? items.length : items.filter((c) => (k === "liked" ? c.liked === 1 : k === "disliked" ? c.liked === -1 : c.chat > 0)).length);
  const names = new Set(items.flatMap((c) => c.names.map((n) => n.name)));
  return (
    <section>
      <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">{liked ? t.likedTitle : t.historyTitle}</h2>
          <p className="text-sm text-muted-foreground">
            {liked
              ? t.likedSub
              : t.historySub(items.length, names.size)}
          </p>
          {!liked && (
            <div className="mt-3 flex gap-0.5 rounded-md bg-muted p-0.5 text-[13px]">
              {(
                [
                  ["all", t.filter.all],
                  ["liked", t.filter.liked],
                  ["disliked", t.filter.disliked],
                  ["chat", t.filter.chat],
                ] as const
              ).map(([k, label]) => (
                <button key={k} onClick={() => setFilter(k)} className={cn("h-7 rounded-[4px] px-2.5 transition-colors", filter === k ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}>
                  {label} <span className="data text-[11px] text-muted-foreground">{count(k)}</span>
                </button>
              ))}
            </div>
          )}
        </div>
        <input
          value={q}
          onChange={(e) => onQ(e.target.value)}
          placeholder={t.filterPlaceholder}
          className="data h-9 w-64 rounded-md border border-input bg-transparent px-3 text-[13px] outline-none focus:border-primary"
        />
      </div>
      {shown.length === 0 ? (
        <p className="panel px-5 py-8 text-sm text-muted-foreground">
          {q ? t.noMatch(q) : f === "liked" ? t.noLiked : f === "disliked" ? t.noDisliked : f === "chat" ? t.noChats : t.nothingHere}
        </p>
      ) : (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {shown.slice(0, 150).map((c) => (
            <article key={c.id} className="panel group flex flex-col p-4">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <h3 className="truncate text-[16px] font-semibold tracking-tight">{c.project}</h3>
                  <div className="data mt-0.5 text-[11px] text-muted-foreground">
                    {(t.source as Record<string, string>)[c.source] ?? c.source} · {ago(c.ts)}
                    {c.form ? ` · ${c.form}` : ""}
                  </div>
                </div>
                <button onClick={() => onForget(c.id)} aria-label={t.forgetAria} title={t.forget} className="shrink-0 rounded p-1 text-muted-foreground/40 opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100">
                  <Trash2 className="size-3.5" />
                </button>
              </div>
              <p className="mt-2 line-clamp-3 text-[13.5px] leading-snug">{c.pitch}</p>
              {c.built_with && (
                <p className="mt-1.5 line-clamp-2 text-[12.5px] leading-snug text-muted-foreground">
                  <span className="text-foreground/80">{t.builtWith}</span> {c.built_with}
                </p>
              )}
              {c.how && <p className="mt-1.5 line-clamp-2 text-[12.5px] leading-snug text-muted-foreground">{c.how}</p>}
              {c.note && <p className="mt-1.5 text-[12px] text-muted-foreground/80">{t.youSaid} {c.note}</p>}
              <div className="mt-auto flex flex-wrap gap-1 pt-3">
                {c.names.map((n) => (
                  <button key={n.name} onClick={() => onOpen(n.name)} title={n.angle} className="data inline-flex h-6 items-center rounded bg-accent px-2 text-[12px] text-accent-foreground hover:text-primary">
                    @{n.name}
                  </button>
                ))}
              </div>
              <div className="mt-3 flex items-center gap-1 border-t border-border/70 pt-2.5">
                <VerdictBtns liked={c.liked} onVerdict={(v) => onVerdict(c.id, v)} />
                <button onClick={() => onChat(c)} className="ml-1 inline-flex h-7 items-center gap-1.5 rounded px-1.5 text-[12px] text-muted-foreground hover:bg-muted hover:text-foreground">
                  <MessageSquare className="size-3.5" />
                  {c.chat ? t.chatCount(c.chat / 2) : t.ask}
                </button>
                {c.liked === 1 && c.names.length > 0 && (
                  <button onClick={() => onMore(c)} disabled={more != null} title={t.moreTitle} className="ml-auto inline-flex h-7 items-center gap-1.5 rounded px-1.5 text-[12px] text-muted-foreground hover:bg-muted hover:text-primary disabled:opacity-50">
                    {more === c.id ? <Loader2 className="size-3.5 animate-spin" /> : <Sparkles className="size-3.5" />}
                    {t.moreLike}
                  </button>
                )}
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
