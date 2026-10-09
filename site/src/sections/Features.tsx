import { XPWindow } from "../xp/XPWindow";
import { XPIcon } from "../xp/icons";
import { features } from "../content/copy";
import "./features.css";

export function Features() {
  return (
    <section className="section" id="features" aria-labelledby="features-title">
      <div className="wrap">
        <div className="section__head">
          <p className="eyebrow">What is in it</p>
          <h2 id="features-title">Built for work you will review before it ships</h2>
        </div>
        <XPWindow layout="static" id="features-window" title="Legatus features" icon="folder" active>
          <ul className="features">
            {features.map((f) => (
              <li key={f.title} className="feature">
                <XPIcon name={f.icon} size={44} />
                <div>
                  <h3>{f.title}</h3>
                  <p>{f.body}</p>
                </div>
              </li>
            ))}
          </ul>
        </XPWindow>
      </div>
    </section>
  );
}
