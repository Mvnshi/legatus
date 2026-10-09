import { expect, test, type Page } from "@playwright/test";
import { openSite } from "./helpers";

test.use({ viewport: { width: 1440, height: 900 } });

async function webglWorks(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const c = document.createElement("canvas");
    return Boolean(c.getContext("webgl2") ?? c.getContext("webgl"));
  });
}

test.describe("deployment path and first view", () => {
  test("is served from /legatus/ and every asset URL stays under it", async ({ page }) => {
    const bad: string[] = [];
    page.on("response", (r) => {
      const url = new URL(r.url());
      if (url.pathname.startsWith("/legatus/")) return;
      bad.push(`${r.status()} ${url.pathname}`);
    });
    const response = await page.goto("./");
    expect(response?.status()).toBe(200);
    expect(new URL(page.url()).pathname).toBe("/legatus/");
    await openSite(page);
    await page.getByRole("tab", { name: "All runs" }).first().scrollIntoViewIfNeeded();
    expect(bad, "requests outside /legatus/").toEqual([]);
    const refs = await page.evaluate(() =>
      [...document.querySelectorAll<HTMLElement>("link[rel=stylesheet], link[rel=icon], link[rel=modulepreload], script[src], img[src]")].map(
        (n) => n.getAttribute("href") ?? n.getAttribute("src") ?? "",
      ),
    );
    for (const ref of refs) expect(ref.startsWith("/legatus/") || ref.startsWith("data:"), ref).toBe(true);
  });

  test("makes no request to any other host", async ({ page }) => {
    const hosts = new Set<string>();
    page.on("request", (r) => hosts.add(new URL(r.url()).host));
    await openSite(page);
    await page.waitForLoadState("networkidle");
    expect([...hosts]).toEqual(["127.0.0.1:4173"]);
  });

  test("shows the headline, explanation and both actions without any interaction, and no boot screen", async ({ page }) => {
    await page.goto("./");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.getByText("Queue coding tasks, isolate every change, and review the checks and diff in one local workspace.")).toBeVisible();
    await expect(page.getByRole("link", { name: "Try the demo" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Download Legatus" })).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    const inFirstScreen = await page.getByRole("link", { name: "Try the demo" }).evaluate((el) => el.getBoundingClientRect().bottom < innerHeight);
    expect(inFirstScreen).toBe(true);
  });

  test("the pre-paint headline is in the HTML before any script runs", async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled: false });
    const page = await context.newPage();
    await page.goto("http://127.0.0.1:4173/legatus/");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Your agents. One command center.");
    await expect(page.getByRole("link", { name: "Try the demo" })).toHaveAttribute("href", "#demo");
    await expect(page.getByRole("link", { name: "Download Legatus" })).toHaveAttribute("href", "#install");
    await context.close();
  });
});

test.describe("navigation outside the desktop", () => {
  test("the header stays on screen and every link lands on its section", async ({ page }) => {
    await openSite(page);
    const nav = page.getByRole("navigation", { name: "Main" });
    for (const [name, id] of [
      ["Demo", "demo"],
      ["How it works", "how"],
      ["What is verified", "status"],
      ["Install", "install"],
    ] as const) {
      await nav.getByRole("link", { name }).click();
      await expect(page).toHaveURL(new RegExp(`#${id}$`));
      await expect(page.locator(`#${id}`)).toBeInViewport();
      await expect(nav).toBeInViewport({ ratio: 1 });
    }
    await expect(nav.getByRole("link", { name: /GitHub/ })).toHaveAttribute("href", "https://github.com/Mvnshi/legatus");
  });

  test("the skip links work", async ({ page }) => {
    await openSite(page);
    await page.keyboard.press("Tab");
    const skip = page.getByRole("link", { name: "Skip to content" });
    await expect(skip).toBeFocused();
    await expect(skip).toBeInViewport();
    const inline = page.getByRole("link", { name: "Skip the demo desktop" });
    await inline.focus();
    await expect(inline).toBeInViewport();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/#how$/);
  });
});

test.describe("content", () => {
  test("How it works: each step is a tab with its own explanation", async ({ page }) => {
    await openSite(page);
    const steps = page.getByRole("tablist", { name: "Steps of a run" });
    await expect(steps.getByRole("tab")).toHaveCount(5);
    await steps.getByRole("tab", { name: /Keep going at a limit/ }).click();
    await expect(page.getByRole("tabpanel").filter({ hasText: "another login continues from there" })).toBeVisible();
    await steps.getByRole("tab", { name: /Keep going at a limit/ }).press("ArrowDown");
    await expect(steps.getByRole("tab", { name: /Check and review/ })).toHaveAttribute("aria-selected", "true");
  });

  test("What is verified: leads with what is not verified and matches STATUS.md", async ({ page }) => {
    await openSite(page);
    const groups = page.getByRole("tablist", { name: "Verification groups" });
    await expect(groups.getByRole("tab").first()).toHaveAttribute("aria-selected", "true");
    await expect(groups.getByRole("tab").first()).toContainText("Not verified yet");
    const panel = page.getByRole("tabpanel").filter({ hasText: "Claude Code." });
    await expect(panel).toContainText("never been run against a real install");
    await expect(panel).toContainText("set aside for 30 minutes");
    await groups.getByRole("tab", { name: /Run for real/ }).click();
    await expect(page.getByRole("tabpanel").filter({ hasText: "Codex CLI 0.162" })).toBeVisible();
  });

  test("The real cockpit: four screenshots, each with alt text, loaded only when opened", async ({ page }) => {
    await openSite(page);
    const tabs = page.getByRole("tablist", { name: "Cockpit screens" });
    await expect(tabs.getByRole("tab")).toHaveCount(4);
    const img = page.getByRole("img", { name: /first login hit its usage limit/ });
    await img.scrollIntoViewIfNeeded();
    await expect(img).toBeVisible();
    await expect.poll(() => img.evaluate((i: HTMLImageElement) => i.complete && i.naturalWidth)).toBeGreaterThan(1000);
    await tabs.getByRole("tab", { name: "Logins" }).click();
    await expect(page.getByRole("img", { name: /work-1 \(codex\) at its limit/ })).toBeVisible();
  });

  test("Install: release facts beside the dialog, the right file for the chosen system, real links", async ({ page }) => {
    await openSite(page);
    const dialog = page.getByRole("group", { name: "Install Legatus" });
    const release = page.getByRole("group", { name: "Release information" });
    await dialog.scrollIntoViewIfNeeded();
    await expect(release).toContainText("v0.1.0");
    await expect(release).toContainText("Pre-release");
    await expect(release).toContainText("October 9, 2026");
    await expect(release).toContainText("GitHub Actions");

    await dialog.getByRole("radio", { name: "Windows" }).click();
    await dialog.getByRole("radio", { name: "x64 (Intel or AMD)" }).click();
    const download = dialog.getByRole("link", { name: /Download for Windows x64/ });
    await expect(download).toHaveAttribute(
      "href",
      "https://github.com/Mvnshi/legatus/releases/download/v0.1.0/legatus_v0.1.0_windows_amd64.zip",
    );
    await expect(dialog).toContainText("9197c96aab9cf35e1f92ed520bad155bf560faf827efa5f880874ca7cb773c44");
    await expect(dialog).toContainText("Get-FileHash");

    await dialog.getByRole("radio", { name: "macOS" }).click();
    await dialog.getByRole("radio", { name: "ARM64" }).click();
    await expect(dialog.getByRole("link", { name: /Download for macOS ARM64/ })).toHaveAttribute(
      "href",
      /legatus_v0\.1\.0_darwin_arm64\.tar\.gz$/,
    );
    await expect(dialog).toContainText("shasum -a 256");
    await expect(dialog).toContainText("e7b6b28ea71dfeb372509a4c965a1e2aafe3ddaf1e30e66d0e0c372f954d191d");
  });

  test("Install: compatibility disclosures sit beside the install steps", async ({ page }) => {
    await openSite(page);
    const compat = page.getByRole("group", { name: "Before you rely on it" });
    await compat.scrollIntoViewIfNeeded();
    await expect(compat).toContainText("Claude Code");
    await expect(compat).toContainText("not code-signed");
    await expect(compat).toContainText("not an operating-system sandbox");
  });

  test("Install: the copy buttons copy", async ({ page, context }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await openSite(page);
    const dialog = page.getByRole("group", { name: "Install Legatus" });
    await dialog.getByRole("radio", { name: "Linux" }).click();
    await dialog.getByRole("radio", { name: "x64 (Intel or AMD)" }).click();
    await dialog.getByRole("button", { name: /Copy Run command/ }).click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("./legatus_v0.1.0_linux_amd64/legatus demo");
  });

  test("licensing lives in the footer, not in the hero or beside the install steps", async ({ page }) => {
    await openSite(page);
    const footer = page.getByRole("contentinfo");
    await expect(footer).toContainText("MIT license");
    await expect(footer).toContainText("Use only your own logins");
    await expect(footer).toContainText("THIRD_PARTY.md");
    await expect(footer).toContainText("Microsoft");
    await expect(page.locator(".hero")).not.toContainText(/MIT|licen[sc]e/i);
    await expect(page.locator("#install")).not.toContainText(/MIT license/i);
  });

  test("makes no claim it cannot back: no customers, benchmarks or invented numbers", async ({ page }) => {
    await openSite(page);
    const text = (await page.locator("main").innerText()).toLowerCase();
    for (const banned of ["trusted by", "customers", "testimonial", "faster than", "% faster", "x faster", "production-ready", "enterprise-grade", "used by thousands", "stars on github"]) {
      expect(text, banned).not.toContain(banned);
    }
  });
});

test.describe("optional effects", () => {
  test("the Paper Shaders wallpaper is lazy, and the static wallpaper is always there", async ({ page }) => {
    const chunks: string[] = [];
    page.on("request", (r) => {
      if (/WallpaperShader/.test(r.url())) chunks.push(r.url());
    });
    await page.goto("./");
    await expect(page.locator(".wallpaper__art").first()).toBeVisible();
    // The shader chunk is not part of the first load.
    expect(chunks).toEqual([]);
    test.skip(!(await webglWorks(page)), "this browser has no WebGL");
    await page.locator(".demo-section__frame").scrollIntoViewIfNeeded();
    await expect(page.locator(".wallpaper").first()).toHaveAttribute("data-shader", "on", { timeout: 10_000 });
    await expect(page.locator(".wallpaper__shader canvas")).toHaveCount(1);
    expect(chunks.length).toBe(1);
  });

  test("without WebGL there is no shader and the SVG wallpaper still shows", async ({ page }) => {
    const chunks: string[] = [];
    page.on("request", (r) => {
      if (/WallpaperShader/.test(r.url())) chunks.push(r.url());
    });
    await openSite(page);
    await page.waitForTimeout(1500);
    await expect(page.locator(".wallpaper").first()).toHaveAttribute("data-shader", "off");
    await expect(page.locator(".wallpaper__shader")).toHaveCount(0);
    await expect(page.locator(".wallpaper__art").first()).toBeVisible();
    expect(chunks).toEqual([]);
  });

  test("if the shader chunk fails to load the page carries on", async ({ page }) => {
    test.skip(!(await (async () => { await page.goto("./"); return webglWorks(page); })()), "this browser has no WebGL");
    await page.route(/WallpaperShader/, (route) => route.abort());
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto("./");
    await page.locator(".demo-section__frame").scrollIntoViewIfNeeded();
    await page.waitForTimeout(1500);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.getByRole("group", { name: /Legatus Cockpit/ })).toBeVisible();
    await expect(page.locator(".wallpaper__art").first()).toBeVisible();
  });
});
