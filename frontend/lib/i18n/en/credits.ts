// Credits and plan (CreditsDialog.tsx).
import { num } from "../index";

const en = {
  paidCredits: (n: number) => `+${num(n)} credits`,
  paidPro: (days: number) => `Pro for ${days} days`,
  title: "Credits and plan",
  description: (daily: string) =>
    `Every AI feature spends credits. Claim ${daily} free each day you come back; credits you buy add on top and never expire. Cached answers are free.`,
  balance: "Balance",
  freeDaily: "Free each day",
  claim: "Claim",
  holdToClaim: "Hold to claim",
  claimTitle: (daily: string) => `Today's ${daily} free credits`,
  claimBody: (n: string) => `Claim them to add ${n} to your balance. They wait for you each day, but only if you come and take them.`,
  claimFull: "Your balance is already above today's allowance; claiming keeps your daily streak.",
  claimed: (n: string) => `+${n} credits claimed`,
  claimedNone: "Claimed. Your balance was already above the allowance.",
  claimToast: (n: string) => `${n} free credits are waiting for you today.`,
  checkFailed: "The check didn't load. Reload the page and try again.",
  aiCost: "AI cost, last 60",
  topUp: "Top up",
  /** payment methods by id; the server's label is used for any other */
  gateways: { ton: "TON", stars: "Telegram Stars" } as Record<string, string>,
  noPayments: "Payments are not set up on this install yet. An admin can enable TON (TON_WALLET) or Telegram Stars (the bot) in .env.",
  /** packs by id (backend/internal/app/billing.go); the server's text is used for any other */
  packs: {
    c1000: { title: "1,000 credits", note: "" },
    c5000: { title: "5,000 credits", note: "20% off" },
    c20000: { title: "20,000 credits", note: "30% off" },
    pro30: { title: "Pro, 30 days", note: "Telegram alerts, everything unlocked" },
  } as Record<string, { title: string; note: string }>,
  aiCredits: "AI credits",
  proFromBalance: "Or pay from your balance",
  proCredits: (n: string) => `${n} credits`,
  proShort: (n: string) => `${n} more credits needed`,
  unlockTitle: "Your first top-up unlocks everything",
  unlockBody: (daily: string) =>
    `Any pack, even the smallest, opens every search in full, the rule builder, quiet-days and keyword filters, unlimited trackers, and ${daily} free AI credits a day instead of a handful. It stays unlocked for good.`,
  prices: "Prices",
  pricesSub: "Credits per action.",
  free: "free",
  ledger: "Ledger",
  lastEntries: (n: number) => `last ${n} entries`,
  nothingSpent: "Nothing spent yet.",
  cachedTokens: (n: string) => `${n} prompt tokens served from the provider's cache`,
  tok: "tok",
  cached: (pct: number) => ` · ${pct}% cached`,
  sendExactly: "Send exactly this amount ",
  withComment: "with the comment",
  commentWhy: ". The comment is how the payment finds your account.",
  address: "Address",
  amount: "Amount",
  comment: "Comment",
  openWallet: "Open wallet",
  payInTelegram: "Pay in Telegram. Credits arrive the moment Telegram confirms.",
  payStars: "Pay with Stars",
  expired: "This checkout expired. Money sent with its comment still counts.",
  waiting: "Waiting for the payment…",
  copy: (label: string) => `Copy ${label.toLowerCase()}`,
};

export default en;
