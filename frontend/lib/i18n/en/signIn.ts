// The sign-in screen (SignIn.tsx).
const en = {
  headline: "Every Telegram username for sale, rated and ranked.",
  pitch:
    "Listings from every marketplace, gathered in one place, with filters sharp enough to cut straight to the good ones. Save trackers, get project ideas for names you can buy today, and hear about price drops in Telegram.",
  live: "Live market",
  title: "Sign in with Telegram",
  subtitle: "Your trackers, taste and credits, on the web and in Telegram.",
  continue: "Continue with Telegram",
  /** split around the TELEGRAM_BOT_TOKEN code tag */
  noBot: ["Sign-in goes through Telegram, and this install has no bot yet. Set ", " in .env and restart the server."] as [string, string],
  newHere: "New with Telegram? Just continue: the account is made for you.",
  denied: "Denied in Telegram. Nobody was signed in.",
  expired: "The request expired.",
  pressStart: (bot: string) => `Telegram should have opened. Press Start in @${bot}, then Approve.`,
  tapApprove: "Now tap Approve in Telegram.",
  continueIn: "Continue in Telegram",
  startAgain: "Start again",
  openBot: (bot: string) => `Open @${bot}`,
};

export default en;
