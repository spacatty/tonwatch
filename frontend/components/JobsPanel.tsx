"use client";

import { Play, Square } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api, type JobStatus, type Meta, type StatsResp } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { getT, num, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { ago } from "./bits";

const ORDER = ["auctions", "buynow", "sweep", "chain", "prices", "llm", "probe", "words"];

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

/** The status line in the reader's language when there is a translation for its format, else as the backend wrote it. */
function progressText(dict: Record<string, (a: string[]) => string>, j: JobStatus) {
  const fn = j.progress_key ? dict[j.progress_key] : undefined;
  return fn ? fn(j.progress_args ?? []) : j.progress;
}

export function JobsPanel({ open, onClose, stats, meta }: { open: boolean; onClose: () => void; stats: StatsResp | null; meta: Meta | null }) {
  const jobs = useJobs(open ? 2000 : 15000);
  const t = useT().jobs;
  const s = stats?.stats;

  const run = async (name: string, running: boolean) => {
    await (running ? api.stopJob(name) : api.runJob(name));
    const tj = getT().jobs;
    const title = tj.titles[name] ?? name;
    toast(running ? tj.stopping(title) : tj.started(title));
  };

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="w-full gap-0 overflow-y-auto p-0 sm:max-w-[460px]">
        <div className="px-6 pt-7 pb-5">
          <SheetTitle className="text-2xl font-bold">{t.title}</SheetTitle>
          <SheetDescription>{t.description}</SheetDescription>
        </div>
        <ul className="space-y-1 px-3">
          {jobs.map((j) => (
            <li key={j.name} className="rounded-lg px-3 py-3 transition-colors hover:bg-muted/60">
              <div className="flex items-start gap-3">
                <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", j.running ? "animate-pulse bg-positive" : j.last_error ? "bg-destructive" : "bg-chart-2/50")} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-medium">{t.titles[j.name] ?? j.name}</span>
                    <Button size="xs" variant={j.running ? "outline" : "secondary"} onClick={() => run(j.name, j.running)}>
                      {j.running ? <Square className="size-3" /> : <Play className="size-3" />}
                      {j.running ? t.stop : t.runNow}
                    </Button>
                  </div>
                  <p className="mt-0.5 text-sm text-muted-foreground">{t.descs[j.name] ?? j.desc}</p>
                  {j.progress && <p className="num mt-1.5 text-sm">{progressText(t.progress, j)}</p>}
                  {j.last_error && <p className="mt-1.5 text-sm break-words text-destructive">{j.last_error}</p>}
                  <p className="num mt-1.5 text-xs text-muted-foreground">
                    {t.lastRun(ago(j.last_run))}
                    {j.last_run ? t.took(fmtDur(j.last_duration_ms / 1000)) : ""}
                    {j.every_sec ? t.every(fmtDur(j.every_sec)) : ""}
                    {j.next_run && !j.running ? t.nextIn(fmtDur(Math.max(0, j.next_run - Date.now() / 1000))) : ""}
                  </p>
                </div>
              </div>
            </li>
          ))}
        </ul>

        <section className="mx-6 my-6 rounded-lg bg-accent p-4 text-sm text-accent-foreground">
          <h3 className="font-semibold">{t.aiFilter}</h3>
          <p className="num mt-1">
            {meta?.llm_enabled ? (
              t.rated(meta.model ?? "", num(s?.scored ?? 0), num(s?.llm_queue ?? 0), num(s?.junk ?? 0), stats?.llm_usage?.cost.toFixed(4) ?? "0")
            ) : (
              t.aiOff
            )}
          </p>
          {meta?.llm_enabled && (
            <Button
              size="xs"
              variant="outline"
              className="mt-3"
              onClick={() => confirm(t.rerateConfirm) && api.resetLLM().then(() => toast(getT().jobs.rerateStarted))}
            >
              {t.rerate}
            </Button>
          )}
        </section>
        <p className="px-6 pb-8 text-xs text-muted-foreground">
          {meta?.proxies ? t.proxies(meta.proxies) : t.noProxies}
          {t.probe(meta?.probe_min_score ?? 7)}
          {meta?.chain_off ? t.chainOff : stats?.chain_done_at ? t.chainDone(ago(Number(stats.chain_done_at))) : t.chainRunning}
        </p>
      </SheetContent>
    </Sheet>
  );
}

function fmtDur(sec: number) {
  const d = getT().jobs.dur;
  if (sec < 60) return d.s(Math.round(sec));
  if (sec < 3600) return d.m(Math.round(sec / 60));
  return d.h(Math.round(sec / 3600));
}
