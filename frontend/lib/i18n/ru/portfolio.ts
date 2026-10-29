import { ruPlural } from "../index";
import type en from "../en/portfolio";

const ru: typeof en = {
  removed: (name) => `@${name} удалён из портфеля`,
  saved: (name) => `@${name} сохранён`,
  names: "Юзернеймы",
  listedForResale: (n) => `${n} ${ruPlural(n, "выставлен", "выставлено", "выставлено")} на перепродажу`,
  paid: "Потрачено",
  paidSub: "сумма известных цен покупки",
  typicalNow: "Типичная цена",
  unpriced: (n) => `${n} ${ruPlural(n, "юзернейм", "юзернейма", "юзернеймов")} пока без типичной цены`,
  typicalSub: "за сколько продаются юзернеймы с такой оценкой",
  difference: "Разница",
  differenceSub: "типичная цена минус то, что вы заплатили",
  yourNames: "Ваши юзернеймы",
  yourNamesSub: "Купленные через приложение или добавленные вручную. К каждому можно привязать проект.",
  add: "Добавить юзернейм",
  empty: "Здесь пока пусто. Добавьте свои юзернеймы и цены покупки — типичная цена для их оценки обновляется по мере новых продаж.",
  noConcept: "Проект не привязан",
  typical: "Типичная",
  diff: "Разница",
  edit: "Изменить",
  openFragment: "Открыть на Fragment",
  remove: (name) => `Удалить @${name}`,
  username: "Юзернейм",
  paidTon: "Цена, TON",
  concept: "Проект",
  conceptPlaceholder: "Coffee Club — сообщество любителей кофе",
  note: "Заметка",
  optional: "необязательно",
};

export default ru;
