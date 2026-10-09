import { useState } from "react";
import { XPWindow } from "../xp/XPWindow";
import { XPTab, XPTabList, XPTabPanel, XPTabs } from "../xp/controls";
import { realCockpit } from "../content/copy";
import { Inline } from "../lib/Inline";
import runShot from "../../../assets/cockpit-run.png";
import runsShot from "../../../assets/cockpit-runs.png";
import loginsShot from "../../../assets/cockpit-logins.png";
import automationsShot from "../../../assets/cockpit-automations.png";
import "./real.css";

const shots = [
  {
    id: "run",
    label: "A run",
    src: runShot,
    w: 2560,
    h: 2000,
    alt: "A run in the Legatus cockpit: the first login hit its usage limit and the work continued on another login, then passed its checks and an independent review.",
  },
  {
    id: "runs",
    label: "All runs",
    src: runsShot,
    w: 2560,
    h: 1280,
    alt: "The Runs page of the cockpit: four finished runs, each with its branch, the login that implemented it, and its checks and review steps, all marked succeeded.",
  },
  {
    id: "logins",
    label: "Logins",
    src: loginsShot,
    w: 2560,
    h: 1640,
    alt: "The Logins page: work-1 (codex) at its limit with a countdown to its reset, work-2 (codex) and second-opinion (claude) ready, and a form to add a login.",
  },
  {
    id: "automations",
    label: "Automations",
    src: automationsShot,
    w: 2560,
    h: 840,
    alt: "The Automations page: a schedule, weekly mon 09:00, turned on, with a Run now button.",
  },
] as const;

export function RealCockpit() {
  const [tab, setTab] = useState<string>(shots[0].id);
  return (
    <section className="section" id="cockpit" aria-labelledby="cockpit-title">
      <div className="wrap">
        <div className="section__head">
          <p className="eyebrow">The real thing</p>
          <h2 id="cockpit-title">{realCockpit.title}</h2>
          <p className="section__lede">
            <Inline text={realCockpit.lede} /> It listens only on your own computer and needs a secret key.
          </p>
        </div>
        <XPWindow layout="static" id="real-window" title="Legatus cockpit (screenshots)" icon="cockpit" active>
          <XPTabs value={tab} onValueChange={setTab} className="real">
            <XPTabList label="Cockpit screens">
              {shots.map((s) => (
                <XPTab key={s.id} value={s.id}>
                  {s.label}
                </XPTab>
              ))}
            </XPTabList>
            {shots.map((s) => (
              <XPTabPanel key={s.id} value={s.id} className="real__panel">
                <img src={s.src} width={s.w} height={s.h} alt={s.alt} loading="lazy" decoding="async" />
              </XPTabPanel>
            ))}
          </XPTabs>
        </XPWindow>
      </div>
    </section>
  );
}
