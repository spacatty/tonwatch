// Screenshots for the user guides, from the running local app, with headless Chrome over CDP.
// Run from the repo: GUIDE_SESSION=<fw_session token of a demo account> node docs/guides/shoot.mjs [scene…]
// Needs the frontend (GUIDE_URL, default http://localhost:3002) and backend up. Scenes that ask the AI spend credits.
import { spawn } from "node:child_process";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, "img");
const base = process.env.GUIDE_URL ?? "http://localhost:3002";
const session = process.env.GUIDE_SESSION;
const chrome = process.env.CHROME ?? "C:/Program Files/Google/Chrome/Application/chrome.exe";
const W = 1440, H = 900;
if (!session) throw new Error("set GUIDE_SESSION to a demo account's fw_session token");

// --- a bare CDP client ---

const profile = join(tmpdir(), "guide-shoot-" + process.pid);
const proc = spawn(chrome, ["--headless=new", "--remote-debugging-port=0", `--user-data-dir=${profile}`, "--hide-scrollbars", `--window-size=${W},${H}`, "about:blank"], { stdio: ["ignore", "ignore", "pipe"] });
const wsUrl = await new Promise((ok, fail) => {
  let buf = "";
  proc.stderr.on("data", (d) => {
    buf += d;
    const m = buf.match(/ws:\/\/\S+/);
    if (m) ok(m[0]);
  });
  proc.on("exit", () => fail(new Error("chrome exited: " + buf)));
});
const port = new URL(wsUrl).port;
const target = (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()).find((t) => t.type === "page");
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((ok) => ws.addEventListener("open", ok, { once: true }));
let seq = 0;
const pending = new Map();
ws.addEventListener("message", (e) => {
  const m = JSON.parse(e.data);
  if (m.id && pending.has(m.id)) {
    const { ok, fail } = pending.get(m.id);
    pending.delete(m.id);
    m.error ? fail(new Error(m.error.message)) : ok(m.result);
  }
});
const cdp = (method, params = {}) =>
  new Promise((ok, fail) => {
    const id = ++seq;
    pending.set(id, { ok, fail });
    ws.send(JSON.stringify({ id, method, params }));
  });

// --- page helpers ---

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function js(expr) {
  const r = await cdp("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true });
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text);
  return r.result.value;
}
/** Wait until fn (a JS expression) is truthy. */
async function until(expr, ms = 20000) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    if (await js(expr).catch(() => false)) return;
    await sleep(250);
  }
  throw new Error("timed out waiting for: " + expr);
}
// textContent, not innerText: labels are uppercased by CSS
const hasText = (t) => `document.body.textContent.includes(${JSON.stringify(t)})`;
/** Click the innermost visible button/link/tab whose text includes t. */
async function click(t, { exact = false } = {}) {
  await until(`!!(${findEl})(${JSON.stringify(t)}, ${exact})`);
  await js(`void (${findEl})(${JSON.stringify(t)}, ${exact}).click()`);
  await sleep(600);
}
const findEl = (t, exact) =>
  [...document.querySelectorAll("button, a, [role=tab], [role=menuitem]")]
    .filter((e) => e.offsetParent !== null)
    .find((e) => (exact ? e.innerText.trim() === t : e.innerText.includes(t)) || e.getAttribute("aria-label") === t);
async function type(selector, text) {
  await until(`!!document.querySelector(${JSON.stringify(selector)})`);
  await js(`void document.querySelector(${JSON.stringify(selector)}).focus()`);
  await cdp("Input.insertText", { text });
  await sleep(300);
}
async function home() {
  await cdp("Page.navigate", { url: base + "/" });
  await until(`document.readyState === "complete" && ${hasText("TonWatch")}`);
  // the Next.js dev badge sits on top of the profile corner
  await js(`void document.head.insertAdjacentHTML("beforeend", "<style>nextjs-portal{display:none!important}</style>")`);
  await sleep(2500);
}
/** Screenshot the viewport, or a clip {x, y, width, height}, or the element matched by a JS expression. */
async function shot(name, { clip, el, pad = 16 } = {}) {
  if (el) {
    // bring its top into view; anything taller than the window is cut at the window's bottom
    const r = await js(`(() => { const e = ${el}; const b0 = e.getBoundingClientRect(); if (b0.top < 0 || b0.bottom > innerHeight) { e.scrollIntoView({block: "start"}); scrollBy(0, -${pad}); } const b = e.getBoundingClientRect(); return {x: b.x, y: b.y, width: b.width, height: b.height}; })()`);
    await sleep(600);
    const y = Math.max(0, r.y - pad);
    clip = { x: Math.max(0, r.x - pad), y, width: Math.min(W, r.width + pad * 2), height: Math.min(H - y, r.height + pad * 2) };
  }
  const { data } = await cdp("Page.captureScreenshot", { format: "png", captureBeyondViewport: false, ...(clip && { clip: { ...clip, scale: 1 } }) });
  writeFileSync(join(out, name + ".png"), Buffer.from(data, "base64"));
  console.log("  img/" + name + ".png");
}

// --- scenes: one per screenshot the guides use ---

const scenes = {
  async "snipes-home"() {
    await home();
    await shot("snipes-home");
  },
  async "snipes-cards"() {
    await home();
    await shot("snipes-cards", { el: `[...document.querySelectorAll("section")].find(s => s.innerText.includes("Выгодные предложения"))`, pad: 12 });
  },
  async "snipes-settings"() {
    await home();
    await click("Изменить");
    await until(hasText("Никогда не предлагать"));
    await sleep(500);
    await shot("snipes-settings", { el: `document.querySelector("[role=dialog]")`, pad: 0 });
  },
  async "snipes-settings-ai"() {
    await home();
    await click("Изменить");
    await until(hasText("Никогда не предлагать"));
    // sample answers, typed but never saved
    const areas = `document.querySelectorAll("[role=dialog] textarea")`;
    await js(`void ${areas}[0].focus()`);
    await cdp("Input.insertText", { text: "Делаю ботов и мини-приложения для авторов в Telegram, ищу короткие имена под новые проекты" });
    await js(`void ${areas}[1].focus()`);
    await cdp("Input.insertText", { text: "Казино, ставки, трейдинг, всё про финансы и крипту" });
    await js(`void ${areas}[1].blur()`);
    await js(`void [...document.querySelectorAll("[role=dialog] section")].find(s => s.innerText.includes("Никогда не предлагать")).scrollIntoView({block: "end"})`);
    await sleep(500);
    await shot("snipes-settings-ai", { el: `document.querySelector("[role=dialog]")`, pad: 0 });
  },
  async "ai-filter"() {
    await home();
    await click("Фильтр", { exact: true });
    await type("textarea", "Короткие настоящие слова, которые можно купить сразу до 50 TON");
    await click("Собрать фильтр");
    await until(hasText("Сохранить как трекер"), 90000);
    await sleep(2500);
    await shot("ai-filter");
  },
  async "ask-question"() {
    await home();
    await click("Консультация");
    await until(hasText("Для чего нужен юзернейм?"));
    await type("textarea", "Кофейня спешелти в Берлине, до 100 TON");
    await shot("ask-question");
  },
  async "ask-answer"() {
    await home();
    await click("Консультация");
    await sleep(1500);
    // the conversation is kept, so ask only when there is no answer yet
    if (!(await js(hasText("Все совпадения")))) {
      await type("textarea", "Кофейня спешелти в Берлине, до 100 TON");
      await click("Спросить", { exact: true });
      await until(hasText("Все совпадения"), 180000);
    }
    await sleep(1500);
    // start at the request itself, not the bottom of the list
    await js(`void [...document.querySelectorAll("main *")].find(e => e.children.length === 0 && e.textContent.includes("Кофейня спешелти в Берлине, до 100 TON")).scrollIntoView({block: "start"})`);
    await js(`void [...document.querySelectorAll("*")].filter(e => e.scrollHeight > e.clientHeight + 50 && getComputedStyle(e).overflowY !== "visible").forEach(e => e.scrollTop -= 24)`);
    await sleep(800);
    await shot("ask-answer");
  },
  async "ai-filter-prompt"() {
    await home();
    await click("Фильтр", { exact: true });
    await until(hasText("Готовые фразы"));
    await type("textarea", "Аукционы без ставок хотя бы 5 дней, до 30 TON, красивые или крипто");
    await sleep(500);
    await shot("ai-filter-prompt");
  },
  async "ideas-taste"() {
    await home();
    await click("Идеи", { exact: true });
    await until(hasText("Ваш вкус"));
    if (!(await js(hasText("Показать другие")))) await click("Ваш вкус");
    await until(`!!document.querySelector('[aria-label^="Нравится: @"]')`);
    // a couple of likes, so the summary line has something to say
    for (let i = 0; i < 2; i++) {
      await js(`void document.querySelector('[aria-label^="Нравится: @"]').click()`);
      await sleep(900);
    }
    await sleep(1000);
    await shot("ideas-taste", { el: `[...document.querySelectorAll("section.panel")].find(s => s.innerText.includes("Ваш вкус"))`, pad: 12 });
  },
  async "ideas-studio"() {
    await home();
    await click("Идеи", { exact: true });
    await until(hasText("Проекты, которые можно начать"));
    // the taste panel remembers being open; fold it so the ideas show
    if (await js(hasText("Показать другие"))) await click("Ваш вкус");
    if (!(await js(hasText("Первый шаг:")))) {
      await click("Придумать идеи");
      await until(hasText("Первый шаг:"), 180000);
    }
    await sleep(1500);
    await shot("ideas-studio");
  },
  async "ideas-card"() {
    await scenes["ideas-studio"]();
    await shot("ideas-card", { el: `[...document.querySelectorAll("article")].find(e => e.textContent.includes("Первый шаг:"))`, pad: 12 });
  },
  async "idea-chat"() {
    await scenes["ideas-studio"]();
    await js(`void [...document.querySelectorAll("article")][0].querySelector("button[title^='Спросите']").click()`);
    await until(`!!document.querySelector("[role=dialog] textarea")`);
    await sleep(800);
    // the chat is kept with the idea, so ask only once
    if (await js(`[...document.querySelectorAll("[role=dialog] button")].some(b => b.textContent.includes("Как это сделать, по шагам?"))`)) {
      await click("Как это сделать, по шагам?");
      await until(`!document.querySelector("[role=dialog]").textContent.includes("Думаю…") && document.querySelector("[role=dialog]").textContent.length > 1200`, 180000);
    }
    await sleep(1500);
    // from the question down, not the tail of the answer
    await js(`void [...document.querySelectorAll("[role=dialog] *")].filter(e => e.scrollHeight > e.clientHeight + 50).forEach(e => e.scrollTop = 0)`);
    await sleep(500);
    await shot("idea-chat", { el: `document.querySelector("[role=dialog]")`, pad: 0 });
  },
};

// --- run ---

try {
  mkdirSync(out, { recursive: true });
  await cdp("Page.enable");
  await cdp("Runtime.enable");
  await cdp("Emulation.setDeviceMetricsOverride", { width: W, height: H, deviceScaleFactor: 1, mobile: false });
  await cdp("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: "dark" }] });
  await cdp("Network.enable");
  await cdp("Network.setCookie", { name: "fw_session", value: session, url: base });
  await cdp("Page.navigate", { url: base + "/" });
  await sleep(1500);
  await js(`localStorage.setItem("tonwatch.lang", "ru"); localStorage.setItem("theme", "dark")`);
  const pick = process.argv.slice(2);
  for (const name of pick.length ? pick : Object.keys(scenes)) {
    try {
      await scenes[name]();
    } catch (e) {
      console.error(`  ${name}: ${e.message}`);
    }
  }
} finally {
  ws.close();
  proc.kill();
  await sleep(500);
  rmSync(profile, { recursive: true, force: true });
}
