"use client";

// The slice of Telegram's Mini App API (telegram-web-app.js) the app uses.
// https://core.telegram.org/bots/webapps#initializing-mini-apps
export interface TgWebApp {
  initData: string;
  colorScheme: "light" | "dark";
  platform: string;
  ready(): void;
  expand(): void;
  openLink(url: string): void;
  openTelegramLink(url: string): void;
  openInvoice(url: string, cb?: (status: "paid" | "cancelled" | "failed" | "pending") => void): void;
  requestWriteAccess?(cb?: (granted: boolean) => void): void;
  onEvent(event: "themeChanged", cb: () => void): void;
}

declare global {
  interface Window {
    Telegram?: { WebApp?: TgWebApp };
  }
}

/** True when this page was opened as a Mini App: Telegram puts the launch data in the URL hash. */
export const launchedInTelegram = () =>
  typeof window !== "undefined" && (/tgWebApp(Data|Platform)/.test(window.location.hash) || !!window.Telegram?.WebApp?.initData);

let loading: Promise<TgWebApp | null> | null = null;

/** Loads telegram-web-app.js (only inside Telegram) and resolves to the Mini App, or null. */
export function loadMiniApp(): Promise<TgWebApp | null> {
  if (!launchedInTelegram()) return Promise.resolve(null);
  loading ??= new Promise((resolve) => {
    const done = () => resolve(window.Telegram?.WebApp?.initData ? window.Telegram.WebApp : null);
    if (window.Telegram?.WebApp) return done();
    const s = document.createElement("script");
    s.src = "https://telegram.org/js/telegram-web-app.js";
    s.onload = done;
    s.onerror = () => resolve(null);
    document.head.appendChild(s);
  });
  return loading;
}

/** The Mini App if it is already loaded (synchronous; for click handlers). */
export const miniApp = (): TgWebApp | null => (window.Telegram?.WebApp?.initData ? window.Telegram.WebApp : null);

/** Opens a link the right way: inside Telegram, t.me links stay in Telegram and others open outside. */
export function openExternal(url: string) {
  const tg = miniApp();
  if (tg) {
    if (url.startsWith("https://t.me/")) tg.openTelegramLink(url);
    else tg.openLink(url);
    return;
  }
  window.open(url, "_blank", "noopener");
}
