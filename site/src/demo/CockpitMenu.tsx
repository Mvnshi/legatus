import {
  XPMenu,
  XPMenuBar,
  XPMenuItem,
  XPMenuLink,
  XPMenuRadioGroup,
  XPMenuRadioItem,
  XPMenuSeparator,
} from "../xp/controls";
import { canInjectLimit, type DemoState } from "./engine";
import type { DemoActions } from "./useDemo";
import { COCKPIT_TABS, type CockpitTab } from "./views";
import { links } from "../content/copy";
import type { WinId } from "../desktop/windows";

/** The cockpit's menu bar. Every item does something real in the demo. */
export function CockpitMenu({
  state,
  actions,
  tab,
  onTab,
  openWindow,
  closeWindow,
  about,
}: {
  state: DemoState;
  actions: DemoActions;
  tab: CockpitTab;
  onTab: (t: CockpitTab) => void;
  openWindow: (id: WinId) => void;
  closeWindow: (id: WinId) => void;
  about: () => void;
}) {
  const canPlay = state.phase === "idle" || state.phase === "paused";
  return (
    <XPMenuBar label="Cockpit menu">
      <XPMenu label="File">
        <XPMenuItem icon="replay" disabled={state.phase === "idle"} onSelect={actions.replay}>
          Replay the run
        </XPMenuItem>
        <XPMenuItem icon="report" onSelect={() => openWindow("evidence")}>
          Open evidence.md
        </XPMenuItem>
        <XPMenuSeparator />
        <XPMenuItem onSelect={() => closeWindow("cockpit")}>Close cockpit</XPMenuItem>
      </XPMenu>
      <XPMenu label="View">
        <XPMenuRadioGroup value={tab} onValueChange={(v) => onTab(v as CockpitTab)}>
          {COCKPIT_TABS.map((t) => (
            <XPMenuRadioItem key={t.id} value={t.id}>
              {t.label}
            </XPMenuRadioItem>
          ))}
        </XPMenuRadioGroup>
        <XPMenuSeparator />
        <XPMenuItem icon="logins" onSelect={() => openWindow("logins")}>
          Show logins
        </XPMenuItem>
      </XPMenu>
      <XPMenu label="Run">
        <XPMenuItem icon="play" disabled={!canPlay} onSelect={actions.play}>
          {state.phase === "paused" ? "Resume" : "Start"}
        </XPMenuItem>
        <XPMenuItem icon="pause" disabled={state.phase !== "running"} onSelect={actions.pause}>
          Pause
        </XPMenuItem>
        <XPMenuItem icon="limit" disabled={!canInjectLimit(state)} onSelect={actions.injectLimit}>
          Inject usage limit
        </XPMenuItem>
      </XPMenu>
      <XPMenu label="Help">
        <XPMenuItem icon="info" onSelect={about}>
          About this simulation
        </XPMenuItem>
        <XPMenuItem icon="status" onSelect={() => openWindow("notes")}>
          What is verified
        </XPMenuItem>
        <XPMenuSeparator />
        <XPMenuLink href={links.status}>Read docs/STATUS.md</XPMenuLink>
        <XPMenuLink href={links.repo}>Legatus on GitHub</XPMenuLink>
      </XPMenu>
    </XPMenuBar>
  );
}
