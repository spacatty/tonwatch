"use client";

import { Activity, Search } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { admin, type Payment, type User } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Avatar } from "./AccountDialog";
import { useAuth } from "./Auth";
import { ago } from "./bits";

type Overview = Awaited<ReturnType<typeof admin.overview>>;

/** Admin: who uses the install, what they paid, and the switches only admins have. */
export function AdminPanel({ onJobs }: { onJobs: () => void }) {
  const { user: me } = useAuth();
  const [ov, setOv] = useState<Overview | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [payments, setPayments] = useState<Payment[]>([]);
  const [q, setQ] = useState("");
  const [tab, setTab] = useState<"users" | "payments">("users");

  const load = useCallback(() => {
    admin.overview().then(setOv).catch((e) => toast.error(e.message));
    admin.payments().then((r) => setPayments(r.payments)).catch(() => {});
  }, []);
  useEffect(load, [load]);
  useEffect(() => {
    const t = setTimeout(() => admin.users(q).then((r) => setUsers(r.users)).catch(() => {}), 200);
    return () => clearTimeout(t);
  }, [q]);

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
    const v = prompt(`Credits to give ${u.label} (negative takes them away):`, "500");
    const n = v ? parseInt(v, 10) : NaN;
    if (Number.isFinite(n) && n !== 0) update(u, { grant: n }, `${n > 0 ? "+" : ""}${n} credits for ${u.label}`);
  };

  const paidTon = ov?.paid.TON ?? 0;
  const paidStars = ov?.paid.XTR ?? 0;

  return (
    <div className="space-y-8">
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-border bg-border md:grid-cols-4">
        <Tile label="Accounts" v={ov ? ov.users.toLocaleString() : "–"} />
        <Tile label="Paid in TON" v={ov ? paidTon.toLocaleString(undefined, { maximumFractionDigits: 2 }) : "–"} />
        <Tile label="Paid in Stars" v={ov ? paidStars.toLocaleString() : "–"} />
        <Tile label="AI spend since start" v={ov ? `$${ov.llm_usage.cost.toFixed(2)}` : "–"} />
      </div>

      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-[13px] text-muted-foreground">
        <span>
          Bot: <b className="font-medium text-foreground">{ov?.bot ? `@${ov.bot}` : "not set"}</b>
        </span>
        <span>
          TON payments: <b className={cn("font-medium", ov?.gateways.ton ? "text-positive" : "text-foreground")}>{ov?.gateways.ton ? "on" : "off"}</b>
        </span>
        <span>
          Stars: <b className={cn("font-medium", ov?.gateways.stars ? "text-positive" : "text-foreground")}>{ov?.gateways.stars ? "on" : "off"}</b>
        </span>
        <span>
          AI rating queue: <b className="data font-medium text-foreground">{ov?.stats.llm_queue.toLocaleString() ?? "–"}</b>
        </span>
        <Button variant="outline" size="sm" className="ml-auto h-8 gap-1.5" onClick={onJobs}>
          <Activity className="size-3.5" />
          Collectors
        </Button>
      </div>

      <div>
        <div className="mb-3 flex flex-wrap items-center gap-3">
          <div className="flex rounded-md border border-border p-0.5 text-[13.5px]">
            {(["users", "payments"] as const).map((t) => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={cn("rounded px-3 py-1 capitalize transition-colors", tab === t ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
              >
                {t}
              </button>
            ))}
          </div>
          {tab === "users" && (
            <label className="relative ml-auto w-full max-w-72">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="Login, email, @handle or Telegram id"
                className="h-9 w-full rounded-md border border-input bg-background pr-3 pl-8 text-[13.5px] outline-none focus:border-ring"
              />
            </label>
          )}
        </div>

        {tab === "users" ? (
          <div className="overflow-x-auto rounded-lg border border-border bg-card">
            <table className="w-full min-w-[720px] text-[13.5px]">
              <thead>
                <tr className="border-b border-border text-left text-[12.5px] text-muted-foreground">
                  <th className="px-4 py-2.5 font-normal">Account</th>
                  <th className="px-4 py-2.5 font-normal">Signs in with</th>
                  <th className="px-4 py-2.5 font-normal">Plan</th>
                  <th className="px-4 py-2.5 text-right font-normal">Credits</th>
                  <th className="px-4 py-2.5 text-right font-normal">Seen</th>
                  <th className="w-10" />
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.id} className="border-b border-border/50 last:border-0 hover:bg-row-hover">
                    <td className="px-4 py-2.5">
                      <div className="flex items-center gap-2.5">
                        <Avatar photo={u.tg_photo} label={u.label} className="size-7 text-[12px]" />
                        <div className="min-w-0">
                          <div className="flex items-center gap-1.5 font-medium">
                            <span className="truncate">{u.label}</span>
                            {u.role === "admin" && <span className="rounded-sm border border-primary/40 px-1 text-[10px] text-primary">ADMIN</span>}
                          </div>
                          <div className="truncate text-[12px] text-muted-foreground">{u.email || `#${u.id} · joined ${ago(u.created_at)}`}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-muted-foreground">
                      {[u.tg_id ? (u.tg_username ? `@${u.tg_username}` : "Telegram") : null, u.has_password ? "password" : null, u.tg_2fa ? "2FA" : null].filter(Boolean).join(" · ")}
                    </td>
                    <td className="px-4 py-2.5">
                      {u.plan === "pro" ? (
                        <span className="text-primary">Pro{u.plan_until > Date.now() / 1000 ? ` until ${new Date(u.plan_until * 1000).toLocaleDateString()}` : ""}</span>
                      ) : (
                        <span className="text-muted-foreground">Free</span>
                      )}
                    </td>
                    <td className="data px-4 py-2.5 text-right">{u.credits.toLocaleString()}</td>
                    <td className="data px-4 py-2.5 text-right text-[12.5px] text-muted-foreground">{u.seen_at ? ago(u.seen_at) : "–"}</td>
                    <td className="px-2 py-2.5">
                      <DropdownMenu>
                        <DropdownMenuTrigger render={<Button variant="ghost" size="icon" className="size-7" aria-label={`Manage ${u.label}`} />}>⋯</DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-48">
                          <DropdownMenuItem onClick={() => grant(u)}>Give credits…</DropdownMenuItem>
                          <DropdownMenuItem onClick={() => update(u, { plan: u.plan === "pro" ? "free" : "pro" }, u.plan === "pro" ? "Pro grant removed" : `${u.label} is Pro`)}>
                            {u.plan === "pro" ? "Remove Pro grant" : "Grant Pro"}
                          </DropdownMenuItem>
                          {u.id !== me.id && (
                            <>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem onClick={() => update(u, { role: u.role === "admin" ? "user" : "admin" }, u.role === "admin" ? "Admin removed" : `${u.label} is an admin`)}>
                                {u.role === "admin" ? "Remove admin" : "Make admin"}
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
                      No accounts match.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border bg-card">
            <table className="w-full min-w-[640px] text-[13.5px]">
              <thead>
                <tr className="border-b border-border text-left text-[12.5px] text-muted-foreground">
                  <th className="px-4 py-2.5 font-normal">When</th>
                  <th className="px-4 py-2.5 font-normal">Account</th>
                  <th className="px-4 py-2.5 font-normal">Pack</th>
                  <th className="px-4 py-2.5 text-right font-normal">Amount</th>
                  <th className="px-4 py-2.5 font-normal">Status</th>
                </tr>
              </thead>
              <tbody>
                {payments.map((p) => (
                  <tr key={p.id} className="border-b border-border/50 last:border-0">
                    <td className="data px-4 py-2.5 text-[12.5px] text-muted-foreground">{ago(p.paid_at || p.created_at)}</td>
                    <td className="px-4 py-2.5">{p.user ?? `#${p.user_id}`}</td>
                    <td className="px-4 py-2.5 text-muted-foreground">{p.pack}</td>
                    <td className="data px-4 py-2.5 text-right">{p.currency === "XTR" ? `${p.amount} ★` : `${p.amount} TON`}</td>
                    <td className="px-4 py-2.5">
                      <span className={cn("text-[12.5px]", p.status === "paid" ? "text-positive" : p.status === "pending" ? "text-warning" : "text-muted-foreground")}>{p.status}</span>
                    </td>
                  </tr>
                ))}
                {payments.length === 0 && (
                  <tr>
                    <td colSpan={5} className="px-4 py-6 text-center text-muted-foreground">
                      No payments yet.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

function Tile({ label, v }: { label: string; v: string }) {
  return (
    <div className="bg-card px-5 py-4">
      <div className="text-[12.5px] text-muted-foreground">{label}</div>
      <div className="data mt-1.5 text-2xl font-semibold tracking-tight">{v}</div>
    </div>
  );
}
