"use client";

import { FolderPlus, ListTree, Plus, X } from "lucide-react";
import { useT } from "@/lib/i18n";
import { FIELDS, isGroup, newGroup, newLeaf, type RuleNode } from "@/lib/rules";
import { cn } from "@/lib/utils";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";

const FIELD_GROUPS: { key: "money" | "quality" | "time" | "what" | "shape"; fields: string[] }[] = [
  { key: "money", fields: ["buyout", "price", "value_ratio", "vs_last_sale", "last_sale"] },
  { key: "quality", fields: ["score", "kind", "rated", "junk"] },
  { key: "time", fields: ["quiet_days", "age_days", "ends_in_hours"] },
  { key: "what", fields: ["status", "market", "buy_now", "watched"] },
  { key: "shape", fields: ["keywords", "name", "meaning", "len", "has_digits", "has_underscore"] },
];

/** The rule tree under the filter chips: groups of rules joined by "all of" or "any of", any rule
 *  or group flipped with NOT, nested as deep as needed. Every edit re-runs the list. */
export function RuleTree({ value, onChange, onRemove }: { value: RuleNode; onChange: (r: RuleNode) => void; onRemove: () => void }) {
  const t = useT().rules;
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <div className="mb-2 flex items-center gap-2 px-1">
        <ListTree className="size-4 text-primary" />
        <span className="text-[13px] font-medium">{t.title}</span>
        <span className="text-[12px] text-muted-foreground">{t.hint}</span>
        <button onClick={onRemove} className="ml-auto text-[12px] text-muted-foreground hover:text-destructive">
          {t.removeAll}
        </button>
      </div>
      <Group node={value} onChange={onChange} depth={0} />
    </div>
  );
}

function Group({ node, onChange, onDelete, depth }: { node: RuleNode; onChange: (r: RuleNode) => void; onDelete?: () => void; depth: number }) {
  const t = useT().rules;
  const rules = node.rules ?? [];
  const setChild = (i: number, r: RuleNode) => onChange({ ...node, rules: rules.map((x, j) => (j === i ? r : x)) });
  const delChild = (i: number) => onChange({ ...node, rules: rules.filter((_, j) => j !== i) });
  return (
    <div className={cn("space-y-1.5", depth > 0 && "rounded-lg border-l-2 border-primary/30 bg-muted/30 py-2 pr-2 pl-3")}>
      <div className="flex flex-wrap items-center gap-1.5">
        <NotToggle on={!!node.not} onChange={(not) => onChange({ ...node, not })} />
        <div className="flex rounded-md border border-border p-0.5 text-[12px]">
          {(["and", "or"] as const).map((op) => (
            <button
              key={op}
              onClick={() => onChange({ ...node, op })}
              className={cn("rounded px-2 py-0.5 transition-colors", (node.op ?? "and") === op ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:text-foreground")}
            >
              {op === "and" ? t.allOf : t.anyOf}
            </button>
          ))}
        </div>
        <span className="text-[12px] text-muted-foreground">{t.theseRules}</span>
        {onDelete && (
          <button onClick={onDelete} aria-label={t.removeGroup} title={t.removeGroup} className="ml-auto grid size-6 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-destructive">
            <X className="size-3.5" />
          </button>
        )}
      </div>
      {rules.map((r, i) =>
        isGroup(r) ? (
          <Group key={i} node={r} onChange={(x) => setChild(i, x)} onDelete={() => delChild(i)} depth={depth + 1} />
        ) : (
          <Leaf key={i} node={r} onChange={(x) => setChild(i, x)} onDelete={() => delChild(i)} />
        ),
      )}
      {rules.length === 0 && <p className="px-1 text-[12.5px] text-muted-foreground">{t.empty}</p>}
      <div className="flex flex-wrap gap-1.5 pt-0.5">
        <DropdownMenu>
          <DropdownMenuTrigger className="inline-flex h-7 items-center gap-1 rounded-md border border-dashed border-border px-2 text-[12px] text-muted-foreground hover:border-primary/50 hover:text-foreground">
            <Plus className="size-3.5" />
            {t.addRule}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="max-h-80 w-60">
            {FIELD_GROUPS.map((g, gi) => (
              <div key={g.key}>
                {gi > 0 && <DropdownMenuSeparator />}
                <DropdownMenuGroup>
                  <DropdownMenuLabel>{t.fieldGroups[g.key]}</DropdownMenuLabel>
                  {g.fields.map((f) => (
                    <DropdownMenuItem key={f} onClick={() => onChange({ ...node, rules: [...rules, newLeaf(f)] })}>
                      {t.fields[f]}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuGroup>
              </div>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        {depth < 4 && (
          <button
            onClick={() => onChange({ ...node, rules: [...rules, newGroup((node.op ?? "and") === "and" ? "or" : "and")] })}
            className="inline-flex h-7 items-center gap-1 rounded-md border border-dashed border-border px-2 text-[12px] text-muted-foreground hover:border-primary/50 hover:text-foreground"
          >
            <FolderPlus className="size-3.5" />
            {t.addGroup}
          </button>
        )}
      </div>
    </div>
  );
}

function NotToggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  const t = useT().rules;
  return (
    <button
      onClick={() => onChange(!on)}
      aria-pressed={on}
      title={t.notHint}
      className={cn(
        "data h-6 rounded px-1.5 text-[10.5px] font-semibold tracking-wide transition-colors",
        on ? "bg-destructive/15 text-destructive" : "text-muted-foreground/60 hover:bg-muted hover:text-foreground",
      )}
    >
      {t.notWord.toUpperCase()}
    </button>
  );
}

const inputCls = "h-7 rounded-md border border-input bg-background px-2 text-[12.5px] outline-none focus:border-ring";

function Leaf({ node, onChange, onDelete }: { node: RuleNode; onChange: (r: RuleNode) => void; onDelete: () => void }) {
  const t = useT().rules;
  const def = FIELDS[node.field ?? ""];
  if (!def) return null;
  const set = (patch: Partial<RuleNode>) => onChange({ ...node, ...patch });
  const num = (v: string) => (v === "" ? "" : Number(v));

  let editor: React.ReactNode = null;
  if (def.kind === "num") {
    if (node.cmp === "between") {
      const [a, b] = Array.isArray(node.value) ? (node.value as unknown[]) : [node.value, node.value];
      editor = (
        <span className="inline-flex items-center gap-1">
          <input type="number" step="any" value={String(a ?? "")} onChange={(e) => set({ value: [num(e.target.value), b] })} className={cn(inputCls, "data w-20")} />
          <span className="text-[12px] text-muted-foreground">–</span>
          <input type="number" step="any" value={String(b ?? "")} onChange={(e) => set({ value: [a, num(e.target.value)] })} className={cn(inputCls, "data w-20")} />
        </span>
      );
    } else {
      editor = (
        <input
          type="number"
          step="any"
          value={Array.isArray(node.value) ? "" : String(node.value ?? "")}
          onChange={(e) => set({ value: num(e.target.value) })}
          className={cn(inputCls, "data w-20")}
        />
      );
    }
  } else if (def.kind === "set") {
    const vs = (node.value as string[]) ?? [];
    editor = (
      <span className="inline-flex flex-wrap gap-1">
        {def.options!.map((o) => (
          <button
            key={o}
            onClick={() => set({ value: vs.includes(o) ? vs.filter((x) => x !== o) : [...vs, o] })}
            className={cn("h-6 rounded-full border px-2 text-[11.5px] transition-colors", vs.includes(o) ? "border-primary/50 bg-accent text-accent-foreground" : "border-border text-muted-foreground hover:text-foreground")}
          >
            {t.values[o] ?? o}
          </button>
        ))}
      </span>
    );
  } else if (def.kind === "text") {
    const isList = node.field === "keywords";
    editor = (
      <input
        value={isList ? ((node.value as string[]) ?? []).join(", ") : String(node.value ?? "")}
        onChange={(e) => set({ value: isList ? e.target.value.split(",").map((s) => s.trim()).filter(Boolean) : e.target.value })}
        placeholder={isList ? t.keywordsPlaceholder : ""}
        className={cn(inputCls, "w-44")}
      />
    );
  } else {
    editor = (
      <select value={String(node.value)} onChange={(e) => set({ value: e.target.value === "true" })} className={cn(inputCls, "pr-1")}>
        <option value="true">{t.yes}</option>
        <option value="false">{t.no}</option>
      </select>
    );
  }

  return (
    <div className="group/leaf flex flex-wrap items-center gap-1.5 rounded-md px-1 py-0.5 hover:bg-muted/50" title={t.fieldHints[node.field!] ?? ""}>
      <NotToggle on={!!node.not} onChange={(not) => set({ not })} />
      <span className="text-[13px] font-medium">{t.fields[node.field!]}</span>
      {def.cmps.length > 1 ? (
        <select
          value={node.cmp}
          onChange={(e) => {
            const cmp = e.target.value;
            const v = node.value;
            set({ cmp, value: cmp === "between" ? (Array.isArray(v) ? v : [v, v]) : Array.isArray(v) && def.kind === "num" ? v[0] : v });
          }}
          className={cn(inputCls, "pr-1")}
        >
          {def.cmps.map((c) => (
            <option key={c} value={c}>
              {t.cmps[c]}
            </option>
          ))}
        </select>
      ) : (
        <span className="text-[12.5px] text-muted-foreground">{t.cmps[def.cmps[0]]}</span>
      )}
      {editor}
      {def.unit && <span className="text-[12px] text-muted-foreground">{def.unit === "×" ? t.timesTypical[node.field!] ?? "×" : def.unit}</span>}
      <button onClick={onDelete} aria-label={t.removeRule} title={t.removeRule} className="ml-auto grid size-6 place-items-center rounded text-muted-foreground opacity-0 group-hover/leaf:opacity-100 hover:bg-muted hover:text-destructive [@media(hover:none)]:opacity-100">
        <X className="size-3.5" />
      </button>
    </div>
  );
}
