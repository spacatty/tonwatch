import { getLang } from "@/lib/i18n";
import { serverError } from "@/lib/i18n/server-error";

export type Status = "auction" | "sale" | "market" | "owned" | "available";

export interface Username {
  name: string;
  status: Status;
  frag_status: string;
  price: number | null;
  buy_now: number | null;
  /** what owning it costs right now: buy-now on an auction, the listing price otherwise */
  buyout: number | null;
  usd: number | null;
  ends_at: number | null;
  last_sale: number | null;
  owner: string | null;
  nft_address: string | null;
  nft_friendly?: string;
  chain_market: string | null;
  sale_contract: string | null;
  len: number;
  junk: boolean;
  score: number | null;
  category: string | null;
  meaning: string | null;
  watched: boolean;
  hidden: boolean;
  note: string | null;
  src: number;
  frag_at: number | null;
  chain_at: number | null;
  first_seen: number;
}

export interface EventRow {
  name: string;
  ts: number;
  kind: string;
  detail: string;
}

export interface JobStatus {
  name: string;
  desc: string;
  /** the backend's format for `progress`, and its arguments: the panel translates by them */
  progress_key?: string;
  progress_args?: string[];
  running: boolean;
  progress: string;
  last_run: number;
  last_duration_ms: number;
  last_error: string;
  next_run: number;
  every_sec: number;
}

export interface StatsResp {
  stats: {
    total: number;
    by_status: Record<string, number>;
    scored: number;
    /** names whose facts the model recorded */
    facts?: number;
    /** names waiting for their facts */
    llm_queue: number;
    /** names the premium pass would re-read */
    premium_queue?: number;
    junk: number;
    good: number;
    watched: number;
    hidden?: number;
    by_category: Record<string, number>;
  };
  /** admins only */
  llm_usage?: { prompt_tokens: number; completion_tokens: number; cost: number };
  chain_done_at?: string;
}

export interface Meta {
  categories: string[];
  llm_enabled: boolean;
  probe_min_score: number;
  /** SKIP_CHAIN: the tonapi chain crawl and price jobs are not scheduled */
  chain_off?: boolean;
  /** admins only */
  model?: string;
  proxies?: number;
}

export interface Tracker {
  id: number;
  name: string;
  view: string;
  filters: string;
  params: string;
  seen_at: number;
  created_at: number;
  total: number;
  new: number;
}

export interface SuggestPick {
  fit: number;
  reason: string;
  item: Username;
}

export interface SuggestIdea {
  name: string;
  status: string;
  item?: Username;
}

export interface SuggestResult {
  summary: string;
  keywords: string[];
  picks: SuggestPick[];
  ideas: SuggestIdea[];
  searched: number;
}

export interface Settings {
  max_buyout: number; // 0 = no cap
  min_score: number;
  kinds: string[];
  /** read-only: the install's free daily allowance */
  daily_credits: number;
  about: string;
  /** negative prompt: kinds of project and name never to suggest */
  avoid: string;
  /** read-only: bought or granted */
  plan: "free" | "pro";
  /** read-only: their linked Telegram, where alerts go */
  telegram_chat: string;
}

export interface Taste {
  likes: string[];
  skips: string[];
  /** from behaviour: stars, buys, long looks */
  soft_likes: string[];
  soft_skips: string[];
  profile: string;
}

export interface Angle {
  angle: string;
  project: string;
  fit: number;
}

export interface PortfolioItem {
  name: string;
  bought: number | null;
  bought_at: number | null;
  concept: string;
  note: string;
  added_at: number;
  item: Username;
  typical: number | null;
}

export interface Portfolio {
  items: PortfolioItem[];
  paid: number;
  now: number;
  typical: Record<number, number>;
  unpriced: number;
  listed_now: number;
}

export interface ConceptName {
  name: string;
  angle: string;
}

/** One generated project, kept in the history forever. */
export interface Concept {
  id: number;
  ts: number;
  source: "set" | "rotate" | "refine" | "imagine";
  project: string;
  pitch: string;
  first_move: string;
  form?: string;
  how?: string;
  /** the data it runs on and the Telegram surface: proof it can be built */
  built_with?: string;
  note?: string;
  names: ConceptName[];
  /** messages in its chat */
  chat: number;
  /** 1 liked, -1 disliked, 0 none */
  liked: number;
}

export interface ChatMsg {
  role: "user" | "assistant";
  content: string;
  ts: number;
}

export interface AlertsStatus {
  bot_set: boolean;
  bot: string;
  plan: string;
  chat: string;
  active: boolean;
  last_run: number;
  sent: number;
  why: string;
}

export interface IdeaItem {
  angle: string;
  item: Username;
}

export interface Idea {
  project: string;
  pitch: string;
  first_move: string;
  /** the data it runs on and the Telegram surface: proof it can be built */
  built_with?: string;
  /** the concept's row in the history (0 for very old sets) */
  id: number;
  items: IdeaItem[];
}

export interface IdeasResult {
  generated_at: number;
  profile: string;
  ideas: Idea[];
  searched: number;
  usage: { prompt_tokens: number; completion_tokens: number; cost: number };
  credits: number;
  settings: Settings;
}

export interface LedgerEntry {
  ts: number;
  feature: string;
  credits: number;
  tokens: number;
  cached?: number;
  cost: number;
  note?: string;
}

export interface Credits {
  balance: number;
  daily: number;
  prices: Record<string, number>;
  ledger: LedgerEntry[];
}

export interface User {
  id: number;
  tg_id: number;
  tg_username: string;
  tg_name: string;
  tg_photo: string;
  role: "user" | "admin";
  plan: "free" | "pro";
  /** unix; Pro bought until then (0 = never bought) */
  plan_until: number;
  credits: number;
  created_at: number;
  /** how to show the account: @handle or name */
  label: string;
  seen_at?: number;
  /** the language the bot writes to them in: en | ru */
  lang?: string;
}

/** Which sign-in doors this install has open. */
export interface AuthInfo {
  telegram: boolean;
  bot: string;
  widget: boolean;
  public_url: string;
}

export interface Pack {
  id: string;
  title: string;
  note?: string;
  credits?: number;
  pro_days?: number;
  ton: number;
  stars: number;
}

export interface Payment {
  id: number;
  user_id: number;
  gateway: "ton" | "stars";
  pack: string;
  credits: number;
  pro_days: number;
  amount: number;
  currency: "TON" | "XTR";
  memo: string;
  status: "pending" | "paid" | "expired";
  ext_id?: string;
  created_at: number;
  expires_at: number;
  paid_at?: number;
  user?: string;
}

/** How to pay a checkout: TON fields, or a Stars invoice link. */
export interface PayHow {
  address?: string;
  amount?: number;
  comment?: string;
  ton_link?: string;
  tonkeeper_link?: string;
  invoice_link?: string;
}

/** Fired after any call that spent credits, so the sidebar meter stays current. */
export const creditsChanged = new EventTarget();
/** Fired when the backend says the session is gone (401), so the app shows sign-in again. */
export const authLost = new EventTarget();

// The Mini App signs in with a bearer token held in memory (Telegram's webview is unreliable with
// cookies); the web uses the HttpOnly session cookie and never sees a token.
let bearer: string | null = null;
export const setBearer = (t: string | null) => {
  bearer = t;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (bearer) headers.set("Authorization", `Bearer ${bearer}`);
  // the language the AI writes its answers in
  headers.set("X-Lang", getLang());
  const r = await fetch(path, { cache: "no-store", ...init, headers });
  const body = await r.json().catch(() => ({}));
  if (r.status === 401 && !path.startsWith("/api/auth/")) authLost.dispatchEvent(new Event("lost"));
  if (!r.ok) throw Object.assign(new Error(serverError(body.error ?? `HTTP ${r.status}`)), { status: r.status });
  return body as T;
}

export const api = {
  list: (params: URLSearchParams) =>
    req<{ items: Username[]; total: number }>(`/api/usernames?${params}`),
  get: (name: string) =>
    req<{ item: Username; events: EventRow[] }>(`/api/usernames/${encodeURIComponent(name)}`),
  refresh: (name: string) =>
    req<{ item: Username }>(`/api/usernames/${encodeURIComponent(name)}/refresh`, { method: "POST" }),
  watch: (name: string, watched: boolean, note?: string) =>
    req(`/api/usernames/${encodeURIComponent(name)}/watch`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(note === undefined ? { watched } : { watched, note }),
    }),
  /** archive a name out of every list, or bring it back */
  hide: (name: string, hidden: boolean) =>
    req(`/api/usernames/${encodeURIComponent(name)}/hide`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ hidden }) }),
  stats: () => req<StatsResp>("/api/stats"),
  jobs: () => req<JobStatus[]>("/api/jobs"),
  runJob: (name: string) => req(`/api/jobs/${name}/run`, { method: "POST" }),
  stopJob: (name: string) => req(`/api/jobs/${name}/stop`, { method: "POST" }),
  events: () => req<EventRow[]>("/api/events"),
  meta: () => req<Meta>("/api/meta"),
  resetLLM: () => req("/api/llm/reset", { method: "POST" }),
  /** typical prices per score ("len" absent) and per score and length bucket ("4", "5", "6-7", "8-10", "11+") */
  medians: () => req<{ score: number; len?: string; sold: number; sold_count: number; ask: number; ask_count: number }[]>("/api/market/medians"),
  trackers: () => req<Tracker[]>("/api/trackers"),
  createTracker: (t: { name: string; view: string; filters: string; params: string }) =>
    req<{ id: number }>("/api/trackers", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(t) }),
  updateTracker: (id: number, t: Partial<{ name: string; view: string; filters: string; params: string }>) =>
    req(`/api/trackers/${id}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(t) }),
  deleteTracker: (id: number) => req(`/api/trackers/${id}`, { method: "DELETE" }),
  trackerSeen: (id: number) => req(`/api/trackers/${id}/seen`, { method: "POST" }),
  expandTopic: (topic: string) =>
    spend(req<{ keywords: string[] }>("/api/ai/expand", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ topic }) })),
  suggest: (body: { brief: string; max_price?: number; buyable_only?: boolean; exclude?: string[] }) =>
    spend(req<SuggestResult>("/api/ai/suggest", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) })),
  /** the AI consultation, kept on the server: asking returns at once, the answer arrives in the background */
  aiChat: () => req<{ turns: AITurn[] }>("/api/ai/chat"),
  aiAsk: (body: { brief: string; max_price?: number; buyable_only?: boolean; exclude?: string[] }) =>
    req<{ turns: AITurn[] }>("/api/ai/chat", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }),
  aiClear: () => req<{ turns: AITurn[] }>("/api/ai/chat", { method: "DELETE" }),
  aiFilters: () => req<{ recent: AIFilterResult[] }>("/api/ai/filter"),
  aiFilter: (prompt: string) =>
    spend(req<{ filter: AIFilterResult }>("/api/ai/filter", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ prompt }) })),
};

/** An AI filter: the request, and the table rules it became (filters in the dashboard's own shape). */
export interface AIFilterResult {
  prompt: string;
  view: string;
  filters: Record<string, unknown>;
  summary: string;
  /** what the request asked for that no rule can say */
  could_not?: string;
  at: number;
}

export interface AITurn {
  id: number;
  brief: string;
  opts: { buyable_only?: boolean; max_price?: number | null };
  status: "pending" | "done" | "error";
  result?: SuggestResult;
  error?: string;
  created_at: number;
}

async function spend<T>(p: Promise<T>): Promise<T> {
  try {
    return await p;
  } finally {
    creditsChanged.dispatchEvent(new Event("change"));
  }
}

const json = (body: unknown): RequestInit => ({ headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
export const studio = {
  settings: () => req<Settings>("/api/settings"),
  saveSettings: (s: Partial<Settings>) => req<Settings>("/api/settings", { method: "PUT", ...json(s) }),
  taste: () => req<Taste>("/api/taste"),
  judge: (name: string, verdict: "like" | "skip" | "clear") => req<Taste>("/api/taste", { method: "POST", ...json({ name, verdict }) }),
  resetTaste: () => req<Taste>("/api/taste", { method: "POST", ...json({ reset: true }) }),
  tasteSample: () => req<{ items: Username[] }>("/api/taste/sample"),
  ideas: () => req<IdeasResult>("/api/ideas"),
  generateIdeas: () => spend(req<IdeasResult>("/api/ideas", { method: "POST" })),
  rotateIdea: (index: number, note?: string) => spend(req<IdeasResult>("/api/ideas/rotate", { method: "POST", ...json({ index, note }) })),
  credits: () => req<Credits>("/api/credits"),
  angle: (name: string) => spend(req<{ angle: Angle; cached?: boolean }>("/api/ai/angle", { method: "POST", ...json({ name }) })),
  angles: (names: string[]) => spend(req<{ angles: Record<string, Angle>; cached?: boolean }>("/api/ai/angles", { method: "POST", ...json({ names }) })),
  /** a behaviour signal that teaches the taste for free; never throws */
  signal: (name: string, kind: "star" | "buy" | "open" | "dismiss") =>
    req("/api/taste/signal", { method: "POST", ...json({ name, kind }) }).catch(() => {}),
  portfolio: () => req<Portfolio>("/api/portfolio"),
  savePortfolio: (p: { name: string; bought: number | null; bought_at?: number | null; concept: string; note: string }) =>
    req<Portfolio>("/api/portfolio", { method: "POST", ...json(p) }),
  removePortfolio: (name: string) => req<Portfolio>(`/api/portfolio/${encodeURIComponent(name)}`, { method: "DELETE" }),
  history: (name?: string, limit = 300) => req<{ concepts: Concept[] }>(`/api/ideas/history?limit=${limit}${name ? `&name=${encodeURIComponent(name)}` : ""}`),
  forget: (id: number) => req(`/api/ideas/history/${id}`, { method: "DELETE" }),
  verdict: (id: number, verdict: "like" | "dislike" | "clear") => req<{ liked: number }>(`/api/ideas/history/${id}/verdict`, { method: "POST", ...json({ verdict }) }),
  chat: (id: number) => req<{ messages: ChatMsg[]; price: number }>(`/api/ideas/chat/${id}`),
  ask: (id: number, question: string) => spend(req<{ messages: ChatMsg[]; credits: number }>(`/api/ideas/chat/${id}`, { method: "POST", ...json({ question }) })),
  imagine: (name: string, note?: string) =>
    spend(req<{ concept: Concept; fit: number; credits: number }>("/api/ideas/imagine", { method: "POST", ...json({ name, note }) })),
  alerts: () => req<AlertsStatus>("/api/alerts"),
  alertTest: () => req<AlertsStatus>("/api/alerts/test", { method: "POST" }),
};

type Session = { user: User; token: string };
/** The bot is answering: pending (link not opened yet), waiting (asked in Telegram), ok, denied, expired. */
export type HandshakeStatus = "pending" | "waiting" | "ok" | "denied" | "expired";

export const account = {
  me: () => req<{ user: User | null; auth: AuthInfo }>("/api/auth/me"),
  logout: () => req("/api/auth/logout", { method: "POST" }),
  logoutOthers: () => req("/api/auth/logout-all", { method: "POST" }),
  /** the language the bot writes to this account in */
  setLang: (lang: string) => req("/api/auth/lang", { method: "POST", ...json({ lang }) }),
  webApp: (init_data: string) => req<Session>("/api/auth/telegram/webapp", { method: "POST", ...json({ init_data }) }),
  widget: (data: Record<string, unknown>) => req<Session>("/api/auth/telegram/widget", { method: "POST", ...json(data) }),
  /** start "Continue with Telegram": a bot link to open, and a secret to poll */
  tgStart: () => req<{ secret: string; url: string; expires_in: number }>("/api/auth/telegram/start", { method: "POST" }),
  tgPoll: (secret: string) => req<{ status: HandshakeStatus; user?: User; token?: string }>(`/api/auth/telegram/poll/${encodeURIComponent(secret)}`),
};

export const billing = {
  packs: () => req<{ packs: Pack[]; gateways: { id: "ton" | "stars"; label: string }[] }>("/api/billing/packs"),
  checkout: (pack: string, gateway: string) => req<{ payment: Payment; pay: PayHow }>("/api/billing/checkout", { method: "POST", ...json({ pack, gateway }) }),
  payment: (id: number) => req<{ payment: Payment; user: User }>(`/api/billing/payments/${id}`),
  payments: () => req<{ payments: Payment[] }>("/api/billing/payments"),
};

export interface UserTotals {
  total: number;
  pro: number;
  admins: number;
  telegram: number;
  active_1d: number;
  active_7d: number;
  active_30d: number;
  new_7d: number;
  /** credits sitting on balances */
  credits: number;
}

export interface DayStat {
  day: string;
  signups: number;
  active: number;
  events: number;
  spent: number;
  ai_cost: number;
  paid: number;
}

export interface FeatureUse {
  feature: string;
  runs: number;
  credits: number;
  cost: number;
  users: number;
}

/** One line of the people audit trail: sign-ins, AI runs, checkouts, admin changes. */
export interface Activity {
  id: number;
  user_id: number;
  actor_id?: number;
  ts: number;
  kind: string;
  detail?: string;
  user?: string;
  actor?: string;
}

export interface UserFootprint {
  sessions: number;
  trackers: number;
  concepts: number;
  portfolio: number;
  starred: number;
  hidden: number;
  spent: number;
  ai_cost: number;
  bought: number;
  granted: number;
}

export interface AdminUserDetail {
  user: User & { login: string; email: string; tg_2fa: boolean };
  footprint: UserFootprint;
  ledger: LedgerEntry[];
  payments: Payment[];
  activity: Activity[];
}

export interface AdminUserUpdate {
  role?: string;
  plan?: string;
  grant?: number;
  set_credits?: number;
  pro_days?: number;
  note?: string;
}

export const admin = {
  overview: () =>
    req<{
      users: number;
      totals: UserTotals;
      paid: Record<string, number>;
      llm_usage: { cost: number };
      stats: StatsResp["stats"];
      gateways: Record<string, boolean>;
      bot: string;
      daily_credits: number;
    }>("/api/admin/overview"),
  stats: (days = 30) => req<{ days: number; daily: DayStat[]; features: FeatureUse[]; kinds: { kind: string; n: number }[] }>(`/api/admin/stats?days=${days}`),
  users: (q = "") => req<{ users: User[] }>(`/api/admin/users?q=${encodeURIComponent(q)}`),
  user: (id: number) => req<AdminUserDetail>(`/api/admin/users/${id}`),
  updateUser: (id: number, b: AdminUserUpdate) => req<{ user: User }>(`/api/admin/users/${id}`, { method: "POST", ...json(b) }),
  logoutUser: (id: number) => req<{ ok: boolean }>(`/api/admin/users/${id}/logout`, { method: "POST" }),
  activity: (f: { user?: number; kind?: string; before?: number; limit?: number } = {}) => {
    const p = new URLSearchParams();
    Object.entries(f).forEach(([k, v]) => v && p.set(k, String(v)));
    return req<{ activity: Activity[] }>(`/api/admin/activity?${p}`);
  },
  payments: () => req<{ payments: Payment[] }>("/api/admin/payments"),
};

export const fragmentUrl = (name: string) => `https://fragment.com/username/${name}`;
export const getgemsUrl = (friendly: string) => `https://getgems.io/nft/${friendly}`;
export const tonviewerUrl = (friendly: string) => `https://tonviewer.com/${friendly}`;
