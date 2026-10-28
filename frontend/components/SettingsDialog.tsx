"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { studio, type AlertsStatus, type Settings } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";

interface Props {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  settings: Settings;
  categories: string[];
  onSaved: (s: Settings) => void;
  /** open the wallet to buy Pro */
  onUpgrade: () => void;
  /** open the account to link Telegram */
  onAccount: () => void;
}

const BUDGETS = [25, 50, 100, 250, 500, 1000, 0];

/** Sniper settings: the budget and bar that Snipes and Ideas work within. */
export function SettingsDialog({ open, onOpenChange, settings, categories, onSaved, onUpgrade, onAccount }: Props) {
  const [d, setD] = useState<Settings>(settings);
  const [busy, setBusy] = useState(false);
  const [alerts, setAlerts] = useState<AlertsStatus | null>(null);
  useEffect(() => {
    if (!open) return;
    setD(settings);
    studio.alerts().then(setAlerts).catch(() => {});
  }, [open, settings]);

  const testAlert = async () => {
    try {
      onSaved(await studio.saveSettings(d));
      setAlerts(await studio.alertTest());
      toast.success("Sent a test message to Telegram");
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const save = async () => {
    setBusy(true);
    try {
      const s = await studio.saveSettings(d);
      onSaved(s);
      onOpenChange(false);
      toast.success("Settings saved");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Sniper settings</DialogTitle>
          <DialogDescription>What counts as worth it, and how the AI should think about you. Snipes and Ideas only look at names inside these limits.</DialogDescription>
        </DialogHeader>

        <DialogBody className="pb-2">
          <Section title="What counts">
            <Field label="Budget per name" hint={d.max_buyout ? `Buy-now price up to ${d.max_buyout.toLocaleString()} TON` : "No cap"}>
              <div className="flex flex-wrap gap-1.5">
                {BUDGETS.map((b) => (
                  <Pick key={b} on={d.max_buyout === b} onClick={() => setD({ ...d, max_buyout: b })}>
                    {b ? `≤ ${b.toLocaleString()}` : "Any"}
                  </Pick>
                ))}
                <label className="data flex h-8 items-center gap-1.5 rounded-md border border-input px-2.5 text-[13px] focus-within:border-primary">
                  <input
                    type="number"
                    min={0}
                    value={d.max_buyout || ""}
                    onChange={(e) => setD({ ...d, max_buyout: Number(e.target.value) || 0 })}
                    placeholder="custom"
                    className="w-16 bg-transparent outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none"
                  />
                  <span className="text-muted-foreground">TON</span>
                </label>
              </div>
            </Field>

            <Field label="Minimum AI score" hint="Names rated below this are left out of the home screen and the AI's picks.">
              <div className="flex flex-wrap gap-1.5">
                {[5, 6, 7, 8, 9].map((n) => (
                  <Pick key={n} on={d.min_score === n} onClick={() => setD({ ...d, min_score: n })}>
                    {n}+
                  </Pick>
                ))}
              </div>
            </Field>

            <Field label="Kinds you care about" hint={d.kinds.length ? "Ideas lean on these first; Snipes stays open to all." : "None picked: any kind."}>
              <div className="flex flex-wrap gap-1.5">
                {categories
                  .filter((c) => c !== "junk")
                  .map((c) => {
                    const on = d.kinds.includes(c);
                    return (
                      <Pick key={c} on={on} onClick={() => setD({ ...d, kinds: on ? d.kinds.filter((k) => k !== c) : [...d.kinds, c] })} className="capitalize">
                        {c}
                      </Pick>
                    );
                  })}
              </div>
            </Field>
          </Section>

          <Section title="How the AI sees you">
            <Field label="About you" hint="One or two lines on what you build or collect.">
              <Textarea value={d.about} onChange={(e) => setD({ ...d, about: e.target.value.slice(0, 400) })} placeholder="I run a small studio making tools for Telegram creators…" rows={3} className="text-[14px]" />
            </Field>
            <Field label="Never suggest" hint="A negative prompt for every AI feature: ideas, imagine, why-this-name and chats steer clear of it.">
              <Textarea value={d.avoid ?? ""} onChange={(e) => setD({ ...d, avoid: e.target.value.slice(0, 400) })} placeholder="Banks, crypto startups, trading bots, futures, spot, anything finance…" rows={3} className="text-[14px]" />
            </Field>
          </Section>

          <Section title="Plan and alerts">
            <Field
              label="Plan"
              hint={
                d.plan === "pro"
                  ? "Pro: Telegram alerts for new names inside these settings, tracked auctions an hour from closing, and price drops."
                  : "Free: everything but Telegram alerts."
              }
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className={cn("rounded-md border px-2.5 py-1 text-[13px] font-medium", d.plan === "pro" ? "border-primary text-primary" : "border-border")}>
                  {d.plan === "pro" ? "Pro" : "Free"}
                </span>
                <Button size="sm" variant="outline" className="h-8" onClick={onUpgrade}>
                  {d.plan === "pro" ? "Extend Pro" : "Get Pro"}
                </Button>
              </div>
            </Field>

            {d.plan === "pro" && (
              <Field
                label="Telegram alerts"
                hint={
                  !alerts?.bot_set
                    ? "This install has no Telegram bot yet."
                    : alerts.active
                      ? `Every 5 minutes, from @${alerts.bot}, to your linked Telegram.`
                      : alerts.why
                }
              >
                <div className="flex flex-wrap items-center gap-1.5">
                  {alerts?.bot_set && !d.telegram_chat ? (
                    <Button size="sm" variant="outline" onClick={onAccount} className="h-8">
                      Link Telegram
                    </Button>
                  ) : (
                    <Button size="sm" variant="outline" onClick={testAlert} disabled={!alerts?.active} className="h-8">
                      Send a test
                    </Button>
                  )}
                  {alerts && alerts.sent > 0 && <span className="text-xs text-muted-foreground">{alerts.sent} sent so far</span>}
                </div>
              </Field>
            )}
          </Section>
        </DialogBody>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={save} disabled={busy} className="min-w-24">
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** A group of fields under a small heading, on a rule. */
function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="border-t border-border py-5 first:border-t-0 first:pt-1">
      <h3 className="mb-4 text-[12px] font-medium tracking-wide text-muted-foreground uppercase">{title}</h3>
      <div className="space-y-5">{children}</div>
    </section>
  );
}

/** Label and hint on the left, the control on the right; stacked on narrow screens. */
function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="grid gap-x-8 gap-y-2.5 sm:grid-cols-[190px_1fr]">
      <div>
        <div className="text-[14px] font-medium">{label}</div>
        {hint && <div className="mt-1 text-[12.5px] leading-snug text-muted-foreground">{hint}</div>}
      </div>
      <div className="min-w-0 sm:pt-0.5">{children}</div>
    </div>
  );
}

function Pick({ on, onClick, className, children }: { on: boolean; onClick: () => void; className?: string; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "data h-8 rounded-md border px-3 text-[13px] transition-colors",
        on ? "border-primary bg-primary/10 text-primary" : "border-input text-foreground hover:border-muted-foreground/50 hover:bg-muted",
        className,
      )}
    >
      {children}
    </button>
  );
}
