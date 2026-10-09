import { expect, test } from "@playwright/test";
import { cockpit, control, fastForward, guideWindow, loginsWindow, openSite, taskbar } from "./helpers";

test.describe("the simulated demo", () => {
  // The run is driven by timers, so these tests install a fake clock before the page loads and move it by hand.
  test.beforeEach(async ({ page }) => {
    await page.clock.install({ time: new Date("2026-10-09T09:41:00Z") });
  });

  test("is labelled as a simulation and starts queued", async ({ page }) => {
    await openSite(page);
    await expect(page.getByTestId("sim-badge")).toHaveText(/Simulated demo/);
    await expect(cockpit(page)).toContainText("Simulated demo");
    await expect(cockpit(page).getByTestId("run-status")).toHaveText("Queued, not started");
    await expect(control(page, "Start")).toBeEnabled();
    await expect(control(page, "Pause")).toHaveAttribute("aria-disabled", "true");
    await expect(control(page, "Replay")).toHaveAttribute("aria-disabled", "true");
    await expect(control(page, "Inject limit")).toHaveAttribute("aria-disabled", "true");
  });

  test("plays through an injected limit, a checkpoint, a continuation, checks and review to 'Ready for your review'", async ({
    page,
  }) => {
    await openSite(page);
    await control(page, "Start").click();
    await expect(cockpit(page).getByTestId("run-status")).not.toHaveText("Queued, not started");

    // work-1 is working; cut it off.
    await fastForward(page, 3000);
    await expect(control(page, "Inject limit")).not.toHaveAttribute("aria-disabled", "true");
    await control(page, "Inject limit").click();
    await expect(control(page, "Inject limit")).toHaveAttribute("aria-disabled", "true");

    await fastForward(page, 1500);
    await expect(loginsWindow(page).locator('[data-login="work-1"]')).toHaveAttribute("data-status", "limited");
    await expect(page.getByRole("status").filter({ hasText: "work-1 reached its usage limit" })).toBeVisible();

    await fastForward(page, 1500);
    await expect(cockpit(page).getByTestId("checkpoint")).toContainText("legatus: implement (interrupted by a usage limit)");

    await fastForward(page, 4000);
    await expect(loginsWindow(page).locator('[data-login="work-2"]')).toHaveAttribute("data-status", /working|ready/);

    await fastForward(page, 15_000);
    await expect(cockpit(page).getByTestId("run-status")).toHaveText("Ready for your review");
    await expect(cockpit(page).getByTestId("ready")).toContainText("Ready for your review");
    await expect(page.getByRole("status").filter({ hasText: "Ready for your review" })).toBeVisible();

    await cockpit(page).getByRole("tab", { name: "Review", exact: true }).click();
    await expect(cockpit(page).getByTestId("verdict")).toContainText("Approved by second-opinion");
    await cockpit(page).getByRole("tab", { name: "Checks", exact: true }).click();
    await expect(cockpit(page).getByRole("tabpanel")).toContainText("2 command(s) passed");

    // The report is the same story, in the shape of Legatus's evidence.md.
    await cockpit(page).getByRole("tab", { name: "Overview", exact: true }).click();
    await cockpit(page).getByRole("button", { name: "Open the report" }).click();
    const report = page.getByRole("group", { name: /evidence\.md/ });
    await expect(report).toBeVisible();
    await expect(report).toContainText("succeeded");
    await expect(report).toContainText("work-1` (codex) hit its usage limit during `implement`");
    await expect(report).toContainText("Nothing was pushed or published");
  });

  test("the limit also arrives on its own, and the run still finishes", async ({ page }) => {
    await openSite(page);
    await control(page, "Start").click();
    await fastForward(page, 6500);
    await expect(loginsWindow(page).locator('[data-login="work-1"]')).toHaveAttribute("data-status", "limited");
    await fastForward(page, 20_000);
    await expect(cockpit(page).getByTestId("run-status")).toHaveText("Ready for your review");
  });

  test("pause freezes the run and Resume carries on", async ({ page }) => {
    await openSite(page);
    await control(page, "Start").click();
    await fastForward(page, 3000);
    await control(page, "Pause").click();
    await expect(cockpit(page).getByTestId("run-status")).toHaveText("Paused");
    await cockpit(page).getByRole("tab", { name: "Journal", exact: true }).click();
    const lines = cockpit(page).getByRole("log", { name: "Run journal" }).locator("li");
    const before = await lines.count();
    await fastForward(page, 10_000);
    expect(await lines.count()).toBe(before);
    await expect(control(page, "Resume")).toBeVisible();
    await control(page, "Resume").click();
    await fastForward(page, 3000);
    expect(await lines.count()).toBeGreaterThan(before);
  });

  test("replay starts the run again from the beginning", async ({ page }) => {
    await openSite(page);
    await control(page, "Start").click();
    await fastForward(page, 30_000);
    await expect(cockpit(page).getByTestId("run-status")).toHaveText("Ready for your review");
    await control(page, "Replay").click();
    await expect(cockpit(page).getByTestId("run-status")).not.toHaveText("Ready for your review");
    await expect(cockpit(page).getByTestId("ready")).toHaveCount(0);
    await expect(loginsWindow(page).locator('[data-login="work-1"]')).toHaveAttribute("data-status", "ready");
    await fastForward(page, 30_000);
    await expect(cockpit(page).getByTestId("run-status")).toHaveText("Ready for your review");
  });

  test("the hero's 'Try the demo' scrolls to the desktop and starts the run", async ({ page }) => {
    await openSite(page);
    await page.getByRole("link", { name: "Try the demo" }).click();
    await expect(page).toHaveURL(/#demo$/);
    await expect(cockpit(page).getByTestId("run-status")).not.toHaveText("Queued, not started");
  });

  test("the Getting started checklist follows what happens", async ({ page }) => {
    await openSite(page);
    const items = guideWindow(page).locator("li[data-done]");
    await expect(items.nth(0)).toHaveAttribute("data-done", "false");
    await control(page, "Start").click();
    await expect(items.nth(0)).toHaveAttribute("data-done", "true");
    await fastForward(page, 7000);
    await expect(items.nth(1)).toHaveAttribute("data-done", "true");
    await fastForward(page, 20_000);
    await expect(items.nth(2)).toHaveAttribute("data-done", "true");
    await expect(items.nth(3)).toHaveAttribute("data-done", "false");
  });
});

test.describe("cockpit menus and tabs", () => {
  test("the menu bar works: View switches tabs, Run controls the run", async ({ page }) => {
    await openSite(page);
    await cockpit(page).getByRole("menuitem", { name: "View" }).click();
    await page.getByRole("menuitemradio", { name: "Journal" }).click();
    await expect(cockpit(page).getByRole("tab", { name: "Journal", exact: true })).toHaveAttribute("aria-selected", "true");
    await expect(cockpit(page).getByRole("log", { name: "Run journal" })).toBeVisible();

    await cockpit(page).getByRole("menuitem", { name: "Run" }).click();
    await page.getByRole("menuitem", { name: "Start" }).click();
    await expect(cockpit(page).getByTestId("run-status")).not.toHaveText("Queued, not started");

    await cockpit(page).getByRole("menuitem", { name: "Help" }).click();
    await page.getByRole("menuitem", { name: "What is verified" }).click();
    await expect(page.getByRole("group", { name: "Verification notes" })).toBeVisible();
  });

  test("tabs switch with the mouse and with arrow keys", async ({ page }) => {
    await openSite(page);
    const tabs = cockpit(page).getByRole("tablist", { name: "Run details" });
    await tabs.getByRole("tab", { name: "Checks", exact: true }).click();
    await expect(cockpit(page).getByRole("tabpanel")).toContainText("A failing check goes back to the agent");
    await tabs.getByRole("tab", { name: "Checks", exact: true }).focus();
    await page.keyboard.press("ArrowRight");
    await expect(tabs.getByRole("tab", { name: "Review", exact: true })).toHaveAttribute("aria-selected", "true");
    await page.keyboard.press("Home");
    await expect(tabs.getByRole("tab", { name: "Overview", exact: true })).toHaveAttribute("aria-selected", "true");
  });

  test("a disabled-looking control explains itself in a tooltip", async ({ page }) => {
    await openSite(page);
    await control(page, "Inject limit").hover();
    await expect(page.getByRole("tooltip")).toContainText("Start the run first");
  });
});

test.describe("the Start menu", () => {
  test("opens, lists real destinations, and 'Try the demo' starts the run", async ({ page }) => {
    await openSite(page);
    const start = taskbar(page).getByRole("button", { name: "Start", exact: true });
    await start.click();
    const menu = page.getByRole("menu").first();
    await expect(menu).toBeVisible();
    await expect(menu.getByRole("menuitem", { name: /Download Legatus/ })).toHaveAttribute("href", "#install");
    await expect(menu.getByRole("menuitem", { name: /Source on GitHub/ })).toHaveAttribute(
      "href",
      "https://github.com/Mvnshi/legatus",
    );
    await menu.getByRole("menuitem", { name: /Try the demo/ }).click();
    await expect(menu).toBeHidden();
    await expect(cockpit(page).getByTestId("run-status")).not.toHaveText("Queued, not started");
  });

  test("Escape closes it and returns focus to the Start button", async ({ page }) => {
    await openSite(page);
    const start = taskbar(page).getByRole("button", { name: "Start", exact: true });
    await start.click();
    await expect(page.getByRole("menu").first()).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("menu")).toHaveCount(0);
    await expect(start).toBeFocused();
  });

  test("keyboard: open with Enter, move with arrows, open the All windows submenu", async ({ page }) => {
    await openSite(page);
    const start = taskbar(page).getByRole("button", { name: "Start", exact: true });
    await start.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("menu").first()).toBeVisible();
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("ArrowDown");
    const sub = page.getByRole("menuitem", { name: /All windows/ });
    await sub.focus();
    await page.keyboard.press("ArrowRight");
    await expect(page.getByRole("menuitem", { name: "evidence.md" })).toBeVisible();
    await page.getByRole("menuitem", { name: "evidence.md" }).press("Enter");
    await expect(page.getByRole("group", { name: /evidence\.md/ })).toBeVisible();
    await expect(taskbar(page).getByRole("button", { name: /evidence\.md/ })).toBeVisible();
  });

  test("the boot animation is optional: only the menu starts it, and it can be skipped", async ({ page }) => {
    await openSite(page);
    await expect(page.getByRole("dialog", { name: /Starting the Legatus desktop/ })).toHaveCount(0);
    await taskbar(page).getByRole("button", { name: "Start", exact: true }).click();
    await page.getByRole("menuitem", { name: /Replay boot animation/ }).click();
    const boot = page.getByRole("dialog", { name: /Starting the Legatus desktop/ });
    await expect(boot).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(boot).toBeHidden();
    await expect(cockpit(page)).toBeVisible();
  });
});

test.describe("sound", () => {
  test.beforeEach(async ({ page }) => {
    await page.clock.install({ time: new Date("2026-10-09T09:41:00Z") });
  });

  test("starts muted and the toggle is a real switch", async ({ page }) => {
    await page.addInitScript(() => {
      (window as unknown as { __audio: number }).__audio = 0;
      const Real = window.AudioContext;
      window.AudioContext = class extends Real {
        constructor(...args: ConstructorParameters<typeof Real>) {
          super(...args);
          (window as unknown as { __audio: number }).__audio += 1;
        }
      } as typeof Real;
    });
    await openSite(page);
    const toggle = taskbar(page).getByRole("button", { name: "Sound effects" });
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await control(page, "Start").click();
    await fastForward(page, 8000);
    expect(await page.evaluate(() => (window as unknown as { __audio: number }).__audio)).toBe(0);
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(await page.evaluate(() => (window as unknown as { __audio: number }).__audio)).toBeGreaterThan(0);
  });
});
