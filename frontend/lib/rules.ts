import { getT } from "@/lib/i18n";

/** A rule tree, as the backend reads it (backend/internal/store/rules.go): groups join their
 *  rules with AND or OR, any node can be negated, leaves compare one field with a value. */
export interface RuleNode {
  op?: "and" | "or";
  not?: boolean;
  rules?: RuleNode[];
  field?: string;
  cmp?: string;
  value?: unknown;
}

export type FieldKind = "num" | "set" | "text" | "bool";

export interface FieldDef {
  kind: FieldKind;
  cmps: string[];
  /** for sets: the values to pick from */
  options?: string[];
  /** what a new rule on this field starts as */
  initial: { cmp: string; value: unknown };
  /** unit shown after a number */
  unit?: string;
}

const NUM = ["gte", "lte", "gt", "lt", "eq", "ne", "between"];
export const STATUSES = ["auction", "sale", "market", "owned", "available"];
export const KINDS = ["word", "name", "brand", "phrase", "crypto", "number", "short", "geo", "slang", "foreign", "pattern", "junk"];

/** Every field a rule can look at, in the order the picker lists them. */
export const FIELDS: Record<string, FieldDef> = {
  score: { kind: "num", cmps: NUM, initial: { cmp: "gte", value: 7 } },
  buyout: { kind: "num", cmps: NUM, initial: { cmp: "lte", value: 50 }, unit: "TON" },
  price: { kind: "num", cmps: NUM, initial: { cmp: "lte", value: 50 }, unit: "TON" },
  value_ratio: { kind: "num", cmps: NUM, initial: { cmp: "lte", value: 0.5 }, unit: "×" },
  vs_last_sale: { kind: "num", cmps: NUM, initial: { cmp: "lt", value: 1 }, unit: "×" },
  last_sale: { kind: "num", cmps: NUM, initial: { cmp: "gte", value: 100 }, unit: "TON" },
  quiet_days: { kind: "num", cmps: NUM, initial: { cmp: "gte", value: 5 } },
  age_days: { kind: "num", cmps: NUM, initial: { cmp: "lte", value: 2 } },
  ends_in_hours: { kind: "num", cmps: NUM, initial: { cmp: "lte", value: 24 } },
  len: { kind: "num", cmps: NUM, initial: { cmp: "lte", value: 6 } },
  kind: { kind: "set", cmps: ["in", "nin"], options: KINDS, initial: { cmp: "in", value: ["crypto"] } },
  status: { kind: "set", cmps: ["in", "nin"], options: STATUSES, initial: { cmp: "in", value: ["auction"] } },
  market: { kind: "set", cmps: ["in", "nin"], options: ["getgems", "marketapp"], initial: { cmp: "in", value: ["getgems"] } },
  keywords: { kind: "text", cmps: ["any"], initial: { cmp: "any", value: [] } },
  name: { kind: "text", cmps: ["contains", "starts", "ends", "eq"], initial: { cmp: "contains", value: "" } },
  meaning: { kind: "text", cmps: ["contains"], initial: { cmp: "contains", value: "" } },
  has_digits: { kind: "bool", cmps: ["is"], initial: { cmp: "is", value: false } },
  has_underscore: { kind: "bool", cmps: ["is"], initial: { cmp: "is", value: false } },
  buy_now: { kind: "bool", cmps: ["is"], initial: { cmp: "is", value: true } },
  rated: { kind: "bool", cmps: ["is"], initial: { cmp: "is", value: true } },
  junk: { kind: "bool", cmps: ["is"], initial: { cmp: "is", value: false } },
  watched: { kind: "bool", cmps: ["is"], initial: { cmp: "is", value: true } },
};

export const newLeaf = (field: string): RuleNode => ({ field, ...FIELDS[field].initial, value: structuredClone(FIELDS[field].initial.value) });
export const newGroup = (op: "and" | "or" = "and"): RuleNode => ({ op, rules: [] });
export const isGroup = (r: RuleNode) => !r.field;

/** A leaf with nothing in it yet (an empty text box) is left out of the query rather than sent half-written. */
function complete(r: RuleNode): boolean {
  if (isGroup(r)) return true;
  const v = r.value;
  if (Array.isArray(v)) return v.length > 0 && (r.cmp !== "between" || v.every((x) => x !== "" && x != null && !Number.isNaN(Number(x))));
  if (typeof v === "string") return v.trim() !== "";
  if (typeof v === "number") return !Number.isNaN(v);
  return v != null;
}

/** The tree as the backend should get it: unfinished leaves and empty groups pruned, null when nothing is left. */
export function forQuery(r: RuleNode | null | undefined): RuleNode | null {
  if (!r) return null;
  if (!isGroup(r)) return complete(r) ? r : null;
  const rules = (r.rules ?? []).map(forQuery).filter((x): x is RuleNode => x != null);
  if (rules.length === 0) return null;
  return { ...r, rules };
}

/** One leaf in words, for tooltips and saved-tracker names: "score ≥ 7", "kind is one of crypto, brand". */
export function describe(r: RuleNode): string {
  const t = getT().rules;
  if (isGroup(r)) {
    const parts = (r.rules ?? []).map(describe).filter(Boolean);
    const inner = parts.length > 1 ? `(${parts.join(r.op === "or" ? ` ${t.orWord} ` : ` ${t.andWord} `)})` : parts[0] ?? "";
    return r.not && inner ? `${t.notWord} ${inner}` : inner;
  }
  const f = r.field!;
  const v = Array.isArray(r.value) ? (r.value as unknown[]).join(r.cmp === "between" ? "–" : ", ") : typeof r.value === "boolean" ? (r.value ? t.yes : t.no) : String(r.value);
  const s = `${t.fields[f] ?? f} ${t.cmps[r.cmp ?? ""] ?? r.cmp} ${v}`;
  return r.not ? `${t.notWord} ${s}` : s;
}
