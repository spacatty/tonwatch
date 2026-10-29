"use client";

import { Languages } from "lucide-react";
import type { Settings } from "@/lib/api";
import { LANGS, useLang, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type AILang = Settings["ai_lang"];

/** Buttons for the language the AI writes in: the interface's, or one picked for good. `compact`
 *  is the short form that sits next to a Generate button (Auto · EN · RU). */
export function AILangPicker({ value, onChange, compact }: { value: AILang; onChange: (v: AILang) => void; compact?: boolean }) {
  const t = useT().settings;
  const ui = useLang();
  const options: { id: AILang; label: string }[] = [
    { id: "", label: compact ? t.aiLangShort : t.aiLangAuto },
    ...LANGS.map((l) => ({ id: l.id as AILang, label: compact ? l.short : l.label })),
  ];
  const writes = LANGS.find((l) => l.id === (value || ui))?.label ?? "";

  if (compact)
    return (
      <div className="flex h-9 items-center rounded-md border border-input p-0.5 text-[12.5px]" title={t.aiLangTitle(writes)} role="radiogroup" aria-label={t.aiLang}>
        <Languages className="mx-1.5 size-3.5 text-muted-foreground" />
        {options.map((o) => (
          <button
            key={o.id || "auto"}
            role="radio"
            aria-checked={value === o.id}
            onClick={() => value !== o.id && onChange(o.id)}
            className={cn("data h-full rounded px-2 transition-colors", value === o.id ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
          >
            {o.label}
          </button>
        ))}
      </div>
    );

  return (
    <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label={t.aiLang}>
      {options.map((o) => (
        <button
          key={o.id || "auto"}
          role="radio"
          aria-checked={value === o.id}
          onClick={() => onChange(o.id)}
          className={cn(
            "h-8 rounded-md border px-3 text-[13px] transition-colors",
            value === o.id ? "border-primary bg-primary/10 text-primary" : "border-input text-foreground hover:border-muted-foreground/50 hover:bg-muted",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
