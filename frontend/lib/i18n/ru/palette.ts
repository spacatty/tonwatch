import type en from "../en/palette";

const ru: typeof en = {
  title: "Перейти к юзернейму",
  description: "Поиск юзернеймов, переход между разделами, запуск сборщиков",
  placeholder: "Введите юзернейм, например darkmoon",
  notInDb: "Этого юзернейма пока нет в базе.",
  tooShort: "Введите хотя бы две буквы.",
  usernames: "Юзернеймы",
  add: "Добавить",
  track: (name) => `Проверить @${name} на Fragment и отслеживать`,
  goTo: "Разделы",
  data: "Данные",
  openCollectors: "Открыть сборщики",
};

export default ru;
