// Shared bits (bits.tsx): countdowns, scores, statuses, "ago".
const en = {
  closing: "closing",
  left: { d: (d: number, h: number) => `${d}d ${h}h`, h: (h: number, m: string) => `${h}h ${m}m`, s: (s: number) => `${s}s`, m: (m: number, s: string) => `${m}m ${s}s` },
  junk: "junk",
  scoreOf: (s: number) => `AI score ${s} of 10`,
  status: { auction: "Auction", sale: "For sale", market: "Market", sold: "Sold", owned: "Owned" },
  deal: (pct: number, score: number, median: string) => `${pct}% under the typical price of names rated ${score} with a similar length (${median} TON)`,
  never: "never",
  ago: { s: (n: number) => `${n}s ago`, m: (n: number) => `${n}m ago`, h: (n: number) => `${n}h ago`, d: (n: number) => `${n}d ago` },
};

export default en;
