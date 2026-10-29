import en from "./en/errors";
import { getLang, getT } from "./index";

// The API answers errors in English. This turns the ones it knows into the reader's language;
// anything else (a network error, a message from Telegram) passes through as it is.
const fixed = new Map(Object.entries(en).flatMap(([k, v]) => (typeof v === "string" ? [[v, k]] : [])));

export function serverError(msg: string): string {
  if (getLang() === "en") return msg;
  const t = getT();
  const k = fixed.get(msg) as keyof typeof t.errors | undefined;
  if (k) return t.errors[k] as string;
  let m = msg.match(/^the bot cannot write to you yet: open @(\w+) in Telegram/);
  if (m) return t.errors.botCantWrite(m[1]);
  m = msg.match(/^this pack is not sold for (.+)$/);
  if (m) return t.errors.packNotSold(m[1]);
  m = msg.match(/^not enough credits: (\w+) costs (\d+)$/);
  if (m) return t.errors.noCredits(t.common.features[m[1] as keyof typeof t.common.features] ?? m[1], +m[2]);
  return msg;
}
