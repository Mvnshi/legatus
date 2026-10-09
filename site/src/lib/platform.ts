import type { OS } from "../content/release";

/** A best guess at the visitor's system, used only to preselect a download. They can change it. */
export function detectOS(): OS {
  if (typeof navigator === "undefined") return "windows";
  const uaData = (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData;
  const hint = `${uaData?.platform ?? ""} ${navigator.platform ?? ""} ${navigator.userAgent ?? ""}`.toLowerCase();
  if (/android|iphone|ipad|ipod/.test(hint)) return "windows"; // phones cannot run Legatus; show the common default
  if (/mac/.test(hint)) return "macos";
  if (/linux|x11|cros/.test(hint)) return "linux";
  return "windows";
}

export function isPhone(): boolean {
  if (typeof navigator === "undefined") return false;
  return /android|iphone|ipad|ipod/i.test(navigator.userAgent ?? "");
}
