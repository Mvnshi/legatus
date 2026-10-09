import { XPIcon } from "../xp/icons";
import { links } from "../content/copy";
import "./footer.css";

export function SiteFooter() {
  return (
    <footer className="site-footer">
      <div className="wrap site-footer__inner">
        <div className="site-footer__brand">
          <XPIcon name="legatus" size={36} />
          <div>
            <p className="site-footer__name">Legatus</p>
            <p className="site-footer__tag">Early software (v0.1). Open source.</p>
          </div>
        </div>

        <div className="site-footer__cols">
          <section aria-labelledby="footer-license">
            <h2 id="footer-license">License</h2>
            <p>
              Legatus is released under the <a href={links.license} target="_blank" rel="noopener noreferrer">MIT license</a>.
            </p>
            <p>
              Use only your own logins, within each provider's terms. Legatus never asks for or stores a password.
            </p>
          </section>

          <section aria-labelledby="footer-notices">
            <h2 id="footer-notices">This website</h2>
            <p>
              Built with XP.css (MIT), Radix UI (MIT), Motion (MIT) and Paper Shaders (Apache-2.0), with two components
              adapted from React Bits (MIT with the Commons Clause). Full notices are in{" "}
              <a href={links.thirdParty} target="_blank" rel="noopener noreferrer">THIRD_PARTY.md</a>.
            </p>
            <p>
              The wallpaper and icons are original. Windows and Windows XP are trademarks of Microsoft, which has no
              connection to this project. Codex and Claude Code are named only to say what Legatus works with.
            </p>
          </section>

          <nav aria-labelledby="footer-links">
            <h2 id="footer-links">Project</h2>
            <ul>
              <li><a href={links.repo} target="_blank" rel="noopener noreferrer">Source on GitHub</a></li>
              <li><a href={links.design} target="_blank" rel="noopener noreferrer">How it works (design notes)</a></li>
              <li><a href={links.status} target="_blank" rel="noopener noreferrer">What is verified</a></li>
              <li><a href={links.changelog} target="_blank" rel="noopener noreferrer">Changelog</a></li>
              <li><a href={links.security} target="_blank" rel="noopener noreferrer">Security policy</a></li>
              <li><a href={links.contributing} target="_blank" rel="noopener noreferrer">Contributing</a></li>
              <li><a href={links.issues} target="_blank" rel="noopener noreferrer">Issues</a></li>
            </ul>
          </nav>
        </div>
      </div>
    </footer>
  );
}
