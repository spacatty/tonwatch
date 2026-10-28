"use client";

import { Play, Square } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api, type JobStatus, type Meta, type StatsResp } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { cn } from "@/lib/utils";
import { ago } from "./bits";

const ORDER = ["auctions", "buynow", "sweep", "chain", "prices", "llm", "probe", "words"];
const TITLES: Record<string, string> = {
  auctions: "Closing auctions",
  buynow: "Buy-now prices",
  sweep: "Fragment listings",
  chain: "Every minted name",
  prices: "Marketplace prices",
  llm: "AI rating",
  probe: "Hidden listing probe",
  words: "Word search",
};

export function useJobs(poll: number) {
  const [jobs, setJobs] = useState<JobStatus[]>([]);
  useEffect(() => {
    if (poll <= 0) return; // not an admin: there is nothing to poll
    const f = () => api.jobs().then(setJobs).catch(() => {});
    f();
    const t = setInterval(f, poll);
    return () => clearInterval(t);
  }, [poll]);
  return [...jobs].sort((a, b) => ORDER.indexOf(a.name) - ORDER.indexOf(b.name));
}

export function JobsPanel({ open, onClose, stats, meta }: { open: boolean; onClose: () => void; stats: StatsResp | null; meta: Meta | null }) {
  const jobs = useJobs(open ? 2000 : 15000);
  const s = stats?.stats;

  const run = async (name: string, running: boolean) => {
    await (running ? api.stopJob(name) : api.runJob(name));
    toast(running ? `Stopping ${TITLES[name] ?? name}` : `Started ${TITLES[name] ?? name}`);
  };

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="w-full gap-0 overflow-y-auto p-0 sm:max-w-[460px]">
        <div className="px-6 pt-7 pb-5">
          <SheetTitle className="text-2xl font-bold">Collectors</SheetTitle>
          <SheetDescription>They run on their own schedule. Start one now to refresh sooner.</SheetDescription>
        </div>
        <ul className="space-y-1 px-3">
          {jobs.map((j) => (
            <li key={j.name} className="rounded-lg px-3 py-3 transition-colors hover:bg-muted/60">
              <div className="flex items-start gap-3">
                <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", j.running ? "animate-pulse bg-positive" : j.last_error ? "bg-destructive" : "bg-chart-2/50")} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-medium">{TITLES[j.name] ?? j.name}</span>
                    <Button size="xs" variant={j.running ? "outline" : "secondary"} onClick={() => run(j.name, j.running)}>
                      {j.running ? <Square className="size-3" /> : <Play className="size-3" />}
                      {j.running ? "Stop" : "Run now"}
                    </Button>
                  </div>
                  <p className="mt-0.5 text-sm text-muted-foreground">{j.desc}</p>
                  {j.progress && <p className="num mt-1.5 text-sm">{j.progress}</p>}
                  {j.last_error && <p className="mt-1.5 text-sm break-words text-destructive">{j.last_error}</p>}
                  <p className="num mt-1.5 text-xs text-muted-foreground">
                    Last run {ago(j.last_run)}
                    {j.last_run ? `, took ${fmtDur(j.last_duration_ms / 1000)}` : ""}
                    {j.every_sec ? ` · every ${fmtDur(j.every_sec)}` : ""}
                    {j.next_run && !j.running ? ` · next in ${fmtDur(Math.max(0, j.next_run - Date.now() / 1000))}` : ""}
                  </p>
                </div>
              </div>
            </li>
          ))}
        </ul>

        <section className="mx-6 my-6 rounded-lg bg-accent p-4 text-sm text-accent-foreground">
          <h3 className="font-semibold">AI filter</h3>
          <p className="num mt-1">
            {meta?.llm_enabled ? (
              <>
                {meta.model} has rated {s?.scored.toLocaleString() ?? 0} names; {s?.llm_queue.toLocaleString() ?? 0} are waiting. Rules dropped {s?.junk.toLocaleString() ?? 0} as
                obvious junk. Spent ${stats?.llm_usage?.cost.toFixed(4) ?? "0"} since the backend started.
              </>
            ) : (
              "Add OPENROUTER_API_KEY to .env and restart the backend to rate names."
            )}
          </p>
          {meta?.llm_enabled && (
            <Button
              size="xs"
              variant="outline"
              className="mt-3"
              onClick={() => confirm("Clear every AI score and rate all names again? This spends tokens.") && api.resetLLM().then(() => toast("Re-rating started"))}
            >
              Re-rate everything
            </Button>
          )}
        </section>
        <p className="px-6 pb-8 text-xs text-muted-foreground">
          {meta?.proxies ? `${meta.proxies} proxies in rotation. ` : "No proxies configured. "}
          The probe opens the Fragment page of every name rated {meta?.probe_min_score ?? 7}+ to find auctions and sales that Fragment's own lists hide.
          {meta?.chain_off
            ? " The chain crawl is off (SKIP_CHAIN)."
            : stats?.chain_done_at
              ? ` Last full chain crawl ${ago(Number(stats.chain_done_at))}.`
              : " The first full chain crawl is still running."}
        </p>
      </SheetContent>
    </Sheet>
  );
}

function fmtDur(sec: number) {
  if (sec < 60) return `${Math.round(sec)}s`;
  if (sec < 3600) return `${Math.round(sec / 60)}m`;
  return `${Math.round(sec / 3600)}h`;
}
