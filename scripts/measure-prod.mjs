// Measure heavy-system (8 GPUs) detail page + exit latency on the PRODUCTION hub.
import { readFile } from "node:fs/promises";
import { chromium } from "playwright-core";

const BASE = process.env.BESZEL_MEASURE_BASE_URL ?? "http://127.0.0.1:8090";
const targetsFile = process.env.BESZEL_MEASURE_TARGETS_FILE ?? new URL("measure-targets.local.json", import.meta.url);
const targets = JSON.parse(await readFile(targetsFile, "utf8"));
if (
  !Array.isArray(targets) ||
  targets.length === 0 ||
  targets.some(
    (target) =>
      typeof target !== "object" ||
      target === null ||
      typeof target.name !== "string" ||
      typeof target.href !== "string" ||
      !target.href.startsWith("/system/")
  )
) {
  throw new Error("Measurement targets must be a non-empty JSON array of { name, href } objects.");
}
const browser = await chromium.launch({ channel: "chrome", headless: false, args: ["--start-minimized"] });
const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
page.setDefaultTimeout(15000);

await page.goto(BASE + "/", { waitUntil: "domcontentloaded" });
await page.waitForSelector("text=所有客户端", { timeout: 15000 });
await page.waitForTimeout(3000);

const comboVisible = page.locator("button[role='combobox']").first();
const results = [];

for (const t of targets) {
  const link = page.locator(`a[href="${t.href}"]`).first();
  await link.waitFor({ state: "visible" });
  for (let i = 1; i <= 3; i++) {
    await link.hover();
    await page.waitForTimeout(200);
    const t0 = Date.now();
    await link.click();
    await page.waitForURL(BASE + t.href, { timeout: 8000 });
    const urlMs = Date.now() - t0;
    let skel = -1;
    try { await comboVisible.waitFor({ state: "visible", timeout: 8000 }); skel = Date.now() - t0; } catch {}
    // exit timing
    const e0 = Date.now();
    await page.locator('a[href="/"]').first().click();
    await page.waitForURL(BASE + "/", { timeout: 8000 });
    const exitMs = Date.now() - e0;
    results.push({ sys: t.name, iter: i, enterUrlMs: urlMs, skeletonMs: skel, exitMs });
    await page.waitForTimeout(400);
  }
}
console.log(JSON.stringify(results, null, 1));
await browser.close();
