import { useState } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { XPWindow } from "../xp/XPWindow";
import { XPExplorerPane, XPTaskGroup } from "../xp/XPExplorerPane";
import { XPIcon } from "../xp/icons";
import { CodeLine } from "../xp/CodeLine";
import { moves } from "../content/copy";
import { Inline } from "../lib/Inline";
import "./how.css";

export function HowItWorks() {
  const [move, setMove] = useState(moves[0]!.id);
  return (
    <section className="section" id="how" aria-labelledby="how-title">
      <div className="wrap">
        <div className="section__head">
          <p className="eyebrow">How it works</p>
          <h2 id="how-title">From a task to something you can review</h2>
        </div>
        <XPWindow layout="static" id="how-window" title="How a run goes" icon="queue" active>
          <Tabs.Root value={move} onValueChange={setMove} orientation="vertical">
            <XPExplorerPane
              sidebar={
                <XPTaskGroup title="A run, in order" icon="queue">
                  <Tabs.List className="how__list" aria-label="Steps of a run">
                    {moves.map((m, i) => (
                      <Tabs.Trigger key={m.id} value={m.id} className="how__tab">
                        <span className="how__num" aria-hidden="true">
                          {i + 1}
                        </span>
                        <span>
                          <span className="how__tab-title">{m.title}</span>
                          <span className="how__tab-sub">{m.summary}</span>
                        </span>
                      </Tabs.Trigger>
                    ))}
                  </Tabs.List>
                </XPTaskGroup>
              }
            >
              {moves.map((m) => (
                <Tabs.Content key={m.id} value={m.id} className="how__panel">
                  <div className="how__panel-head">
                    <XPIcon name={m.icon} size={44} />
                    <h3>{m.title}</h3>
                  </div>
                  <p>
                    <Inline text={m.body} />
                  </p>
                  {m.command ? <CodeLine text={m.command} label="Example command" wrap /> : null}
                  <p className="how__demo">
                    <XPIcon name="info" size={20} />
                    <span>
                      <strong>Demo:</strong> {m.inDemo} <a href="#demo">Open the demo</a>
                    </span>
                  </p>
                </Tabs.Content>
              ))}
            </XPExplorerPane>
          </Tabs.Root>
        </XPWindow>
      </div>
    </section>
  );
}
