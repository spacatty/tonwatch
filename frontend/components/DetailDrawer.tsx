"use client";

import { Briefcase, Copy, ExternalLink, Lightbulb, Loader2, RotateCw, Star } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { api, fragmentUrl, getgemsUrl, studio, tonviewerUrl, type Concept, type EventRow, type Username } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { locale, useLang, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { ago, Countdown, Score, StatusPill, Ton } from "./bits";

const SRC = [1, 2, 4, 8];

export function DetailDrawer({ name, imagine, onClose, onChange }: { name: string | null; imagine?: boolean; onClose: () => void; onChange: (u: Username) => void }) {
  const [concepts, setConcepts] = useState<Concept[]>([]);
  const [imagining, setImagining] = useState(false);
  const [say, setSay] = useState("");
  const [u, setU] = useState<Username | null>(null);
  const [events, setEvents] = useState<EventRow[]>([]);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const t = useT();
  const td = t.detailDrawer;
  const lang = useLang();

  useEffect(() => {
    if (!name) return;
    setU(null);
    setEvents([]);
    api
      .get(name)
      .then((r) => {
        setU(r.item);
        setEvents(r.events);
        setNote(r.item.note ?? "");
      })
      .catch((e) => toast.error(`@${name}: ${e.message}`));
    setConcepts([]);
    setSay("");
    studio.history(name, 20).then((r) => setConcepts(r.concepts)).catch(() => {});
    // a long look is a soft like: ten seconds on one name teaches the taste for free
    const timer = setTimeout(() => studio.signal(name, "open"), 10_000);
    return () => clearTimeout(timer);
  }, [name]);

  // one full project for this name; prior ones are passed as "already made", so each press differs
  const imagineOne = async (note?: string) => {
    if (!name || imagining) return;
    setImagining(true);
    try {
      const r = await studio.imagine(name, note);
      setConcepts((cs) => [r.concept, ...cs]);
      setSay("");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setImagining(false);
    }
  };
  const autoRan = useRef<string | null>(null);
  useEffect(() => {
    if (imagine && name && autoRan.current !== name) {
      autoRan.current = name;
      imagineOne();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [imagine, name]);

  const own = async () => {
    if (!u) return;
    try {
      await studio.savePortfolio({ name: u.name, bought: u.last_sale ?? u.buyout ?? null, concept: "", note: "" });
      toast.success(td.inPortfolio(u.name));
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const refresh = async () => {
    if (!name) return;
    setBusy(true);
    try {
      const r = await api.refresh(name);
      setU(r.item);
      onChange(r.item);
      const d = await api.get(name);
      setEvents(d.events);
      toast.success(td.upToDate(name));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const setWatch = async (watched: boolean) => {
    if (!u) return;
    await api.watch(u.name, watched, note);
    const nu = { ...u, watched, note };
    setU(nu);
    onChange(nu);
    toast(watched ? td.tracking(u.name) : td.stoppedTracking(u.name));
  };

  return (
    <Sheet open={!!name} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="w-full gap-0 overflow-y-auto p-0 sm:max-w-[440px]">
        <div className="px-6 pt-7 pb-5">
          <SheetTitle className="data text-[26px] leading-tight font-semibold break-all">@{name}</SheetTitle>
          <SheetDescription className="mt-1 text-sm">
            {u?.meaning || (u ? (u.score == null ? td.notRated : td.noMeaning) : "")}
          </SheetDescription>
          {u && (
            <div className="mt-4 flex flex-wrap items-center gap-3">
              <StatusPill u={u} />
              <Score u={u} />
              {u.category && <span className="text-xs text-muted-foreground">{t.filterBar.categories[u.category] ?? u.category}</span>}
              <span className="num text-xs text-muted-foreground">{td.characters(u.len)}</span>
            </div>
          )}
        </div>

        {!u ? (
          <div className="space-y-3 px-6">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-32 w-full" />
          </div>
        ) : (
          <>
            <div data-ui className="mx-6 grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-border bg-border">
              <Figure k={u.status === "auction" ? td.currentBid : u.status === "owned" ? td.lastSale : td.price}>
                {u.price != null ? <Ton v={u.price} /> : u.last_sale != null ? <Ton v={u.last_sale} /> : "–"}
              </Figure>
              <Figure k={td.endsIn}>{u.ends_at ? <Countdown ts={u.ends_at} /> : "–"}</Figure>
              <Figure k={td.buyout}>{u.buyout != null ? <Ton v={u.buyout} className="text-positive" /> : "–"}</Figure>
              <Figure k={td.usd}>{u.usd != null && u.price != null ? <span className="num">${Math.round(u.usd).toLocaleString(locale())}</span> : "–"}</Figure>
            </div>

            <div className="flex flex-wrap gap-2 px-6 pt-5">
              <Button nativeButton={false} render={<a href={fragmentUrl(u.name)} target="_blank" rel="noreferrer" onClick={() => u.buyout != null && studio.signal(u.name, "buy")} />} className="h-9 px-4">
                {u.status === "owned" ? td.makeOffer : u.status === "auction" ? td.placeBid : u.status === "sale" ? td.buyOnFragment : td.openInFragment}
                <ExternalLink className="size-3.5" />
              </Button>
              {u.nft_friendly && (
                <Button variant="outline" nativeButton={false} render={<a href={getgemsUrl(u.nft_friendly)} target="_blank" rel="noreferrer" />} className="h-9">
                  Getgems
                </Button>
              )}
              <Button variant="outline" onClick={refresh} disabled={busy} className="h-9">
                <RotateCw className={cn("size-3.5", busy && "animate-spin")} />
                {td.refresh}
              </Button>
              <Button variant={u.watched ? "secondary" : "outline"} onClick={() => setWatch(!u.watched)} className="h-9">
                <Star className={cn("size-3.5", u.watched && "fill-warning text-warning")} />
                {u.watched ? td.trackingBtn : td.track}
              </Button>
              <Button variant="outline" onClick={own} className="h-9" title={td.ownTitle}>
                <Briefcase className="size-3.5" />
                {td.own}
              </Button>
            </div>
            {u.status === "owned" && (
              <p className="px-6 pt-3 text-xs text-muted-foreground">
                {td.notListed}
              </p>
            )}

            <section className="px-6 pt-7">
              <div className="mb-3 flex items-center justify-between gap-3">
                <h3 className="text-sm font-semibold">{td.whatToDo}</h3>
                <Button size="sm" variant={concepts.length ? "outline" : "default"} onClick={() => imagineOne()} disabled={imagining} className="h-8">
                  {imagining ? <Loader2 className="size-3.5 animate-spin" /> : <Lightbulb className="size-3.5" />}
                  {concepts.length ? td.anotherIdea : td.imagine}
                  <span className="data rounded-sm bg-foreground/10 px-1 text-[10px]">{td.credits(2)}</span>
                </Button>
              </div>
              {concepts.length === 0 && !imagining && (
                <p className="text-sm text-muted-foreground">{td.imagineHint}</p>
              )}
              {imagining && concepts.length === 0 && <Skeleton className="h-28 rounded-lg" />}
              {concepts.length > 0 && (
                <div className="space-y-2">
                  {concepts.map((c, i) => (
                    <article key={c.id} className={cn("rounded-lg border px-3.5 py-3", i === 0 ? "border-primary/40 bg-accent/40" : "border-border")}>
                      <div className="flex items-baseline justify-between gap-2">
                        <h4 className="text-[15px] font-semibold tracking-tight">{c.project}</h4>
                        <span className="data shrink-0 text-[11px] text-muted-foreground">{c.form || (c.source === "imagine" ? "" : c.source === "set" ? td.fromSet : c.source)}</span>
                      </div>
                      <p className="mt-1 text-[13px] leading-snug">{c.pitch}</p>
                      {c.built_with && <p className="mt-1.5 text-[13px] leading-snug text-muted-foreground"><span className="text-foreground/80">{td.builtWith}</span> {c.built_with}</p>}
                      {c.how && <p className="mt-1.5 text-[13px] leading-snug text-muted-foreground"><span className="text-foreground/80">{td.handle}</span> {c.how}</p>}
                      {c.first_move && <p className="mt-1 text-[13px] text-muted-foreground"><span className="text-foreground/80">{td.firstMove}</span> {c.first_move}</p>}
                      {c.names.length > 1 && (
                        <p className="data mt-1.5 text-[11px] text-muted-foreground">{td.withNames(c.names.filter((n) => n.name !== name).map((n) => "@" + n.name).join(", "))}</p>
                      )}
                    </article>
                  ))}
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (say.trim()) imagineOne(say.trim());
                    }}
                    className="flex gap-1.5 pt-1"
                  >
                    <input
                      value={say}
                      onChange={(e) => setSay(e.target.value.slice(0, 300))}
                      placeholder={td.refinePlaceholder}
                      className="data h-8 min-w-0 flex-1 rounded-md border border-input bg-transparent px-2.5 text-[13px] outline-none focus:border-primary"
                    />
                    <Button type="submit" size="sm" variant="outline" disabled={!say.trim() || imagining} className="h-8">
                      {td.go}
                    </Button>
                  </form>
                </div>
              )}
            </section>

            <section className="px-6 pt-7">
              <h3 className="mb-2 text-sm font-semibold">{td.yourNote}</h3>
              <Textarea value={note} onChange={(e) => setNote(e.target.value)} rows={2} placeholder={td.notePlaceholder} />
              {note !== (u.note ?? "") && (
                <Button size="sm" className="mt-2" onClick={() => setWatch(true)}>
                  {td.saveAndTrack}
                </Button>
              )}
            </section>

            <section className="px-6 pt-7">
              <h3 className="mb-3 text-sm font-semibold">{td.history}</h3>
              {events.length === 0 ? (
                <p className="text-sm text-muted-foreground">{td.noHistory}</p>
              ) : (
                <ol data-ui className="relative space-y-3 border-l border-border pl-4">
                  {events.map((e, i) => (
                    <li key={i} className="relative text-sm">
                      <span className={cn("absolute top-1.5 -left-[21px] size-2.5 rounded-full ring-2 ring-background", i === 0 ? "bg-primary" : "bg-chart-2")} />
                      <div>{e.detail}</div>
                      <div className="num text-xs text-muted-foreground">
                        {new Date(e.ts * 1000).toLocaleString(lang === "ru" ? "ru-RU" : undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}
                      </div>
                    </li>
                  ))}
                </ol>
              )}
            </section>

            <section className="px-6 pt-7 pb-8">
              <h3 className="mb-2 text-sm font-semibold">{td.onRecord}</h3>
              <dl className="space-y-2 text-sm">
                <Row k={td.owner}>
                  {u.owner ? (
                    <button
                      className="inline-flex max-w-full items-center gap-1.5 truncate text-left hover:text-primary"
                      onClick={() => navigator.clipboard.writeText(u.owner!).then(() => toast(td.ownerCopied))}
                    >
                      <span className="truncate">{u.owner}</span>
                      <Copy className="size-3 shrink-0" />
                    </button>
                  ) : (
                    "–"
                  )}
                </Row>
                {u.nft_friendly && (
                  <Row k="NFT">
                    <a href={tonviewerUrl(u.nft_friendly)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:text-primary">
                      {td.viewOnTonviewer} <ExternalLink className="size-3" />
                    </a>
                  </Row>
                )}
                <Row k={td.fragmentChecked}>{ago(u.frag_at)}</Row>
                <Row k={td.chainChecked}>{ago(u.chain_at)}</Row>
                <Row k={td.foundThrough}>
                  {SRC.filter((b) => u.src & b)
                    .map((b) => td.sources[b])
                    .join(", ") || td.addedByYou}
                </Row>
              </dl>
            </section>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}

function Figure({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <div className="bg-card px-4 py-3">
      <div className="label">{k}</div>
      <div className="data mt-1 text-xl font-medium">{children}</div>
    </div>
  );
}

function Row({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-4">
      <dt className="w-32 shrink-0 text-muted-foreground">{k}</dt>
      <dd className="min-w-0 flex-1">{children}</dd>
    </div>
  );
}
