"use client";

import { Activity, Bookmark, Briefcase, ChevronDown, Copy, Crosshair, Lightbulb, Menu, Pencil, Plus, Radio, Search, Settings2, Shield, Sparkles, Trash2, Wand2 } from "lucide-react";
import { useEffect, useState } from "react";
import { locale, num, useT } from "@/lib/i18n";
import { creditsChanged, studio, type Credits, type Meta, type StatsResp, type Tracker } from "@/lib/api";
import { VIEWS, type ViewKey } from "@/lib/views";
import { cn } from "@/lib/utils";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { useAuth } from "./Auth";
import { Mark } from "./bits";
import { LangSwitch } from "./LangSwitch";
import { ProfileMenu } from "./ProfileMenu";

/** What the main area shows: a built-in view, a saved tracker, or the AI chat. */
export type Mode =
  | { kind: "home" }
  | { kind: "view"; key: ViewKey }
  | { kind: "tracker"; id: number }
  | { kind: "ai" }
  | { kind: "aifilter" }
  | { kind: "ideas" }
  | { kind: "portfolio" }
  | { kind: "fresh" }
  | { kind: "admin" };

interface Props {
  mode: Mode;
  onView: (v: ViewKey) => void;
  onTracker: (t: Tracker) => void;
  onAI: () => void;
  onAIFilter: () => void;
  onTrackerRename: (t: Tracker) => void;
  onTrackerDuplicate: (t: Tracker) => void;
  onTrackerDelete: (t: Tracker) => void;
  onIdeas: () => void;
  onPortfolio: () => void;
  onFresh: () => void;
  onHome: () => void;
  onNewTracker: () => void;
  trackers: Tracker[];
  stats: StatsResp | null;
  meta: Meta | null;
  jobsRunning: number;
  onJobs: () => void;
  onSearch: () => void;
  onCredits: () => void;
  onSupport: () => void;
  onAccount: () => void;
  onAdmin: () => void;
}

function useCredits() {
  const [c, setC] = useState<Credits | null>(null);
  useEffect(() => {
    const load = () => studio.credits().then(setC).catch(() => {});
    load();
    creditsChanged.addEventListener("change", load);
    return () => creditsChanged.removeEventListener("change", load);
  }, []);
  return c;
}

const itemCls = (active: boolean) =>
  cn(
    "group relative flex h-8 w-full items-center gap-2.5 rounded px-2.5 text-left text-[13.5px] transition-colors",
    active
      ? "bg-accent text-accent-foreground before:absolute before:inset-y-1.5 before:-left-3 before:w-[2px] before:rounded-r before:bg-primary"
      : "text-foreground/75 hover:bg-muted hover:text-foreground",
  );
const iconCls = (active: boolean) => cn("size-4 shrink-0 stroke-[1.6]", active ? "text-primary" : "text-muted-foreground group-hover:text-foreground");

/** The catalog under "Everything", broadest first: everything buyable, then by kind of sale, then names not on sale. */
const CATALOG: ViewKey[] = ["buyable", "auction", "ending", "sale", "market", "owned"];

/** A titled group of the navigation that folds away; each remembers being folded. */
function Section({ id, label, action, children }: { id: string; label: string; action?: React.ReactNode; children: React.ReactNode }) {
  const key = `tonwatch.nav.${id}`;
  const [open, setOpen] = useState(true);
  useEffect(() => {
    try {
      if (localStorage.getItem(key) === "0") setOpen(false);
    } catch {}
  }, [key]);
  const toggle = () => {
    setOpen(!open);
    try {
      localStorage.setItem(key, open ? "0" : "1");
    } catch {}
  };
  return (
    <section className="mb-4">
      <div className="mb-1 flex items-center justify-between pr-1">
        <button onClick={toggle} aria-expanded={open} className="label group flex items-center gap-1 rounded px-2.5 py-0.5 hover:text-foreground">
          {label}
          <ChevronDown className={cn("size-3 opacity-0 transition group-hover:opacity-100", !open && "-rotate-90 opacity-100")} />
        </button>
        {action}
      </div>
      {open && <div className="flex flex-col gap-0.5">{children}</div>}
    </section>
  );
}

function NavBody(p: Props) {
  const { mode, stats, meta, jobsRunning } = p;
  const s = stats?.stats;
  const credits = useCredits();
  const { user } = useAuth();
  const t = useT().sidebar;
  const viewItem = (key: ViewKey) => {
    const v = VIEWS.find((x) => x.key === key)!;
    const active = mode.kind === "view" && mode.key === v.key;
    const n = s && v.count ? v.count(s.by_status, s.watched, s.hidden ?? 0) : undefined;
    return (
      <button key={v.key} onClick={() => p.onView(v.key)} className={itemCls(active)}>
        <v.icon className={iconCls(active)} />
        <span className="min-w-0 truncate">{v.label}</span>
        {n != null && n > 0 && <span className="data ml-auto text-[11px] text-muted-foreground">{n.toLocaleString(locale())}</span>}
      </button>
    );
  };
  return (
    <div className="flex h-full flex-col border-r border-border bg-sidebar pt-5 pb-4">
      <div className="mb-6 flex items-center gap-2.5 px-5">
        <Mark className="size-7" />
        <div className="leading-tight">
          <div className="text-[15px] font-semibold">TonWatch</div>
          <div className="text-xs text-muted-foreground">{t.tagline}</div>
        </div>
      </div>

      <div className="px-3">
        <button onClick={p.onSearch} className="mb-2 flex h-8 w-full items-center gap-2 rounded-md bg-muted px-2.5 text-[13px] text-muted-foreground transition-colors hover:text-foreground">
          <Search className="size-3.5" />
          {t.search}
          <kbd className="ml-auto rounded bg-background px-1.5 text-[11px]">Ctrl K</kbd>
        </button>
      </div>

      <ScrollArea className="mt-3 min-h-0 flex-1 px-3">
        <Section id="mine" label={t.sections.mine}>
          <button onClick={p.onHome} className={itemCls(mode.kind === "home")}>
            <Crosshair className={iconCls(mode.kind === "home")} />
            {t.snipes}
          </button>
          {viewItem("watch")}
          <button onClick={p.onPortfolio} className={itemCls(mode.kind === "portfolio")}>
            <Briefcase className={iconCls(mode.kind === "portfolio")} />
            {t.portfolio}
          </button>
          {viewItem("hidden")}
        </Section>

        <Section id="ai" label={t.sections.ai}>
          <button onClick={p.onIdeas} className={itemCls(mode.kind === "ideas")}>
            <Lightbulb className={iconCls(mode.kind === "ideas")} />
            {t.ideas}
          </button>
          {/* the chat subgroup: both pages talk to the AI, Ideas is a studio of its own */}
          <div className="label mt-1.5 px-2.5">{t.sections.chat}</div>
          <div className="ml-[17px] flex flex-col gap-0.5 border-l border-border pl-2">
            <button onClick={p.onAI} className={itemCls(mode.kind === "ai")}>
              <Sparkles className={iconCls(mode.kind === "ai")} />
              {t.askAI}
              <span className="data ml-auto rounded-sm border border-primary/40 px-1 text-[10px] text-primary">BETA</span>
            </button>
            <button onClick={p.onAIFilter} className={itemCls(mode.kind === "aifilter")}>
              <Wand2 className={iconCls(mode.kind === "aifilter")} />
              {t.aiFilter}
            </button>
          </div>
        </Section>

        <Section id="catalog" label={t.browse}>
          <button onClick={p.onFresh} className={itemCls(mode.kind === "fresh")}>
            <Radio className={iconCls(mode.kind === "fresh")} />
            {t.fresh}
            <span className="ml-auto size-1.5 animate-pulse rounded-full bg-positive" />
          </button>
          {viewItem("all")}
          <div className="ml-[17px] flex flex-col gap-0.5 border-l border-border pl-2">{CATALOG.map((k) => viewItem(k))}</div>
        </Section>

        <Section
          id="trackers"
          label={t.trackers}
          action={
            <button onClick={p.onNewTracker} aria-label={t.saveTracker} title={t.saveTracker} className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground">
              <Plus className="size-4" />
            </button>
          }
        >
        <nav className="flex flex-col gap-0.5">
          {p.trackers.map((tr) => {
            const active = mode.kind === "tracker" && mode.id === tr.id;
            return (
              <div key={tr.id} className="group/tr relative">
                <button onClick={() => p.onTracker(tr)} className={cn(itemCls(active), "[@media(hover:none)]:pr-9")} title={tr.name}>
                  <Bookmark className={iconCls(active)} />
                  <span className="min-w-0 truncate">{tr.name}</span>
                  <span className="ml-auto flex shrink-0 items-center gap-1.5">
                    {tr.new > 0 && !active && <span className="data rounded-sm bg-primary px-1 text-[10px] font-semibold text-primary-foreground">+{tr.new > 99 ? "99" : tr.new}</span>}
                    <span className="data text-[11px] text-muted-foreground">{num(tr.total)}</span>
                  </span>
                </button>
                {/* the gear appears on hover (always on touch screens) and covers the counts while it shows */}
                <DropdownMenu>
                  <DropdownMenuTrigger
                    aria-label={t.trackerMenu(tr.name)}
                    title={t.trackerMenu(tr.name)}
                    className={cn(
                      "absolute top-1/2 right-1 grid size-6 -translate-y-1/2 place-items-center rounded text-muted-foreground opacity-0 transition-opacity hover:bg-muted hover:text-foreground focus-visible:opacity-100 group-hover/tr:opacity-100 data-popup-open:opacity-100 [@media(hover:none)]:opacity-100",
                      active ? "bg-accent" : "bg-sidebar",
                    )}
                  >
                    <Settings2 className="size-3.5" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" side="right" className="w-44">
                    <DropdownMenuItem onClick={() => p.onTrackerRename(tr)}>
                      <Pencil />
                      {t.renameTracker}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => p.onTrackerDuplicate(tr)}>
                      <Copy />
                      {t.duplicateTracker}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onClick={() => p.onTrackerDelete(tr)}>
                      <Trash2 />
                      {t.deleteTracker}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            );
          })}
          {p.trackers.length === 0 && <p className="px-2 py-1 text-[13px] text-muted-foreground">{t.noTrackers}</p>}
        </nav>
        </Section>

        {user.role === "admin" && (
          <Section id="admin" label={t.admin}>
            <button onClick={p.onAdmin} className={itemCls(mode.kind === "admin")}>
              <Shield className={iconCls(mode.kind === "admin")} />
              {t.usersPayments}
            </button>
            <button onClick={p.onJobs} className={itemCls(false)}>
              <Activity className={iconCls(false)} />
              {t.collectors}
              {jobsRunning > 0 && (
                <span className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground">
                  <span className="size-1.5 animate-pulse rounded-full bg-positive" />
                  {t.running(jobsRunning)}
                </span>
              )}
            </button>
          </Section>
        )}
      </ScrollArea>

      <div className="space-y-3 px-5 pt-4 text-xs text-muted-foreground">
        <button onClick={p.onCredits} className="data flex w-full items-center justify-between rounded-md border border-border bg-card px-2.5 py-1.5 text-[12px] transition-colors hover:border-primary" title={t.creditsHint}>
          <span className="flex items-center gap-1.5">
            {t.credits}
            {credits?.claim.available && <span className="size-1.5 rounded-full bg-primary" title={t.claimReady} />}
          </span>
          <span className={cn("font-semibold", credits && credits.balance < 10 ? "text-warning" : "text-primary")}>{credits ? num(credits.balance) : "–"}</span>
        </button>
        <div className="data space-y-1 text-[12px]">
          <div className="flex justify-between">
            <span>{t.namesTracked}</span>
            <span className="text-foreground">{s ? num(s.total) : "–"}</span>
          </div>
          <div className="flex justify-between">
            <span>{t.rated}</span>
            <span className="text-foreground">{s ? num(s.scored) : "–"}</span>
          </div>
          {meta && !meta.llm_enabled && user.role === "admin" && <p className="pt-1 text-warning">{t.aiOff}</p>}
        </div>
        <div className="flex items-center border-t border-border pt-3">
          <ProfileMenu onAccount={p.onAccount} onCredits={p.onCredits} onSupport={p.onSupport} />
        </div>
      </div>
    </div>
  );
}

/** The sidebar's width bounds: it never shrinks below the default, only widens. */
const MIN_W = 252;
const MAX_W = 480;
const WIDTH_KEY = "tonwatch.sidebar.width";
const clampW = (w: number) => Math.round(Math.min(MAX_W, Math.max(MIN_W, w)));

export function Sidebar(props: Props) {
  const t = useT().sidebar;
  const [width, setWidth] = useState(MIN_W);
  const [dragging, setDragging] = useState(false);
  useEffect(() => {
    try {
      const w = Number(localStorage.getItem(WIDTH_KEY));
      if (w) setWidth(clampW(w));
    } catch {}
  }, []);
  const save = (w: number) => {
    try {
      localStorage.setItem(WIDTH_KEY, String(w));
    } catch {}
  };

  const startDrag = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const x0 = e.clientX;
    const w0 = width;
    let w = w0;
    setDragging(true);
    const move = (ev: PointerEvent) => {
      w = clampW(w0 + ev.clientX - x0);
      setWidth(w);
    };
    const up = () => {
      setDragging(false);
      save(w);
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      document.body.style.removeProperty("cursor");
      document.body.style.removeProperty("user-select");
    };
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };
  const onKey = (e: React.KeyboardEvent) => {
    const step = e.shiftKey ? 40 : 10;
    const d = e.key === "ArrowRight" ? step : e.key === "ArrowLeft" ? -step : 0;
    if (!d) return;
    e.preventDefault();
    const w = clampW(width + d);
    setWidth(w);
    save(w);
  };
  const reset = () => {
    setWidth(MIN_W);
    save(MIN_W);
  };

  return (
    <aside className="sticky top-0 hidden h-dvh shrink-0 lg:block" style={{ width }}>
      <NavBody {...props} />
      <div
        role="separator"
        aria-orientation="vertical"
        aria-valuemin={MIN_W}
        aria-valuemax={MAX_W}
        aria-valuenow={width}
        tabIndex={0}
        title={t.resize}
        aria-label={t.resize}
        onPointerDown={startDrag}
        onDoubleClick={reset}
        onKeyDown={onKey}
        className={cn(
          "absolute inset-y-0 -right-1 z-20 w-2 cursor-col-resize outline-none after:absolute after:inset-y-0 after:left-1/2 after:w-[2px] after:-translate-x-1/2 after:transition-colors hover:after:bg-primary/60 focus-visible:after:bg-primary",
          dragging && "after:bg-primary",
        )}
      />
    </aside>
  );
}

export function MobileBar(props: Props & { title: string }) {
  const [open, setOpen] = useState(false);
  const t = useT().sidebar;
  const close = <T extends unknown[]>(fn: (...a: T) => void) => (...a: T) => {
    fn(...a);
    setOpen(false);
  };
  return (
    <div className="sticky top-0 z-30 flex h-12 items-center gap-3 border-b border-border bg-background px-4 lg:hidden">
      <button onClick={() => setOpen(true)} aria-label={t.openMenu} className="-ml-1 rounded-md p-1.5 hover:bg-muted">
        <Menu className="size-5" />
      </button>
      <Mark className="size-5" />
      <span className="truncate font-medium">{props.title}</span>
      <LangSwitch className="ml-auto" />
      <button onClick={props.onSearch} aria-label={t.search} className="rounded-md p-1.5 hover:bg-muted">
        <Search className="size-5" />
      </button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="left" className="w-[280px] bg-background p-0">
          <SheetTitle className="sr-only">{t.menu}</SheetTitle>
          <NavBody
            {...props}
            onView={close(props.onView)}
            onTracker={close(props.onTracker)}
            onAI={close(props.onAI)}
            onAIFilter={close(props.onAIFilter)}
            onIdeas={close(props.onIdeas)}
            onPortfolio={close(props.onPortfolio)}
            onFresh={close(props.onFresh)}
            onHome={close(props.onHome)}
            onNewTracker={close(props.onNewTracker)}
            onJobs={close(props.onJobs)}
            onSearch={close(props.onSearch)}
            onCredits={close(props.onCredits)}
            onSupport={close(props.onSupport)}
            onAccount={close(props.onAccount)}
            onAdmin={close(props.onAdmin)}
          />
        </SheetContent>
      </Sheet>
    </div>
  );
}
