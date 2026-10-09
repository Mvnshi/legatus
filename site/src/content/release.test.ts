import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { commandsFor, downloadUrl, findAsset, formatSize, release } from "./release";

const changelog = readFileSync(new URL("../../../CHANGELOG.md", import.meta.url), "utf8");

describe("release facts", () => {
  it("are for the newest released version in CHANGELOG.md", () => {
    const newest = /^## \[(\d+\.\d+\.\d+)\]/m.exec(changelog)?.[1];
    expect(newest, "CHANGELOG.md has no released version").toBeDefined();
    expect(release.version, "src/content/release.ts is out of date with CHANGELOG.md").toBe(newest);
    expect(release.tag).toBe(`v${newest}`);
  });

  it("name files the way .github/workflows/release.yml builds them", () => {
    const workflow = readFileSync(new URL("../../../.github/workflows/release.yml", import.meta.url), "utf8");
    expect(workflow).toContain('name="legatus_${TAG}_${os}_${arch}"');
    for (const asset of release.assets) {
      const goos = asset.os === "macos" ? "darwin" : asset.os;
      const ext = asset.os === "windows" ? "zip" : "tar.gz";
      expect(asset.name).toBe(`legatus_${release.tag}_${goos}_${asset.arch}.${ext}`);
      expect(workflow).toContain(`${goos}/${asset.arch}`);
    }
  });

  it("covers both architectures on every system, with a full SHA-256 for each file", () => {
    for (const os of ["windows", "macos", "linux"] as const) {
      for (const arch of ["amd64", "arm64"] as const) {
        const a = findAsset(os, arch);
        expect(a.sha256).toMatch(/^[0-9a-f]{64}$/);
        expect(downloadUrl(a)).toBe(`https://github.com/Mvnshi/legatus/releases/download/${release.tag}/${a.name}`);
      }
    }
  });

  it("formats sizes and builds commands that match the archive layout", () => {
    expect(formatSize(3478095)).toBe("3.3 MB");
    const win = commandsFor(findAsset("windows", "amd64"));
    expect(win.run).toBe(".\\legatus_v0.1.0_windows_amd64\\legatus.exe demo");
    expect(win.verify).toContain("Get-FileHash");
    const mac = commandsFor(findAsset("macos", "arm64"));
    expect(mac.verify).toBe("shasum -a 256 legatus_v0.1.0_darwin_arm64.tar.gz");
    expect(mac.run).toBe("./legatus_v0.1.0_darwin_arm64/legatus demo");
    expect(commandsFor(findAsset("linux", "amd64")).verify.startsWith("sha256sum ")).toBe(true);
  });
});
