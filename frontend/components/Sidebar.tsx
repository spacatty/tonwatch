"use client";

import { Activity, Bookmark, Briefcase, Crosshair, Lightbulb, Menu, Moon, Plus, Search, Shield, Sparkles, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";
import { creditsChanged, studio, type Credits, type Meta, type StatsResp, type Tracker } from "@/lib/api";
import { VIEWS, type ViewKey } from "@/lib/views";
import { cn } from "@/lib/utils";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Avatar } from "./AccountDialog";
import { useAuth } from "./Auth";
import { Mark } from "./bits";

/** What the main area shows: a built-in view, a saved tracker, or the AI chat. */
export type Mode =
  | { kind: "home" }
  | { kind: "view"; key: ViewKey }
  | { kind: "tracker"; id: number }
  | { kind: "ai" }
  | { kind: "ideas" }
  | { kind: "portfolio" }
  | { kind: "admin" };

interface Props {
  mode: Mode;
  onView: (v: ViewKey) => void;
  onTracker: (t: Tracker) => void;
  onAI: () => void;
  onIdeas: () => void;
  onPortfolio: () => void;
  onHome: () => void;
  onNewTracker: () => void;
  trackers: Tracker[];
  stats: StatsResp | null;
  meta: Meta | null;
  jobsRunning: number;
  onJobs: () => void;
  onSearch: () => void;
  onCredits: () => void;
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

function NavBody(p: Props) {
  const { mode, stats, meta, jobsRunning } = p;
  const { resolvedTheme, setTheme } = useTheme();
  const s = stats?.stats;
  const credits = useCredits();
  const { user, inTelegram } = useAuth();
  return (
    <div className="flex h-full flex-col border-r border-border bg-sidebar pt-5 pb-4">
      <div className="mb-6 flex items-center gap-2.5 px-5">
        <Mark className="size-7" />
        <div className="leading-tight">
          <div className="text-[15px] font-semibold">TON Watch</div>
          <div className="text-xs text-muted-foreground">Telegram usernames</div>
        </div>
      </div>

      <div className="px-3">
        <button onClick={p.onSearch} className="mb-2 flex h-8 w-full items-center gap-2 rounded-md bg-muted px-2.5 text-[13px] text-muted-foreground transition-colors hover:text-foreground">
          <Search className="size-3.5" />
          Search
          <kbd className="ml-auto rounded bg-background px-1.5 text-[11px]">Ctrl K</kbd>
        </button>
        <button onClick={p.onHome} className={itemCls(mode.kind === "home")}>
          <Crosshair className={iconCls(mode.kind === "home")} />
          Snipes
        </button>
        <button onClick={p.onAI} className={itemCls(mode.kind === "ai")}>
          <Sparkles className={iconCls(mode.kind === "ai")} />
          Ask AI
          <span className="data ml-auto rounded-sm border border-primary/40 px-1 text-[10px] text-primary">BETA</span>
        </button>
        <button onClick={p.onIdeas} className={itemCls(mode.kind === "ideas")}>
          <Lightbulb className={iconCls(mode.kind === "ideas")} />
          Ideas
        </button>
        <button onClick={p.onPortfolio} className={itemCls(mode.kind === "portfolio")}>
          <Briefcase className={iconCls(mode.kind === "portfolio")} />
          Portfolio
        </button>
      </div>

      <ScrollArea className="mt-4 min-h-0 flex-1 px-3">
        <div className="label mb-1.5 px-2.5">Browse</div>
        <nav className="flex flex-col gap-0.5">
          {VIEWS.map((v) => {
            const active = mode.kind === "view" && mode.key === v.key;
            const n = s && v.count ? v.count(s.by_status, s.watched, s.hidden ?? 0) : undefined;
            return (
              <button key={v.key} onClick={() => p.onView(v.key)} className={itemCls(active)}>
                <v.icon className={iconCls(active)} />
                {v.label}
                {n != null && n > 0 && <span className="data ml-auto text-[11px] text-muted-foreground">{n.toLocaleString()}</span>}
              </button>
            );
          })}
        </nav>

        <div className="mt-6 mb-1 flex items-center justify-between px-2">
          <span className="label">Trackers</span>
          <button onClick={p.onNewTracker} aria-label="Save current filters as a tracker" className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground">
            <Plus className="size-4" />
          </button>
        </div>
        <nav className="flex flex-col gap-0.5">
          {p.trackers.map((t) => {
            const active = mode.kind === "tracker" && mode.id === t.id;
            return (
              <button key={t.id} onClick={() => p.onTracker(t)} className={itemCls(active)} title={t.name}>
                <Bookmark className={iconCls(active)} />
                <span className="min-w-0 truncate">{t.name}</span>
                <span className="ml-auto flex shrink-0 items-center gap-1.5">
                  {t.new > 0 && !active && <span className="data rounded-sm bg-primary px-1 text-[10px] font-semibold text-primary-foreground">+{t.new > 99 ? "99" : t.new}</span>}
                  <span className="data text-[11px] text-muted-foreground">{t.total.toLocaleString()}</span>
                </span>
              </button>
            );
          })}
          {p.trackers.length === 0 && <p className="px-2 py-1 text-[13px] text-muted-foreground">Save any filter set to follow it here.</p>}
        </nav>

        {user.role === "admin" && (
          <>
            <div className="label mt-6 mb-1.5 px-2.5">Admin</div>
            <button onClick={p.onAdmin} className={itemCls(mode.kind === "admin")}>
              <Shield className={iconCls(mode.kind === "admin")} />
              Users and payments
            </button>
            <button onClick={p.onJobs} className={itemCls(false)}>
              <Activity className={iconCls(false)} />
              Collectors
              {jobsRunning > 0 && (
                <span className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground">
                  <span className="size-1.5 animate-pulse rounded-full bg-positive" />
                  {jobsRunning} running
                </span>
              )}
            </button>
          </>
        )}
      </ScrollArea>

      <div className="space-y-3 px-5 pt-4 text-xs text-muted-foreground">
        <button onClick={p.onCredits} className="data flex w-full items-center justify-between rounded-md border border-border bg-card px-2.5 py-1.5 text-[12px] transition-colors hover:border-primary" title="AI credits: every AI feature spends them. Click for the ledger and top-up.">
          <span>AI credits</span>
          <span className={cn("font-semibold", credits && credits.balance < 10 ? "text-warning" : "text-primary")}>{credits ? credits.balance.toLocaleString() : "–"}</span>
        </button>
        <div className="data space-y-1 text-[12px]">
          <div className="flex justify-between">
            <span>Names tracked</span>
            <span className="text-foreground">{s?.total.toLocaleString() ?? "–"}</span>
          </div>
          <div className="flex justify-between">
            <span>Rated</span>
            <span className="text-foreground">{s?.scored.toLocaleString() ?? "–"}</span>
          </div>
          {meta && !meta.llm_enabled && user.role === "admin" && <p className="pt-1 text-warning">AI rating is off. Add OPENROUTER_API_KEY to .env.</p>}
        </div>
        <div className="flex items-center gap-2 border-t border-border pt-3">
          <button onClick={p.onAccount} className="flex min-w-0 flex-1 items-center gap-2.5 rounded-md py-1 text-left hover:text-foreground" title="Account">
            <Avatar photo={user.tg_photo} label={user.label} className="size-7 text-[12px]" />
            <span className="min-w-0">
              <span className="block truncate text-[13px] font-medium text-foreground">{user.label}</span>
              <span className="block text-[11.5px]">{user.plan === "pro" ? "Pro" : "Free"} plan</span>
            </span>
          </button>
          {!inTelegram && (
            <button
              onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
              aria-label={resolvedTheme === "dark" ? "Light theme" : "Dark theme"}
              title={resolvedTheme === "dark" ? "Light theme" : "Dark theme"}
              className="rounded-md p-1.5 hover:bg-muted hover:text-foreground"
            >
              {resolvedTheme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

export function Sidebar(props: Props) {
  return (
    <aside className="sticky top-0 hidden h-dvh w-[252px] shrink-0 lg:block">
      <NavBody {...props} />
    </aside>
  );
}

export function MobileBar(props: Props & { title: string }) {
  const [open, setOpen] = useState(false);
  const close = <T extends unknown[]>(fn: (...a: T) => void) => (...a: T) => {
    fn(...a);
    setOpen(false);
  };
  return (
    <div className="sticky top-0 z-30 flex h-12 items-center gap-3 border-b border-border bg-background px-4 lg:hidden">
      <button onClick={() => setOpen(true)} aria-label="Open menu" className="-ml-1 rounded-md p-1.5 hover:bg-muted">
        <Menu className="size-5" />
      </button>
      <Mark className="size-5" />
      <span className="truncate font-medium">{props.title}</span>
      <button onClick={props.onSearch} aria-label="Search" className="ml-auto rounded-md p-1.5 hover:bg-muted">
        <Search className="size-5" />
      </button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="left" className="w-[280px] bg-background p-0">
          <SheetTitle className="sr-only">Menu</SheetTitle>
          <NavBody
            {...props}
            onView={close(props.onView)}
            onTracker={close(props.onTracker)}
            onAI={close(props.onAI)}
            onIdeas={close(props.onIdeas)}
            onPortfolio={close(props.onPortfolio)}
            onHome={close(props.onHome)}
            onNewTracker={close(props.onNewTracker)}
            onJobs={close(props.onJobs)}
            onSearch={close(props.onSearch)}
            onCredits={close(props.onCredits)}
            onAccount={close(props.onAccount)}
            onAdmin={close(props.onAdmin)}
          />
        </SheetContent>
      </Sheet>
    </div>
  );
}
