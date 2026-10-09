import { useCallback, useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState, type ReactNode } from "react";
import { AnimatePresence, useReducedMotion } from "motion/react";
import { Wallpaper } from "../xp/Wallpaper";
import { XPWindow, windowDomId } from "../xp/XPWindow";
import { XPTaskbar } from "../xp/XPTaskbar";
import { XPStartMenu, type StartEntry } from "../xp/XPStartMenu";
import { XPBalloonNotice, type BalloonNotice } from "../xp/XPBalloonNotice";
import { XPIcon, type XPIconName } from "../xp/icons";
import { BootSplash } from "./BootSplash";
import { useDemo } from "../demo/useDemo";
import { CockpitMenu } from "../demo/CockpitMenu";
import {
  CockpitBody,
  EvidenceBody,
  GuideBody,
  LoginsBody,
  NotesBody,
  RunToolbar,
  statusFields,
  type CockpitTab,
} from "../demo/views";
import { limitHasHappened } from "../demo/engine";
import { playCue } from "../lib/sound";
import { TRY_DEMO_EVENT } from "../lib/events";
import { release, REPO_URL } from "../content/release";
import { links } from "../content/copy";
import {
  WIN_IDS,
  computeLayout,
  fromLayout,
  reduce,
  taskbarWindows,
  type WinId,
} from "./windows";
import "./desktop.css";

const META: Record<WinId, { title: string; short: string; icon: XPIconName }> = {
  cockpit: { title: "Legatus Cockpit · Simulated demo", short: "Cockpit", icon: "cockpit" },
  logins: { title: "Logins", short: "Logins", icon: "logins" },
  guide: { title: "Getting started", short: "Getting started", icon: "book" },
  evidence: { title: "evidence.md · Report", short: "evidence.md", icon: "report" },
  notes: { title: "Verification notes", short: "Verification notes", icon: "status" },
};

const ABOUT_TEXT =
  "This desktop is a scripted simulation that runs in your browser. It does not run Legatus or an agent, and its names, files and hashes are sample data. To see the real engine do the same thing, run legatus demo.";

/** Notices from the engine and notices the desktop makes up for itself share one balloon. */
const ENGINE_NOTICE_BASE = 100;
const LOCAL_NOTICE_BASE = 1000;

function initialLayout() {
  const width = typeof window === "undefined" ? 1240 : Math.min(window.innerWidth - 48, 1240);
  return computeLayout(width, 660);
}

export function Desktop() {
  const reduced = useReducedMotion();
  const stage = useRef<HTMLDivElement>(null);
  const buttons = useRef(new Map<string, HTMLButtonElement>());
  const { state: demo, actions } = useDemo();
  const [desk, dispatch] = useReducer(reduce, undefined, () => fromLayout(initialLayout()));
  const [tab, setTab] = useState<CockpitTab>("overview");
  const [reportOpened, setReportOpened] = useState(false);
  const [muted, setMuted] = useState(true);
  const [booting, setBooting] = useState(false);
  const [balloon, setBalloon] = useState<BalloonNotice | null>(null);
  const localNotice = useRef(LOCAL_NOTICE_BASE);

  /* ----- size: lay the windows out for the room there is, and keep them inside it ----- */
  const modeRef = useRef(desk.mode);
  modeRef.current = desk.mode;
  useLayoutEffect(() => {
    const el = stage.current;
    if (!el) return;
    let first = true;
    const measure = () => {
      const { width, height } = el.getBoundingClientRect();
      const layout = computeLayout(width, height);
      if (first || layout.mode !== modeRef.current) {
        first = false;
        dispatch({ type: "layout", layout });
      } else if (layout.mode === "floating") {
        dispatch({ type: "bounds", bounds: layout.bounds });
      }
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  /* ----- window helpers ----- */
  const focusWindow = useCallback(
    (id: WinId) => {
      requestAnimationFrame(() => {
        const el = document.getElementById(windowDomId(id));
        el?.focus({ preventScroll: true });
        if (el && modeRef.current === "stacked") {
          el.scrollIntoView({ block: "nearest", behavior: reduced ? "auto" : "smooth" });
        }
      });
    },
    [reduced],
  );

  const openWindow = useCallback(
    (id: WinId) => {
      dispatch({ type: "open", id });
      if (id === "evidence") setReportOpened(true);
      focusWindow(id);
    },
    [focusWindow],
  );

  const closeWindow = useCallback((id: WinId) => dispatch({ type: "close", id }), []);

  const onTaskbar = useCallback(
    (id: string) => {
      const win = id as WinId;
      dispatch({ type: "taskbar", id: win });
      if (win === "evidence") setReportOpened(true);
      focusWindow(win);
    },
    [focusWindow],
  );

  const registerButton = useCallback((id: string, el: HTMLButtonElement | null) => {
    if (el) buttons.current.set(id, el);
    else buttons.current.delete(id);
  }, []);

  const targetFor = useMemo(() => {
    const out = {} as Record<WinId, () => DOMRect | null>;
    for (const id of WIN_IDS) out[id] = () => buttons.current.get(id)?.getBoundingClientRect() ?? null;
    return out;
  }, []);

  /* ----- balloon notices: from the engine's story, and from the desktop itself ----- */
  const showLocal = useCallback((n: Omit<BalloonNotice, "id">) => {
    localNotice.current += 1;
    setBalloon({ ...n, id: localNotice.current });
  }, []);

  const about = useCallback(
    () => showLocal({ tone: "info", title: "Simulated demo", text: ABOUT_TEXT }),
    [showLocal],
  );

  const seenNotices = useRef(0);
  useEffect(() => {
    if (demo.notices.length < seenNotices.current) seenNotices.current = 0; // a replay starts the story again
    const fresh = demo.notices.slice(seenNotices.current);
    seenNotices.current = demo.notices.length;
    const latest = fresh.at(-1);
    if (!latest) return;
    const n: BalloonNotice = {
      id: ENGINE_NOTICE_BASE + latest.id + (demo.startedAt % 1000) * 10,
      tone: latest.tone,
      title: latest.title,
      text: latest.text,
      ...(latest.sticky ? { sticky: true } : {}),
    };
    if (latest.tone === "success") {
      n.actionLabel = "Open the report";
      n.onAction = () => {
        openWindow("evidence");
        setBalloon(null);
      };
    }
    setBalloon(n);
  }, [demo.notices, demo.startedAt, openWindow]);

  // A short welcome, once, for people who have not started the demo. It never blocks anything.
  useEffect(() => {
    const t = window.setTimeout(() => {
      setBalloon((current) =>
        current
          ? current
          : {
              id: LOCAL_NOTICE_BASE,
              tone: "info",
              title: "Simulated demo",
              text: "Press Start in the cockpit to watch a usage limit get handled. Nothing here runs a real agent.",
              actionLabel: "Start the demo",
              onAction: () => {
                actions.play();
                setBalloon(null);
              },
            },
      );
    }, 1800);
    return () => window.clearTimeout(t);
  }, [actions]);

  /* ----- sound: muted until a person turns it on ----- */
  const playedLines = useRef(0);
  useEffect(() => {
    if (demo.journal.length < playedLines.current) playedLines.current = 0;
    const fresh = demo.journal.slice(playedLines.current);
    playedLines.current = demo.journal.length;
    if (muted) return;
    for (const line of fresh) if (line.cue) playCue(line.cue);
  }, [demo.journal, muted]);

  const toggleSound = useCallback(() => {
    setMuted((m) => {
      if (m) playCue("on");
      return !m;
    });
  }, []);

  /* ----- the demo, from the Start menu and the balloons ----- */
  const tryDemo = useCallback(() => {
    openWindow("cockpit");
    setTab("overview");
    if (demo.phase === "idle" || demo.phase === "paused") actions.play();
  }, [actions, demo.phase, openWindow]);

  // The hero's "Try the demo" button lives outside the desktop and asks for the same thing.
  const tryRef = useRef(tryDemo);
  tryRef.current = tryDemo;
  useEffect(() => {
    const handler = () => tryRef.current();
    window.addEventListener(TRY_DEMO_EVENT, handler);
    return () => window.removeEventListener(TRY_DEMO_EVENT, handler);
  }, []);

  const restartBoot = useCallback(() => setBooting(true), []);
  const bootDone = useCallback(() => {
    setBooting(false);
    document.querySelector<HTMLElement>(".xp-start")?.focus();
  }, []);

  /* ----- Start menu ----- */
  const pinned: StartEntry[] = [
    { id: "try", icon: "play", title: "Try the demo", subtitle: "Start the sample task", onSelect: tryDemo },
    { id: "cockpit", icon: "cockpit", title: "Cockpit", subtitle: "Steps, journal, checks, review", onSelect: () => openWindow("cockpit") },
    { id: "install", icon: "download", title: "Download Legatus", subtitle: `${release.tag} · pre-release`, href: "#install" },
    { id: "status", icon: "status", title: "What is verified", subtitle: "Read it before you rely on it", href: "#status" },
  ];
  const places: StartEntry[] = [
    { id: "how", icon: "queue", title: "How it works", href: "#how" },
    { id: "source", icon: "branch", title: "Source on GitHub", href: REPO_URL },
    { id: "releases", icon: "download", title: "Releases", href: release.allReleasesUrl },
    { id: "design", icon: "book", title: "Design notes", href: links.design },
    { id: "security", icon: "lock", title: "Security policy", href: links.security },
  ];
  const programs: StartEntry[] = WIN_IDS.map((id) => ({
    id,
    icon: META[id].icon,
    title: META[id].short,
    onSelect: () => openWindow(id),
  }));

  /* ----- rendering ----- */
  const open = WIN_IDS.filter((id) => desk.wins[id].open);

  const windowContent = (id: WinId): { menu?: ReactNode; status?: ReactNode[]; body: ReactNode; bodyClassName?: string } => {
    switch (id) {
      case "cockpit":
        return {
          menu: (
            <>
              <CockpitMenu
                state={demo}
                actions={actions}
                tab={tab}
                onTab={setTab}
                openWindow={openWindow}
                closeWindow={closeWindow}
                about={about}
              />
              <RunToolbar state={demo} actions={actions} />
            </>
          ),
          status: statusFields(demo),
          body: <CockpitBody state={demo} tab={tab} onTab={setTab} openWindow={openWindow} />,
          bodyClassName: "body--flush",
        };
      case "logins":
        return { body: <LoginsBody state={demo} />, bodyClassName: "body--tight", status: [`${Object.values(demo.logins).filter((l) => l.status === "limited").length} at a limit`, "Never sees a password"] };
      case "guide":
        return { body: <GuideBody state={demo} reportOpened={reportOpened} />, bodyClassName: "body--tight" };
      case "evidence":
        return { body: <EvidenceBody state={demo} />, status: ["Sample report", "Nothing was pushed or published"], bodyClassName: "body--flush" };
      case "notes":
        return { body: <NotesBody />, status: ["From docs/STATUS.md"] };
    }
  };

  const area = (
    <div className="desktop__area" aria-label="Simulated desktop windows">
      <AnimatePresence>
        {open.map((id) => {
          const w = desk.wins[id];
          const content = windowContent(id);
          return (
            <XPWindow
              key={id}
              id={id}
              title={META[id].title}
              icon={META[id].icon}
              layout={desk.mode}
              active={desk.active === id}
              minimized={w.minimized}
              maximized={w.maximized}
              rect={w.rect}
              zIndex={w.z}
              bounds={desk.bounds}
              menu={content.menu}
              status={content.status}
              bodyClassName={content.bodyClassName}
              getTaskbarTarget={targetFor[id]}
              onFocus={() => dispatch({ type: "focus", id })}
              onMinimize={() => dispatch({ type: "minimize", id })}
              onMaximize={() => dispatch({ type: "maximize", id })}
              onClose={() => dispatch({ type: "close", id })}
              onMove={(x, y) => dispatch({ type: "move", id, x, y })}
            >
              {content.body}
            </XPWindow>
          );
        })}
      </AnimatePresence>
    </div>
  );

  return (
    <div
      ref={stage}
      className={`desktop desktop--${desk.mode} ${booting ? "is-booting" : ""}`}
      data-mode={desk.mode}
      data-phase={demo.phase}
      data-limit={limitHasHappened(demo) ? "hit" : "none"}
    >
      <Wallpaper />
      {desk.mode === "stacked" ? (
        <p className="desktop__banner" data-testid="sim-banner">
          <XPIcon name="info" size={16} /> <strong>Simulated demo</strong> · scripted in your browser
        </p>
      ) : null}
      <DesktopIcons onOpen={openWindow} stacked={desk.mode === "stacked"} />
      {area}
      <div className="desktop__balloon">
        <XPBalloonNotice notice={balloon} onDismiss={() => setBalloon(null)} />
      </div>
      <XPTaskbar
        start={
          <XPStartMenu
            pinned={pinned}
            places={places}
            programs={programs}
            soundMuted={muted}
            onToggleSound={toggleSound}
            onRestart={restartBoot}
          />
        }
        windows={taskbarWindows(desk).map((id) => ({
          id,
          title: META[id].short,
          icon: META[id].icon,
          active: desk.active === id,
          minimized: desk.wins[id].minimized,
        }))}
        onWindowClick={onTaskbar}
        registerButton={registerButton}
        soundMuted={muted}
        onToggleSound={toggleSound}
        onAbout={about}
      />
      <AnimatePresence>{booting ? <BootSplash onDone={bootDone} /> : null}</AnimatePresence>
    </div>
  );
}

function DesktopIcons({ onOpen, stacked }: { onOpen: (id: WinId) => void; stacked: boolean }) {
  const items: { id: WinId; label: string; icon: XPIconName }[] = [
    { id: "cockpit", label: "Cockpit", icon: "cockpit" },
    { id: "logins", label: "Logins", icon: "logins" },
    { id: "guide", label: "Getting started", icon: "book" },
    { id: "evidence", label: "evidence.md", icon: "report" },
    { id: "notes", label: "Verification notes", icon: "status" },
  ];
  return (
    <ul className={`deskicons ${stacked ? "deskicons--row" : ""}`} aria-label="Desktop icons">
      {items.map((it) => (
        <li key={it.id}>
          <button type="button" className="deskicon" onClick={() => onOpen(it.id)} data-icon={it.id}>
            <XPIcon name={it.icon} size={stacked ? 36 : 40} />
            <span>{it.label}</span>
          </button>
        </li>
      ))}
      <li>
        <a className="deskicon" href="#install">
          <XPIcon name="download" size={stacked ? 36 : 40} />
          <span>Install Legatus</span>
        </a>
      </li>
    </ul>
  );
}
