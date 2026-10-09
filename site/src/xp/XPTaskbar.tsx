import { useEffect, useState, type ReactNode } from "react";
import { XPIcon, type XPIconName } from "./icons";
import { XPTooltip } from "./controls";
import "./taskbar.css";

export interface TaskbarWindow {
  id: string;
  title: string;
  icon: XPIconName;
  /** This is the window with the focus. */
  active: boolean;
  minimized: boolean;
}

interface XPTaskbarProps {
  /** The Start button (an XPStartMenu). */
  start: ReactNode;
  windows: TaskbarWindow[];
  onWindowClick: (id: string) => void;
  /** Lets the desktop find a button, so a minimizing window can fly toward it. */
  registerButton: (id: string, el: HTMLButtonElement | null) => void;
  soundMuted: boolean;
  onToggleSound: () => void;
  onAbout: () => void;
}

function useClock(): string {
  const format = () =>
    new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(new Date());
  const [text, setText] = useState(format);
  useEffect(() => {
    const t = window.setInterval(() => setText(format()), 15_000);
    return () => window.clearInterval(t);
  }, []);
  return text;
}

export function XPTaskbar({
  start,
  windows,
  onWindowClick,
  registerButton,
  soundMuted,
  onToggleSound,
  onAbout,
}: XPTaskbarProps) {
  const clock = useClock();
  return (
    <div className="xp-taskbar" role="group" aria-label="Taskbar">
      {start}
      <div className="xp-taskbar__windows" role="group" aria-label="Open windows">
        {windows.map((w) => (
          <button
            key={w.id}
            type="button"
            ref={(el) => registerButton(w.id, el)}
            className={`xp-taskbtn ${w.active && !w.minimized ? "is-active" : ""} ${w.minimized ? "is-minimized" : ""}`}
            aria-pressed={w.active && !w.minimized}
            onClick={() => onWindowClick(w.id)}
            data-taskbar={w.id}
          >
            <XPIcon name={w.icon} size={18} />
            <span className="xp-taskbtn__label">{w.title}</span>
            {w.minimized ? <span className="visually-hidden"> (minimized)</span> : null}
          </button>
        ))}
      </div>
      <p className="xp-taskbar__badge" data-testid="sim-badge">
        <XPIcon name="info" size={16} />
        <span>Simulated demo</span>
      </p>
      <div className="xp-tray" role="group" aria-label="Notification area">
        <XPTooltip label="About this simulation">
          <button type="button" className="xp-tray__btn" onClick={onAbout} aria-label="About this simulation">
            <XPIcon name="info" size={18} />
          </button>
        </XPTooltip>
        <XPTooltip label={soundMuted ? "Sound is off. Turn it on." : "Sound is on. Turn it off."}>
          <button
            type="button"
            className="xp-tray__btn"
            aria-pressed={!soundMuted}
            aria-label="Sound effects"
            onClick={onToggleSound}
          >
            <XPIcon name={soundMuted ? "speaker-off" : "speaker"} size={18} />
          </button>
        </XPTooltip>
        <span className="xp-tray__clock" aria-label="Clock">
          {clock}
        </span>
      </div>
    </div>
  );
}
