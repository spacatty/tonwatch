// The AI filter (AIFilter.tsx) and the rule bar it leaves above the table (Dashboard.tsx).
const en = {
  placeholder: "Describe the names you want, e.g. auctions with no bids for 5 days, under 30 TON, beautiful or crypto",
  off: "Add OPENROUTER_API_KEY to .env to use the AI filter",
  costs: "1 credit · becomes a live filter you can refresh and save",
  build: "Build filter",
  building: "Building…",
  startersTitle: "Starter phrases",
  starters: [
    "kind: crypto",
    "kind: real word",
    "looks like a brand",
    "looks like a bank",
    "beautiful (score 7+)",
    "short, up to 6 letters",
    "no digits",
    "under 30 TON",
    "buy now, no bidding",
    "no bids for 5 days",
    "auction ends today",
    "cheap for its quality",
  ],
  examplesTitle: "Try",
  examples: [
    "Auctions with no bids for at least 5 days, under 30 TON, beautiful or crypto",
    "Short real words I can buy now under 50 TON",
    "Names that look like a fintech brand, no digits, cheapest first",
    "Auctions ending today rated 8+",
  ],
  recentTitle: "Your recent filters",
  recentHint: "Opening one costs nothing: the rules are saved, the list is fetched fresh.",
  // the rule bar above the table
  bar: "AI filter",
  couldNot: "Left out",
  refresh: "Refresh",
  refreshed: "List refreshed",
  edit: "Edit request",
  save: "Save as tracker",
  dismiss: "Close the AI filter bar",
  quietSoon: "Price history starts 28 Oct 2026, so “no bids for N days” only finds names once that many days have passed.",
};

export default en;
