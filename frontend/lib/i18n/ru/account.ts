import type en from "../en/account";

const ru: typeof en = {
  title: "Аккаунт",
  plan: (pro) => (pro ? "Тариф Pro" : "Бесплатный тариф"),
  admin: " · администратор",
  signedInWith: "Вход через Telegram",
  language: "Язык",
  languageHint: "На нём будут интерфейс, ответы ИИ и сообщения бота в Telegram.",
  signedOutOthers: "Вы вышли на всех других устройствах",
  signOutOthers: "Выйти на других устройствах",
  signOut: "Выйти",
};

export default ru;
