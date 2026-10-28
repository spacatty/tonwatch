"use client";

import { ArrowDown, ArrowUp, Briefcase, Copy, ExternalLink, Eye, EyeOff, Gem, Heart, Lightbulb, ListFilter, MoreHorizontal, RotateCw, Star, X } from "lucide-react";
import { toast } from "sonner";
import { fragmentUrl, getgemsUrl, studio, type Username } from "@/lib/api";
import { cn } from "@/lib/utils";
import { SORTS, STATUS_LABELS, type SortKey } from "@/lib/views";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { Countdown, discount, statusLabel, type Medians } from "./bits";

interface Props {
  items: Username[];
  total: number;
  loading: boolean;
  busy: Record<string, boolean>;
  onOpen: (name: string) => void;
  onWatch: (u: Username) => void;
  onLike: (u: Username) => void;
  onHide: (u: Username) => void;
  onImagine: (name: string) => void;
  onRefresh: (name: string) => void;
  showEnds: boolean;
  /** auction views: keep "Ends" visible at every width */
  endsFirst?: boolean;
  /** the Hidden view: the archive button brings names back instead */
  hiddenView?: boolean;
  empty: React.ReactNode;
  sorts: SortKey[];
  defaultSort: string;
  onSorts: (s: SortKey[]) => void;
  statusOptions: string[];
  statuses: string[];
  onStatuses: (s: string[]) => void;
  medians: Medians;
}

const colLabel = (c: string) => SORTS.find((s) => s.value === c)?.label ?? c;

/** A header click cycles that column: ascending → descending → off. Columns stack in click order. */
export function cycleSort(sorts: SortKey[], col: string): SortKey[] {
  const i = sorts.findIndex((s) => s.col === col);
  if (i < 0) return [...sorts, { col, dir: "asc" }];
  if (sorts[i].dir === "asc") return sorts.map((s, j) => (j === i ? { ...s, dir: "desc" } : s));
  return sorts.filter((_, j) => j !== i);
}

const DOT: Record<string, string> = {
  auction: "bg-warning",
  sale: "bg-positive",
  market: "bg-primary",
  owned: "bg-muted-foreground/60",
  available: "bg-foreground",
};

const fmt = (v: number) => v.toLocaleString(undefined, { maximumFractionDigits: v < 10 ? 2 : v < 1000 ? 1 : 0 });

/**
 * The table: rules and alignment, no bands, no fills. One line per name, 48px. The identity
 * column takes the width; every number is right-aligned in tabular figures; only the name and the
 * price carry the full ink. The action is a text link in one fixed column, and the verdicts
 * (track, like, imagine, hide, more) appear on the row when the pointer is on it.
 */
export function NameTable(p: Props) {
  const { items, loading, busy, onOpen, onWatch, onLike, onHide, onImagine, onRefresh, showEnds, empty, sorts, medians, hiddenView } = p;
  const H = (props: { col: string; label: string; align?: "right"; className?: string }) => (
    <SortHeader {...props} sorts={sorts} defaultSort={p.defaultSort} onSorts={p.onSorts} />
  );
  const endsCls = !p.endsFirst && "hidden xl:table-cell";
  const cols = 6 + (showEnds ? 1 : 0);

  return (
    <div className="panel overflow-hidden">
      {/* one line above the table: the count, the order, and the one shortcut */}
      <div className="flex min-h-11 flex-wrap items-center gap-x-2 gap-y-1.5 border-b border-border px-4 py-2 text-[13px]">
        <span className="data text-foreground">
          {loading && items.length === 0 ? "…" : p.total.toLocaleString()} <span className="text-muted-foreground">names</span>
        </span>
        <span className="mx-1 text-muted-foreground/50">·</span>
        {sorts.length === 0 ? (
          <span className="text-muted-foreground">by {colLabel(p.defaultSort).toLowerCase()}</span>
        ) : (
          <div className="flex flex-wrap items-center gap-1">
            <span className="text-muted-foreground">by</span>
            {sorts.map((s, i) => (
              <span key={s.col} className="inline-flex h-6 items-center overflow-hidden rounded border border-border bg-muted/60">
                <button
                  onClick={() => p.onSorts(sorts.map((x, j) => (j === i ? { ...x, dir: x.dir === "asc" ? "desc" : "asc" } : x)))}
                  className="inline-flex items-center gap-1 px-1.5 hover:text-foreground"
                  title="Flip direction"
                >
                  {sorts.length > 1 && <span className="data text-[11px] text-muted-foreground">{i + 1}</span>}
                  {colLabel(s.col)}
                  {s.dir === "asc" ? <ArrowUp className="size-3" /> : <ArrowDown className="size-3" />}
                </button>
                <button onClick={() => p.onSorts(sorts.filter((_, j) => j !== i))} aria-label={`Stop sorting by ${colLabel(s.col)}`} className="border-l border-border px-1 text-muted-foreground hover:text-foreground">
                  <X className="size-3" />
                </button>
              </span>
            ))}
          </div>
        )}
        {!(sorts.length === 1 && sorts[0].col === "value") && (
          <button
            onClick={() => p.onSorts([{ col: "value", dir: "asc" }])}
            title="Cheapest for its AI score: price compared with what names rated the same usually go for"
            className="ml-auto inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <Gem className="size-3.5" />
            Best value
          </button>
        )}
      </div>

      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] border-collapse text-[14px]">
          <thead>
            <tr className="h-9 whitespace-nowrap text-[12px] text-muted-foreground">
              <H col="name" label="Username" className="pl-4" />
              <H col="score" label="Score" align="right" className="hidden w-[64px] md:table-cell" />
              <th className="hidden w-[132px] pl-5 text-left font-normal sm:table-cell">
                <StatusHeader {...p} />
              </th>
              {showEnds && <H col="ending" label="Ends" align="right" className={cn("w-[84px]", endsCls)} />}
              <th className="w-[136px]" />
              <H col="buyout" label="Price" align="right" className="w-[190px] pr-2" />
              <th className="w-[44px] pr-3" />
            </tr>
          </thead>
          <tbody>
            {items.map((u) => {
              const href = u.status === "market" && u.nft_friendly ? getgemsUrl(u.nft_friendly) : fragmentUrl(u.name);
              const instant = u.buyout != null;
              const price = u.buyout ?? u.price;
              const d = discount(u, medians);
              const sub =
                instant && u.status === "auction" && u.price != null
                  ? `bid ${fmt(u.price)}`
                  : price != null && u.usd != null && u.price != null && !instant
                    ? `$${Math.round(u.usd).toLocaleString()}`
                    : price == null && u.last_sale != null
                      ? "last sale"
                      : "";
              const verb = u.status === "market" || u.status === "sale" ? "Buy" : u.status === "owned" ? "Offer" : u.status === "auction" ? (instant ? "Buy now" : "Bid") : "Open";
              return (
                <tr key={u.name} onClick={() => onOpen(u.name)} className="group h-12 cursor-pointer border-t border-border/50 transition-colors hover:bg-row-hover">
                  {/* identity: the name in ink, its meaning in grey on the same line */}
                  <td className="max-w-0 pr-4 pl-4">
                    <div className="flex min-w-0 items-baseline gap-2.5">
                      <span className="data shrink-0 text-[15px] font-medium text-foreground">
                        {u.watched && <Star className="mr-1.5 inline size-3 fill-warning text-warning align-[-1px]" aria-label="Tracked" />}@{u.name}
                      </span>
                      <span className="truncate text-[13px] text-muted-foreground">{u.meaning || (u.score == null ? (u.junk ? "looks like junk" : "not rated yet") : "")}</span>
                    </div>
                  </td>

                  <td className="data hidden pr-0 text-right md:table-cell">
                    {u.score == null ? (
                      <span className="text-muted-foreground/50">—</span>
                    ) : (
                      <span title={`AI score ${u.score} of 10`}>
                        <span className={cn("text-[15px]", u.score >= 8 ? "font-semibold text-primary" : "text-foreground")}>{u.score}</span>
                        <span className="text-[12px] text-muted-foreground">/10</span>
                      </span>
                    )}
                  </td>

                  <td className="hidden pl-5 whitespace-nowrap sm:table-cell">
                    <span className={cn("inline-flex items-center gap-2 text-[13px]", u.status === "owned" ? "text-muted-foreground" : "text-foreground")}>
                      <span className={cn("size-1.5 shrink-0 rounded-full", DOT[u.status])} />
                      {statusLabel(u)}
                    </span>
                  </td>

                  {showEnds && (
                    <td className={cn("text-right text-[13px]", endsCls)}>{u.ends_at ? <Countdown ts={u.ends_at} /> : <span className="text-muted-foreground/50">—</span>}</td>
                  )}

                  {/* verdicts, shown when the pointer is on the row, just left of the price */}
                  <td className="pl-3 whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                    <div className="flex items-center justify-end gap-0.5 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
                      <RowBtn label={u.watched ? "Stop tracking" : "Track"} onClick={() => onWatch(u)} hover="hover:text-warning">
                        <Star className={cn("size-[15px]", u.watched && "fill-warning text-warning")} />
                      </RowBtn>
                      <RowBtn label="Like: shapes your taste" onClick={() => onLike(u)} hover="hover:text-primary">
                        <Heart className="size-[15px]" />
                      </RowBtn>
                      <RowBtn label="Imagine a project for it · 2 credits" onClick={() => onImagine(u.name)} hover="hover:text-primary">
                        <Lightbulb className="size-[15px]" />
                      </RowBtn>
                      <RowBtn label={hiddenView ? "Bring it back" : "Never show again"} onClick={() => onHide(u)} hover="hover:text-destructive">
                        {hiddenView ? <Eye className="size-[15px]" /> : <EyeOff className="size-[15px]" />}
                      </RowBtn>
                    </div>
                  </td>

                  {/* the price is the button: one outlined pill, green-edged when it buys instantly */}
                  <td className="pr-2 text-right whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                    <div className="inline-flex items-center gap-2">
                      {d != null && d >= 0.15 && (
                        <span className="data rounded-sm bg-positive/15 px-1 text-[11px] font-medium text-positive" title={`${Math.round(d * 100)}% under what names rated ${u.score} typically sell for`}>
                          −{Math.round(d * 100)}%
                        </span>
                      )}
                      {sub && !(d != null && d >= 0.15) && <span className="data text-[11px] text-muted-foreground">{sub}</span>}
                      <a
                        href={href}
                        target="_blank"
                        rel="noreferrer"
                        onClick={() => instant && studio.signal(u.name, "buy")}
                        title={`${verb}${u.status === "market" ? ` on ${u.chain_market ?? "the marketplace"}` : " on Fragment"}`}
                        className={cn(
                          "data inline-flex h-8 min-w-[108px] items-center justify-end gap-1.5 rounded-md border px-2.5 text-[14px] transition-colors",
                          instant
                            ? "border-positive/50 font-semibold text-foreground hover:bg-positive/10"
                            : price != null
                              ? "border-border text-foreground hover:border-input hover:bg-muted"
                              : "border-border/60 text-muted-foreground hover:bg-muted",
                        )}
                      >
                        {price != null ? (
                          <>
                            {fmt(price)} <span className="text-[12px] font-normal text-muted-foreground">TON</span>
                          </>
                        ) : u.last_sale != null ? (
                          <span className="text-[12px]">sold {fmt(u.last_sale)}</span>
                        ) : (
                          <span className="text-[12px]">{verb}</span>
                        )}
                        <ExternalLink className={cn("size-3", instant ? "text-positive" : "opacity-50")} />
                      </a>
                    </div>
                  </td>

                  {/* the kebab, always */}
                  <td className="pr-3 whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                    <div className="flex items-center justify-end">
                      <DropdownMenu>
                        <DropdownMenuTrigger className="inline-flex size-7 items-center justify-center rounded text-muted-foreground/60 hover:bg-muted hover:text-foreground" aria-label="More">
                          <MoreHorizontal className="size-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-52">
                          <DropdownMenuItem onClick={() => onWatch(u)}>
                            <Star className="size-3.5" />
                            {u.watched ? "Stop tracking" : "Track"}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => onLike(u)}>
                            <Heart className="size-3.5" />
                            Like
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => onImagine(u.name)}>
                            <Lightbulb className="size-3.5" />
                            Imagine a project
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => onHide(u)}>
                            {hiddenView ? <Eye className="size-3.5" /> : <EyeOff className="size-3.5" />}
                            {hiddenView ? "Bring it back" : "Never show again"}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => onRefresh(u.name)} disabled={busy[u.name]}>
                            <RotateCw className={cn("size-3.5", busy[u.name] && "animate-spin")} />
                            Re-check on Fragment
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => {
                              navigator.clipboard?.writeText("@" + u.name);
                              toast(`@${u.name} copied`);
                            }}
                          >
                            <Copy className="size-3.5" />
                            Copy @{u.name}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() =>
                              studio
                                .savePortfolio({ name: u.name, bought: u.last_sale ?? u.buyout ?? null, concept: "", note: "" })
                                .then(() => toast.success(`@${u.name} is in your portfolio`))
                                .catch((e) => toast.error(e.message))
                            }
                          >
                            <Briefcase className="size-3.5" />I own it
                          </DropdownMenuItem>
                          {u.nft_friendly && (
                            <DropdownMenuItem render={<a href={getgemsUrl(u.nft_friendly)} target="_blank" rel="noreferrer" />}>
                              <ExternalLink className="size-3.5" />
                              Open on Getgems
                            </DropdownMenuItem>
                          )}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </td>
                </tr>
              );
            })}
            {loading &&
              items.length === 0 &&
              Array.from({ length: 12 }).map((_, i) => (
                <tr key={i} className="h-12 border-t border-border/50">
                  <td className="pl-4">
                    <Skeleton className="h-3.5 w-48" />
                  </td>
                  <td className="hidden md:table-cell">
                    <Skeleton className="ml-auto h-3 w-8" />
                  </td>
                  <td className="hidden pl-5 sm:table-cell">
                    <Skeleton className="h-3 w-16" />
                  </td>
                  {showEnds && <td className={cn(endsCls)} />}
                  <td className="pr-2 text-right">
                    <Skeleton className="ml-auto h-8 w-[108px] rounded-md" />
                  </td>
                  <td />
                </tr>
              ))}
            {!loading && items.length === 0 && (
              <tr>
                <td colSpan={cols} className="px-4 py-16 text-center text-sm text-muted-foreground">
                  {empty}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function RowBtn({ label, onClick, hover, children }: { label: string; onClick: () => void; hover: string; children: React.ReactNode }) {
  return (
    <button onClick={onClick} aria-label={label} title={label} className={cn("inline-flex size-7 items-center justify-center rounded transition-colors hover:bg-muted active:scale-90", hover)}>
      {children}
    </button>
  );
}

/** Sentence-case header, grey; the sort glyph only on an active column. */
function SortHeader({ col, label, align, className, sorts, defaultSort, onSorts }: { col: string; label: string; align?: "right"; className?: string; sorts: SortKey[]; defaultSort: string; onSorts: (s: SortKey[]) => void }) {
  const i = sorts.findIndex((s) => s.col === col);
  const active = i >= 0;
  const isDefault = sorts.length === 0 && defaultSort === col;
  const dir = active ? sorts[i].dir : undefined;
  const next = !active ? "ascending" : dir === "asc" ? "descending" : "off";
  return (
    <th className={cn("font-normal", align === "right" ? "text-right" : "text-left", className)} aria-sort={active ? (dir === "asc" ? "ascending" : "descending") : "none"}>
      <button
        onClick={() => onSorts(cycleSort(sorts, col))}
        title={`Sort ${next}`}
        className={cn("inline-flex h-7 items-center gap-1 rounded hover:text-foreground", (active || isDefault) && "text-foreground", align === "right" && "flex-row-reverse")}
      >
        {label}
        {active && (dir === "asc" ? <ArrowUp className="size-3 text-primary" /> : <ArrowDown className="size-3 text-primary" />)}
        {active && sorts.length > 1 && <span className="data text-[10px] text-primary">{i + 1}</span>}
      </button>
    </th>
  );
}

function StatusHeader({ statusOptions, statuses, onStatuses }: Props) {
  if (statusOptions.length < 2) return <span>Status</span>;
  const on = statuses.length > 0;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className={cn("inline-flex h-7 items-center gap-1 rounded hover:text-foreground", on && "text-foreground")}>
        Status
        <ListFilter className="size-3" />
        {on && <span className="data text-[11px] text-primary">{statuses.length}</span>}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-48">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Show only</DropdownMenuLabel>
          {statusOptions.map((s) => (
            <DropdownMenuCheckboxItem key={s} checked={statuses.includes(s)} onCheckedChange={(v) => onStatuses(v ? [...statuses, s] : statuses.filter((x) => x !== s))}>
              {STATUS_LABELS[s] ?? s}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuGroup>
        {on && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => onStatuses([])}>Show all statuses</DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
