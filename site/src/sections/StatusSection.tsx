import { useState } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { XPWindow } from "../xp/XPWindow";
import { XPExplorerPane, XPTaskGroup } from "../xp/XPExplorerPane";
import { XPIcon } from "../xp/icons";
import { statusGroups, type StatusGroup } from "../content/status";
import { links } from "../content/copy";
import "./status.css";

export function StatusSection() {
  const [group, setGroup] = useState<StatusGroup["id"]>("not");
  return (
    <section className="section" id="status" aria-labelledby="status-title">
      <div className="wrap">
        <div className="section__head">
          <p className="eyebrow">An honest status</p>
          <h2 id="status-title">What is verified, and what is not</h2>
          <p className="section__lede">
            Legatus is early software (v0.1). The core is tested on Linux, macOS and Windows with stand-in agents and real{" "}
            <code>git</code>, and it has completed a real task with the real Codex CLI on one Windows PC. Several things have
            not been run for real yet, and they are listed first.
          </p>
        </div>
        <XPWindow layout="static" id="status-window" title="Verification · docs/STATUS.md" icon="status" active>
          <Tabs.Root value={group} onValueChange={(v) => setGroup(v as StatusGroup["id"])} orientation="vertical">
            <XPExplorerPane
              sidebar={
                <>
                  <XPTaskGroup title="Verification" icon="status">
                    <Tabs.List className="how__list" aria-label="Verification groups">
                      {statusGroups
                        .slice()
                        .sort((a, b) => order.indexOf(a.id) - order.indexOf(b.id))
                        .map((g) => (
                          <Tabs.Trigger key={g.id} value={g.id} className="how__tab how__tab--status" data-group={g.id}>
                            <XPIcon name={g.icon} size={22} />
                            <span>
                              <span className="how__tab-title">{g.title}</span>
                              <span className="how__tab-sub">{g.items.length} items</span>
                            </span>
                          </Tabs.Trigger>
                        ))}
                    </Tabs.List>
                  </XPTaskGroup>
                  <XPTaskGroup title="Read the source" icon="book" tone="plain">
                    <ul className="task-links">
                      <li>
                        <a href={links.status} target="_blank" rel="noopener noreferrer">
                          docs/STATUS.md <span className="visually-hidden">(opens in a new tab)</span>
                        </a>
                      </li>
                      <li>
                        <a href={links.backlog} target="_blank" rel="noopener noreferrer">
                          Backlog <span className="visually-hidden">(opens in a new tab)</span>
                        </a>
                      </li>
                    </ul>
                  </XPTaskGroup>
                </>
              }
            >
              {statusGroups.map((g) => (
                <Tabs.Content key={g.id} value={g.id} className="status__panel" tabIndex={-1}>
                  <h3>
                    <XPIcon name={g.icon} size={26} /> {g.title}
                  </h3>
                  <p className="status__lead">{g.lead}</p>
                  <ul className="status__list">
                    {g.items.map((i) => (
                      <li key={i.lead}>
                        <strong>{i.lead}.</strong> {i.text}
                      </li>
                    ))}
                  </ul>
                </Tabs.Content>
              ))}
            </XPExplorerPane>
          </Tabs.Root>
        </XPWindow>
      </div>
    </section>
  );
}

const order: StatusGroup["id"][] = ["not", "real", "tested", "limits"];
