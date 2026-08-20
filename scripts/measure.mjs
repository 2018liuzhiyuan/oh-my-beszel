// Measure system-detail entry latency with real hover/click against the WSL hub.
// Run: node measure.mjs
import { chromium } from "playwright-core";

const BASE = "http://127.0.0.1:18090";
const SYSTEM_HREF_PREFIX = "/system/";
const ROW_NAME = "wsl-self";

const browser = await chromium.launch({ channel: "chrome", headless: false, args: ["--start-minimized"] });
const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
page.setDefaultTimeout(15000);

await page.goto(BASE + "/", { waitUntil: "domcontentloaded" });
await page.waitForSelector("text=所有客户端", { timeout: 15000 });
await page.waitForTimeout(2500); // let first data tick arrive

const rowLink = page.locator(`a[href^="${SYSTEM_HREF_PREFIX}"]`, { hasText: ROW_NAME }).first();
await rowLink.waitFor({ state: "visible" });
// selector-only (no options object) to be compatible with the locator wait path
const comboVisible = page.locator("button[role='combobox']").first();

const runOnce = async (hoverMs, label) => {
  // exit to home first
  await page.locator('a[href="/"]').first().click();
  await page.waitForURL(BASE + "/");
  await page.waitForTimeout(400);

  // hover the row/table body to fire onPointerEnter preload, wait, then click
  await rowLink.hover();
  if (hoverMs > 0) await page.waitForTimeout(hoverMs);

  const t0 = Date.now();
  await rowLink.click();
  const urlMs = await (async () => {
    while (Date.now() - t0 < 8000) {
      if (page.url().startsWith(BASE + SYSTEM_HREF_PREFIX)) return Date.now() - t0;
      await page.waitForTimeout(20);
    }
    return -1;
  })();
  let skeletonMs = -1;
  try {
    await comboVisible.waitFor({ state: "visible", timeout: 8000 });
    skeletonMs = Date.now() - t0;
  } catch {}
  return { label, hoverMs, urlMs, skeletonMs };
};

const results = [];
// cold click (no hover dwell)
results.push(await runOnce(0, "cold-click"));
// warm clicks with hover dwell 100 / 200 / 400 / 800 ms
for (const h of [100, 200, 400, 800]) results.push(await runOnce(h, `hover-${h}ms`));

console.log(JSON.stringify(results, null, 1));
await browser.close();
