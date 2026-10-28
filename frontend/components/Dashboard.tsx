"use client";

import { Bookmark, MoreHorizontal, Plus } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { api, studio, type Meta, type Settings, type StatsResp, type Tracker, type Username } from "@/lib/api";
import { buildParams, DEFAULT_FILTERS, normalizeFilters, PRESET_TRACKERS, searchOnlyParams, viewStatuses, VIEWS, type Filters, type ViewKey } from "@/lib/views";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { AccountDialog } from "./AccountDialog";
import { AdminPanel } from "./AdminPanel";
import { AskAI } from "./AskAI";
import { useAuth } from "./Auth";
import { type Medians } from "./bits";
import { CommandPalette } from "./CommandPalette";
import { DetailDrawer } from "./DetailDrawer";
import { FilterBar } from "./FilterBar";
import { Ideas } from "./Ideas";
import { Portfolio } from "./Portfolio";
import { SettingsDialog } from "./SettingsDialog";
import { CreditsDialog } from "./CreditsDialog";
import { JobsPanel, useJobs } from "./JobsPanel";
import { NameTable } from "./NameTable";
import { MobileBar, Sidebar, type Mode } from "./Sidebar";
import { Snipes } from "./Snipes";

const PAGE = 60;
// per account: two people sharing a browser keep their own view and starter trackers
const lsKey = (uid: number) => `fragwatch.state.v5.${uid}`;
const presetsKey = (uid: number) => `fragwatch.presets.v2.${uid}`;

function loadState(uid: number, admin: boolean): { mode: Mode; filters: Filters } | null {
  try {
    // the single-user install kept its state under v4; the first (admin) account inherits it
    for (const key of [lsKey(uid), ...(admin ? ["fragwatch.state.v4"] : [])]) {
      const s = JSON.parse(localStorage.getItem(key) ?? "null");
      if (s) return { mode: s.mode?.kind === "admin" && !admin ? { kind: "home" } : s.mode, filters: normalizeFilters(s.filters) };
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
  if (f.digits) return `Names with ${f.digits}`;
  if (f.q) return `“${f.q}”`;
  if (f.keywords.length) return f.keywords.slice(0, 3).join(", ");
  if (f.maxBuyout) return `${viewLabel} under ${f.maxBuyout} TON`;
  return viewLabel;
}

const sameFilters = (a: Filters, b: Filters) => JSON.stringify(a) === JSON.stringify(b);

export default function Dashboard() {
  const { user } = useAuth();
  const isAdmin = user.role === "admin";
  const [mode, setMode] = useState<Mode>({ kind: "home" });
  const [filters, setFilters] = useState<Filters>(DEFAULT_FILTERS);
  const [hydrated, setHydrated] = useState(false);

  const [items, setItems] = useState<Username[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState<StatsResp | null>(null);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [trackers, setTrackers] = useState<Tracker[]>([]);
  const [medians, setMedians] = useState<Medians>({});
  const [settings, setSettings] = useState<Settings>({ max_buyout: 100, min_score: 7, kinds: [], daily_credits: 100, about: "", avoid: "", plan: "free", telegram_chat: "" });
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [creditsOpen, setCreditsOpen] = useState(false);
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
    }
    setHydrated(true);
    api.meta().then(setMeta).catch(() => {});
    studio.settings().then(setSettings).catch(() => {});
  }, [user.id, isAdmin]);
  useEffect(() => {
    if (!hydrated) return;
    try {
      localStorage.setItem(lsKey(user.id), JSON.stringify({ mode, filters }));
    } catch {}
  }, [mode, filters, hydrated, user.id]);

  // typical price per AI score, the yardstick for "deals": what such names actually sold for, or
  // (where there are too few sales) what sellers ask. Asks run far above real sales.
  useEffect(() => {
    const f = () =>
      api
        .medians()
        .then((m) =>
          setMedians(
            Object.fromEntries(
              m.flatMap((x) => (x.sold_count >= 30 ? [[x.score, x.sold]] : x.ask_count >= 10 ? [[x.score, x.ask]] : [])),
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
      } catch (e) {
        if (id === reqId.current) toast.error(`Could not load names: ${(e as Error).message}. Is the backend running on :8080?`, { id: "load" });
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
  }, [loadPage, hydrated, listMode]);

  // infinite scroll
  useEffect(() => {
    const el = sentinel.current;
    if (!el) return;
    const io = new IntersectionObserver((es) => {
      if (es[0].isIntersecting && !loading && items.length < total) loadPage(items.length);
    });
    io.observe(el);
    return () => io.disconnect();
  }, [loading, items.length, total, loadPage]);

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
      toast(u.watched ? `Stopped tracking @${u.name}` : `Tracking @${u.name}`, {
        action: { label: "Undo", onClick: () => api.watch(u.name, u.watched).then(() => patchItem(u)) },
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
      toast(u.hidden ? `@${u.name} is back` : `@${u.name} hidden from every list`, {
        action: { label: "Undo", onClick: () => api.hide(u.name, u.hidden).then(() => loadPage(0)) },
      });
    } catch (e) {
      toast.error((e as Error).message);
      loadPage(0);
    }
  };
  const likeName = async (u: Username) => {
    try {
      await studio.judge(u.name, "like");
      toast(`Liked @${u.name}: it shapes your taste for Ideas`);
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
    toast(`Tracking @${name}`);
  };

  const track = async (name: string) => {
    const t = toast.loading(`Checking @${name} on Fragment…`);
    try {
      await api.refresh(name);
      await api.watch(name, true);
      toast.success(`@${name} added to your watchlist`, { id: t });
      setSelected(name);
      if (listMode) loadPage(0);
    } catch (e) {
      toast.error(`@${name}: ${(e as Error).message}`, { id: t });
    }
  };

  const goTop = () => window.scrollTo({ top: 0 });
  const changeView = (v: ViewKey) => {
    setMode({ kind: "view", key: v });
    setItems([]);
    setTotal(0);
    goTop();
  };
  const browse = (v: ViewKey, patch: Partial<Filters>) => {
    setFilters({ ...DEFAULT_FILTERS, ...patch });
    changeView(v);
  };
  const openTracker = (t: Tracker) => {
    try {
      setFilters(normalizeFilters(JSON.parse(t.filters)));
    } catch {
      setFilters(DEFAULT_FILTERS);
    }
    setMode({ kind: "tracker", id: t.id });
    setItems([]);
    setTotal(0);
    goTop();
    api.trackerSeen(t.id).then(loadTrackers).catch(() => {});
  };

  const trackerDirty = (() => {
    if (!tracker) return false;
    try {
      return !sameFilters(normalizeFilters(JSON.parse(tracker.filters)), filters);
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
        toast.success("Tracker renamed");
      } else {
        const v = VIEWS.find((x) => x.key === saveDlg.view)!;
        const r = await api.createTracker({ name, view: saveDlg.view, filters: JSON.stringify(saveDlg.filters), params: trackerParams(v, saveDlg.filters) });
        setFilters(saveDlg.filters);
        setMode({ kind: "tracker", id: r.id });
        toast.success(`Saved “${name}”. New matches show in the sidebar.`);
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
    toast.success(`“${tracker.name}” updated`);
    loadTrackers();
  };

  const deleteTracker = async () => {
    if (!tracker || !confirm(`Delete the tracker “${tracker.name}”?`)) return;
    await api.deleteTracker(tracker.id);
    toast(`Deleted “${tracker.name}”`);
    setMode({ kind: "view", key: viewKey });
    loadTrackers();
  };

  const openSave = (f = filters, v: ViewKey = viewKey) => setSaveDlg({ name: suggestName(f, VIEWS.find((x) => x.key === v)!.label), view: v, filters: f });

  const title =
    mode.kind === "home"
      ? "Snipes"
      : mode.kind === "ai"
        ? "Ask AI"
        : mode.kind === "ideas"
          ? "Ideas"
          : mode.kind === "portfolio"
            ? "Portfolio"
            : mode.kind === "admin"
              ? "Users and payments"
              : tracker
                ? tracker.name
                : view.label;
  const subtitle =
    mode.kind === "admin"
      ? "Accounts, plans, credits and every payment. Only admins see this."
      : mode.kind === "home"
      ? "The best names for the money: across the market, inside your trackers, and by theme."
      : mode.kind === "ai"
        ? "Describe your project and get usernames that fit, ranked by the AI, with what each costs."
        : mode.kind === "ideas"
          ? "Teach it your taste, then get projects worth starting on names you can buy today."
        : mode.kind === "portfolio"
          ? "What you own, what you paid, what names like them go for now, and the project each is for."
        : closingSoon
          ? `${closingSoon.toLocaleString()} name${closingSoon === 1 ? "" : "s"} rated 7+ close within the hour.`
          : tracker
            ? `Saved search in ${view.label}.`
            : `${view.blurb}.`;

  const sidebarProps = {
    mode,
    onView: changeView,
    onTracker: openTracker,
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
                        Update tracker
                      </Button>
                    )}
                    <DropdownMenu>
                      <DropdownMenuTrigger render={<Button variant="outline" size="icon" className="size-9" aria-label="Tracker options" />}>
                        <MoreHorizontal className="size-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-52">
                        <DropdownMenuItem onClick={() => setSaveDlg({ name: tracker.name, view: viewKey, filters, renameId: tracker.id })}>Rename</DropdownMenuItem>
                        <DropdownMenuItem onClick={() => openSave()}>Save as a new tracker</DropdownMenuItem>
                        {trackerDirty && <DropdownMenuItem onClick={() => openTracker(tracker)}>Discard changes</DropdownMenuItem>}
                        <DropdownMenuSeparator />
                        <DropdownMenuItem variant="destructive" onClick={deleteTracker}>
                          Delete tracker
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </>
                ) : (
                  <Button variant="outline" className="h-9" onClick={() => openSave()}>
                    <Bookmark className="size-4" />
                    Save as tracker
                  </Button>
                ))}
              {mode.kind !== "ai" && mode.kind !== "ideas" && mode.kind !== "portfolio" && mode.kind !== "admin" && (
                <Button onClick={() => setPaletteOpen(true)} className="h-9 px-4">
                  <Plus className="size-4" />
                  Track a username
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
              <FilterBar
                view={view}
                f={filters}
                set={set}
                patch={patchFilters}
                categories={meta?.categories ?? []}
                categoryCounts={stats?.stats.by_category ?? {}}
                llmEnabled={!!meta?.llm_enabled}
              />

              {hiddenMatches > 0 && (
                <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/30 bg-accent px-4 py-2.5 text-sm text-accent-foreground">
                  <span className="data text-[13px]">
                    +{hiddenMatches.toLocaleString()} more name{hiddenMatches === 1 ? "" : "s"} match your search but are hidden by this view or your filters.
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
                    Show all {allMatches!.toLocaleString()}
                  </Button>
                </div>
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
                    <>Nothing tracked yet. Star a name, or use “Track a username”.</>
                  ) : (
                    <>
                      No names match these filters.{" "}
                      <button className="text-primary hover:underline" onClick={() => setFilters({ ...DEFAULT_FILTERS, sorts: filters.sorts })}>
                        Reset filters
                      </button>
                    </>
                  )
                }
              />
              <div ref={sentinel} className="data h-10 pt-2 text-center text-xs text-muted-foreground">
                {items.length > 0 && (items.length < total ? (loading ? "loading more…" : "") : `all ${total.toLocaleString()} shown`)}
              </div>
            </div>
          )}
        </main>
      </div>

      <CreditsDialog open={creditsOpen} onOpenChange={setCreditsOpen} />
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
            <DialogTitle>{saveDlg?.renameId ? "Rename tracker" : "Save as tracker"}</DialogTitle>
            <DialogDescription>
              {saveDlg?.renameId
                ? "Give this saved search a new name."
                : "It keeps this view, every filter and the sort. The sidebar and Snipes show how many names match and how many are new."}
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
                Name
              </label>
              <input
                id="tracker-name"
                autoFocus
                value={saveDlg?.name ?? ""}
                onChange={(e) => setSaveDlg((d) => d && { ...d, name: e.target.value })}
                placeholder="e.g. VPN names under 100 TON"
                maxLength={60}
                className="h-10 w-full rounded-md border border-input bg-background px-3 text-[15px] outline-none focus:border-ring focus:ring-3 focus:ring-ring/15"
              />
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setSaveDlg(null)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!saveDlg?.name.trim()} className="min-w-24">
                {saveDlg?.renameId ? "Rename" : "Save tracker"}
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
