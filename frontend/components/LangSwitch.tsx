"use client";

import { LANGS, setLang, useLang, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** EN | RU, side by side: one tap switches the whole interface (and the bot, for a signed-in account).
 *  `full` spells the languages out, for places with room (the account dialog). */
export function LangSwitch({ className, full }: { className?: string; full?: boolean }) {
  const lang = useLang();
  const t = useT();
  return (
    <div role="radiogroup" aria-label={t.common.language} className={cn("inline-flex rounded-md border border-border p-0.5", full ? "text-[13px]" : "data text-[11px]", className)}>
      {LANGS.map((l) => (
        <button
          key={l.id}
          role="radio"
          aria-checked={lang === l.id}
          title={l.label}
          onClick={() => setLang(l.id)}
          className={cn("rounded transition-colors", full ? "px-3 py-1" : "px-1.5 py-0.5", lang === l.id ? "bg-primary/15 text-primary" : "text-muted-foreground hover:text-foreground")}
        >
          {full ? l.label : l.short}
        </button>
      ))}
    </div>
  );
}
