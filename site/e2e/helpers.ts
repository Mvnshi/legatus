import { expect, type Locator, type Page } from "@playwright/test";

/** The demo desktop's windows, found by their accessible names. */
export const cockpit = (page: Page) => page.getByRole("group", { name: /Legatus Cockpit/ });
export const loginsWindow = (page: Page) => page.getByRole("group", { name: "Logins", exact: true });
export const guideWindow = (page: Page) => page.getByRole("group", { name: "Getting started" });
export const taskbar = (page: Page) => page.getByRole("group", { name: "Taskbar" });
export const startButton = (page: Page) => taskbar(page).getByRole("button", { name: "Start", exact: true });

/** Makes WebGL unavailable, so the optional wallpaper shader never starts. Keeps timer-driven tests fast. */
export async function withoutWebGL(page: Page) {
  await page.addInitScript(() => {
    const real = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (this: HTMLCanvasElement, type: string, ...rest: unknown[]) {
      if (type === "webgl" || type === "webgl2" || type === "experimental-webgl") return null;
      return (real as (...a: unknown[]) => unknown).call(this, type, ...rest);
    } as typeof real;
  });
}

/**
 * CI machines fall back to wider fonts than a developer machine does. LEGATUS_WIDE_FONTS=1 reproduces that
 * locally by forcing DejaVu Sans, which is what most Linux runners use.
 */
async function withWideFonts(page: Page) {
  await page.addInitScript(() => {
    const style = document.createElement("style");
    style.textContent = ':root{--font-ui:"DejaVu Sans",sans-serif;--font-title:"DejaVu Sans",sans-serif}';
    document.addEventListener("DOMContentLoaded", () => document.head.append(style));
  });
}

export async function openSite(page: Page, options: { shader?: boolean } = {}) {
  if (!options.shader) await withoutWebGL(page);
  if (process.env.LEGATUS_WIDE_FONTS) await withWideFonts(page);
  await page.goto("./");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(/Your agents\.\s*One command center\./);
  // The desktop measures itself before it paints; wait until the cockpit is there.
  await expect(cockpit(page)).toBeVisible();
}

/** The run toolbar button, by its visible label (the cockpit has a lot of buttons). */
export const control = (page: Page, name: string): Locator =>
  cockpit(page).getByRole("group", { name: "Run controls" }).getByRole("button", { name, exact: true });

/** Advance the page's clock so the scripted run moves along without real waiting. */
export async function fastForward(page: Page, ms: number) {
  // Step in small slices: the demo advances by what each 50 ms tick reports.
  for (let spent = 0; spent < ms; spent += 500) {
    await page.clock.runFor(Math.min(500, ms - spent));
  }
}

export async function boxOf(locator: Locator) {
  const box = await locator.boundingBox();
  if (!box) throw new Error("element has no box");
  return box;
}
