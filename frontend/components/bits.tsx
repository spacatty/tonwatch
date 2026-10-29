"use client";

import { useEffect, useState } from "react";
import type { Username } from "@/lib/api";
import { getT, locale, num, useLang, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** An amount in TON, in the data face. */
export function Ton({ v, className, unit = true }: { v: number; className?: string; unit?: boolean }) {
  useLang(); // 12.5 → 12,5 the moment the language switches
  return (
    <span className={cn("data", className)}>
      {v.toLocaleString(locale(), { maximumFractionDigits: v < 100 ? 2 : 0 })}
      {unit && <span className="ml-1 text-[0.78em] font-normal text-muted-foreground">TON</span>}
    </span>
  );
}

// One shared 1s clock for every countdown on the page.
const subs = new Set<(n: number) => void>();
let timer: ReturnType<typeof setInterval> | null = null;
export function useNow(stepMs = 1000) {
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));
  useEffect(() => {
    let last = 0;
    const f = (n: number) => {
      if (n * 1000 - last >= stepMs) {
        last = n * 1000;
        setNow(n);
      }
    };
    subs.add(f);
    timer ??= setInterval(() => subs.forEach((s) => s(Math.floor(Date.now() / 1000))), 1000);
    return () => {
      subs.delete(f);
      if (subs.size === 0 && timer) {
        clearInterval(timer);
        timer = null;
      }
    };
  }, [stepMs]);
  return now;
}

export function fmtLeft(left: number) {
  const t = getT().bits;
  if (left <= 0) return t.closing;
  const d = Math.floor(left / 86400);
  const h = Math.floor((left % 86400) / 3600);
  const m = Math.floor((left % 3600) / 60);
  const s = left % 60;
  if (d > 0) return t.left.d(d, h);
  if (h > 0) return t.left.h(h, String(m).padStart(2, "0"));
  if (m === 0) return t.left.s(s);
  return t.left.m(m, String(s).padStart(2, "0"));
}

export function Countdown({ ts, className }: { ts: number; className?: string }) {
  const now = useNow();
  useT(); // re-render on a language switch
  const left = ts - now;
  return (
    <span
      className={cn(
        "data",
        left <= 0 ? "text-muted-foreground" : left < 3600 ? "font-medium text-destructive" : left < 6 * 3600 ? "text-warning" : "text-foreground",
        className,
      )}
      title={new Date(ts * 1000).toLocaleString(locale())}
    >
      {fmtLeft(left)}
    </span>
  );
}

/** AI score as its number and ten segments, lit up to the score. */
export function Score({ u, className }: { u: Pick<Username, "score" | "junk">; className?: string }) {
  const t = useT().bits;
  if (u.score == null) {
    return <span className={cn("data text-xs text-muted-foreground", className)}>{u.junk ? t.junk : "—"}</span>;
  }
  const s = u.score;
  return (
    <span className={cn("inline-flex items-center gap-2", className)} title={t.scoreOf(s)}>
      <span className={cn("data w-5 text-right text-[15px] font-semibold", s >= 8 ? "text-primary" : s >= 6 ? "text-foreground" : "text-muted-foreground")}>{s}</span>
      <span className="flex gap-[2px]">
        {Array.from({ length: 10 }, (_, i) => (
          <span key={i} className={cn("h-3 w-[3px] rounded-[1px]", i < s ? (s >= 8 ? "bg-primary" : s >= 6 ? "bg-foreground/70" : "bg-muted-foreground/60") : "bg-border")} />
        ))}
      </span>
    </span>
  );
}

const DOT: Record<string, string> = {
  auction: "bg-warning",
  sale: "bg-positive",
  market: "bg-primary",
  owned: "bg-muted-foreground/60",
  available: "bg-foreground",
};

export function statusLabel(u: Username) {
  const t = getT().bits.status;
  if (u.status === "auction") return t.auction;
  if (u.status === "sale") return t.sale;
  if (u.status === "market") return u.chain_market ?? t.market;
  if (u.status === "owned") return u.frag_status === "sold" ? t.sold : t.owned;
  return u.status;
}

export function StatusPill({ u, className }: { u: Username; className?: string }) {
  useT(); // re-render on a language switch
  return (
    <span title={statusLabel(u)} className={cn("inline-flex max-w-full min-w-0 items-center gap-1.5 text-[13px] whitespace-nowrap", u.status === "owned" ? "text-muted-foreground" : "text-foreground", className)}>
      <span className={cn("size-1.5 shrink-0 rounded-full", DOT[u.status])} />
      <span className="truncate">{statusLabel(u)}</span>
    </span>
  );
}

/** Typical prices keyed by medianKey: "9" for a score alone, "9:4" for a score and a length bucket. */
export type Medians = Record<string, number>;

/** The length buckets that sell alike (the backend's rating.LenBucket). */
export function lenBucket(len: number) {
  return len <= 4 ? "4" : len === 5 ? "5" : len <= 7 ? "6-7" : len <= 10 ? "8-10" : "11+";
}

export function medianKey(score: number, bucket?: string) {
  return bucket ? `${score}:${bucket}` : String(score);
}

/** The typical price for a name's AI score and length, falling back to the score alone. */
export function typicalFor(u: Pick<Username, "score" | "len">, medians: Medians): number | null {
  if (u.score == null) return null;
  return medians[medianKey(u.score, lenBucket(u.len))] ?? medians[medianKey(u.score)] ?? null;
}

/** How far a name's buyout sits under the typical price for its AI score and length (0.4 = 40% cheaper). */
export function discount(u: Pick<Username, "buyout" | "score" | "len">, medians: Medians): number | null {
  const typ = typicalFor(u, medians);
  if (u.buyout == null || !typ) return null;
  return 1 - u.buyout / typ;
}

export function Deal({ u, medians, className }: { u: Pick<Username, "buyout" | "score" | "len">; medians: Medians; className?: string }) {
  const t = useT().bits;
  const d = discount(u, medians);
  if (d == null || d < 0.15) return null;
  return (
    <span
      className={cn("data inline-flex h-5 items-center rounded px-1.5 text-[11px] font-semibold", d >= 0.5 ? "bg-positive text-background" : "bg-pill-positive text-pill-positive-ink", className)}
      title={t.deal(Math.round(d * 100), u.score!, num(typicalFor(u, medians) ?? 0))}
    >
      −{Math.min(99, Math.round(d * 100))}%
    </span>
  );
}

export function ago(ts: number | null) {
  const t = getT().bits;
  if (!ts) return t.never;
  const s = Math.floor(Date.now() / 1000) - ts;
  if (s < 60) return t.ago.s(s);
  if (s < 3600) return t.ago.m(Math.floor(s / 60));
  if (s < 86400) return t.ago.h(Math.floor(s / 3600));
  return t.ago.d(Math.floor(s / 86400));
}

/** TonWatch's mark: an @ inside a cut gem, drawn for this product. */
export function Mark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden>
      <path d="M8 3h16l6 8-14 18L2 11z" fill="var(--primary)" />
      <path d="M2 11h28M8 3l4 8 4 18M24 3l-4 8-4 18" stroke="var(--primary-foreground)" strokeOpacity=".35" strokeWidth="1" fill="none" />
      <circle cx="16" cy="13.5" r="3" stroke="var(--primary-foreground)" strokeWidth="1.8" fill="none" />
    </svg>
  );
}
