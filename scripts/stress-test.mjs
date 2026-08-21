// High-intensity user simulation against a Beszel release hub.
// Usage: node stress-test.mjs <baseUrl> [systemName]
// Exercises: cold load, nav cycles, dialogs, filter, theme/view, logout/login,
// chart time ranges, new hardware info bar, alert sheet. Collects console
// errors and interaction latencies throughout.
import { chromium } from "playwright-core";

const BASE = process.argv[2] || "http://127.0.0.1:18090";
const SYS = process.argv[3] || "wsl-self";
const EMAIL = process.argv[4] || "EMAIL";
const PASSWORD = process.argv[5] || "PASSWORD";
const browser = await chromium.launch({ channel: "chrome", headless: false, args: ["--start-minimized"] });
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
page.setDefaultTimeout(12000);

const errors = [];
page.on("console", (m) => { if (m.type() === "error") errors.push(m.text().slice(0, 200)); });
page.on("pageerror", (e) => errors.push("PAGEERROR: " + String(e).slice(0, 200)));

const results = {};
const t = async (label, fn) => {
  const t0 = Date.now();
  try { await fn(); results[label] = Date.now() - t0; }
  catch (e) { results[label] = "FAIL: " + String(e).slice(0, 100); }
};

// 1) cold load: goto -> table rendered
await t("coldLoad", async () => {
  await page.goto(BASE + "/", { waitUntil: "domcontentloaded" });
  await page.locator('a[href^="/system/"]').first().waitFor({ state: "visible" });
});

// login if needed (AUTO_LOGIN instances skip)
if (await page.getByRole("textbox", { name: /电子邮件|email/i }).count() === 1) {
  await page.getByRole("textbox", { name: /电子邮件|email/i }).fill(EMAIL);
  await page.getByRole("textbox", { name: /密码|password/i }).fill(PASSWORD);
  await page.getByRole("button", { name: /登录|login|sign in/i }).click();
  await page.locator('a[href^="/system/"]').first().waitFor({ state: "visible" });
}

const sysLink = page.locator("a[href^='/system/']", { hasText: SYS }).first();
const combo = page.locator("button[role='combobox']").first();
const home = page.locator('a[href="/"]').first();

// 2) warm nav x8
await t("nav8", async () => {
  for (let i = 0; i < 8; i++) {
    await sysLink.click();
    await page.waitForURL(BASE + "/system/**");
    await combo.waitFor({ state: "visible" });
    await home.click();
    await page.waitForURL(BASE + "/");
    await sysLink.waitFor({ state: "visible" });
  }
});

// 3) chart time range switches on detail page
await sysLink.click();
await page.waitForURL(BASE + "/system/**");
await combo.waitFor({ state: "visible" });
const ranges = ["1 分钟", "12 小时", "1 周", "1 小时"];
for (const r of ranges) {
  await t("range:" + r, async () => {
    await combo.click();
    await page.getByRole("option", { name: r }).click();
    await page.waitForTimeout(400);
    await page.locator("h3").first().waitFor({ state: "visible" });
  });
}

// 4) hardware info bar present (new fields)
await t("hwInfoBar", async () => {
  const barText = await page.locator("h1").first().locator("xpath=following-sibling::div").first().innerText();
  results.hwBarSample = barText.replace(/\n/g, " | ").slice(0, 260);
});

// 5) alert sheet: go home first, open row alert button, find GpuMemoryFree card
await home.click();
await page.waitForURL(BASE + "/");
await t("alertSheet", async () => {
  const bell = page.getByRole("button", { name: /^警报$|^alert$/i }).first();
  await bell.click();
  const card = page.getByText(/Free GPU Memory|剩余显存|空闲显存/i).first();
  await card.waitFor({ state: "visible", timeout: 4000 });
  results.gpuFreeAlertCard = "visible";
  await page.keyboard.press("Escape");
  await page.waitForTimeout(300);
});

// 6) filter typing / backspace / X
await home.click();
await page.waitForURL(BASE + "/");
await t("filterCycle", async () => {
  const box = page.getByRole("textbox", { name: /过滤|filter/i });
  await box.click();
  await box.type(SYS.slice(0, 4));
  await page.waitForTimeout(300);
  for (let i = 0; i < 4; i++) await box.press("Backspace");
  await page.waitForTimeout(300);
  await box.type(SYS.slice(0, 3));
  await page.waitForTimeout(300);
  await page.getByRole("button", { name: /清除|clear/i }).click();
  await page.waitForTimeout(300);
});

// 7) dialogs: add-system open/close x3
await t("addDialog3", async () => {
  for (let i = 0; i < 3; i++) {
    await page.getByRole("button", { name: /添加 系统|add system/i }).click();
    await page.getByRole("dialog").first().waitFor({ state: "visible" });
    await page.getByRole("button", { name: "Close" }).click();
    await page.getByRole("dialog").first().waitFor({ state: "hidden" });
  }
});

// 8) theme toggle x4
await t("theme4", async () => {
  const btn = page.getByRole("button", { name: /切换主题|toggle theme/i });
  for (let i = 0; i < 4; i++) { await btn.click(); await page.waitForTimeout(120); }
});

// 9) logout/login x3
await t("logoutLogin3", async () => {
  for (let i = 0; i < 3; i++) {
    await page.getByRole("button", { name: /user actions/i }).click();
    await page.getByRole("menuitem", { name: /登出|log ?out/i }).click();
    await page.waitForTimeout(800);
    await page.getByRole("textbox", { name: /电子邮件|email/i }).fill(EMAIL);
    await page.getByRole("textbox", { name: /密码|password/i }).fill(PASSWORD);
    await page.getByRole("button", { name: /登录|login|sign in/i }).click();
    await page.locator('a[href^="/system/"]').first().waitFor({ state: "visible" });
  }
});

results.consoleErrors = errors.length ? errors.slice(0, 5) : "none";
console.log(JSON.stringify(results, null, 1));
await browser.close();
