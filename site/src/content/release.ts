/**
 * Facts about the latest published release, copied from https://github.com/Mvnshi/legatus/releases/tag/v0.1.0.
 * The sizes and digests are what GitHub reports for each file; the digests were checked against the release's own
 * SHA256SUMS file. Nothing here is estimated.
 *
 * When a new release is published, update this file. `release.test.ts` fails if the changelog is ahead of it.
 */

export type OS = "windows" | "macos" | "linux";
export type Arch = "amd64" | "arm64";

export interface ReleaseAsset {
  os: OS;
  arch: Arch;
  name: string;
  bytes: number;
  sha256: string;
}

export const REPO_URL = "https://github.com/Mvnshi/legatus";

export const release = {
  version: "0.1.0",
  tag: "v0.1.0",
  prerelease: true,
  /** ISO date the release was published. */
  publishedOn: "2026-10-09",
  url: `${REPO_URL}/releases/tag/v0.1.0`,
  allReleasesUrl: `${REPO_URL}/releases`,
  checksumsUrl: `${REPO_URL}/releases/download/v0.1.0/SHA256SUMS`,
  builtBy: "GitHub Actions, from the v0.1.0 tag",
  assets: [
    {
      os: "windows",
      arch: "amd64",
      name: "legatus_v0.1.0_windows_amd64.zip",
      bytes: 3478095,
      sha256: "9197c96aab9cf35e1f92ed520bad155bf560faf827efa5f880874ca7cb773c44",
    },
    {
      os: "windows",
      arch: "arm64",
      name: "legatus_v0.1.0_windows_arm64.zip",
      bytes: 3092920,
      sha256: "15f269414988bf1610744c5706568425e297af68270344801d2f9e5fd37d2844",
    },
    {
      os: "macos",
      arch: "amd64",
      name: "legatus_v0.1.0_darwin_amd64.tar.gz",
      bytes: 3435823,
      sha256: "0e9b996a62f4365b576048c466bf56b91bdfbd646abd4206eeb9051bb171f30f",
    },
    {
      os: "macos",
      arch: "arm64",
      name: "legatus_v0.1.0_darwin_arm64.tar.gz",
      bytes: 3153045,
      sha256: "e7b6b28ea71dfeb372509a4c965a1e2aafe3ddaf1e30e66d0e0c372f954d191d",
    },
    {
      os: "linux",
      arch: "amd64",
      name: "legatus_v0.1.0_linux_amd64.tar.gz",
      bytes: 3401380,
      sha256: "d98407970c384ccba7648ac332952a846ee24a692a0da2b164d37d5ce06be9e9",
    },
    {
      os: "linux",
      arch: "arm64",
      name: "legatus_v0.1.0_linux_arm64.tar.gz",
      bytes: 3053689,
      sha256: "d9c2d090a294ad4de3791b159705487ffa9ee12d8779013a0150d0017114df98",
    },
  ] satisfies ReleaseAsset[],
} as const;

export function downloadUrl(asset: ReleaseAsset): string {
  return `${REPO_URL}/releases/download/${release.tag}/${asset.name}`;
}

export function findAsset(os: OS, arch: Arch): ReleaseAsset {
  const found = release.assets.find((a) => a.os === os && a.arch === arch);
  // Every OS has both architectures, so this only fails if the table above is edited wrongly.
  if (!found) throw new Error(`no release asset for ${os}/${arch}`);
  return found;
}

export function formatSize(bytes: number): string {
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatDate(iso: string): string {
  return new Intl.DateTimeFormat("en", { year: "numeric", month: "long", day: "numeric", timeZone: "UTC" }).format(
    new Date(`${iso}T00:00:00Z`),
  );
}

export const OS_LABEL: Record<OS, string> = { windows: "Windows", macos: "macOS", linux: "Linux" };
export const ARCH_LABEL: Record<Arch, string> = { amd64: "x64 (Intel or AMD)", arm64: "ARM64" };

/** What to type after downloading: unpack, check the hash, run the built-in demo. */
export function commandsFor(asset: ReleaseAsset): { verify: string; unpack: string; run: string } {
  const folder = asset.name.replace(/\.(zip|tar\.gz)$/, "");
  if (asset.os === "windows") {
    return {
      verify: `Get-FileHash .\\${asset.name} -Algorithm SHA256`,
      unpack: `Expand-Archive .\\${asset.name} -DestinationPath .`,
      run: `.\\${folder}\\legatus.exe demo`,
    };
  }
  return {
    verify: asset.os === "macos" ? `shasum -a 256 ${asset.name}` : `sha256sum ${asset.name}`,
    unpack: `tar -xzf ${asset.name}`,
    run: `./${folder}/legatus demo`,
  };
}
