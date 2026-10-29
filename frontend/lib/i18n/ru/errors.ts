import type en from "../en/errors";

const ru: typeof en = {
  notFound: "Не найдено",
  invalidUsername: "Такого юзернейма быть не может",
  signInFirst: "Сначала войдите",
  adminsOnly: "Только для администраторов",
  tooManyAttempts: "Слишком много попыток. Подождите минуту",
  botNotConfigured: "Telegram-бот не настроен",
  accountNotFound: "Аккаунт не найден",
  selfAdmin: "Нельзя снять права администратора с самого себя",
  grantRange: "Начислить можно не больше ±1 000 000",
  topicRequired: "Укажите тему",
  briefRequired: "Опишите, что вам нужно",
  noTelegram: "Telegram не привязан",
  unknownPack: "Неизвестный пакет или способ оплаты",
  questionRequired: "Напишите вопрос",
  unknownConcept: "Такой идеи нет",
  nameRequired: "Укажите юзернейм",
  unknownName: "Такого юзернейма нет в базе",
  kindRequired: "Нужны юзернейм и известный тип отметки",
  paramsRequired: "Нужны название и параметры",
  namesRequired: "Укажите юзернеймы",
  tooFewNames: "В ваши настройки попадает слишком мало юзернеймов. Поднимите бюджет или снизьте минимальную оценку",
  nothingToRotate: "Заменять нечего: сначала получите идеи",
  tooFewUnused: "Неиспользованных юзернеймов почти не осталось. Получите новую подборку",
  modelEmpty: "Модель не дала ничего путного. Попробуйте ещё раз",
  botCantWrite: (bot) => `Бот пока не может вам писать: откройте @${bot} в Telegram и нажмите «Запустить»`,
  packNotSold: (gateway) => `Этот пакет нельзя купить через ${gateway}`,
  noCredits: (feature, cost) => `Не хватает кредитов: «${feature}» стоит ${cost}`,
};

export default ru;
