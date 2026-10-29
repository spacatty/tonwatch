// Ask AI (AskAI.tsx).
import type { ReactNode } from "react";

const en = {
  examples: [
    "I'm launching a VPN app and need a short, trustworthy name",
    "A specialty coffee shop in Berlin",
    "Crypto trading signals channel, under 50 TON",
    "Personal brand for a travel photographer",
    "AI assistant bot for lawyers",
  ],
  steps: ["Reading your request", "Searching every username we track", "Ranking what fits", "Checking fresh ideas on Fragment"],
  intro:
    "Describe what the username is for. The AI turns it into search terms, looks through every name we track, ranks the ones that fit and checks a few fresh ideas live on Fragment.",
  buyableNowTag: "buyable now",
  buyoutTag: (price: string) => `buyout ≤ ${price} TON`,
  failed: (why: string) => `Couldn't get suggestions: ${why}`,
  placeholder: "I need a username for…",
  placeholderOff: "Add OPENROUTER_API_KEY to .env to use Ask AI",
  buyableNow: "Buyable now",
  buyoutUpTo: "Buyout up to",
  upTo: "up to",
  hello: "What is the username for?",
  enterHint: "Enter to send · Shift+Enter for a new line",
  any: "any",
  clearChat: "Clear chat",
  ask: "Ask",
  /** after the AI's own summary; n() renders a number */
  summary: (searched: number, picked: number, n: (v: number) => ReactNode): ReactNode[] => [" I looked through ", n(searched), " matching names and picked ", n(picked), " that fit."],
  searchedFor: "Searched for",
  browseAll: "Browse all matches",
  saveTracker: "Save as tracker",
  moreLike: "More like this",
  canGetNow: "You can get these now",
  ownedOffer: "Owned: make the owner an offer",
  nothingFits: "Nothing in the database fits well yet. Try the fresh ideas below, or describe it differently.",
  freshIdeas: "Fresh ideas, checked on Fragment",
  freshNote: "Invented for you. “Taken” names can still get an offer.",
  status: { available: "Available", auction: "Auction", sale: "For sale", sold: "Taken", taken: "Taken" } as Record<string, string>,
  fitTitle: "How well it fits your request",
  bid: "bid",
  priceOn: (market: string | null) => `price on ${market ?? "market"}`,
  offerOnly: "offer only",
  track: (name: string) => `Track @${name}`,
  open: (name: string) => `Open @${name}`,
};

export default en;
