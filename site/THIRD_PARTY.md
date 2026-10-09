# Third-party notices for the website

The Legatus program (the Go code in the rest of this repository) is released under the [MIT license](../LICENSE).
The website in this folder uses the open-source packages below. They are separate works under their own licenses.
Their full license texts are in each package's folder under `node_modules` after `npm ci`, and are shipped with
the packages on npm.

## Shipped in the website

| Package | License | What it is used for |
| --- | --- | --- |
| [React](https://react.dev) and React DOM | MIT, Meta Platforms, Inc. and affiliates | The user interface |
| [XP.css](https://github.com/botoxparty/XP.css) 0.2.6 | MIT, Copyright 2020 Adam Hammad, Jordan Scales | The Luna window frames, title bars, caption buttons, tabs, status bars and progress bars |
| [Radix UI primitives](https://www.radix-ui.com) (Tabs, Dropdown Menu, Menubar, Tooltip, Radio Group) | MIT, Copyright 2022 WorkOS | Accessible tabs, menus, the Start menu, tooltips and radio groups |
| [Motion](https://motion.dev) | MIT, Copyright 2024 Motion B.V. | Window open, close and minimize motion, balloon notices |
| [Paper Shaders](https://shaders.paper.design) (`@paper-design/shaders-react`) | Apache-2.0, Copyright 2026 Paper | One slow mesh-gradient effect over the wallpaper's sky. Loaded lazily, off with reduced motion or without WebGL |

## Adapted components with their own license

Two small components are adapted from [React Bits](https://github.com/DavidHDev/react-bits) by David Haz, which is
released under **MIT + Commons Clause**, not plain MIT:

| File | Adapted from | What changed |
| --- | --- | --- |
| `src/reactbits/CountUp.tsx` | `TextAnimations/CountUp` | Counts whenever its target changes instead of once on scroll, honours reduced motion, drops unused options |
| `src/reactbits/ShinyText.tsx` | `TextAnimations/ShinyText` | Rewritten as a one-shot sweep driven by a Motion value; no looping, pointer following or canvas colour parsing |

The original license text is in [`src/reactbits/LICENSE.md`](src/reactbits/LICENSE.md). The Commons Clause does not
allow selling, sublicensing or redistributing the components themselves. **These two files are therefore not
covered by this repository's MIT license** and must stay under the React Bits terms. Anyone copying this website's
source into another project should review those terms or replace the two files.

## Original work

The wallpaper (`src/xp/Wallpaper.tsx`) and every icon (`src/xp/icons.tsx`) are drawn for this project. They do not
use any artwork from Windows. Windows and Windows XP are trademarks of Microsoft, which has no connection with this
project.

## Used only to build and test (not shipped)

Vite, `@vitejs/plugin-react`, TypeScript, Vitest and Playwright (MIT or Apache-2.0), and `@axe-core/playwright`
(MPL-2.0, used in the browser tests only).
