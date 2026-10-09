import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { hero } from "./copy";

const html = readFileSync(new URL("../../index.html", import.meta.url), "utf8");

describe("the pre-paint shell in index.html", () => {
  it("shows the same headline, explanation and actions as the hero", () => {
    expect(html).toContain(`>${hero.title}</h1>`);
    expect(html).toContain(`>${hero.lede}</p>`);
    expect(html).toContain(`>${hero.primary}</a>`);
    expect(html).toContain(`>${hero.secondary}</a>`);
  });

  it("points at the right deployment path and anchors", () => {
    expect(html).toContain('href="#demo"');
    expect(html).toContain('href="#install"');
    expect(html).toContain("https://mvnshi.github.io/legatus/");
  });
});
