# The Legatus website

A React, TypeScript and Vite app that is built to static files and served from
<https://mvnshi.github.io/legatus/> (GitHub Pages, path `/legatus/`). It does not change the Go program or the local
cockpit in `internal/server/ui`.

## Work on it

You need Node.js 22 or newer.

```text
cd site
npm ci
npm run dev          # http://localhost:5173/legatus/
npm run typecheck
npm test             # unit tests: the demo engine, the window manager, and the copy against docs/STATUS.md
npm run build        # type-check, then production build into site/dist
npm run e2e          # browser tests against the production build, served at /legatus/
```

`npm run e2e` starts `vite preview` itself, so run `npm run build` first. If Playwright cannot find its own Chromium,
point it at one you have: `LEGATUS_CHROMIUM=/path/to/chromium npm run e2e`. First time on a machine:
`npx playwright install chromium`.

## What is where

```text
src/xp/          the Luna components: XPWindow, XPTaskbar, XPStartMenu, XPBalloonNotice, XPExplorerPane,
                 XPInstallDialog, plus tabs, menus, tooltips, icons and the wallpaper
src/desktop/     the demo desktop: window manager (windows.ts, tested), layout, boot screen
src/demo/        the simulated run (engine.ts, tested) and what each window shows
src/sections/    the page: header, hero, demo frame, how it works, features, real cockpit, status, install, footer
src/content/     the words: copy, the verification claims (checked against docs/STATUS.md), release facts
src/reactbits/   two components adapted from React Bits, under their own license (see THIRD_PARTY.md)
e2e/             Playwright tests: every interaction, keyboard, phone, reduced motion, axe, the /legatus/ path
```

## Rules this site keeps

- **Say what is true.** The demo is labelled "Simulated demo" in the taskbar, the cockpit title, the status bar and
  beside the desktop. The verification text lives in `src/content/status.ts`, and `status.test.ts` fails if
  `docs/STATUS.md` lists something under "Not verified yet" that the site does not.
- **No customers, benchmarks or invented numbers.** `e2e/page.spec.ts` checks the page for the usual offenders.
- **When you publish a release, update `src/content/release.ts`** (version, date, sizes and SHA-256 digests from the
  release page). `release.test.ts` fails if `CHANGELOG.md` is ahead of it.
- **No third-party requests.** The page loads nothing from any other host. The test checks it.
- **Sound starts muted.** Nothing audio is created until a person turns it on.
- **Motion is optional.** Reduced motion turns off the shader, the looping animations and the window flights. The boot
  screen only appears when asked for, from the Start menu.
- **The wallpaper shader is the only effect that needs WebGL,** and it is lazy-loaded. Without WebGL, with reduced
  motion, or if its chunk fails to load, the static SVG wallpaper is all there is.

## Things worth knowing

- XP.css is loaded inside a CSS `@layer`, so the site's own styles always win over its bare-element rules. Its
  published CSS has a few selectors that are not valid, so `vite.config.ts` turns on Lightning CSS error recovery for
  the minifier.
- React Three Fiber was left out on purpose: no 3D element would make the design better, and it would add weight.
- The Windows-style caption buttons are drawn 24px wide (XP's own are 21px) to meet WCAG 2.2 target size.
