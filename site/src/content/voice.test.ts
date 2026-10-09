import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

/**
 * The page's own words must say things, not announce them. Copy that describes the page ("these are
 * screenshots of...", "pick one to read...", "everything below...") or declares itself honest is filler: the
 * reader can see what they are looking at. This keeps it from coming back.
 */
const files = [
  "src/content/copy.ts",
  "src/content/status.ts",
  "src/sections/Hero.tsx",
  "src/sections/DemoSection.tsx",
  "src/sections/HowItWorks.tsx",
  "src/sections/Features.tsx",
  "src/sections/RealCockpit.tsx",
  "src/sections/StatusSection.tsx",
  "src/sections/InstallSection.tsx",
  "src/sections/SiteFooter.tsx",
  "src/demo/views.tsx",
  "src/desktop/Desktop.tsx",
  "src/xp/XPInstallDialog.tsx",
];

const banned: [RegExp, string][] = [
  [/\bthese are (screenshots|screens)\b/i, "describes the medium"],
  [/\bpick one\b/i, "tells the reader how to read the page"],
  [/\beverything below\b/i, "points at the page"],
  [/\b(further|listed) (down|below)\b/i, "points at the page"],
  [/\bhonest(ly)?\b/i, "declares itself honest; just be so"],
  [/\bin this section\b/i, "points at the page"],
  [/\bas you can see\b/i, "states the obvious"],
  [/\bscripted in your browser\b/i, "states the obvious"],
  [/\bsee the full list\b/i, "signposting"],
];

const withoutComments = (src: string) => src.replace(/\/\*[\s\S]*?\*\//g, "").replace(/^\s*\/\/.*$/gm, "");

describe("the page's voice", () => {
  for (const file of files) {
    it(`${file} has no filler`, () => {
      const text = withoutComments(readFileSync(new URL(`../../${file}`, import.meta.url), "utf8"));
      for (const [pattern, why] of banned) {
        expect(text, `${file}: ${why} (${pattern})`).not.toMatch(pattern);
      }
    });
  }
});
