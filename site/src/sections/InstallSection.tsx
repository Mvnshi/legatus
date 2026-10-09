import { XPWindow } from "../xp/XPWindow";
import { XPInstallDialog } from "../xp/XPInstallDialog";
import { XPIcon } from "../xp/icons";
import { CodeLine } from "../xp/CodeLine";
import { compatibility } from "../content/status";
import { links } from "../content/copy";
import { formatDate, formatSize, release } from "../content/release";
import "./install-section.css";

const sizes = release.assets.map((a) => a.bytes);

export function InstallSection() {
  return (
    <section className="section" id="install" aria-labelledby="install-title">
      <div className="wrap">
        <div className="section__head">
          <p className="eyebrow">Download</p>
          <h2 id="install-title">Install Legatus</h2>
          <p className="section__lede">
            One program, no installer. Download a build for your system, or build it from source with Go. The built-in demo
            needs no agent and no account.
          </p>
        </div>
        <div className="install-layout">
          <XPInstallDialog />
          <aside className="install-side" aria-label="Release information and compatibility">
            <XPWindow layout="static" id="release-window" title="Release information" icon="info" active>
              <div className="release">
                <dl className="release__facts">
                  <div>
                    <dt>Version</dt>
                    <dd>
                      {release.tag} <span className="pill">{release.prerelease ? "Pre-release" : "Release"}</span>
                    </dd>
                  </div>
                  <div>
                    <dt>Published</dt>
                    <dd>{formatDate(release.publishedOn)}</dd>
                  </div>
                  <div>
                    <dt>Built by</dt>
                    <dd>{release.builtBy}</dd>
                  </div>
                  <div>
                    <dt>Files</dt>
                    <dd>
                      {release.assets.length} builds, {formatSize(Math.min(...sizes))} to {formatSize(Math.max(...sizes))} each, and a
                      SHA256SUMS file
                    </dd>
                  </div>
                  <div>
                    <dt>Systems</dt>
                    <dd>Windows, macOS and Linux, each for x64 and ARM64</dd>
                  </div>
                  <div>
                    <dt>You need</dt>
                    <dd>
                      <code>git</code>, and for real runs a signed-in Codex CLI or Claude Code
                    </dd>
                  </div>
                </dl>
                <ul className="release__links">
                  <li>
                    <a href={release.url} target="_blank" rel="noopener noreferrer">
                      Release notes <span className="visually-hidden">(opens in a new tab)</span>
                    </a>
                  </li>
                  <li>
                    <a href={release.checksumsUrl} target="_blank" rel="noopener noreferrer">
                      SHA256SUMS <span className="visually-hidden">(opens in a new tab)</span>
                    </a>
                  </li>
                  <li>
                    <a href={links.changelog} target="_blank" rel="noopener noreferrer">
                      Changelog <span className="visually-hidden">(opens in a new tab)</span>
                    </a>
                  </li>
                </ul>
                <h3 className="release__sub">Or build from source</h3>
                <p className="release__note">With Go 1.26 or newer:</p>
                <CodeLine text="go install github.com/Mvnshi/legatus/cmd/legatus@latest" label="go install command" wrap />
              </div>
            </XPWindow>

            <XPWindow layout="static" id="compat-window" title="Before you rely on it" icon="warning" active>
              <div className="compat">
                <ul>
                  {compatibility.map((c) => (
                    <li key={c}>
                      <XPIcon name="warning" size={18} />
                      <span>{c}</span>
                    </li>
                  ))}
                </ul>
                <p>
                  <a href="#status">See the full list</a> of what is verified and what is not.
                </p>
              </div>
            </XPWindow>
          </aside>
        </div>
      </div>
    </section>
  );
}
