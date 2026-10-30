import { ruPlural } from "../index";
import type en from "../en/fresh";

const ru: typeof en = {
  live: "В эфире",
  paused: "На паузе",
  pause: "Пауза",
  resume: "Продолжить",
  checked: (ago) => `проверено ${ago} назад`,
  arrived: (n) => `${n} ${ruPlural(n, "новое имя", "новых имени", "новых имён")} с открытия страницы`,
  filters: "Фильтры",
  reset: "Сбросить",
  venue: "Где",
  venues: { auction: "Аукцион Fragment", sale: "Продажа Fragment", getgems: "Getgems", marketapp: "Marketapp", market: "Другие площадки", owned: "Не продаётся" },
  kind: "Тип",
  anyKind: "Любой тип",
  length: "Длина",
  price: "Цена, TON",
  buyout: "Выкуп до, TON",
  from: "от",
  to: "до",
  score: "Оценка AI",
  anyScore: "Любая",
  unscored: "Включая ещё без оценки",
  buyNow: "Только можно купить сейчас",
  buyNowHint: "Имена с ценой, которую можно заплатить прямо сейчас: листинг или «купить сразу» на аукционе",
  hideJunk: "Скрыть мусор",
  cols: { age: "Найден", name: "Имя", kind: "Тип", len: "Длина", score: "Оценка", venue: "Где", price: "Цена" },
  buyNowAt: (v) => `купить сразу ${v}`,
  watch: (name) => `Следить за @${name}`,
  watching: "Отслеживается",
  empty: "Под эти фильтры пока ничего нового. Новые имена появятся здесь, как только сборщики их найдут.",
  loadFailed: (why) => `Не удалось загрузить ленту: ${why}`,
  age: { s: (n) => `${n} с`, m: (n) => `${n} мин`, h: (n) => `${n} ч`, d: (n) => `${n} дн.` },
  cap: (n) => `В ленте остаются ${n} самых новых имён.`,
};

export default ru;
