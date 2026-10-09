import * as Menu from "@radix-ui/react-dropdown-menu";
import { XPIcon, type XPIconName } from "./icons";
import "./startmenu.css";

export interface StartEntry {
  id: string;
  icon: XPIconName;
  title: string;
  subtitle?: string;
  /** An action inside the page. */
  onSelect?: () => void;
  /** A link: same-page anchors stay in the tab, anything else opens in a new one. */
  href?: string;
}

interface XPStartMenuProps {
  pinned: StartEntry[];
  places: StartEntry[];
  programs: StartEntry[];
  soundMuted: boolean;
  onToggleSound: () => void;
  onRestart: () => void;
}

function Entry({ entry, big }: { entry: StartEntry; big?: boolean }) {
  const inner = (
    <>
      <XPIcon name={entry.icon} size={big ? 32 : 20} />
      <span className="xp-sm-item__text">
        <span className="xp-sm-item__title">{entry.title}</span>
        {entry.subtitle ? <span className="xp-sm-item__sub">{entry.subtitle}</span> : null}
      </span>
    </>
  );
  const cls = `xp-sm-item ${big ? "xp-sm-item--big" : ""}`;
  if (entry.href) {
    const external = !entry.href.startsWith("#");
    return (
      <Menu.Item asChild className={cls}>
        <a href={entry.href} {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}>
          {inner}
        </a>
      </Menu.Item>
    );
  }
  return (
    <Menu.Item className={cls} onSelect={entry.onSelect}>
      {inner}
    </Menu.Item>
  );
}

/**
 * The Start menu: a Radix dropdown (so arrow keys, type-ahead, Escape and focus return all work) dressed as the
 * two-column Luna menu. "All Programs" is a real submenu.
 */
export function XPStartMenu({ pinned, places, programs, soundMuted, onToggleSound, onRestart }: XPStartMenuProps) {
  return (
    <Menu.Root modal={false}>
      <Menu.Trigger className="xp-start" aria-label="Start">
        <XPIcon name="legatus" size={26} className="xp-start__logo" />
        <span className="xp-start__label" aria-hidden="true">
          start
        </span>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="xp-startmenu" side="top" align="start" sideOffset={0} collisionPadding={8} loop>
          <div className="xp-startmenu__head">
            <XPIcon name="legatus" size={44} />
            <div>
              <strong>Legatus</strong>
              <span>Command center for coding agents</span>
            </div>
          </div>
          <div className="xp-startmenu__cols">
            <div className="xp-startmenu__left">
              {pinned.map((e) => (
                <Entry key={e.id} entry={e} big />
              ))}
              <Menu.Separator className="xp-sm-sep" />
              <Menu.Sub>
                <Menu.SubTrigger className="xp-sm-item xp-sm-item--programs">
                  <XPIcon name="all-programs" size={22} />
                  <span className="xp-sm-item__text">
                    <span className="xp-sm-item__title">All windows</span>
                  </span>
                  <span className="xp-sm-item__arrow" aria-hidden="true">
                    ▶
                  </span>
                </Menu.SubTrigger>
                <Menu.Portal>
                  <Menu.SubContent className="xp-startmenu xp-startmenu--sub" sideOffset={2} alignOffset={-6} collisionPadding={8}>
                    {programs.map((e) => (
                      <Entry key={e.id} entry={e} />
                    ))}
                  </Menu.SubContent>
                </Menu.Portal>
              </Menu.Sub>
            </div>
            <div className="xp-startmenu__right">
              {places.map((e) => (
                <Entry key={e.id} entry={e} />
              ))}
            </div>
          </div>
          <div className="xp-startmenu__foot">
            <Menu.CheckboxItem className="xp-sm-foot" checked={!soundMuted} onCheckedChange={onToggleSound}>
              <XPIcon name={soundMuted ? "speaker-off" : "speaker"} size={20} />
              <span>Sound effects: {soundMuted ? "off" : "on"}</span>
            </Menu.CheckboxItem>
            <Menu.Item className="xp-sm-foot" onSelect={onRestart}>
              <XPIcon name="power" size={20} />
              <span>Replay boot animation</span>
            </Menu.Item>
          </div>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
