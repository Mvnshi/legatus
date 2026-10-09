import { XPIcon } from "../xp/icons";
import { REPO_URL } from "../content/release";
import "./header.css";

// `short` is what fits on a phone, so every destination stays on screen without scrolling.
const nav = [
  { href: "#demo", label: "Demo", short: "Demo" },
  { href: "#how", label: "How it works", short: "How" },
  { href: "#status", label: "What is verified", short: "Verified" },
  { href: "#install", label: "Install", short: "Install" },
];

/** Page navigation that is always on screen, whatever is happening inside the simulated desktop. */
export function SiteHeader() {
  return (
    <header className="site-header">
      <div className="site-header__inner">
        <a className="site-header__brand" href="#top" aria-label="Legatus, back to the top">
          <XPIcon name="legatus" size={30} />
          <span>Legatus</span>
        </a>
        <nav className="site-header__nav" aria-label="Main">
          <ul>
            {nav.map((n) => (
              <li key={n.href}>
                <a href={n.href}>
                  <span className="nav-long">{n.label}</span>
                  <span className="nav-short" aria-hidden="true">
                    {n.short}
                  </span>
                </a>
              </li>
            ))}
            <li>
              <a className="site-header__github" href={REPO_URL} target="_blank" rel="noopener noreferrer">
                GitHub
                <span className="visually-hidden"> (opens in a new tab)</span>
              </a>
            </li>
          </ul>
        </nav>
      </div>
    </header>
  );
}
