import { Desktop } from "../desktop/Desktop";
import { demoIntro } from "../content/copy";
import { Inline } from "../lib/Inline";
import "./demo-section.css";

export function DemoSection() {
  return (
    <section className="demo-section" id="demo" aria-labelledby="demo-title">
      <div className="wrap">
        <div className="demo-section__head">
          <p className="eyebrow">{demoIntro.label}</p>
          <h2 id="demo-title">{demoIntro.title}</h2>
          <p className="demo-section__lede">
            <Inline text={demoIntro.lede} />
          </p>
        </div>
        <a className="skip-inline" href="#how">
          Skip the demo desktop
        </a>
        <div className="demo-section__frame" role="region" aria-label="Simulated demo desktop">
          <Desktop />
        </div>
        <p className="demo-section__note">
          <strong>{demoIntro.label}.</strong> {demoIntro.disclosure}
        </p>
        <p className="demo-section__keys">{demoIntro.keyboard}</p>
      </div>
    </section>
  );
}
