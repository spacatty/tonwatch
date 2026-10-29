"use client";

import { useSyncExternalStore } from "react";
import { en, type Dict } from "./en";
import { ru } from "./ru";

// The interface speaks English or Russian. The words live in ./en and ./ru, one file per screen;
// ru is typed against en, so a key missing from either side fails the build. Components read the
// words with useT() (t.sidebar.trackers); code outside React reads them with getT().

export type Lang = "en" | "ru";
export type { Dict };

export const LANGS: { id: Lang; label: string; short: string }[] = [
  { id: "en", label: "English", short: "EN" },
  { id: "ru", label: "Русский", short: "RU" },
];

const dicts: Record<Lang, Dict> = { en, ru };
const KEY = "tonwatch.lang";

/** "ru", "ru-RU" → ru; anything else → en. */
export function langOf(code: string | null | undefined): Lang | null {
  if (!code) return null;
  const c = code.toLowerCase();
  return c === "ru" || c.startsWith("ru-") ? "ru" : c === "en" || c.startsWith("en-") ? "en" : null;
}

function saved(): Lang | null {
  try {
    return langOf(localStorage.getItem(KEY));
  } catch {
    return null;
  }
}

/** The Telegram language of the person who opened the Mini App, read from the launch data in the URL. */
function telegramLang(): Lang | null {
  try {
    const data = new URLSearchParams(window.location.hash.slice(1)).get("tgWebAppData");
    const user = data && new URLSearchParams(data).get("user");
    return user ? langOf(JSON.parse(user).language_code) : null;
  } catch {
    return null;
  }
}

function detect(): Lang {
  return saved() ?? telegramLang() ?? (navigator.languages ?? [navigator.language]).map(langOf).find(Boolean) ?? "en";
}

let current: Lang | null = null;
const subs = new Set<() => void>();
const listeners = new Set<(l: Lang) => void>();

export function getLang(): Lang {
  if (current == null) {
    if (typeof window === "undefined") return "en";
    current = detect();
    document.documentElement.lang = current;
  }
  return current;
}

/** Switches the language. `explicit` is a choice the person made; it is kept on this device. */
export function setLang(l: Lang, explicit = true) {
  if (explicit) {
    try {
      localStorage.setItem(KEY, l);
    } catch {}
  }
  if (l === getLang()) return;
  current = l;
  document.documentElement.lang = l;
  subs.forEach((f) => f());
  if (explicit) listeners.forEach((f) => f(l));
}

/** The account's language from the server: used when this device has not picked one. */
export function adoptLang(l: string | undefined) {
  const v = langOf(l);
  if (v && !saved()) setLang(v, false);
}

/** Runs when the person picks a language (the account remembers it for the bot). */
export function onLangPicked(f: (l: Lang) => void) {
  listeners.add(f);
  return () => {
    listeners.delete(f);
  };
}

const subscribe = (f: () => void) => {
  subs.add(f);
  return () => {
    subs.delete(f);
  };
};

export function useLang(): Lang {
  return useSyncExternalStore(subscribe, getLang, () => "en");
}

export function useT(): Dict {
  return dicts[useLang()];
}

/** The words for the current language, outside React (toasts, callbacks, plain functions). */
export function getT(): Dict {
  return dicts[getLang()];
}

/** Russian plural: 1 имя, 2 имени, 5 имён (21 имя, 11 имён). */
export function ruPlural(n: number, one: string, few: string, many: string): string {
  const a = Math.abs(n) % 100;
  const b = a % 10;
  if (a > 10 && a < 20) return many;
  if (b === 1) return one;
  if (b >= 2 && b <= 4) return few;
  return many;
}

/** English plural: 1 name, 2 names. */
export const enPlural = (n: number, one: string, many: string) => (n === 1 ? one : many);

/** A count as the reader writes numbers: 12,345 in English, 12 345 in Russian. */
export const num = (n: number, digits = 0) => n.toLocaleString(getLang() === "ru" ? "ru-RU" : "en-US", { maximumFractionDigits: digits });

/** The locale for Intl formatting: dates, prices and counts follow the interface language. */
export const locale = () => (getLang() === "ru" ? "ru-RU" : "en-US");
