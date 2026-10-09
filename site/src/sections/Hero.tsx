import { XPIcon } from "../xp/icons";
import { XPLinkButton } from "../xp/controls";
import { hero } from "../content/copy";
import { requestTryDemo } from "../lib/events";
import "./hero.css";

export function Hero() {
  return (
    <section className="hero" id="top" aria-labelledby="hero-title">
      <div className="hero__inner">
        <div className="hero__text">
          <p className="hero__eyebrow">{hero.facts[0]} · {hero.facts[1]}</p>
          <h1 id="hero-title" className="hero__title">
            {hero.title.split(". ").map((line, i, all) => (
              <span key={line} className="hero__line">
                {i < all.length - 1 ? `${line}. ` : line}
              </span>
            ))}
          </h1>
          <p className="hero__lede">{hero.lede}</p>
          <div className="hero__actions">
            <XPLinkButton variant="go" icon="play" href="#demo" onClick={requestTryDemo}>
              {hero.primary}
            </XPLinkButton>
            <XPLinkButton icon="download" href="#install">
              {hero.secondary}
            </XPLinkButton>
          </div>
          <p className="hero__fact">{hero.facts[2]}. Your code and your logins stay on your machine.</p>
        </div>
        <div className="hero__art" aria-hidden="true">
          <div className="hero__orb">
            <XPIcon name="legatus" size={150} />
          </div>
          <span className="hero__chip hero__chip--a">
            <XPIcon name="queue" size={26} />
          </span>
          <span className="hero__chip hero__chip--b">
            <XPIcon name="branch" size={26} />
          </span>
          <span className="hero__chip hero__chip--c">
            <XPIcon name="review" size={26} />
          </span>
        </div>
      </div>
    </section>
  );
}
