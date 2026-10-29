import type en from "../en/auth";

const ru: typeof en = {
  telegramFailed: (why) => `Не удалось войти через Telegram: ${why}`,
  backendDown: (why) => `Сервер не отвечает: ${why}`,
};

export default ru;
