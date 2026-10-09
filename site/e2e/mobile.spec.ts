import { expect, test, devices } from "@playwright/test";
import { cockpit, control, fastForward, openSite, startButton } from "./helpers";

test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true, userAgent: devices["iPhone 13"].userAgent });

test.beforeEach(async ({ page }) => {
  await page.clock.install({ time: new Date("2026-10-09T09:41:00Z") });
});

test.describe("phone layout", () => {
  test("never scrolls sideways", async ({ page }) => {
    await openSite(page);
    for (const y of [0, 700, 1500, 2600, 3600, 5200, 7000]) {
      await page.evaluate((top) => window.scrollTo(0, top), y);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow, `horizontal overflow at y=${y}`).toBeLessThanOrEqual(0);
    }
  });

  test("windows are stacked in one column, fully visible, and cannot be dragged", async ({ page }) => {
    await openSite(page);
    await expect(page.locator(".desktop")).toHaveAttribute("data-mode", "stacked");
    const boxes = await page.locator("[data-window]").evaluateAll((els) =>
      els.map((el) => {
        const r = el.getBoundingClientRect();
        return { id: el.getAttribute("data-window"), x: r.x, w: r.width, y: r.y + scrollY, h: r.height };
      }),
    );
    expect(boxes.map((b) => b.id)).toEqual(["cockpit", "logins", "guide"]);
    for (const b of boxes) {
      expect(b.x).toBeGreaterThanOrEqual(0);
      expect(b.x + b.w).toBeLessThanOrEqual(390);
      expect(b.w).toBeGreaterThan(300);
    }
    for (let i = 1; i < boxes.length; i++) expect(boxes[i]!.y).toBeGreaterThanOrEqual(boxes[i - 1]!.y + boxes[i - 1]!.h - 1);
    await expect(page.locator("[data-drag-handle]")).toHaveCount(0);
  });

  test("controls are comfortable to touch", async ({ page }) => {
    await openSite(page);
    const small: string[] = [];
    const check = async (label: string, loc: ReturnType<typeof page.locator>) => {
      for (const el of await loc.all()) {
        const box = await el.boundingBox();
        if (box && (box.height < 36 || box.width < 36)) small.push(`${label}: ${Math.round(box.width)}x${Math.round(box.height)}`);
      }
    };
    await check("title bar button", page.locator("#win-cockpit .title-bar-controls button"));
    await check("toolbar", cockpit(page).getByRole("group", { name: "Run controls" }).getByRole("button"));
    await check("tab", cockpit(page).getByRole("tab"));
    await check("taskbar", page.getByRole("group", { name: "Open windows" }).getByRole("button"));
    await check("start", startButton(page));
    await check("nav", page.getByRole("navigation", { name: "Main" }).getByRole("link"));
    await check("desktop icon", page.locator(".deskicon"));
    expect(small).toEqual([]);
  });

  test("the whole header navigation fits on screen", async ({ page }) => {
    await openSite(page);
    const links = page.getByRole("navigation", { name: "Main" }).getByRole("link");
    expect(await links.count()).toBe(5);
    for (const link of await links.all()) await expect(link).toBeInViewport({ ratio: 1 });
  });

  test("minimize folds a window to its title bar and the taskbar button unfolds it", async ({ page }) => {
    await openSite(page);
    const win = page.locator("#win-logins");
    const open = (await win.boundingBox())!.height;
    await win.getByRole("button", { name: "Minimize" }).tap();
    await expect.poll(async () => (await win.boundingBox())!.height).toBeLessThan(open / 2);
    await expect(win.getByRole("button", { name: "Close" })).toBeVisible();
    await page.getByRole("group", { name: "Open windows" }).getByRole("button", { name: /Logins/ }).tap();
    await expect.poll(async () => (await win.boundingBox())!.height).toBeGreaterThan(open - 4);
  });

  test("the whole demo can be played by touch, down to 'Ready for your review'", async ({ page }) => {
    await openSite(page);
    await control(page, "Start").tap();
    await fastForward(page, 3000);
    await control(page, "Inject limit").tap();
    await fastForward(page, 30_000);
    await expect(cockpit(page).getByTestId("run-status")).toHaveText("Ready for your review");
    await expect(page.getByRole("status").filter({ hasText: "Ready for your review" })).toBeVisible();
    await cockpit(page).getByRole("button", { name: "Open the report" }).tap();
    await expect(page.getByRole("group", { name: /evidence\.md/ })).toBeVisible();
  });

  test("the Start menu opens above the taskbar and fits the screen", async ({ page }) => {
    await openSite(page);
    await startButton(page).tap();
    const menu = page.getByRole("menu").first();
    await expect(menu).toBeVisible();
    const box = (await menu.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(390);
    expect(box.y).toBeGreaterThanOrEqual(0);
  });

  test("the install dialog and release facts stack and stay readable", async ({ page }) => {
    await openSite(page);
    const dialog = page.getByRole("group", { name: "Install Legatus" });
    await dialog.scrollIntoViewIfNeeded();
    const d = (await dialog.boundingBox())!;
    const r = (await page.getByRole("group", { name: "Release information" }).boundingBox())!;
    expect(r.y).toBeGreaterThan(d.y + d.height - 1);
    expect(d.width).toBeGreaterThan(340);
    const font = await dialog.locator(".xp-install__hint").first().evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    expect(font).toBeGreaterThanOrEqual(15);
  });
});
