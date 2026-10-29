"use client";

import { ArrowUp, Clock, Loader2, Sparkles, Wand2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { api, type AIFilterResult } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { ago } from "./bits";

/** The AI filter: say what you want in words, get the table's own rules for it. The result is a
 *  live view of the table, not a one-off answer, so it stays fresh and can become a tracker. */
export function AIFilter({ llmEnabled, initial, onApply }: { llmEnabled: boolean; initial?: string; onApply: (f: AIFilterResult) => void }) {
  const t = useT().aiFilter;
  const [draft, setDraft] = useState(initial ?? "");
  const [busy, setBusy] = useState(false);
  const [recent, setRecent] = useState<AIFilterResult[]>([]);
  const ref = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    api
      .aiFilters()
      .then((r) => setRecent(r.recent))
      .catch(() => {});
  }, []);
  useEffect(() => {
    if (initial) setDraft(initial);
  }, [initial]);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 180)}px`;
  }, [draft]);

  const run = async (prompt = draft) => {
    prompt = prompt.trim();
    if (!prompt || busy) return;
    setBusy(true);
    try {
      const r = await api.aiFilter(prompt);
      onApply(r.filter);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  // a starter phrase goes at the cursor's end of the request, joined with a comma
  const add = (phrase: string) => {
    setDraft((d) => (d.trim() ? `${d.trim().replace(/[,.]$/, "")}, ${phrase}` : phrase));
    ref.current?.focus();
  };

  return (
    <div className="mx-auto max-w-[820px] space-y-8">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          run();
        }}
        className="rounded-2xl border border-input bg-card shadow-lg shadow-black/5 transition-colors focus-within:border-ring/70 focus-within:ring-4 focus-within:ring-ring/10"
      >
        <textarea
          ref={ref}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              run();
            }
          }}
          rows={2}
          disabled={!llmEnabled}
          placeholder={llmEnabled ? t.placeholder : t.off}
          className="block w-full resize-none bg-transparent px-4 pt-4 pb-2 text-[15px] leading-relaxed outline-none placeholder:text-muted-foreground"
        />
        <div className="flex items-center gap-2 px-3 pb-3">
          <span className="text-[12px] text-muted-foreground">{t.costs}</span>
          <Button type="submit" className="ml-auto h-9 gap-1.5 rounded-full px-4" disabled={!llmEnabled || busy || !draft.trim()}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Wand2 className="size-4" />}
            {busy ? t.building : t.build}
          </Button>
        </div>
      </form>

      <section>
        <h3 className="label mb-2.5 px-1">{t.startersTitle}</h3>
        <div className="flex flex-wrap gap-2">
          {t.starters.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => add(s)}
              className="rounded-full border border-border bg-card px-3 py-1.5 text-[13px] text-muted-foreground transition-colors hover:border-primary/50 hover:text-foreground"
            >
              + {s}
            </button>
          ))}
        </div>
      </section>

      <section>
        <h3 className="label mb-2.5 px-1">{t.examplesTitle}</h3>
        <div className="grid gap-2.5 sm:grid-cols-2">
          {t.examples.map((e) => (
            <button
              key={e}
              type="button"
              onClick={() => run(e)}
              disabled={!llmEnabled || busy}
              className="group flex items-start gap-3 rounded-xl border border-border bg-card p-3.5 text-left text-[14px] transition-all hover:-translate-y-0.5 hover:border-primary/50 hover:shadow-md disabled:opacity-50"
            >
              <Sparkles className="mt-0.5 size-4 shrink-0 text-primary/70 group-hover:text-primary" />
              <span className="flex-1">{e}</span>
              <ArrowUp className="size-4 shrink-0 rotate-45 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
            </button>
          ))}
        </div>
      </section>

      {recent.length > 0 && (
        <section>
          <h3 className="label mb-2.5 px-1">{t.recentTitle}</h3>
          <ul className="divide-y divide-border/60 overflow-hidden rounded-xl border border-border bg-card">
            {recent.map((r) => (
              <li key={r.prompt}>
                <button type="button" onClick={() => onApply(r)} className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/60">
                  <Clock className="size-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[14px]">{r.prompt}</span>
                    {r.summary && <span className="block truncate text-[12.5px] text-muted-foreground">{r.summary}</span>}
                  </span>
                  <span className="data shrink-0 text-[12px] text-muted-foreground">{ago(r.at)}</span>
                </button>
              </li>
            ))}
          </ul>
          <p className="mt-2 px-1 text-[12px] text-muted-foreground">{t.recentHint}</p>
        </section>
      )}
    </div>
  );
}
