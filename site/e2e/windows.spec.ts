import { expect, test } from "@playwright/test";
import { boxOf, cockpit, guideWindow, loginsWindow, openSite, taskbar } from "./helpers";

test.use({ viewport: { width: 1440, height: 900 } });

test.describe("windows on the desktop", () => {
  test("minimize hides a window, its taskbar button restores it, and the button toggles", async ({ page }) => {
    await openSite(page);
    const button = taskbar(page).getByRole("button", { name: /^Cockpit/ });
    await expect(button).toHaveAttribute("aria-pressed", "true");

    await cockpit(page).getByRole("button", { name: "Minimize" }).click();
    await expect(cockpit(page)).toBeHidden();
    await expect(button).toHaveAttribute("aria-pressed", "false");
    await expect(button).toContainText("(minimized)");

    await button.click();
    await expect(cockpit(page)).toBeVisible();
    await expect(button).toHaveAttribute("aria-pressed", "true");

    // Clicking the active window's button minimizes it again.
    await button.click();
    await expect(cockpit(page)).toBeHidden();
  });

  test("a minimized window leaves nothing behind that could block clicks", async ({ page }) => {
    await openSite(page);
    const where = await boxOf(cockpit(page));
    await cockpit(page).getByRole("button", { name: "Minimize" }).click();
    await expect(cockpit(page)).toBeHidden();
    const hit = await page.evaluate(
      ([x, y]) => document.elementFromPoint(x!, y!)?.closest("[data-window]")?.getAttribute("data-window") ?? null,
      [where.x + where.width / 2, where.y + where.height / 2],
    );
    expect(hit).not.toBe("cockpit");
  });

  test("close removes the window from the taskbar and the desktop icon opens it again", async ({ page }) => {
    await openSite(page);
    await guideWindow(page).getByRole("button", { name: "Close" }).click();
    await expect(guideWindow(page)).toBeHidden();
    await expect(taskbar(page).getByRole("button", { name: /Getting started/ })).toHaveCount(0);
    await page.getByRole("button", { name: "Getting started" }).click();
    await expect(guideWindow(page)).toBeVisible();
    await expect(taskbar(page).getByRole("button", { name: /Getting started/ })).toBeVisible();
  });

  test("clicking a window brings it to the front", async ({ page }) => {
    await openSite(page);
    const zOf = (id: string) =>
      page.evaluate((i) => Number(getComputedStyle(document.getElementById(`win-${i}`)!).zIndex), id);
    expect(await zOf("cockpit")).toBeGreaterThan(await zOf("logins"));
    await page.locator("#win-logins .xp-win__body").click({ position: { x: 5, y: 5 } });
    expect(await zOf("logins")).toBeGreaterThan(await zOf("cockpit"));
  });

  test("maximize fills the desktop and Restore puts the window back", async ({ page }) => {
    await openSite(page);
    const before = await boxOf(cockpit(page));
    await cockpit(page).getByRole("button", { name: "Maximize" }).click();
    const stage = await boxOf(page.locator(".desktop"));
    // The window animates to its new size and position; wait for it to arrive instead of sampling mid-flight.
    await expect.poll(async () => Math.round((await boxOf(cockpit(page))).x)).toBe(Math.round(stage.x));
    await expect.poll(async () => Math.round((await boxOf(cockpit(page))).width)).toBe(Math.round(stage.width));
    await cockpit(page).getByRole("button", { name: "Restore" }).click();
    await expect.poll(async () => Math.round((await boxOf(cockpit(page))).width)).toBe(Math.round(before.width));
  });

  test("dragging by the title bar moves a window exactly as far as the pointer goes", async ({ page }) => {
    await openSite(page);
    const win = loginsWindow(page);
    const start = await boxOf(win);
    const grab = { x: start.x + 120, y: start.y + 16 };
    await page.mouse.move(grab.x, grab.y);
    await page.mouse.down();
    await page.mouse.move(grab.x + 100, grab.y + 60, { steps: 6 });
    await page.mouse.up();
    const moved = await boxOf(win);
    expect(Math.round(moved.x - start.x)).toBe(100);
    expect(Math.round(moved.y - start.y)).toBe(60);
    // Dragging also brings the window to the front.
    await expect(taskbar(page).getByRole("button", { name: /Logins/ })).toHaveAttribute("aria-pressed", "true");
  });

  test("a window can never be dragged out of the desktop, past any edge or onto the taskbar", async ({ page }) => {
    await openSite(page);
    const stage = await boxOf(page.locator(".desktop"));
    const win = cockpit(page);
    for (const [dx, dy] of [
      [-3000, -3000],
      [3000, 3000],
      [-3000, 3000],
      [3000, -3000],
    ] as const) {
      const now = await boxOf(win);
      const from = { x: now.x + 200, y: now.y + 16 };
      await page.mouse.move(from.x, from.y);
      await page.mouse.down();
      await page.mouse.move(Math.min(Math.max(from.x + dx, 1), 1439), Math.min(Math.max(from.y + dy, 1), 899), { steps: 8 });
      await page.mouse.up();
      const box = await boxOf(win);
      expect(box.x, "left edge").toBeGreaterThanOrEqual(stage.x - 1);
      expect(box.y, "top edge").toBeGreaterThanOrEqual(stage.y - 1);
      expect(box.x + box.width, "right edge").toBeLessThanOrEqual(stage.x + stage.width + 1);
      // The taskbar is 44px tall and a window may never cover it.
      expect(box.y + box.height, "bottom edge").toBeLessThanOrEqual(stage.y + stage.height - 44 + 1);
    }
  });

  test("a window can be moved from the keyboard, and the arrow keys stop at the desktop edge", async ({ page }) => {
    await openSite(page);
    const win = loginsWindow(page);
    const title = win.getByLabel(/title bar/);
    const before = await boxOf(win);
    await title.focus();
    await page.keyboard.press("ArrowRight");
    await page.keyboard.press("ArrowRight");
    await page.keyboard.press("ArrowDown");
    await expect.poll(async () => Math.round((await boxOf(win)).x - before.x)).toBe(32);
    await expect.poll(async () => Math.round((await boxOf(win)).y - before.y)).toBe(16);
    await page.keyboard.press("Shift+ArrowUp");
    await page.keyboard.press("Shift+ArrowUp");
    const stage = await boxOf(page.locator(".desktop"));
    await expect.poll(async () => Math.round((await boxOf(win)).y)).toBeGreaterThanOrEqual(Math.round(stage.y));
  });

  test("desktop icons open windows and the Install icon goes to the download section", async ({ page }) => {
    await openSite(page);
    await loginsWindow(page).getByRole("button", { name: "Close" }).click();
    await page.getByRole("button", { name: "Logins" }).first().click();
    await expect(loginsWindow(page)).toBeVisible();
    await page.getByRole("button", { name: "evidence.md" }).click();
    await expect(page.getByRole("group", { name: /evidence\.md/ })).toBeVisible();
    await page.locator(".deskicon", { hasText: "Install Legatus" }).click();
    await expect(page).toHaveURL(/#install$/);
  });
});
