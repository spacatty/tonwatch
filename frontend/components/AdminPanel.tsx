"use client";

import { Activity as ActivityIcon, Bot, Coins, CreditCard, LogIn, LogOut, Search, Shield, Sparkles, UserPlus, Zap } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { admin, type Activity, type AdminUserDetail, type DayStat, type FeatureUse, type Payment, type User } from "@/lib/api";
import { getT, locale, num, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { Avatar } from "./AccountDialog";
import { useAuth } from "./Auth";
import { ago } from "./bits";

type Overview = Awaited<ReturnType<typeof admin.overview>>;
type Stats = Awaited<ReturnType<typeof admin.stats>>;
type Tab = "overview" | "users" | "activity" | "payments";

/** Admin: how the install is used, who uses it, what they paid, and the switches only admins have. */
export function AdminPanel({ onJobs }: { onJobs: () => void }) {
  const t = useT().admin;
  const [ov, setOv] = useState<Overview | null>(null);
  const [tab, setTab] = useState<Tab>("overview");
  const [open, setOpen] = useState<number | null>(null);
  const [version, setVersion] = useState(0); // bumped by edits in the account sheet, so the lists refetch

  const load = useCallback(() => {
    admin.overview().then(setOv).catch((e) => toast.error(e.message));
  }, []);
  useEffect(load, [load]);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex rounded-md border border-border bg-card p-0.5 text-[13.5px]">
          {(["overview", "users", "activity", "payments"] as const).map((k) => (
            <button
              key={k}
              onClick={() => setTab(k)}
              className={cn("rounded px-3 py-1 transition-colors", tab === k ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
            >
              {t.tabs[k]}
            </button>
          ))}
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-x-5 gap-y-1 text-[12.5px] text-muted-foreground">
          <span>
            {t.bot}
            <b className="font-medium text-foreground">{ov?.bot ? `@${ov.bot}` : t.notSet}</b>
          </span>
          <span>
            {t.tonPayments}
            <b className={cn("font-medium", ov?.gateways.ton ? "text-positive" : "text-foreground")}>{ov?.gateways.ton ? t.on : t.off}</b>
          </span>
          <span>
            {t.stars}
            <b className={cn("font-medium", ov?.gateways.stars ? "text-positive" : "text-foreground")}>{ov?.gateways.stars ? t.on : t.off}</b>
          </span>
          <span>
            {t.ratingQueue}
            <b className="data font-medium text-foreground">{ov ? num(ov.stats.llm_queue) : "–"}</b>
          </span>
          <Button variant="outline" size="sm" className="h-8 gap-1.5" onClick={onJobs}>
            <ActivityIcon className="size-3.5" />
            {t.collectors}
          </Button>
        </div>
      </div>

      {tab === "overview" && <OverviewTab ov={ov} />}
      {tab === "users" && <UsersTab onOpen={setOpen} version={version} />}
      {tab === "activity" && <ActivityTab onOpen={setOpen} version={version} />}
      {tab === "payments" && <PaymentsTab />}

      <UserSheet
        id={open}
        onClose={() => setOpen(null)}
        onChanged={() => {
          load();
          setVersion((v) => v + 1);
        }}
      />
    </div>
  );
}

// --- overview ---

function OverviewTab({ ov }: { ov: Overview | null }) {
  const t = useT().admin;
  const [days, setDays] = useState(30);
  const [st, setSt] = useState<Stats | null>(null);
  useEffect(() => {
    admin.stats(days).then(setSt).catch((e) => toast.error(e.message));
  }, [days]);

  const tot = ov?.totals;
  const pct = tot && tot.total ? `${Math.round((tot.pro / tot.total) * 100)}%` : "0%";
  const sum = (k: keyof DayStat) => (st ? st.daily.reduce((a, d) => a + (d[k] as number), 0) : 0);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-border bg-border lg:grid-cols-4">
        <Tile label={t.accounts} v={tot ? num(tot.total) : "–"} sub={tot ? t.newThisWeek(tot.new_7d) : ""} />
        <Tile label={t.active24h} v={tot ? num(tot.active_1d) : "–"} sub={tot ? t.activeMore(tot.active_7d, tot.active_30d) : ""} />
        <Tile label={t.pro} v={tot ? num(tot.pro) : "–"} sub={t.proShare(pct)} />
        <Tile label={t.revenue} v={ov ? `${num(ov.paid.TON ?? 0, 2)} TON` : "–"} sub={ov ? `${num(ov.paid.XTR ?? 0)} ★` : ""} />
        <Tile label={t.creditsHeld} v={tot ? num(tot.credits) : "–"} sub={ov ? t.dailyAllowance(ov.daily_credits) : ""} />
        <Tile label={t.aiSpend} v={ov ? `$${ov.llm_usage.cost.toFixed(2)}` : "–"} sub={t.aiSpendSub} />
        <Tile label={t.charts.signups} v={st ? num(sum("signups")) : "–"} sub={t.range(days)} />
        <Tile label={t.charts.spent} v={st ? num(sum("spent")) : "–"} sub={st ? `$${sum("ai_cost").toFixed(2)} · ${t.range(days)}` : ""} />
      </div>

      <div className="flex items-center justify-end">
        <div className="flex rounded-md border border-border bg-card p-0.5 text-[12.5px]">
          {[7, 30, 90].map((d) => (
            <button
              key={d}
              onClick={() => setDays(d)}
              className={cn("rounded px-2.5 py-0.5 transition-colors", days === d ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
            >
              {t.range(d)}
            </button>
          ))}
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <ChartCard title={t.charts.active} data={st?.daily} pick={(d) => d.active} />
        <ChartCard title={t.charts.signups} data={st?.daily} pick={(d) => d.signups} />
        <ChartCard title={t.charts.spent} data={st?.daily} pick={(d) => d.spent} extra={(d) => `$${d.ai_cost.toFixed(3)}`} />
        <ChartCard title={t.charts.events} data={st?.daily} pick={(d) => d.events} />
      </div>

      <div className="grid gap-4 lg:grid-cols-[3fr_2fr]">
        <Features rows={st?.features} />
        <Kinds rows={st?.kinds} />
      </div>
    </div>
  );
}

function Tile({ label, v, sub }: { label: string; v: string; sub?: string }) {
  return (
    <div className="bg-card px-5 py-4">
      <div className="text-[12.5px] text-muted-foreground">{label}</div>
      <div className="data mt-1.5 text-2xl font-semibold tracking-tight">{v}</div>
      {sub ? <div className="mt-0.5 truncate text-[12px] text-muted-foreground">{sub}</div> : null}
    </div>
  );
}

const shortDay = (day: string) => new Date(day + "T00:00:00Z").toLocaleDateString(locale(), { month: "short", day: "numeric", timeZone: "UTC" });

/** One measure per day as thin bars, with a hover readout. Single series: the title names it. */
function ChartCard({ title, data, pick, extra }: { title: string; data?: DayStat[]; pick: (d: DayStat) => number; extra?: (d: DayStat) => string }) {
  const t = useT().admin;
  const [hover, setHover] = useState<number | null>(null);
  const vals = (data ?? []).map(pick);
  const max = Math.max(1, ...vals);
  const total = vals.reduce((a, b) => a + b, 0);
  const h = data?.[hover ?? -1];
  const W = 600,
    H = 140,
    n = Math.max(1, vals.length),
    step = W / n,
    bw = Math.max(2, Math.min(18, step - 2));
  // the scale tops out at a round number; recessive gridlines at half and full
  const nice = niceMax(max);
  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="mb-3 flex items-baseline justify-between gap-3">
        <div>
          <div className="text-[13px] font-medium">{title}</div>
          <div className="text-[12px] text-muted-foreground">
            {h ? (
              <>
                {shortDay(h.day)} · <b className="data font-medium text-foreground">{num(pick(h))}</b>
                {extra ? ` · ${extra(h)}` : ""}
              </>
            ) : (
              <>
                <b className="data font-medium text-foreground">{num(total)}</b> · {t.peak(num(total ? max : 0))}
              </>
            )}
          </div>
        </div>
      </div>
      <svg viewBox={`0 0 ${W} ${H + 18}`} className="h-40 w-full" preserveAspectRatio="none" onMouseLeave={() => setHover(null)} role="img" aria-label={title}>
        {[0.5, 1].map((f) => (
          <line key={f} x1={0} x2={W} y1={H - H * f} y2={H - H * f} stroke="var(--grid-line)" strokeWidth={1} vectorEffect="non-scaling-stroke" />
        ))}
        <line x1={0} x2={W} y1={H} y2={H} stroke="var(--border)" strokeWidth={1} vectorEffect="non-scaling-stroke" />
        {vals.map((v, i) => {
          const bh = v ? Math.max(2, (v / nice) * H) : 0;
          const x = i * step + (step - bw) / 2;
          return (
            <g key={i} onMouseEnter={() => setHover(i)}>
              <rect x={i * step} y={0} width={step} height={H} fill="transparent" />
              {hover === i && <rect x={i * step} y={0} width={step} height={H} fill="var(--row-hover)" />}
              {bh > 0 && <rect x={x} y={H - bh} width={bw} height={bh} rx={Math.min(3, bw / 2)} fill="var(--chart-1)" opacity={hover === null || hover === i ? 1 : 0.55} />}
            </g>
          );
        })}
      </svg>
      {data && data.length > 0 && (
        <div className="-mt-3 flex justify-between text-[11px] text-muted-foreground">
          <span>{shortDay(data[0].day)}</span>
          <span>{shortDay(data[data.length - 1].day)}</span>
        </div>
      )}
    </div>
  );
}

/** Dollars with enough digits that a cent-fraction AI spend does not read as $0.000. */
const usd = (v: number) => (v === 0 ? "$0" : v < 0.01 ? `$${v.toFixed(4)}` : `$${v.toFixed(2)}`);

function niceMax(v: number) {
  const p = Math.pow(10, Math.floor(Math.log10(Math.max(1, v))));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * p >= v) return m * p;
  return v;
}

function Features({ rows }: { rows?: FeatureUse[] }) {
  const t = useT().admin;
  const names: Record<string, string> = useT().common.features;
  const max = Math.max(1, ...(rows ?? []).map((r) => r.credits));
  return (
    <div className="rounded-lg border border-border bg-card">
      <div className="border-b border-border px-4 py-3 text-[13px] font-medium">{t.features}</div>
      <table className="w-full text-[13px]">
        <thead>
          <tr className="text-left text-[12px] text-muted-foreground">
            <th className="px-4 py-2 font-normal">{t.feature}</th>
            <th className="px-4 py-2 text-right font-normal">{t.runs}</th>
            <th className="px-4 py-2 text-right font-normal">{t.people}</th>
            <th className="w-[30%] px-4 py-2 font-normal">{t.credits}</th>
            <th className="px-4 py-2 text-right font-normal">{t.cost}</th>
          </tr>
        </thead>
        <tbody>
          {(rows ?? []).map((r) => (
            <tr key={r.feature} className="border-t border-border/50">
              <td className="px-4 py-2 font-medium">{names[r.feature] ?? r.feature}</td>
              <td className="data px-4 py-2 text-right">{num(r.runs)}</td>
              <td className="data px-4 py-2 text-right">{num(r.users)}</td>
              <td className="px-4 py-2">
                <div className="flex items-center gap-2">
                  <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                    <div className="h-full rounded-full bg-primary" style={{ width: `${(r.credits / max) * 100}%` }} />
                  </div>
                  <span className="data w-12 text-right">{num(r.credits)}</span>
                </div>
              </td>
              <td className="data px-4 py-2 text-right text-muted-foreground">{usd(r.cost)}</td>
            </tr>
          ))}
          {rows && rows.length === 0 && (
            <tr>
              <td colSpan={5} className="px-4 py-6 text-center text-muted-foreground">
                {t.noUsage}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function Kinds({ rows }: { rows?: { kind: string; n: number }[] }) {
  const t = useT().admin;
  const max = Math.max(1, ...(rows ?? []).map((r) => r.n));
  return (
    <div className="rounded-lg border border-border bg-card">
      <div className="border-b border-border px-4 py-3 text-[13px] font-medium">{t.topEvents}</div>
      <div className="space-y-2.5 p-4">
        {(rows ?? []).slice(0, 12).map((r) => (
          <div key={r.kind} className="flex items-center gap-3 text-[13px]">
            <KindIcon kind={r.kind} />
            <span className="w-40 shrink-0 truncate">{kindLabel(r.kind)}</span>
            <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
              <div className="h-full rounded-full bg-primary/70" style={{ width: `${(r.n / max) * 100}%` }} />
            </div>
            <span className="data w-12 text-right">{num(r.n)}</span>
          </div>
        ))}
        {rows && rows.length === 0 && <div className="py-4 text-center text-[13px] text-muted-foreground">{t.noEvents}</div>}
      </div>
    </div>
  );
}

// --- activity ---

function kindLabel(kind: string) {
  const t = getT().admin;
  if (kind.startsWith("ai.")) return t.aiKind((getT().common.features as Record<string, string>)[kind.slice(3)] ?? kind.slice(3));
  return t.kinds[kind] ?? kind;
}

function KindIcon({ kind }: { kind: string }) {
  const [Icon, tone] = kind === "signup"
    ? [UserPlus, "text-positive"]
    : kind === "login"
      ? [LogIn, "text-primary"]
      : kind.startsWith("logout") || kind === "admin.logout"
        ? [LogOut, "text-muted-foreground"]
        : kind.startsWith("ai.")
          ? [Sparkles, "text-primary"]
          : kind === "pay.paid"
            ? [CreditCard, "text-positive"]
            : kind.startsWith("pay.")
              ? [CreditCard, "text-muted-foreground"]
              : kind === "admin.credits"
                ? [Coins, "text-warning"]
                : kind.startsWith("admin.")
                  ? [Shield, "text-warning"]
                  : [Zap, "text-muted-foreground"];
  return (
    <span className={cn("grid size-6 shrink-0 place-items-center rounded-md bg-muted", tone)}>
      <Icon className="size-3.5" />
    </span>
  );
}

function ActivityList({ items, onOpen, showUser = true }: { items: Activity[]; onOpen?: (id: number) => void; showUser?: boolean }) {
  const t = useT().admin;
  return (
    <ul className="divide-y divide-border/50">
      {items.map((a) => (
        <li key={a.id} className="flex items-start gap-3 px-4 py-2.5 text-[13px]">
          <KindIcon kind={a.kind} />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-baseline gap-x-2">
              {showUser && (
                <button className="font-medium hover:underline" onClick={() => onOpen?.(a.user_id)}>
                  {a.user || `#${a.user_id}`}
                </button>
              )}
              <span className={showUser ? "text-muted-foreground" : "font-medium"}>{kindLabel(a.kind)}</span>
              {a.actor && a.actor_id !== a.user_id && <span className="text-[12px] text-warning">{t.by(a.actor)}</span>}
            </div>
            {a.detail && <div className="truncate text-[12px] text-muted-foreground">{a.detail}</div>}
          </div>
          <span className="data shrink-0 text-[12px] text-muted-foreground" title={new Date(a.ts * 1000).toLocaleString(locale())}>
            {ago(a.ts)}
          </span>
        </li>
      ))}
    </ul>
  );
}

const FILTERS = [
  ["all", ""],
  ["login", "login"],
  ["ai", "ai."],
  ["pay", "pay."],
  ["admin", "admin."],
] as const;

function ActivityTab({ onOpen, version }: { onOpen: (id: number) => void; version: number }) {
  const t = useT().admin;
  const [kind, setKind] = useState("");
  const [items, setItems] = useState<Activity[] | null>(null);
  const [more, setMore] = useState(false);
  const PAGE = 100;

  useEffect(() => {
    setItems(null);
    admin
      .activity({ kind, limit: PAGE })
      .then((r) => {
        setItems(r.activity);
        setMore(r.activity.length === PAGE);
      })
      .catch((e) => toast.error(e.message));
  }, [kind, version]);

  const loadMore = () => {
    const last = items?.[items.length - 1];
    if (!last) return;
    admin.activity({ kind, before: last.id, limit: PAGE }).then((r) => {
      setItems((xs) => [...(xs ?? []), ...r.activity]);
      setMore(r.activity.length === PAGE);
    });
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-1.5">
        {FILTERS.map(([k, v]) => (
          <button
            key={k}
            onClick={() => setKind(v)}
            className={cn(
              "rounded-full border px-3 py-1 text-[12.5px] transition-colors",
              kind === v ? "border-primary/50 bg-accent text-accent-foreground" : "border-border text-muted-foreground hover:text-foreground",
            )}
          >
            {t.filters[k]}
          </button>
        ))}
      </div>
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        {items && <ActivityList items={items} onOpen={onOpen} />}
        {items && items.length === 0 && <div className="px-4 py-8 text-center text-[13px] text-muted-foreground">{t.noActivity}</div>}
        {more && (
          <div className="border-t border-border p-2 text-center">
            <Button variant="ghost" size="sm" onClick={loadMore}>
              {t.loadMore}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

// --- users ---

function UsersTab({ onOpen, version }: { onOpen: (id: number) => void; version: number }) {
  const { user: me } = useAuth();
  const t = useT().admin;
  const [users, setUsers] = useState<User[]>([]);
  const [q, setQ] = useState("");

  useEffect(() => {
    const timer = setTimeout(() => admin.users(q).then((r) => setUsers(r.users)).catch(() => {}), 200);
    return () => clearTimeout(timer);
  }, [q, version]);

  const update = async (u: User, b: { role?: string; plan?: string; grant?: number }, done: string) => {
    try {
      const r = await admin.updateUser(u.id, b);
      setUsers((xs) => xs.map((x) => (x.id === u.id ? { ...x, ...r.user } : x)));
      toast.success(done);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };
  const grant = (u: User) => {
    const v = prompt(getT().admin.grantPrompt(u.label), "500");
    const n = v ? parseInt(v, 10) : NaN;
    if (Number.isFinite(n) && n !== 0) update(u, { grant: n }, getT().admin.granted(n, u.label));
  };

  return (
    <div className="space-y-3">
      <label className="relative block w-full max-w-80">
        <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t.search}
          className="h-9 w-full rounded-md border border-input bg-background pr-3 pl-8 text-[13.5px] outline-none focus:border-ring"
        />
      </label>
      <div className="overflow-x-auto rounded-lg border border-border bg-card">
        <table className="w-full min-w-[720px] text-[13.5px]">
          <thead>
            <tr className="border-b border-border text-left text-[12.5px] text-muted-foreground">
              <th className="px-4 py-2.5 font-normal">{t.account}</th>
              <th className="px-4 py-2.5 font-normal">{t.signsInWith}</th>
              <th className="px-4 py-2.5 font-normal">{t.plan}</th>
              <th className="px-4 py-2.5 text-right font-normal">{t.credits}</th>
              <th className="px-4 py-2.5 text-right font-normal">{t.seen}</th>
              <th className="w-10" />
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id} className="cursor-pointer border-b border-border/50 last:border-0 hover:bg-row-hover" onClick={() => onOpen(u.id)}>
                <td className="px-4 py-2.5">
                  <div className="flex items-center gap-2.5">
                    <Avatar photo={u.tg_photo} label={u.label} className="size-7 text-[12px]" />
                    <div className="min-w-0">
                      <div className="flex items-center gap-1.5 font-medium">
                        <span className="truncate">{u.label}</span>
                        {u.role === "admin" && <span className="rounded-sm border border-primary/40 px-1 text-[10px] text-primary">{t.admin}</span>}
                      </div>
                      <div className="truncate text-[12px] text-muted-foreground">{t.joined(u.id, ago(u.created_at))}</div>
                    </div>
                  </div>
                </td>
                <td className="px-4 py-2.5 text-muted-foreground">{u.tg_username ? `@${u.tg_username}` : u.tg_id ? `Telegram ${u.tg_id}` : ""}</td>
                <td className="px-4 py-2.5">
                  <PlanText u={u} />
                </td>
                <td className="data px-4 py-2.5 text-right">{num(u.credits)}</td>
                <td className="data px-4 py-2.5 text-right text-[12.5px] text-muted-foreground">{u.seen_at ? ago(u.seen_at) : "–"}</td>
                <td className="px-2 py-2.5" onClick={(e) => e.stopPropagation()}>
                  <DropdownMenu>
                    <DropdownMenuTrigger render={<Button variant="ghost" size="icon" className="size-7" aria-label={t.manage(u.label)} />}>⋯</DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-48">
                      <DropdownMenuItem onClick={() => onOpen(u.id)}>{t.openAccount}</DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem onClick={() => grant(u)}>{t.giveCredits}</DropdownMenuItem>
                      <DropdownMenuItem onClick={() => update(u, { plan: u.plan === "pro" ? "free" : "pro" }, u.plan === "pro" ? t.proRemoved : t.isPro(u.label))}>
                        {u.plan === "pro" ? t.removePro : t.grantPro}
                      </DropdownMenuItem>
                      {u.id !== me.id && (
                        <>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => update(u, { role: u.role === "admin" ? "user" : "admin" }, u.role === "admin" ? t.adminRemoved : t.isAdmin(u.label))}>
                            {u.role === "admin" ? t.removeAdmin : t.makeAdmin}
                          </DropdownMenuItem>
                        </>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </td>
              </tr>
            ))}
            {users.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-6 text-center text-muted-foreground">
                  {t.noAccounts}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function PlanText({ u }: { u: User }) {
  const t = useT().admin;
  return u.plan === "pro" ? (
    <span className="text-primary">Pro{u.plan_until > Date.now() / 1000 ? t.proUntil(new Date(u.plan_until * 1000).toLocaleDateString(locale())) : ""}</span>
  ) : (
    <span className="text-muted-foreground">{t.free}</span>
  );
}

// --- one account ---

function UserSheet({ id, onClose, onChanged }: { id: number | null; onClose: () => void; onChanged: () => void }) {
  const { user: me } = useAuth();
  const t = useT().admin;
  const features = useT().common.features;
  const [d, setD] = useState<AdminUserDetail | null>(null);
  const [hist, setHist] = useState<"activity" | "ledger" | "payments">("activity");
  const [mode, setMode] = useState<"set" | "add">("set");
  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    if (id == null) return;
    admin
      .user(id)
      .then((r) => {
        setD(r);
        setAmount(String(r.user.credits));
      })
      .catch((e) => toast.error(e.message));
  }, [id]);
  useEffect(() => {
    setD(null);
    setNote("");
    setMode("set");
    setHist("activity");
    load();
  }, [load]);

  const act = async (b: Parameters<typeof admin.updateUser>[1], done: string) => {
    if (!d) return;
    setBusy(true);
    try {
      await admin.updateUser(d.user.id, b);
      toast.success(done);
      load();
      onChanged();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const applyBalance = () => {
    const n = parseInt(amount, 10);
    if (!Number.isFinite(n) || String(n) !== amount.trim().replace(/^\+/, "")) return toast.error(t.badNumber);
    if (mode === "set") act({ set_credits: n, note }, t.balanceSet(n));
    else if (n !== 0) act({ grant: n, note }, t.granted(n, d!.user.label));
    setNote("");
  };

  const signOut = async () => {
    if (!d || !confirm(t.confirmSignOut(d.user.label))) return;
    try {
      await admin.logoutUser(d.user.id);
      toast.success(t.signedOut);
      load();
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const u = d?.user;
  const f = d?.footprint;
  const proBought = u && u.plan_until > Date.now() / 1000;

  return (
    <Sheet open={id != null} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="w-full gap-0 overflow-y-auto p-0 data-[side=right]:sm:max-w-[560px]">
        <div className="border-b border-border p-5 pr-14">
          <div className="flex items-center gap-3">
            <Avatar photo={u?.tg_photo ?? ""} label={u?.label ?? "?"} className="size-11 text-[15px]" />
            <div className="min-w-0">
              <SheetTitle className="flex items-center gap-2 text-[18px] font-semibold tracking-tight">
                <span className="truncate">{u?.label ?? "…"}</span>
                {u?.role === "admin" && <span className="rounded-sm border border-primary/40 px-1 text-[10px] font-normal text-primary">{t.admin}</span>}
              </SheetTitle>
              <SheetDescription className="text-[12.5px]">{u ? t.joined(u.id, ago(u.created_at)) : ""}</SheetDescription>
            </div>
            {u && (
              <div className="ml-auto text-right">
                <div className="text-[11.5px] text-muted-foreground">{t.balance}</div>
                <div className="data text-2xl font-semibold tracking-tight">{num(u.credits)}</div>
              </div>
            )}
          </div>
        </div>

        {d && u && f && (
          <div className="space-y-6 p-5">
            <div className="grid grid-cols-3 gap-px overflow-hidden rounded-lg border border-border bg-border">
              <Mini label={t.stat.spent} v={num(f.spent)} />
              <Mini label={t.stat.bought} v={num(f.bought)} />
              <Mini label={t.stat.granted} v={(f.granted > 0 ? "+" : "") + num(f.granted)} />
              <Mini label={t.stat.aiCost} v={usd(f.ai_cost)} />
              <Mini label={t.stat.sessions} v={num(f.sessions)} />
              <Mini label={t.stat.trackers} v={num(f.trackers)} />
              <Mini label={t.stat.concepts} v={num(f.concepts)} />
              <Mini label={t.stat.portfolio} v={num(f.portfolio)} />
              <Mini label={t.stat.starred} v={num(f.starred)} />
            </div>

            <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-1.5 text-[13px]">
              {u.login && <Row k={t.profile.login} v={u.login} />}
              {u.email && <Row k={t.profile.email} v={u.email} />}
              <Row k={t.profile.telegram} v={u.tg_id ? `${u.tg_username ? "@" + u.tg_username + " · " : ""}${u.tg_id}` : "–"} />
              {u.tg_id ? <Row k={t.profile.twofa} v={u.tg_2fa ? t.yes : t.no} /> : null}
              <Row k={t.profile.lang} v={u.lang ?? "–"} />
              <Row k={t.profile.joined} v={new Date(u.created_at * 1000).toLocaleString(locale())} />
              <Row k={t.profile.seen} v={u.seen_at ? ago(u.seen_at) : "–"} />
            </dl>

            <section className="space-y-2.5">
              <h3 className="flex items-center gap-1.5 text-[13px] font-medium">
                <Coins className="size-3.5 text-muted-foreground" />
                {t.balance}
              </h3>
              <div className="flex flex-wrap items-center gap-2">
                <div className="flex rounded-md border border-border p-0.5 text-[12.5px]">
                  {(["set", "add"] as const).map((m) => (
                    <button
                      key={m}
                      onClick={() => {
                        setMode(m);
                        setAmount(m === "set" ? String(u.credits) : "");
                      }}
                      className={cn("rounded px-2.5 py-1 transition-colors", mode === m ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
                    >
                      {m === "set" ? t.setBalance : t.adjust}
                    </button>
                  ))}
                </div>
                <input
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && applyBalance()}
                  inputMode="numeric"
                  placeholder={mode === "add" ? "+500 / -200" : "0"}
                  className="data h-8 w-28 rounded-md border border-input bg-background px-2.5 text-[13.5px] outline-none focus:border-ring"
                />
                {mode === "add" &&
                  [100, 500, 1000].map((n) => (
                    <button key={n} onClick={() => setAmount(String(n))} className="h-8 rounded-md border border-border px-2 text-[12px] text-muted-foreground hover:text-foreground">
                      +{n}
                    </button>
                  ))}
                <Button size="sm" className="h-8" disabled={busy} onClick={applyBalance}>
                  {t.apply}
                </Button>
              </div>
              <input
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder={t.notePlaceholder}
                className="h-8 w-full rounded-md border border-input bg-background px-2.5 text-[13px] outline-none focus:border-ring"
              />
            </section>

            <section className="space-y-2.5">
              <h3 className="flex items-center gap-1.5 text-[13px] font-medium">
                <Shield className="size-3.5 text-muted-foreground" />
                {t.access}
              </h3>
              <div className="text-[13px] text-muted-foreground">
                {u.plan === "pro" && !proBought ? t.proGranted : proBought ? t.proBought(new Date(u.plan_until * 1000).toLocaleDateString(locale())) : t.onFree}
              </div>
              <div className="flex flex-wrap gap-2">
                <Button size="sm" variant="outline" className="h-8" disabled={busy} onClick={() => act({ plan: u.plan === "pro" && !proBought ? "free" : "pro" }, u.plan === "pro" && !proBought ? t.proRemoved : t.isPro(u.label))}>
                  {u.plan === "pro" && !proBought ? t.removePro : t.grantPro}
                </Button>
                {[7, 30].map((n) => (
                  <Button key={n} size="sm" variant="outline" className="h-8" disabled={busy} onClick={() => act({ pro_days: n }, t.daysAdded(n))}>
                    {t.addDays(n)}
                  </Button>
                ))}
                {proBought && (
                  <Button size="sm" variant="outline" className="h-8" disabled={busy} onClick={() => act({ pro_days: -7 }, t.daysAdded(-7))}>
                    {t.addDays(-7)}
                  </Button>
                )}
                {u.id !== me.id && (
                  <Button size="sm" variant="outline" className="h-8" disabled={busy} onClick={() => act({ role: u.role === "admin" ? "user" : "admin" }, u.role === "admin" ? t.adminRemoved : t.isAdmin(u.label))}>
                    {u.role === "admin" ? t.removeAdmin : t.makeAdmin}
                  </Button>
                )}
                <Button size="sm" variant="outline" className="h-8 gap-1.5 text-destructive hover:text-destructive" onClick={signOut}>
                  <LogOut className="size-3.5" />
                  {t.signOutAll}
                </Button>
              </div>
            </section>

            <section>
              <div className="mb-2 flex rounded-md border border-border p-0.5 text-[12.5px]">
                {(["activity", "ledger", "payments"] as const).map((k) => (
                  <button
                    key={k}
                    onClick={() => setHist(k)}
                    className={cn("flex-1 rounded px-2.5 py-1 transition-colors", hist === k ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
                  >
                    {t.history[k]}
                    <span className="data ml-1 opacity-60">{k === "activity" ? d.activity.length : k === "ledger" ? d.ledger.length : d.payments.length}</span>
                  </button>
                ))}
              </div>
              <div className="overflow-hidden rounded-lg border border-border">
                {hist === "activity" &&
                  (d.activity.length ? <ActivityList items={d.activity} showUser={false} /> : <Empty text={t.noActivity} />)}
                {hist === "ledger" &&
                  (d.ledger.length ? (
                    <ul className="divide-y divide-border/50">
                      {d.ledger.map((e, i) => {
                        const credit = e.feature === "topup" || e.feature === "grant";
                        const signed = credit ? e.credits : -e.credits;
                        return (
                          <li key={i} className="flex items-center gap-3 px-4 py-2 text-[13px]">
                            <span className={cn("grid size-6 shrink-0 place-items-center rounded-md bg-muted", credit ? "text-positive" : "text-primary")}>
                              {e.feature === "grant" ? <Shield className="size-3.5" /> : e.feature === "topup" ? <CreditCard className="size-3.5" /> : <Bot className="size-3.5" />}
                            </span>
                            <div className="min-w-0 flex-1">
                              <div className="font-medium">{(features as Record<string, string>)[e.feature] ?? e.feature}</div>
                              <div className="truncate text-[12px] text-muted-foreground">
                                {[e.note, e.tokens ? t.tokens(num(e.tokens)) : "", e.cost ? `$${e.cost.toFixed(4)}` : ""].filter(Boolean).join(" · ")}
                              </div>
                            </div>
                            <span className={cn("data w-16 text-right font-medium", signed > 0 ? "text-positive" : signed < 0 ? "text-foreground" : "text-muted-foreground")}>
                              {signed > 0 ? "+" : ""}
                              {num(signed)}
                            </span>
                            <span className="data w-16 shrink-0 text-right text-[12px] text-muted-foreground">{ago(e.ts)}</span>
                          </li>
                        );
                      })}
                    </ul>
                  ) : (
                    <Empty text={t.noLedger} />
                  ))}
                {hist === "payments" && (d.payments.length ? <PaymentRows payments={d.payments} showUser={false} /> : <Empty text={t.noPayments} />)}
              </div>
            </section>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

function Mini({ label, v }: { label: string; v: string }) {
  return (
    <div className="bg-card px-3 py-2.5">
      <div className="truncate text-[11.5px] text-muted-foreground">{label}</div>
      <div className="data mt-0.5 text-[15px] font-semibold">{v}</div>
    </div>
  );
}

function Row({ k, v }: { k: string; v: string }) {
  return (
    <>
      <dt className="text-muted-foreground">{k}</dt>
      <dd className="truncate">{v}</dd>
    </>
  );
}

function Empty({ text }: { text: string }) {
  return <div className="px-4 py-6 text-center text-[13px] text-muted-foreground">{text}</div>;
}

// --- payments ---

function PaymentsTab() {
  const t = useT().admin;
  const [payments, setPayments] = useState<Payment[] | null>(null);
  useEffect(() => {
    admin
      .payments()
      .then((r) => setPayments(r.payments))
      .catch(() => setPayments([]));
  }, []);
  return (
    <div className="overflow-x-auto rounded-lg border border-border bg-card">
      {payments && (payments.length ? <PaymentRows payments={payments} /> : <Empty text={t.noPayments} />)}
    </div>
  );
}

function PaymentRows({ payments, showUser = true }: { payments: Payment[]; showUser?: boolean }) {
  const t = useT().admin;
  return (
    <table className="w-full min-w-[480px] text-[13px]">
      <thead>
        <tr className="border-b border-border text-left text-[12px] text-muted-foreground">
          <th className="px-4 py-2 font-normal">{t.when}</th>
          {showUser && <th className="px-4 py-2 font-normal">{t.account}</th>}
          <th className="px-4 py-2 font-normal">{t.pack}</th>
          <th className="px-4 py-2 text-right font-normal">{t.amount}</th>
          <th className="px-4 py-2 font-normal">{t.status}</th>
        </tr>
      </thead>
      <tbody>
        {payments.map((p) => (
          <tr key={p.id} className="border-b border-border/50 last:border-0">
            <td className="data px-4 py-2 text-[12.5px] text-muted-foreground">{ago(p.paid_at || p.created_at)}</td>
            {showUser && <td className="px-4 py-2">{p.user ?? `#${p.user_id}`}</td>}
            <td className="px-4 py-2 text-muted-foreground">{p.pack}</td>
            <td className="data px-4 py-2 text-right">{p.currency === "XTR" ? `${p.amount} ★` : `${p.amount} TON`}</td>
            <td className="px-4 py-2">
              <span className={cn("text-[12.5px]", p.status === "paid" ? "text-positive" : p.status === "pending" ? "text-warning" : "text-muted-foreground")}>{t.statuses[p.status] ?? p.status}</span>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

