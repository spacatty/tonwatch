"use client";

import { useEffect, useState } from "react";
import type { Username } from "@/lib/api";
import { cn } from "@/lib/utils";

/** An amount in TON, in the data face. */
export function Ton({ v, className, unit = true }: { v: number; className?: string; unit?: boolean }) {
  return (
    <span className={cn("data", className)}>
      {v.toLocaleString(undefined, { maximumFractionDigits: v < 100 ? 2 : 0 })}
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
  if (left <= 0) return "closing";
  const d = Math.floor(left / 86400);
  const h = Math.floor((left % 86400) / 3600);
  const m = Math.floor((left % 3600) / 60);
  const s = left % 60;
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${String(m).padStart(2, "0")}m`;
  if (m === 0) return `${s}s`;
  return `${m}m ${String(s).padStart(2, "0")}s`;
}

export function Countdown({ ts, className }: { ts: number; className?: string }) {
  const now = useNow();
  const left = ts - now;
  return (
    <span
      className={cn(
        "data",
        left <= 0 ? "text-muted-foreground" : left < 3600 ? "font-medium text-destructive" : left < 6 * 3600 ? "text-warning" : "text-foreground",
        className,
      )}
      title={new Date(ts * 1000).toLocaleString()}
    >
      {fmtLeft(left)}
    </span>
  );
}

/** AI score as its number and ten segments, lit up to the score. */
export function Score({ u, className }: { u: Pick<Username, "score" | "junk">; className?: string }) {
  if (u.score == null) {
    return <span className={cn("data text-xs text-muted-foreground", className)}>{u.junk ? "junk" : "—"}</span>;
  }
  const s = u.score;
  return (
    <span className={cn("inline-flex items-center gap-2", className)} title={`AI score ${s} of 10`}>
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
  if (u.status === "auction") return "Auction";
  if (u.status === "sale") return "For sale";
  if (u.status === "market") return u.chain_market ?? "Market";
  if (u.status === "owned") return u.frag_status === "sold" ? "Sold" : "Owned";
  return u.status;
}

export function StatusPill({ u, className }: { u: Username; className?: string }) {
  return (
    <span title={statusLabel(u)} className={cn("inline-flex max-w-full min-w-0 items-center gap-1.5 text-[13px] whitespace-nowrap", u.status === "owned" ? "text-muted-foreground" : "text-foreground", className)}>
      <span className={cn("size-1.5 shrink-0 rounded-full", DOT[u.status])} />
      <span className="truncate">{statusLabel(u)}</span>
    </span>
  );
}

export type Medians = Record<number, number>;

/** How far a name's buyout sits under the typical price for its AI score (0.4 = 40% cheaper). */
export function discount(u: Pick<Username, "buyout" | "score">, medians: Medians): number | null {
  if (u.buyout == null || u.score == null || !medians[u.score]) return null;
  return 1 - u.buyout / medians[u.score];
}

export function Deal({ u, medians, className }: { u: Pick<Username, "buyout" | "score">; medians: Medians; className?: string }) {
  const d = discount(u, medians);
  if (d == null || d < 0.15) return null;
  return (
    <span
      className={cn("data inline-flex h-5 items-center rounded px-1.5 text-[11px] font-semibold", d >= 0.5 ? "bg-positive text-background" : "bg-pill-positive text-pill-positive-ink", className)}
      title={`${Math.round(d * 100)}% under the typical price of names rated ${u.score} (${medians[u.score!]?.toLocaleString()} TON)`}
    >
      −{Math.min(99, Math.round(d * 100))}%
    </span>
  );
}

export function ago(ts: number | null) {
  if (!ts) return "never";
  const s = Math.floor(Date.now() / 1000) - ts;
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

/** BidSpot's mark: an @ inside a cut gem, drawn for this product. */
export function Mark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden>
      <path d="M8 3h16l6 8-14 18L2 11z" fill="var(--primary)" />
      <path d="M2 11h28M8 3l4 8 4 18M24 3l-4 8-4 18" stroke="var(--primary-foreground)" strokeOpacity=".35" strokeWidth="1" fill="none" />
      <circle cx="16" cy="13.5" r="3" stroke="var(--primary-foreground)" strokeWidth="1.8" fill="none" />
    </svg>
  );
}
