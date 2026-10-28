"use client";

import { Briefcase, ExternalLink, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { fragmentUrl, studio, type Portfolio as PortfolioData, type PortfolioItem } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ago, Score, StatusPill, Ton } from "./bits";

interface Props {
  onOpen: (name: string) => void;
}

/** Portfolio: the names you own, what you paid, what names of that score go for now, and the concept each is for. */
export function Portfolio({ onOpen }: Props) {
  const [p, setP] = useState<PortfolioData | null>(null);
  const [adding, setAdding] = useState(false);
  const [edit, setEdit] = useState<PortfolioItem | null>(null);

  useEffect(() => {
    studio.portfolio().then(setP).catch((e) => toast.error((e as Error).message));
  }, []);

  const remove = async (name: string) => {
    try {
      setP(await studio.removePortfolio(name));
      toast(`@${name} removed from the portfolio`);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const delta = p ? p.now - p.paid : 0;

  return (
    <div className="space-y-8">
      <section className="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-border bg-border lg:grid-cols-4">
        <Stat label="Names" v={p ? p.items.length.toLocaleString() : null} sub={p ? `${p.listed_now} listed for resale` : ""} />
        <Stat label="Paid" v={p ? <Ton v={p.paid} /> : null} sub="sum of known purchase prices" />
        <Stat label="Typical now" v={p ? <Ton v={p.now} /> : null} sub={p && p.unpriced ? `${p.unpriced} name${p.unpriced === 1 ? "" : "s"} without a typical price yet` : "what names of these scores sell for"} />
        <Stat
          label="Difference"
          v={p ? <span className={cn(delta > 0 ? "text-positive" : delta < 0 ? "text-destructive" : "")}>{delta >= 0 ? "+" : "−"}<Ton v={Math.abs(delta)} /></span> : null}
          sub="typical price minus what you paid"
        />
      </section>

      <section>
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-xl font-semibold tracking-tight">Your names</h2>
            <p className="text-sm text-muted-foreground">Bought through the app or added by hand. Each one can carry the project it is for.</p>
          </div>
          <Button onClick={() => { setEdit(null); setAdding(true); }} className="h-9">
            <Plus className="size-4" />
            Add a name
          </Button>
        </div>

        {(adding || edit) && (
          <PortfolioForm
            initial={edit}
            onDone={(np) => {
              if (np) setP(np);
              setAdding(false);
              setEdit(null);
            }}
          />
        )}

        {p == null ? (
          <div className="space-y-2">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-14 rounded-lg" />
            ))}
          </div>
        ) : p.items.length === 0 ? (
          <div className="panel flex items-center gap-4 px-5 py-8 text-sm text-muted-foreground">
            <Briefcase className="size-6 shrink-0 opacity-50" />
            Nothing here yet. Add the names you own and what you paid; the typical price for their score updates as sales come in.
          </div>
        ) : (
          <div className="panel divide-y divide-border/70">
            {p.items.map((it) => {
              const d = it.typical != null && it.bought != null ? it.typical - it.bought : null;
              return (
                <div key={it.name} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
                  <button onClick={() => onOpen(it.name)} className="min-w-0 flex-1 text-left">
                    <div className="flex items-center gap-2">
                      <span className="data truncate text-[15px] font-semibold">@{it.name}</span>
                      <Score u={it.item} />
                      <StatusPill u={it.item} className="text-[11px] text-muted-foreground" />
                    </div>
                    <div className="truncate text-[12px] text-muted-foreground">
                      {it.concept ? <span className="text-foreground/80">{it.concept}</span> : it.item.meaning || "No concept attached"}
                      {it.note ? ` · ${it.note}` : ""}
                    </div>
                  </button>
                  <div className="data grid w-56 shrink-0 grid-cols-3 text-right text-[13px]">
                    <div>
                      <div className="label">Paid</div>
                      <div>{it.bought != null ? <Ton v={it.bought} /> : "—"}</div>
                    </div>
                    <div>
                      <div className="label">Typical</div>
                      <div>{it.typical != null ? <Ton v={it.typical} /> : "—"}</div>
                    </div>
                    <div>
                      <div className="label">Diff</div>
                      <div className={cn(d != null && d > 0 && "text-positive", d != null && d < 0 && "text-destructive")}>{d != null ? `${d >= 0 ? "+" : "−"}${Math.abs(d).toLocaleString(undefined, { maximumFractionDigits: 1 })}` : "—"}</div>
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <button onClick={() => { setAdding(false); setEdit(it); }} className="h-7 rounded border border-input px-2 text-[12px] text-muted-foreground hover:text-foreground">
                      Edit
                    </button>
                    <a href={fragmentUrl(it.name)} target="_blank" rel="noreferrer" className="inline-flex size-7 items-center justify-center rounded text-muted-foreground/60 hover:text-foreground" aria-label="Open on Fragment">
                      <ExternalLink className="size-3.5" />
                    </a>
                    <button onClick={() => remove(it.name)} className="inline-flex size-7 items-center justify-center rounded text-muted-foreground/60 hover:text-destructive" aria-label={`Remove @${it.name}`}>
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                  <div className="data w-16 shrink-0 text-right text-[11px] text-muted-foreground">{ago(it.added_at)}</div>
                </div>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}

function PortfolioForm({ initial, onDone }: { initial: PortfolioItem | null; onDone: (p: PortfolioData | null) => void }) {
  const [name, setName] = useState(initial?.name ?? "");
  const [bought, setBought] = useState(initial?.bought != null ? String(initial.bought) : "");
  const [concept, setConcept] = useState(initial?.concept ?? "");
  const [note, setNote] = useState(initial?.note ?? "");
  const [busy, setBusy] = useState(false);
  const save = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      const v = parseFloat(bought);
      onDone(await studio.savePortfolio({ name: name.trim(), bought: Number.isFinite(v) ? v : null, concept, note }));
      toast.success(`@${name.replace(/^@/, "")} saved`);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const field = "data h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-[13px] outline-none focus:border-primary";
  return (
    <div className="panel mb-4 grid gap-3 p-4 sm:grid-cols-[1fr_120px_1fr_1fr_auto]">
      <label className="block">
        <div className="label mb-1">Username</div>
        <input value={name} onChange={(e) => setName(e.target.value)} disabled={!!initial} placeholder="@name" className={field} autoFocus={!initial} />
      </label>
      <label className="block">
        <div className="label mb-1">Paid, TON</div>
        <input value={bought} onChange={(e) => setBought(e.target.value)} inputMode="decimal" placeholder="—" className={field} />
      </label>
      <label className="block">
        <div className="label mb-1">Concept</div>
        <input value={concept} onChange={(e) => setConcept(e.target.value.slice(0, 200))} placeholder="Bid Spot — bidding for usernames" className={field} />
      </label>
      <label className="block">
        <div className="label mb-1">Note</div>
        <input value={note} onChange={(e) => setNote(e.target.value.slice(0, 400))} placeholder="optional" className={field} />
      </label>
      <div className="flex items-end gap-1">
        <Button onClick={save} disabled={busy || !name.trim()} className="h-9">
          Save
        </Button>
        <Button variant="ghost" onClick={() => onDone(null)} className="h-9">
          Cancel
        </Button>
      </div>
    </div>
  );
}

function Stat({ label, v, sub }: { label: string; v: React.ReactNode | null; sub: string }) {
  return (
    <div className="bg-card p-5">
      <div className="label">{label}</div>
      <div className="data mt-2 text-3xl font-semibold tracking-tight">{v == null ? <Skeleton className="h-8 w-24" /> : v}</div>
      <div className="mt-1 truncate text-[13px] text-muted-foreground">{sub}</div>
    </div>
  );
}
