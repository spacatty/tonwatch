import type en from "../en/bits";

const ru: typeof en = {
  closing: "завершается",
  left: { d: (d, h) => `${d} дн. ${h} ч`, h: (h, m) => `${h} ч ${m} мин`, s: (s) => `${s} с`, m: (m, s) => `${m} мин ${s} с` },
  junk: "мусор",
  scoreOf: (s) => `Оценка ИИ: ${s} из 10`,
  status: { auction: "Аукцион", sale: "Продаётся", market: "Маркетплейс", sold: "Продан", owned: "Есть владелец" },
  deal: (pct, score, median) => `На ${pct}% дешевле обычной цены юзернеймов с оценкой ${score} и похожей длиной (${median} TON)`,
  never: "никогда",
  ago: { s: (n) => `${n} с назад`, m: (n) => `${n} мин назад`, h: (n) => `${n} ч назад`, d: (n) => `${n} дн. назад` },
};

export default ru;
