"use client";

import { Bookmark, Lock, MoreHorizontal, Pencil, Plus, RefreshCw, Wand2, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { api, studio, type AIFilterResult, type Meta, type Settings, type StatsResp, type Tracker, type Username } from "@/lib/api";
import { getT, num, useT } from "@/lib/i18n";
import { buildParams, buyableApplies, DEFAULT_FILTERS, normalizeFilters, PRESET_TRACKERS, searchOnlyParams, trackerFilters, viewStatuses, VIEWS, type Filters, type ViewKey } from "@/lib/views";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { AccountDialog } from "./AccountDialog";
import { AdminPanel } from "./AdminPanel";
import { AIFilter } from "./AIFilter";
import { RuleTree } from "./RuleTree";
import { AskAI } from "./AskAI";
import { useAuth } from "./Auth";
import { medianKey, type Medians } from "./bits";
import { CommandPalette } from "./CommandPalette";
import { DetailDrawer } from "./DetailDrawer";
import { FilterBar } from "./FilterBar";
import { Ideas } from "./Ideas";
import { Portfolio } from "./Portfolio";
import { SettingsDialog } from "./SettingsDialog";
import { CreditsDialog } from "./CreditsDialog";
import { SupportDialog } from "./Support";
import { JobsPanel, useJobs } from "./JobsPanel";
import { NameTable } from "./NameTable";
import { MobileBar, Sidebar, type Mode } from "./Sidebar";
import { Snipes } from "./Snipes";

const PAGE = 60;
// per account: two people sharing a browser keep their own view and starter trackers
const lsKey = (uid: number) => `fragwatch.state.v5.${uid}`;
const presetsKey = (uid: number) => `fragwatch.presets.v2.${uid}`;

/** When price events began to be recorded: "no bids for N days" can only find names N days after it. */
const PRICE_HISTORY_START = 1793145600; // 2026-10-28 UTC

/** Where an AI filter came from: shown above the table it produced. */
type AIRule = { prompt: string; summary: string; could_not?: string };

function loadState(uid: number, admin: boolean): { mode: Mode; filters: Filters; aiRule: AIRule | null } | null {
  try {
    // the single-user install kept its state under v4; the first (admin) account inherits it
    for (const key of [lsKey(uid), ...(admin ? ["fragwatch.state.v4"] : [])]) {
      const s = JSON.parse(localStorage.getItem(key) ?? "null");
      if (s) return { mode: s.mode?.kind === "admin" && !admin ? { kind: "home" } : s.mode, filters: normalizeFilters(s.filters), aiRule: s.aiRule ?? null };
    }
  } catch {}
  return null;
}

/** The tracker's search as a query string, without paging or sorting, for the backend to count. */
function trackerParams(view: (typeof VIEWS)[number], f: Filters) {
  const p = buildParams(view, f);
  p.delete("sort");
  return p.toString();
}

function suggestName(f: Filters, viewLabel: string) {
  if (f.topic) return f.topic.slice(0, 40);
  if (f.digits) return getT().dashboard.suggest.digits(f.digits);
  if (f.q) return `“${f.q}”`;
  if (f.keywords.length) return f.keywords.slice(0, 3).join(", ");
  if (f.maxBuyout) return getT().dashboard.suggest.under(viewLabel, f.maxBuyout);
  return viewLabel;
}

const sameFilters = (a: Filters, b: Filters) => JSON.stringify(a) === JSON.stringify(b);

export default function Dashboard() {
  const { user } = useAuth();
  const t = useT().dashboard;
  const isAdmin = user.role === "admin";
  const [mode, setMode] = useState<Mode>({ kind: "home" });
  const [filters, setFilters] = useState<Filters>(DEFAULT_FILTERS);
  const [aiRule, setAiRule] = useState<AIRule | null>(null);
  const [aiDraft, setAiDraft] = useState<string | undefined>(undefined);
  const [hydrated, setHydrated] = useState(false);

  const [items, setItems] = useState<Username[]>([]);
  const [total, setTotal] = useState(0);
  // free accounts: the most rows the server shows, and the filters it left out
  const [cap, setCap] = useState<number | null>(null);
  const [locked, setLocked] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState<StatsResp | null>(null);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [trackers, setTrackers] = useState<Tracker[]>([]);
  const [medians, setMedians] = useState<Medians>({});
  const [settings, setSettings] = useState<Settings>({ max_buyout: 100, min_score: 7, kinds: [], daily_credits: 100, about: "", avoid: "", plan: "free", telegram_chat: "" });
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [creditsOpen, setCreditsOpen] = useState(false);
  const [supportOpen, setSupportOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);

  const [selected, setSelected] = useState<string | null>(null);
  const [jobsOpen, setJobsOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [saveDlg, setSaveDlg] = useState<{ name: string; view: ViewKey; filters: Filters; renameId?: number } | null>(null);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const reqId = useRef(0);
  const sentinel = useRef<HTMLDivElement>(null);

  const tracker = mode.kind === "tracker" ? trackers.find((t) => t.id === mode.id) : undefined;
  const viewKey: ViewKey = mode.kind === "view" ? mode.key : tracker ? (tracker.view as ViewKey) : "all";
  const view = VIEWS.find((v) => v.key === viewKey) ?? VIEWS[VIEWS.length - 1];
  const listMode = mode.kind === "view" || mode.kind === "tracker";
  const jobs = useJobs(isAdmin ? 15000 : 0);

  useEffect(() => {
    const s = loadState(user.id, isAdmin);
    if (s) {
      setMode(s.mode);
      setFilters(s.filters);
      setAiRule(s.aiRule);
    }
    setHydrated(true);
    api.meta().then(setMeta).catch(() => {});
    studio.settings().then(setSettings).catch(() => {});
    // the daily credits wait to be claimed: say so once on opening, with the way there
    studio
      .credits()
      .then((c) => {
        if (c.claim.available && c.claim.adds > 0)
          toast(getT().credits.claimToast(num(c.claim.adds)), { id: "claim", duration: 10000, action: { label: getT().credits.claim, onClick: () => setCreditsOpen(true) } });
      })
      .catch(() => {});
  }, [user.id, isAdmin]);
  useEffect(() => {
    if (!hydrated) return;
    try {
      localStorage.setItem(lsKey(user.id), JSON.stringify({ mode, filters, aiRule }));
    } catch {}
  }, [mode, filters, aiRule, hydrated, user.id]);

  // typical price per AI score and length, the yardstick for "deals": what such names actually sold
  // for, or (where there are too few sales) what sellers ask. Asks run far above real sales.
  useEffect(() => {
    const f = () =>
      api
        .medians()
        .then((m) =>
          setMedians(
            Object.fromEntries(
              m.flatMap((x) => {
                const key = medianKey(x.score, x.len);
                return x.sold_count >= 30 ? [[key, x.sold]] : x.ask_count >= 10 ? [[key, x.ask]] : [];
              }),
            ),
          ),
        )
        .catch(() => {});
    f();
    const t = setInterval(f, 5 * 60_000);
    return () => clearInterval(t);
  }, []);

  // trackers: load, create the starter set once, keep counts fresh
  const loadTrackers = useCallback(() => api.trackers().then(setTrackers).catch(() => {}), []);
  useEffect(() => {
    let alive = true;
    (async () => {
      const ts = await api.trackers().catch(() => null);
      if (!alive || !ts) return;
      let seeded = true;
      try {
        seeded = !!localStorage.getItem(presetsKey(user.id)) || (isAdmin && !!localStorage.getItem("fragwatch.presets.v1"));
      } catch {}
      if (ts.length === 0 && !seeded) {
        for (const p of PRESET_TRACKERS) {
          const v = VIEWS.find((x) => x.key === p.view)!;
          const f = { ...DEFAULT_FILTERS, ...p.filters };
          await api.createTracker({ name: p.name, view: p.view, filters: JSON.stringify(f), params: trackerParams(v, f) }).catch(() => {});
        }
        try {
          localStorage.setItem(presetsKey(user.id), "1");
        } catch {}
        setTrackers(await api.trackers().catch(() => []));
      } else setTrackers(ts);
    })();
    const t = setInterval(loadTrackers, 30_000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [loadTrackers, user.id, isAdmin]);

  const baseKey = useMemo(() => buildParams(view, filters).toString(), [view, filters]);

  const loadPage = useCallback(
    async (offset: number) => {
      const id = ++reqId.current;
      setLoading(true);
      try {
        const p = new URLSearchParams(baseKey);
        p.set("limit", String(PAGE));
        p.set("offset", String(offset));
        const r = await api.list(p);
        if (id !== reqId.current) return;
        setItems((xs) => (offset === 0 ? r.items : [...xs, ...r.items.filter((u) => !xs.some((x) => x.name === u.name))]));
        setTotal(r.total);
        setCap(r.cap ?? null);
        setLocked(r.locked?.filter((k) => k !== "depth") ?? []);
      } catch (e) {
        if (id === reqId.current) toast.error(getT().dashboard.loadFailed((e as Error).message), { id: "load" });
      } finally {
        if (id === reqId.current) setLoading(false);
      }
    },
    [baseKey],
  );

  useEffect(() => {
    if (!hydrated || !listMode) return;
    const t = setTimeout(() => loadPage(0), 180);
    return () => clearTimeout(t);
  }, [loadPage, hydrated, listMode, user.unlocked]);

  const reachable = cap != null ? Math.min(total, cap) : total;

  // infinite scroll
  useEffect(() => {
    const el = sentinel.current;
    if (!el) return;
    const io = new IntersectionObserver((es) => {
      if (es[0].isIntersecting && !loading && items.length < reachable) loadPage(items.length);
    });
    io.observe(el);
    return () => io.disconnect();
  }, [loading, items.length, reachable, loadPage]);

  // A search can match names the view or quality filters hide. Count every match to say so.
  const [allMatches, setAllMatches] = useState<number | null>(null);
  const searchKey = searchOnlyParams(filters).toString();
  useEffect(() => {
    if (!searchKey || !listMode) {
      setAllMatches(null);
      return;
    }
    let alive = true;
    const t = setTimeout(() => {
      const p = new URLSearchParams(searchKey);
      p.set("limit", "1");
      api
        .list(p)
        .then((r) => alive && setAllMatches(r.total))
        .catch(() => {});
    }, 250);
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [searchKey, listMode]);
  const hiddenMatches = allMatches != null && !loading ? Math.max(0, allMatches - total) : 0;

  // the finding under the title: on auction views, how many good names close within the hour
  const [closingSoon, setClosingSoon] = useState<number | null>(null);
  const auctionView = viewKey === "ending" || viewKey === "auction";
  const closingKey = listMode && auctionView ? buildParams(view, { ...filters, minScore: Math.max(7, filters.minScore), unscored: false }, { ending_in: "3600", live: "1", limit: "1" }).toString() : "";
  useEffect(() => {
    if (!closingKey) {
      setClosingSoon(null);
      return;
    }
    let alive = true;
    api
      .list(new URLSearchParams(closingKey))
      .then((r) => alive && setClosingSoon(r.total))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [closingKey, total]);

  useEffect(() => {
    const f = () => api.stats().then(setStats).catch(() => {});
    f();
    const t = setInterval(f, 10_000);
    return () => clearInterval(t);
  }, []);

  const set = <K extends keyof Filters>(k: K, v: Filters[K]) => setFilters((f) => ({ ...f, [k]: v }));
  const patchFilters = (p: Partial<Filters>) => setFilters((f) => ({ ...f, ...p }));
  const patchItem = (u: Username) => setItems((xs) => xs.map((x) => (x.name === u.name ? u : x)));

  const refreshOne = async (name: string) => {
    setBusy((b) => ({ ...b, [name]: true }));
    try {
      const r = await api.refresh(name);
      if (r.item) patchItem(r.item);
    } catch (e) {
      toast.error(`@${name}: ${(e as Error).message}`);
    } finally {
      setBusy((b) => ({ ...b, [name]: false }));
    }
  };

  const toggleWatch = async (u: Username) => {
    patchItem({ ...u, watched: !u.watched });
    try {
      await api.watch(u.name, !u.watched);
      if (!u.watched) studio.signal(u.name, "star");
      toast(u.watched ? t.stoppedTracking(u.name) : t.tracking(u.name), {
        action: { label: t.undo, onClick: () => api.watch(u.name, u.watched).then(() => patchItem(u)) },
      });
    } catch {
      patchItem(u);
    }
  };

  const hideName = async (u: Username) => {
    setItems((xs) => xs.filter((x) => x.name !== u.name));
    setTotal((t) => Math.max(0, t - 1));
    try {
      await api.hide(u.name, !u.hidden);
      toast(u.hidden ? t.back(u.name) : t.hidden(u.name), {
        action: { label: t.undo, onClick: () => api.hide(u.name, u.hidden).then(() => loadPage(0)) },
      });
    } catch (e) {
      toast.error((e as Error).message);
      loadPage(0);
    }
  };
  const likeName = async (u: Username) => {
    try {
      await studio.judge(u.name, "like");
      toast(t.liked(u.name));
    } catch (e) {
      toast.error((e as Error).message);
    }
  };
  const [imagineFor, setImagineFor] = useState<string | null>(null);
  const imagineName = (name: string) => {
    setImagineFor(name);
    setSelected(name);
  };

  const watchName = async (name: string) => {
    await api.watch(name, true).catch(() => {});
    studio.signal(name, "star");
    toast(t.tracking(name));
  };

  const track = async (name: string) => {
    const tid = toast.loading(t.checking(name));
    try {
      await api.refresh(name);
      await api.watch(name, true);
      toast.success(t.added(name), { id: tid });
      setSelected(name);
      if (listMode) loadPage(0);
    } catch (e) {
      toast.error(`@${name}: ${(e as Error).message}`, { id: tid });
    }
  };

  const goTop = () => window.scrollTo({ top: 0 });
  const changeView = (v: ViewKey) => {
    setMode({ kind: "view", key: v });
    setItems([]);
    setTotal(0);
    goTop();
  };
  // an AI filter lands on the live table, its rules first; navigating anywhere else drops the bar
  const applyAI = (r: AIFilterResult) => {
    const v = (VIEWS.some((x) => x.key === r.view) ? r.view : "buyable") as ViewKey;
    setFilters(normalizeFilters({ ...DEFAULT_FILTERS, ...r.filters }));
    setAiRule({ prompt: r.prompt, summary: r.summary, could_not: r.could_not });
    changeView(v);
  };
  const renameTracker = (tr: Tracker) => setSaveDlg({ name: tr.name, view: tr.view as ViewKey, filters: trackerFilters(tr.filters), renameId: tr.id });
  const duplicateTracker = async (tr: Tracker) => {
    const name = getT().dashboard.copyOf(tr.name);
    try {
      const r = await api.createTracker({ name, view: tr.view, filters: tr.filters, params: tr.params });
      toast.success(getT().dashboard.duplicated(name));
      await loadTrackers();
      openTracker({ ...tr, id: r.id, name, new: 0 });
    } catch (e) {
      toast.error((e as Error).message);
    }
  };
  const removeTracker = async (tr: Tracker) => {
    if (!confirm(t.confirmDelete(tr.name))) return;
    await api.deleteTracker(tr.id);
    toast(t.deleted(tr.name));
    if (mode.kind === "tracker" && mode.id === tr.id) setMode({ kind: "view", key: tr.view as ViewKey });
    loadTrackers();
  };

  const browse = (v: ViewKey, patch: Partial<Filters>) => {
    setFilters({ ...DEFAULT_FILTERS, ...patch });
    changeView(v);
  };
  const openTracker = (tr: Tracker) => {
    try {
      setFilters(trackerFilters(tr.filters));
    } catch {
      setFilters(DEFAULT_FILTERS);
    }
    setMode({ kind: "tracker", id: tr.id });
    setItems([]);
    setTotal(0);
    goTop();
    api.trackerSeen(tr.id).then(loadTrackers).catch(() => {});
  };

  const trackerDirty = (() => {
    if (!tracker) return false;
    try {
      return !sameFilters(trackerFilters(tracker.filters), filters);
    } catch {
      return true;
    }
  })();

  const saveTracker = async () => {
    if (!saveDlg) return;
    const name = saveDlg.name.trim();
    if (!name) return;
    try {
      if (saveDlg.renameId) {
        await api.updateTracker(saveDlg.renameId, { name });
        toast.success(t.renamed);
      } else {
        const v = VIEWS.find((x) => x.key === saveDlg.view)!;
        const r = await api.createTracker({ name, view: saveDlg.view, filters: JSON.stringify(saveDlg.filters), params: trackerParams(v, saveDlg.filters) });
        setFilters(saveDlg.filters);
        setMode({ kind: "tracker", id: r.id });
        toast.success(t.saved(name));
      }
      setSaveDlg(null);
      loadTrackers();
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const updateTracker = async () => {
    if (!tracker) return;
    await api.updateTracker(tracker.id, { filters: JSON.stringify(filters), params: trackerParams(view, filters) });
    await api.trackerSeen(tracker.id);
    toast.success(t.updated(tracker.name));
    loadTrackers();
  };

  const deleteTracker = async () => {
    if (!tracker || !confirm(t.confirmDelete(tracker.name))) return;
    await api.deleteTracker(tracker.id);
    toast(t.deleted(tracker.name));
    setMode({ kind: "view", key: viewKey });
    loadTrackers();
  };

  const openSave = (f = filters, v: ViewKey = viewKey) => setSaveDlg({ name: suggestName(f, VIEWS.find((x) => x.key === v)!.label), view: v, filters: f });

  const title =
    mode.kind === "home" || mode.kind === "ai" || mode.kind === "aifilter" || mode.kind === "ideas" || mode.kind === "portfolio" || mode.kind === "admin"
      ? t.title[mode.kind]
      : tracker
        ? tracker.name
        : view.label;
  const subtitle =
    mode.kind === "home" || mode.kind === "ai" || mode.kind === "aifilter" || mode.kind === "ideas" || mode.kind === "portfolio" || mode.kind === "admin"
      ? t.subtitle[mode.kind]
      : closingSoon
        ? t.subtitle.closing(num(closingSoon), closingSoon)
        : tracker
          ? t.subtitle.tracker(view.label)
          : `${view.blurb}.`;

  const sidebarProps = {
    mode,
    onView: (v: ViewKey) => {
      setAiRule(null);
      changeView(v);
    },
    onTracker: (tr: Tracker) => {
      setAiRule(null);
      openTracker(tr);
    },
    onAIFilter: () => {
      setAiDraft(undefined);
      setMode({ kind: "aifilter" });
      goTop();
    },
    onTrackerRename: renameTracker,
    onTrackerDuplicate: duplicateTracker,
    onTrackerDelete: removeTracker,
    onHome: () => {
      setMode({ kind: "home" });
      goTop();
    },
    onAI: () => {
      setMode({ kind: "ai" });
      goTop();
    },
    onIdeas: () => {
      setMode({ kind: "ideas" });
      goTop();
    },
    onPortfolio: () => {
      setMode({ kind: "portfolio" });
      goTop();
    },
    onAdmin: () => {
      setMode({ kind: "admin" });
      goTop();
    },
    onAccount: () => setAccountOpen(true),
    onNewTracker: () => openSave(),
    trackers,
    stats,
    meta,
    jobsRunning: jobs.filter((j) => j.running).length,
    onJobs: () => setJobsOpen(true),
    onSearch: () => setPaletteOpen(true),
    onCredits: () => setCreditsOpen(true),
    onSupport: () => setSupportOpen(true),
  };

  return (
    <div className="flex min-h-dvh">
      <Sidebar {...sidebarProps} />
      <div className="min-w-0 flex-1">
        <MobileBar {...sidebarProps} title={title} />
        <main className="mx-auto max-w-[1400px] px-4 pt-5 pb-16 sm:px-6 lg:px-10 lg:pt-8">
          <header className="mb-6 flex flex-wrap items-end justify-between gap-4">
            <div className="min-w-0">
              <h1 className="flex items-center gap-2.5 text-2xl leading-tight font-bold tracking-tight lg:text-[30px]">
                {tracker && <Bookmark className="size-6 shrink-0 text-primary" />}
                <span className="truncate">{title}</span>
              </h1>
              <p className="mt-1 max-w-[75ch] text-[15px] text-muted-foreground">{subtitle}</p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {listMode &&
                (tracker ? (
                  <>
                    {trackerDirty && (
                      <Button className="h-9" onClick={updateTracker}>
                        {t.updateTracker}
                      </Button>
                    )}
                    <DropdownMenu>
                      <DropdownMenuTrigger render={<Button variant="outline" size="icon" className="size-9" aria-label={t.trackerOptions} />}>
                        <MoreHorizontal className="size-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-52">
                        <DropdownMenuItem onClick={() => setSaveDlg({ name: tracker.name, view: viewKey, filters, renameId: tracker.id })}>{t.rename}</DropdownMenuItem>
                        <DropdownMenuItem onClick={() => openSave()}>{t.saveAsNew}</DropdownMenuItem>
                        {trackerDirty && <DropdownMenuItem onClick={() => openTracker(tracker)}>{t.discard}</DropdownMenuItem>}
                        <DropdownMenuSeparator />
                        <DropdownMenuItem variant="destructive" onClick={deleteTracker}>
                          {t.deleteTracker}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </>
                ) : (
                  <Button variant="outline" className="h-9" onClick={() => openSave()}>
                    <Bookmark className="size-4" />
                    {t.saveAsTracker}
                  </Button>
                ))}
              {mode.kind !== "ai" && mode.kind !== "aifilter" && mode.kind !== "ideas" && mode.kind !== "portfolio" && mode.kind !== "admin" && (
                <Button onClick={() => setPaletteOpen(true)} className="h-9 px-4">
                  <Plus className="size-4" />
                  {t.trackUsername}
                </Button>
              )}
            </div>
          </header>

          {mode.kind === "home" && (
            <Snipes settings={settings} onSettings={() => setSettingsOpen(true)} trackers={trackers} medians={medians} categories={meta?.categories ?? []} onOpen={setSelected} onWatch={watchName} onTracker={openTracker} onBrowse={browse} />
          )}

          {mode.kind === "ideas" && <Ideas llmEnabled={!!meta?.llm_enabled} settings={settings} onSettings={() => setSettingsOpen(true)} onOpen={setSelected} />}

          {mode.kind === "portfolio" && <Portfolio onOpen={setSelected} />}

          {mode.kind === "admin" && isAdmin && <AdminPanel onJobs={() => setJobsOpen(true)} />}

          {mode.kind === "aifilter" && <AIFilter llmEnabled={!!meta?.llm_enabled} initial={aiDraft} onApply={applyAI} />}

          {mode.kind === "ai" && (
            <AskAI
              llmEnabled={!!meta?.llm_enabled}
              onOpen={setSelected}
              onWatch={watchName}
              onBrowse={(keywords, topic) => browse("all", { keywords, topic, minScore: 3 })}
              onSaveTracker={(keywords, topic) => openSave({ ...DEFAULT_FILTERS, keywords, topic, minScore: 3 }, "all")}
            />
          )}

          {listMode && (
            <div className="space-y-4">
              {aiRule && mode.kind === "view" && (
                <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-primary/30 bg-accent/60 px-4 py-3">
                  <Wand2 className="size-4 shrink-0 text-primary" />
                  <div className="min-w-0 flex-1">
                    <div className="text-[13px] font-medium text-accent-foreground">
                      {getT().aiFilter.bar}
                      {aiRule.summary ? ` · ${aiRule.summary}` : ""}
                    </div>
                    <div className="truncate text-[12.5px] text-muted-foreground" title={aiRule.prompt}>
                      “{aiRule.prompt}”
                    </div>
                    {aiRule.could_not && (
                      <div className="mt-0.5 text-[12.5px] text-warning">
                        {getT().aiFilter.couldNot}: {aiRule.could_not}
                      </div>
                    )}
                  </div>
                  <div className="flex flex-wrap items-center gap-1.5">
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-8 gap-1.5"
                      onClick={() => {
                        loadPage(0);
                        toast(getT().aiFilter.refreshed, { id: "ai-refresh" });
                      }}
                    >
                      <RefreshCw className="size-3.5" />
                      {getT().aiFilter.refresh}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-8 gap-1.5"
                      onClick={() => {
                        setAiDraft(aiRule.prompt);
                        setMode({ kind: "aifilter" });
                      }}
                    >
                      <Pencil className="size-3.5" />
                      {getT().aiFilter.edit}
                    </Button>
                    <Button size="sm" className="h-8 gap-1.5" onClick={() => setSaveDlg({ name: (aiRule.summary || aiRule.prompt).slice(0, 80), view: viewKey, filters })}>
                      <Bookmark className="size-3.5" />
                      {getT().aiFilter.save}
                    </Button>
                    <button onClick={() => setAiRule(null)} aria-label={getT().aiFilter.dismiss} className="grid size-8 place-items-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground">
                      <X className="size-4" />
                    </button>
                  </div>
                  {Number(filters.quietDays) > 0 && Date.now() / 1000 < PRICE_HISTORY_START + Number(filters.quietDays) * 86400 && (
                    <p className="w-full text-[12px] text-muted-foreground">{getT().aiFilter.quietSoon}</p>
                  )}
                </div>
              )}
              <FilterBar
                view={view}
                f={filters}
                set={set}
                patch={patchFilters}
                categories={meta?.categories ?? []}
                categoryCounts={stats?.stats.by_category ?? {}}
                llmEnabled={!!meta?.llm_enabled}
              />

              {filters.rules && <RuleTree value={filters.rules} onChange={(r) => set("rules", r)} onRemove={() => set("rules", null)} />}

              {hiddenMatches > 0 && (
                <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/30 bg-accent px-4 py-2.5 text-sm text-accent-foreground">
                  <span className="data text-[13px]">
                    {t.hiddenMatches(num(hiddenMatches), hiddenMatches)}
                  </span>
                  <Button
                    size="sm"
                    onClick={() =>
                      browse("all", {
                        q: filters.q,
                        keywords: filters.keywords,
                        topic: filters.topic,
                        digits: filters.digits,
                        digitMode: filters.digitMode,
                        minScore: 0,
                        hideJunk: false,
                        sorts: filters.digits ? [{ col: "len", dir: "asc" }] : [],
                      })
                    }
                  >
                    {t.showAll(num(allMatches!))}
                  </Button>
                </div>
              )}

              {locked.length > 0 && (
                <Upsell text={t.lockedFilters} onUnlock={() => setCreditsOpen(true)} />
              )}

              <NameTable
                items={items}
                total={total}
                loading={loading}
                busy={busy}
                onOpen={setSelected}
                onWatch={toggleWatch}
                onLike={likeName}
                onHide={hideName}
                onImagine={imagineName}
                onRefresh={refreshOne}
                hiddenView={viewKey === "hidden"}
                showEnds={auctionView || viewKey === "watch" || viewKey === "buyable" || viewKey === "all"}
                endsFirst={auctionView}
                sorts={filters.sorts}
                defaultSort={view.sort}
                onSorts={(s) => set("sorts", s)}
                statusOptions={viewStatuses(view)}
                statuses={filters.statuses}
                onStatuses={(s) => set("statuses", s)}
                medians={medians}
                empty={
                  viewKey === "watch" ? (
                    <>{t.emptyWatch}</>
                  ) : (
                    <>
                      {t.emptyFilters}{" "}
                      <button className="text-primary hover:underline" onClick={() => setFilters({ ...DEFAULT_FILTERS, sorts: filters.sorts })}>
                        {t.resetFilters}
                      </button>
                    </>
                  )
                }
              />
              <div ref={sentinel} className="data h-10 pt-2 text-center text-xs text-muted-foreground">
                {items.length > 0 && (items.length < reachable ? (loading ? t.loadingMore : "") : reachable < total ? "" : t.allShown(num(total)))}
              </div>
              {reachable < total && items.length >= reachable && !loading && (
                <Upsell text={t.capped(num(reachable), num(total))} onUnlock={() => setCreditsOpen(true)} />
              )}
              {buyableApplies(view) && filters.buyableOnly && !(items.length < total) && !loading && (
                <div className="pb-6 text-center text-[13px] text-muted-foreground">
                  {t.buyableNote}{" "}
                  <button className="text-primary hover:underline" onClick={() => set("buyableOnly", false)}>
                    {t.showTaken}
                  </button>
                </div>
              )}
            </div>
          )}
        </main>
      </div>

      <CreditsDialog open={creditsOpen} onOpenChange={setCreditsOpen} />
      <SupportDialog open={supportOpen} onOpenChange={setSupportOpen} />
      <AccountDialog open={accountOpen} onOpenChange={setAccountOpen} />
      <SettingsDialog
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        settings={settings}
        categories={meta?.categories ?? []}
        onSaved={setSettings}
        onUpgrade={() => {
          setSettingsOpen(false);
          setCreditsOpen(true);
        }}
        onAccount={() => {
          setSettingsOpen(false);
          setAccountOpen(true);
        }}
      />

      <Dialog open={!!saveDlg} onOpenChange={(o) => !o && setSaveDlg(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{saveDlg?.renameId ? t.renameTracker : t.saveAsTracker}</DialogTitle>
            <DialogDescription>
              {saveDlg?.renameId
                ? t.renameHint
                : t.saveHint}
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              saveTracker();
            }}
            className="contents"
          >
            <DialogBody>
              <label className="mb-2 block text-[14px] font-medium" htmlFor="tracker-name">
                {t.name}
              </label>
              <input
                id="tracker-name"
                autoFocus
                value={saveDlg?.name ?? ""}
                onChange={(e) => setSaveDlg((d) => d && { ...d, name: e.target.value })}
                placeholder={t.namePlaceholder}
                maxLength={60}
                className="h-10 w-full rounded-md border border-input bg-background px-3 text-[15px] outline-none focus:border-ring focus:ring-3 focus:ring-ring/15"
              />
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setSaveDlg(null)}>
                {getT().common.cancel}
              </Button>
              <Button type="submit" disabled={!saveDlg?.name.trim()} className="min-w-24">
                {saveDlg?.renameId ? t.rename : t.saveTracker}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <DetailDrawer name={selected} imagine={imagineFor === selected} onClose={() => { setSelected(null); setImagineFor(null); }} onChange={patchItem} />
      {isAdmin && <JobsPanel open={jobsOpen} onClose={() => setJobsOpen(false)} stats={stats} meta={meta} />}
      <CommandPalette open={paletteOpen} setOpen={setPaletteOpen} onOpenName={setSelected} onTrack={track} onView={changeView} onJobs={isAdmin ? () => setJobsOpen(true) : undefined} />
    </div>
  );
}

/** A line about what the free tier left out, with the way to unlock it. */
function Upsell({ text, onUnlock }: { text: string; onUnlock: () => void }) {
  const t = useT().dashboard;
  return (
    <div className="mx-auto my-3 flex max-w-2xl flex-wrap items-center justify-center gap-x-3 gap-y-1.5 rounded-lg border border-dashed border-border px-4 py-3 text-center text-[13px] text-muted-foreground">
      <Lock className="size-3.5 shrink-0" />
      <span>{text}</span>
      <button className="font-medium text-primary hover:underline" onClick={onUnlock}>
        {t.unlock}
      </button>
    </div>
  );
}
