import { useState } from "react";
import * as Radio from "@radix-ui/react-radio-group";
import { XPWindow } from "./XPWindow";
import { XPIcon } from "./icons";
import { XPLinkButton } from "./controls";
import { CodeLine } from "./CodeLine";
import {
  ARCH_LABEL,
  OS_LABEL,
  commandsFor,
  downloadUrl,
  findAsset,
  formatSize,
  release,
  type Arch,
  type OS,
} from "../content/release";
import { detectOS } from "../lib/platform";
import "./install.css";

const OS_ORDER: OS[] = ["windows", "macos", "linux"];
const ARCH_ORDER: Arch[] = ["amd64", "arm64"];

const archHint: Record<OS, string> = {
  windows: "Not sure? Open Settings, System, About, and read “System type”.",
  macos: "Not sure? Run uname -m in Terminal: x86_64 means x64, arm64 means ARM64 (Apple silicon).",
  linux: "Not sure? Run uname -m: x86_64 means x64, aarch64 means ARM64.",
};

function RadioChoice({ value, children }: { value: string; children: string }) {
  return (
    <Radio.Item value={value} className="xp-radio" id={`radio-${value}`}>
      <span className="xp-radio__dot" aria-hidden="true">
        <Radio.Indicator className="xp-radio__mark" />
      </span>
      <span>{children}</span>
    </Radio.Item>
  );
}

/**
 * A Setup-style dialog for the one thing a visitor came to do: get a build, check it, run the demo.
 * Everything it shows comes from the published release (see content/release.ts), and nothing runs or downloads
 * until the person follows the link.
 */
export function XPInstallDialog() {
  const [os, setOs] = useState<OS>(() => detectOS());
  const [arch, setArch] = useState<Arch>("amd64");
  const asset = findAsset(os, arch);
  const cmd = commandsFor(asset);

  return (
    <XPWindow
      layout="static"
      id="install"
      title="Install Legatus"
      icon="download"
      active
      status={[`${release.tag} · ${release.prerelease ? "pre-release" : "release"}`, `${asset.name} · ${formatSize(asset.bytes)}`]}
    >
      <div className="xp-install">
        <div className="xp-install__banner" aria-hidden="true">
          <XPIcon name="legatus" size={72} />
          <strong>Legatus</strong>
          <span>{release.tag}</span>
        </div>

        <div className="xp-install__main">
          <fieldset className="xp-install__group">
            <legend>1. Your system</legend>
            <Radio.Root
              className="xp-install__radios"
              value={os}
              onValueChange={(v) => setOs(v as OS)}
              aria-label="Operating system"
              orientation="horizontal"
            >
              {OS_ORDER.map((o) => (
                <RadioChoice key={o} value={o}>
                  {OS_LABEL[o]}
                </RadioChoice>
              ))}
            </Radio.Root>
            <Radio.Root
              className="xp-install__radios"
              value={arch}
              onValueChange={(v) => setArch(v as Arch)}
              aria-label="Processor"
              orientation="horizontal"
            >
              {ARCH_ORDER.map((a) => (
                <RadioChoice key={a} value={a}>
                  {ARCH_LABEL[a]}
                </RadioChoice>
              ))}
            </Radio.Root>
            <p className="xp-install__hint">{archHint[os]}</p>
          </fieldset>

          <fieldset className="xp-install__group">
            <legend>2. Download and check</legend>
            <dl className="xp-install__facts">
              <div>
                <dt>File</dt>
                <dd>
                  {asset.name} <span className="xp-install__size">({formatSize(asset.bytes)})</span>
                </dd>
              </div>
              <div>
                <dt>SHA-256</dt>
                <dd>
                  <CodeLine text={asset.sha256} label="SHA-256 checksum" wrap />
                </dd>
              </div>
            </dl>
            <p className="xp-install__hint">
              Compare it with the output of:
            </p>
            <CodeLine text={cmd.verify} label="Checksum command" />
          </fieldset>

          <fieldset className="xp-install__group">
            <legend>3. Unpack and run the demo</legend>
            <CodeLine text={cmd.unpack} label="Unpack command" />
            <CodeLine text={cmd.run} label="Run command" />
            <p className="xp-install__hint">
              Real runs need <code>git</code> and a signed-in Codex CLI or Claude Code.
            </p>
          </fieldset>

          <div className="xp-install__buttons">
            <XPLinkButton variant="go" icon="download" href={downloadUrl(asset)} rel="noopener">
              Download for {OS_LABEL[os]} {arch === "amd64" ? "x64" : "ARM64"}
            </XPLinkButton>
            <XPLinkButton href={release.url} target="_blank" rel="noopener noreferrer">
              Release notes
            </XPLinkButton>
            <XPLinkButton href={release.allReleasesUrl} target="_blank" rel="noopener noreferrer">
              All downloads
            </XPLinkButton>
          </div>
        </div>
      </div>
    </XPWindow>
  );
}
