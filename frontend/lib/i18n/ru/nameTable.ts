import type en from "../en/nameTable";
import { ruPlural } from "../index";

// «Оценка ИИ» → «оценка ИИ»: в русском нельзя опускать регистр целиком, аббревиатуры остаются заглавными
const lowerFirst = (s: string) => s.charAt(0).toLowerCase() + s.slice(1);

const ru: typeof en = {
  names: (n) => ruPlural(n, "юзернейм", "юзернейма", "юзернеймов"),
  byDefault: (col) => `сортировка: ${lowerFirst(col)}`,
  by: "сортировка:",
  flip: "Сменить направление",
  stopSorting: (col) => `Убрать сортировку «${col}»`,
  bestValueHint: "Самые дешёвые для своей оценки ИИ: цена в сравнении с тем, за сколько обычно уходят юзернеймы с такой же оценкой",
  bestValue: "Выгоднее всего",
  colUsername: "Юзернейм",
  colScore: "Оценка",
  colEnds: "Закрытие",
  colPrice: "Выкуп",
  colStatus: "Статус",
  bid: (price) => `ставка ${price} TON`,
  bidUsd: (usd) => `ставка ≈ $${usd}`,
  lastSale: "последняя продажа",
  verb: { buy: "Купить", offer: "Предложить цену", buyNow: "Моментальная продажа", bid: "Ставка", open: "Открыть" },
  on: (verb, market) => `${verb} на ${market}`,
  theMarketplace: "маркетплейсе",
  tracked: "В избранном",
  looksJunk: "похоже на мусор",
  notRated: "ещё без оценки",
  scoreOf: (score) => `Оценка ИИ: ${score} из 10`,
  track: "В избранное",
  stopTracking: "Убрать из избранного",
  likeHint: "Нравится: так мы узнаём ваш вкус",
  like: "Нравится",
  imagineHint: "Придумать проект под юзернейм · 2 кредита",
  imagine: "Придумать проект",
  bringBack: "Вернуть",
  neverShow: "Больше не показывать",
  under: (pct, score) => `На ${pct}% дешевле, чем обычно продаются юзернеймы с оценкой ${score}`,
  sold: (price) => `продан за ${price}`,
  more: "Ещё",
  recheck: "Перепроверить на Fragment",
  copied: (name) => `@${name} скопирован`,
  copy: (name) => `Скопировать @${name}`,
  inPortfolio: (name) => `@${name} теперь в вашем портфеле`,
  iOwnIt: "Это мой",
  openGetgems: "Открыть на Getgems",
  sortNext: { ascending: "Сортировать по возрастанию", descending: "Сортировать по убыванию", off: "Без сортировки" },
  showOnly: "Показывать только",
  showAllStatuses: "Все статусы",
};

export default ru;
