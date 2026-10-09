import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { cockpit, control, fastForward, guideWindow, openSite, startButton, taskbar } from "./helpers";

async function axe(page: Page, label: string) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"]).analyze();
  const summary = results.violations.map((v) => `${v.id} (${v.impact}): ${v.nodes.slice(0, 3).map((n) => n.target.join(" ")).join(" | ")}`);
  expect(summary, `axe violations ${label}`).toEqual([]);
}

test.describe("accessibility (axe, WCAG 2.2 AA)", () => {
  test("desktop: the page at rest", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSite(page);
    await page.getByRole("tab", { name: "Automations" }).scrollIntoViewIfNeeded();
    await axe(page, "desktop at rest");
  });

  test("desktop: after the run has finished, with the report open", async ({ page }) => {
    await page.clock.install({ time: new Date("2026-10-09T09:41:00Z") });
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSite(page);
    await control(page, "Start").click();
    await fastForward(page, 30_000);
    await cockpit(page).getByRole("button", { name: "Open the report" }).click();
    await cockpit(page).getByRole("tab", { name: "Review", exact: true }).click();
    // Scan with the desktop in a clean position, not half-hidden under the sticky header.
    await page.evaluate(() => window.scrollTo(0, document.querySelector(".demo-section__frame")!.getBoundingClientRect().top + scrollY - 90));
    await axe(page, "desktop finished");
  });

  test("desktop: with the Start menu open", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSite(page);
    await startButton(page).click();
    await expect(page.getByRole("menu").first()).toBeVisible();
    await axe(page, "start menu open");
  });

  test("phone: the page at rest", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openSite(page);
    await axe(page, "phone");
  });
});

test.describe("keyboard", () => {
  test("every control on the desktop can be reached with Tab, in a sensible order, and focus is always visible", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSite(page);
    await page.locator("#demo").scrollIntoViewIfNeeded();
    await page.getByRole("link", { name: "Skip the demo desktop" }).focus();
    const seen: string[] = [];
    for (let i = 0; i < 60; i++) {
      await page.keyboard.press("Tab");
      const info = await page.evaluate(() => {
        const el = document.activeElement as HTMLElement | null;
        if (!el || el === document.body) return null;
        const style = getComputedStyle(el);
        const visibleRing = style.outlineStyle !== "none" && parseFloat(style.outlineWidth) >= 2;
        const name = el.getAttribute("aria-label") ?? el.textContent?.trim().slice(0, 40) ?? "";
        return { name, ring: visibleRing, inWin: Boolean(el.closest("[data-window]")), tag: el.tagName };
      });
      if (!info) continue;
      seen.push(info.name);
      if (info.tag !== "CODE") expect(info.ring, `focus ring on "${info.name}"`).toBe(true);
      if (seen.includes("Install Legatus") && seen.length > 30) break;
    }
    // The main controls were all visited.
    for (const wanted of ["Minimize", "Close", "Start", "Pause", "Replay", "Inject limit"]) {
      expect(seen.some((s) => s.includes(wanted)), `Tab never reached "${wanted}"`).toBe(true);
    }
  });

  test("the run can be played with the keyboard alone", async ({ page }) => {
    await page.clock.install({ time: new Date("2026-10-09T09:41:00Z") });
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSite(page);
    await control(page, "Start").focus();
    await page.keyboard.press("Enter");
    await fastForward(page, 3000);
    await control(page, "Inject limit").focus();
    await page.keyboard.press("Space");
    await fastForward(page, 30_000);
    await expect(cockpit(page).getByTestId("run-status")).toHaveText("Ready for your review");
  });

  test("minimize and restore work from the keyboard and focus follows", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSite(page);
    await page.locator("#win-guide").getByRole("button", { name: "Minimize" }).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("#win-guide")).toBeHidden();
    await expect(guideWindow(page)).toHaveCount(0);
    const button = taskbar(page).getByRole("button", { name: /Getting started/ });
    await button.focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("#win-guide")).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.activeElement?.id)).toBe("win-guide");
  });
});

test.describe("reduced motion", () => {
  test.use({ reducedMotion: "reduce" });

  test("no shader, no looping animation, instant window changes", async ({ page }) => {
    const chunks: string[] = [];
    page.on("request", (r) => {
      if (/WallpaperShader/.test(r.url())) chunks.push(r.url());
    });
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSite(page);
    await page.waitForTimeout(1500);
    expect(chunks).toEqual([]);
    await expect(page.locator(".wallpaper__shader")).toHaveCount(0);
    const animated = await page.evaluate(() =>
      document.getAnimations().filter((a) => a instanceof CSSAnimation && (a.effect?.getComputedTiming().iterations ?? 1) === Infinity).length,
    );
    expect(animated, "infinite CSS animations while reduced motion is on").toBe(0);
    const smooth = await page.evaluate(() => getComputedStyle(document.documentElement).scrollBehavior);
    expect(smooth).toBe("auto");

    // Minimize is immediate, with no flight to the taskbar.
    await cockpit(page).getByRole("button", { name: "Minimize" }).click();
    await expect(cockpit(page)).toBeHidden({ timeout: 800 });
  });

  test("the boot screen is a short static card", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSite(page);
    await startButton(page).click();
    await page.getByRole("menuitem", { name: /Replay boot animation/ }).click();
    const bar = page.locator(".boot__bar span");
    await expect(bar).toHaveClass(/is-still/);
    await expect(page.getByRole("dialog", { name: /Starting/ })).toBeHidden({ timeout: 3000 });
  });
});
