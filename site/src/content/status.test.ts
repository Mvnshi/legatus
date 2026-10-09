import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { compatibility, statusGroups } from "./status";

const doc = readFileSync(new URL("../../../docs/STATUS.md", import.meta.url), "utf8");

/** The bold lead-ins of the bullets under a `## Heading` in STATUS.md. */
function leads(heading: string): string[] {
  const start = doc.indexOf(`## ${heading}`);
  expect(start, `STATUS.md has no "${heading}" section`).toBeGreaterThanOrEqual(0);
  const next = doc.indexOf("\n## ", start + 1);
  const section = doc.slice(start, next === -1 ? undefined : next);
  return [...section.matchAll(/^- \*\*(.+?)\*\*/gm)].map((m) => m[1]!.replace(/\.$/, "").trim());
}

const norm = (s: string) => s.toLowerCase().replace(/[’']/g, "'").replace(/\s+/g, " ");

describe("the page's verification claims match docs/STATUS.md", () => {
  it("lists every item under 'Not verified yet'", () => {
    const fromDoc = leads("Not verified yet");
    expect(fromDoc.length).toBeGreaterThan(3);
    const onPage = statusGroups.find((g) => g.id === "not")!.items.map((i) => norm(i.lead));
    for (const lead of fromDoc) {
      expect(
        onPage.some((p) => norm(lead).startsWith(p) || p.startsWith(norm(lead))),
        `"${lead}" is under "Not verified yet" in docs/STATUS.md but missing from the site`,
      ).toBe(true);
    }
  });

  it("does not list anything as unverified that STATUS.md does not", () => {
    const fromDoc = leads("Not verified yet").map(norm);
    for (const item of statusGroups.find((g) => g.id === "not")!.items) {
      expect(
        fromDoc.some((d) => d.startsWith(norm(item.lead)) || norm(item.lead).startsWith(d)),
        `"${item.lead}" is not under "Not verified yet" in docs/STATUS.md`,
      ).toBe(true);
    }
  });

  it("keeps the facts that matter exactly as STATUS.md states them", () => {
    expect(doc).toContain("Codex CLI 0.162");
    expect(doc).toContain("Claude Code 2.1.295");
    expect(doc).toContain("set aside for 30 minutes");
    expect(doc).toContain("twelve created at the same moment");
    const page = JSON.stringify(statusGroups);
    expect(page).toContain("Codex CLI 0.162");
    expect(page).toContain("Claude Code 2.1.295");
    expect(page).toContain("set aside for 30 minutes");
    expect(page).toContain("twelve created at the same moment");
  });

  it("states the compatibility summary without overclaiming", () => {
    const text = compatibility.join(" ");
    expect(text).toMatch(/Claude Code/);
    // What was proven is named, and so is what was not.
    expect(text).toMatch(/Run for real/);
    expect(text).toMatch(/Not run for real/);
    expect(text).toMatch(/not code-signed/i);
    expect(text).not.toMatch(/fully tested|production[- ]ready|battle[- ]tested|enterprise/i);
  });
});
