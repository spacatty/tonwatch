"use client";

import { Check, Copy, Loader2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { billing, creditsChanged, studio, type Credits, type Pack, type Payment, type PayHow } from "@/lib/api";
import { miniApp, openExternal } from "@/lib/telegram";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { useAuth } from "./Auth";
import { ago } from "./bits";

const FEATURES: Record<string, string> = {
  ideas: "Ideas run",
  rotate: "Rotate a concept",
  refine: "Make it mine",
  angle: "Why this name",
  scan: "Why these (grid)",
  profile: "Taste profile",
  ask: "Ask AI",
  topic: "Topic expand",
  imagine: "Imagine a project",
  chat: "Concept chat",
  topup: "Top-up",
  grant: "Granted",
};

type Gateway = { id: "ton" | "stars"; label: string };

/** The wallet: balance, buying credits or Pro (TON, Telegram Stars), and the ledger of what each AI action cost. */
export function CreditsDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { refresh } = useAuth();
  const [c, setC] = useState<Credits | null>(null);
  const [packs, setPacks] = useState<Pack[]>([]);
  const [gateways, setGateways] = useState<Gateway[]>([]);
  const [gw, setGw] = useState<Gateway["id"] | null>(null);
  const [checkout, setCheckout] = useState<{ payment: Payment; pay: PayHow; pack: Pack } | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    const load = () => studio.credits().then(setC).catch(() => {});
    load();
    billing
      .packs()
      .then((r) => {
        setPacks(r.packs);
        setGateways(r.gateways);
        // inside Telegram, Stars is one tap; elsewhere TON is the default
        setGw((g) => g ?? (miniApp() && r.gateways.some((x) => x.id === "stars") ? "stars" : r.gateways[0]?.id ?? null));
      })
      .catch(() => {});
    creditsChanged.addEventListener("change", load);
    return () => creditsChanged.removeEventListener("change", load);
  }, [open]);

  const paid = (p: Payment) => {
    toast.success(p.credits ? `+${p.credits.toLocaleString()} credits` : `Pro for ${p.pro_days} days`);
    setCheckout(null);
    creditsChanged.dispatchEvent(new Event("change"));
    refresh();
  };

  const buy = async (pack: Pack) => {
    if (!gw) return;
    setBusy(pack.id);
    try {
      const r = await billing.checkout(pack.id, gw);
      setCheckout({ ...r, pack });
      if (gw === "stars" && r.pay.invoice_link) {
        const tg = miniApp();
        if (tg) tg.openInvoice(r.pay.invoice_link);
        else openExternal(r.pay.invoice_link);
      }
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const spentCost = c?.ledger.reduce((a, l) => a + (l.feature === "topup" || l.feature === "grant" ? 0 : l.cost), 0) ?? 0;
  const price = (p: Pack) => (gw === "stars" ? `${p.stars.toLocaleString()} ★` : `${p.ton} TON`);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Credits and plan</DialogTitle>
          <DialogDescription>Every AI feature spends credits. You get {c?.daily ?? "–"} free each day; credits you buy add on top and never expire. Cached answers are free.</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-7">
          <div className="grid grid-cols-2 overflow-hidden rounded-lg border border-border sm:grid-cols-3">
            <Stat label="Balance" v={c ? c.balance.toLocaleString() : "–"} accent />
            <Stat label="Free each day" v={c ? c.daily.toLocaleString() : "–"} />
            <Stat label="AI cost, last 60" v={c ? `$${spentCost.toFixed(3)}` : "–"} className="hidden sm:block" />
          </div>

          <section>
            <div className="mb-3 flex flex-wrap items-baseline justify-between gap-3">
              <span className="text-[14px] font-medium">Top up</span>
              {gateways.length > 1 && (
                <div className="flex rounded-md border border-border p-0.5 text-[13px]">
                  {gateways.map((g) => (
                    <button
                      key={g.id}
                      onClick={() => {
                        setGw(g.id);
                        setCheckout(null);
                      }}
                      className={cn("rounded px-2.5 py-1 transition-colors", gw === g.id ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
                    >
                      {g.label}
                    </button>
                  ))}
                </div>
              )}
            </div>
            {gateways.length === 0 ? (
              <p className="rounded-lg border border-dashed border-border px-4 py-5 text-[13.5px] text-muted-foreground">
                Payments are not set up on this install yet. An admin can enable TON (TON_WALLET) or Telegram Stars (the bot) in .env.
              </p>
            ) : checkout ? (
              <Checkout {...checkout} onPaid={paid} onBack={() => setCheckout(null)} />
            ) : (
              <div className="grid gap-2 sm:grid-cols-2">
                {packs.map((p) => (
                  <button
                    key={p.id}
                    onClick={() => buy(p)}
                    disabled={!!busy}
                    className="group flex items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3 text-left transition-colors hover:border-primary disabled:opacity-60"
                  >
                    <span>
                      <span className="block text-[14px] font-medium">{p.title}</span>
                      <span className="text-[12.5px] text-muted-foreground">{p.note ?? (p.credits ? "AI credits" : "")}</span>
                    </span>
                    <span className="data flex items-center gap-2 text-[14px] font-semibold text-primary">
                      {busy === p.id && <Loader2 className="size-3.5 animate-spin" />}
                      {price(p)}
                    </span>
                  </button>
                ))}
              </div>
            )}
          </section>

          <div className="grid gap-x-8 gap-y-3 sm:grid-cols-[190px_1fr]">
            <div>
              <div className="text-[14px] font-medium">Prices</div>
              <div className="mt-1 text-[12.5px] leading-snug text-muted-foreground">Credits per action.</div>
            </div>
            <div className="grid grid-cols-2 gap-x-6 gap-y-1.5 text-[13px]">
              {Object.entries(c?.prices ?? {})
                .sort((a, b) => b[1] - a[1])
                .map(([k, v]) => (
                  <div key={k} className="flex items-baseline justify-between gap-3 border-b border-border/50 pb-1.5">
                    <span className="text-muted-foreground">{FEATURES[k] ?? k}</span>
                    <span className="data">{v === 0 ? "free" : v}</span>
                  </div>
                ))}
            </div>
          </div>

          <div>
            <div className="mb-2.5 flex items-baseline justify-between">
              <span className="text-[14px] font-medium">Ledger</span>
              <span className="text-[12.5px] text-muted-foreground">last {c?.ledger.length ?? 0} entries</span>
            </div>
            <div className="max-h-64 overflow-y-auto rounded-lg border border-border">
              {c?.ledger.length === 0 && <p className="px-4 py-5 text-sm text-muted-foreground">Nothing spent yet.</p>}
              {c?.ledger.map((l, i) => {
                const credit = l.feature === "topup" || l.feature === "grant";
                return (
                  <div key={i} className="flex h-10 items-center gap-3 border-b border-border/50 px-4 text-[13px] last:border-0">
                    <span className="w-32 shrink-0">{FEATURES[l.feature] ?? l.feature}</span>
                    <span className="min-w-0 flex-1 truncate text-muted-foreground">{l.note}</span>
                    {!credit && (
                      <span className="data hidden shrink-0 text-[12px] text-muted-foreground sm:inline" title={l.cached ? `${l.cached.toLocaleString()} prompt tokens served from the provider's cache` : undefined}>
                        {l.tokens.toLocaleString()} tok{l.cached ? ` · ${Math.round((100 * l.cached) / Math.max(1, l.tokens))}% cached` : ""} · ${l.cost.toFixed(4)}
                      </span>
                    )}
                    <span className={cn("data w-14 shrink-0 text-right font-medium", credit && "text-positive")}>
                      {credit ? `+${l.credits.toLocaleString()}` : l.credits ? `−${l.credits}` : "0"}
                    </span>
                    <span className="data w-16 shrink-0 text-right text-[12px] text-muted-foreground">{ago(l.ts)}</span>
                  </div>
                );
              })}
            </div>
          </div>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

/** One open checkout: how to pay it, and a live status until the money lands. */
function Checkout({ payment, pay, pack, onPaid, onBack }: { payment: Payment; pay: PayHow; pack: Pack; onPaid: (p: Payment) => void; onBack: () => void }) {
  const [status, setStatus] = useState(payment.status);
  const done = useRef(onPaid);
  done.current = onPaid;
  useEffect(() => {
    let alive = true;
    const t = setInterval(async () => {
      const r = await billing.payment(payment.id).catch(() => null);
      if (!alive || !r) return;
      setStatus(r.payment.status);
      if (r.payment.status === "paid") {
        clearInterval(t);
        done.current(r.payment);
      }
    }, 4000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [payment.id]);

  const expired = status === "expired";
  return (
    <div className="rounded-lg border border-border bg-card p-4 sm:p-5">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-[15px] font-medium">{pack.title}</span>
        <span className="data text-[15px] font-semibold text-primary">
          {payment.currency === "XTR" ? `${payment.amount.toLocaleString()} ★` : `${payment.amount} TON`}
        </span>
      </div>

      {payment.gateway === "ton" ? (
        <div className="mt-4 space-y-2.5">
          <p className="text-[13px] leading-snug text-muted-foreground">
            Send exactly this amount <b className="text-foreground">with the comment</b>. The comment is how the payment finds your account.
          </p>
          <CopyRow label="Address" value={pay.address!} />
          <CopyRow label="Amount" value={String(pay.amount)} suffix="TON" />
          <CopyRow label="Comment" value={pay.comment!} strong />
          <div className="flex flex-wrap gap-2 pt-1.5">
            <Button className="h-9" onClick={() => openExternal(miniApp() ? pay.tonkeeper_link! : pay.ton_link!)}>
              Open wallet
            </Button>
            {!miniApp() && (
              <Button variant="outline" className="h-9" onClick={() => openExternal(pay.tonkeeper_link!)}>
                Tonkeeper
              </Button>
            )}
          </div>
        </div>
      ) : (
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <p className="w-full text-[13px] text-muted-foreground">Pay in Telegram. Credits arrive the moment Telegram confirms.</p>
          <Button className="h-9" onClick={() => (miniApp() ? miniApp()!.openInvoice(pay.invoice_link!) : openExternal(pay.invoice_link!))}>
            Pay with Stars
          </Button>
        </div>
      )}

      <div className="mt-4 flex items-center justify-between gap-3 border-t border-border pt-3 text-[13px]">
        <span className={cn("flex items-center gap-2", expired ? "text-destructive" : "text-muted-foreground")}>
          {!expired && <Loader2 className="size-3.5 animate-spin" />}
          {expired ? "This checkout expired. Money sent with its comment still counts." : "Waiting for the payment…"}
        </span>
        <button onClick={onBack} className="text-muted-foreground hover:text-foreground">
          Back
        </button>
      </div>
    </div>
  );
}

function CopyRow({ label, value, suffix, strong }: { label: string; value: string; suffix?: string; strong?: boolean }) {
  const [ok, setOk] = useState(false);
  return (
    <div className="flex items-center gap-3 rounded-md border border-border bg-background px-3 py-2">
      <span className="w-16 shrink-0 text-[12.5px] text-muted-foreground">{label}</span>
      <span className={cn("data min-w-0 flex-1 truncate text-[13px]", strong && "font-semibold text-primary")} title={value}>
        {value}
        {suffix && <span className="ml-1 text-muted-foreground">{suffix}</span>}
      </span>
      <button
        aria-label={`Copy ${label.toLowerCase()}`}
        onClick={() => {
          navigator.clipboard.writeText(value).then(() => {
            setOk(true);
            setTimeout(() => setOk(false), 1200);
          });
        }}
        className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        {ok ? <Check className="size-3.5 text-positive" /> : <Copy className="size-3.5" />}
      </button>
    </div>
  );
}

function Stat({ label, v, sub, accent, className }: { label: string; v: string; sub?: string; accent?: boolean; className?: string }) {
  return (
    <div className={cn("border-l border-border px-5 py-4 first:border-l-0", className)}>
      <div className="text-[12.5px] text-muted-foreground">{label}</div>
      <div className={cn("data mt-1.5 text-2xl font-semibold tracking-tight", accent && "text-primary")}>{v}</div>
      {sub && <div className="mt-0.5 text-[12px] text-muted-foreground">{sub}</div>}
    </div>
  );
}
