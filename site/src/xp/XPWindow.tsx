import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
} from "react";
import { animate, motion, useMotionValue, useReducedMotion } from "motion/react";
import { XPIcon, type XPIconName } from "./icons";
import { useScrollable } from "../lib/useScrollable";
import "./window.css";

export interface WindowRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export type WindowControl = "minimize" | "maximize" | "close";

interface XPWindowProps {
  /** Used for the element id and for tests: `win-cockpit`. */
  id: string;
  title: string;
  icon: XPIconName;
  /**
   * floating: dragged around a desktop. stacked: a block in a column (phones). static: a plain framed panel that
   * lives in the page and has no window behaviour at all.
   */
  layout: "floating" | "stacked" | "static";
  active?: boolean;
  minimized?: boolean;
  maximized?: boolean;
  rect?: WindowRect;
  zIndex?: number;
  /** The usable desktop, which a dragged window cannot leave. */
  bounds?: { w: number; h: number };
  controls?: WindowControl[];
  onFocus?: () => void;
  onMinimize?: () => void;
  onMaximize?: () => void;
  onClose?: () => void;
  onMove?: (x: number, y: number) => void;
  /** Where the taskbar button for this window is, so minimizing can fly toward it. */
  getTaskbarTarget?: () => DOMRect | null;
  menu?: ReactNode;
  status?: ReactNode[];
  className?: string;
  bodyClassName?: string;
  children: ReactNode;
}

const KEY_STEP = 16;
const KEY_STEP_BIG = 64;

const clamp = (n: number, lo: number, hi: number) => Math.min(Math.max(n, lo), Math.max(lo, hi));

export const windowDomId = (id: string) => `win-${id}`;

export function XPWindow(props: XPWindowProps) {
  const {
    id,
    layout,
    active = false,
    minimized = false,
    controls = ["minimize", "maximize", "close"],
    menu,
    status,
    className = "",
    bodyClassName = "",
    children,
  } = props;
  const titleId = useId();
  const reduced = useReducedMotion();
  const { ref: bodyRef, scrollable } = useScrollable<HTMLDivElement>();
  // On a phone a minimized window folds up to its title bar; its contents must leave the tab order too.
  const collapsed = layout === "stacked" && minimized;

  const chrome = (
    <>
      <TitleBar {...props} titleId={titleId} controls={controls} reduced={Boolean(reduced)} />
      <div className="xp-win__collapse">
        <div
          className="xp-win__collapse-inner"
          inert={collapsed ? true : undefined}
          aria-hidden={collapsed ? true : undefined}
        >
          {menu}
          <div
            ref={bodyRef}
            className={`xp-win__body ${bodyClassName}`}
            {...(scrollable ? { tabIndex: 0, role: "region", "aria-label": `${props.title}, scrollable` } : {})}
          >
            {children}
          </div>
          {status && status.length > 0 ? (
            <div className="status-bar xp-win__status">
              {status.map((field, i) => (
                <p className="status-bar-field" key={i}>
                  {field}
                </p>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </>
  );

  if (layout === "static") {
    return (
      <div
        className={`xp-win xp-win--static ${active ? "is-active" : ""} ${className}`}
        role="group"
        aria-labelledby={titleId}
        id={windowDomId(id)}
      >
        <div className="window xp-win__frame">{chrome}</div>
      </div>
    );
  }

  if (layout === "stacked") {
    return (
      <StackedWindow {...props} titleId={titleId} controls={controls}>
        {chrome}
      </StackedWindow>
    );
  }

  return (
    <FloatingWindow {...props} titleId={titleId} controls={controls} reduced={Boolean(reduced)}>
      {chrome}
    </FloatingWindow>
  );
}

type TitleProps = XPWindowProps & { titleId: string; controls: WindowControl[]; reduced?: boolean };

function TitleBar(props: TitleProps) {
  const { title, icon, titleId, layout, active, maximized, controls, onMinimize, onMaximize, onClose, onMove, bounds } =
    props;
  const draggable = layout === "floating" && !maximized;
  const x = props.rect?.x ?? 0;
  const y = props.rect?.y ?? 0;

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!draggable || !onMove || !bounds || e.target !== e.currentTarget) return;
    const step = e.shiftKey ? KEY_STEP_BIG : KEY_STEP;
    const w = props.rect?.w ?? 0;
    const h = props.rect?.h ?? 0;
    let nx = x;
    let ny = y;
    if (e.key === "ArrowLeft") nx -= step;
    else if (e.key === "ArrowRight") nx += step;
    else if (e.key === "ArrowUp") ny -= step;
    else if (e.key === "ArrowDown") ny += step;
    else return;
    e.preventDefault();
    onMove(clamp(nx, 0, bounds.w - w), clamp(ny, 0, bounds.h - h));
  };

  return (
    <div
      className={`title-bar xp-win__title ${active ? "" : "is-inactive"}`}
      data-drag-handle={draggable ? "" : undefined}
      tabIndex={draggable ? 0 : undefined}
      onKeyDown={onKeyDown}
      onDoubleClick={(e) => {
        if (layout === "floating" && controls.includes("maximize") && !(e.target as HTMLElement).closest("button")) {
          onMaximize?.();
        }
      }}
      aria-label={draggable ? `${title} title bar. Use the arrow keys to move the window.` : undefined}
    >
      <div className="title-bar-text xp-win__title-text">
        <XPIcon name={icon} size={18} />
        <span id={titleId}>{title}</span>
      </div>
      {layout !== "static" ? (
        <div className="title-bar-controls">
          {controls.includes("minimize") ? <button type="button" aria-label="Minimize" onClick={onMinimize} /> : null}
          {controls.includes("maximize") && layout === "floating" ? (
            <button type="button" aria-label={maximized ? "Restore" : "Maximize"} onClick={onMaximize} />
          ) : null}
          {controls.includes("close") ? <button type="button" aria-label="Close" onClick={onClose} /> : null}
        </div>
      ) : null}
    </div>
  );
}

function StackedWindow(props: TitleProps & { children: ReactNode }) {
  const { id, titleId, minimized = false, active, onFocus, className = "", children } = props;
  return (
    <div
      id={windowDomId(id)}
      className={`xp-win xp-win--stacked ${active ? "is-active" : ""} ${minimized ? "is-minimized" : ""} ${className}`}
      role="group"
      aria-labelledby={titleId}
      tabIndex={-1}
      data-window={id}
      onPointerDownCapture={onFocus}
    >
      <div className="window xp-win__frame">{children}</div>
    </div>
  );
}

function FloatingWindow(props: TitleProps & { children: ReactNode }) {
  const {
    id,
    titleId,
    active,
    minimized = false,
    maximized = false,
    rect = { x: 0, y: 0, w: 480, h: 360 },
    zIndex = 1,
    bounds,
    onFocus,
    onMove,
    getTaskbarTarget,
    className = "",
    reduced,
    children,
  } = props;

  const x = useMotionValue(rect.x);
  const y = useMotionValue(rect.y);
  const w = useMotionValue(rect.w);
  const h = useMotionValue(rect.h);
  const outer = useRef<HTMLDivElement>(null);
  const inner = useRef<HTMLDivElement>(null);
  const drag = useRef<{ px: number; py: number; ox: number; oy: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const [hidden, setHidden] = useState(minimized);

  // Follow the layout: moves, maximizing and resizes of the desktop animate; drags are already in place.
  useEffect(() => {
    if (drag.current) return;
    const t = reduced ? { duration: 0 } : { duration: 0.2, ease: [0.2, 0.7, 0.2, 1] as const };
    const controls = [animate(x, rect.x, t), animate(y, rect.y, t), animate(w, rect.w, t), animate(h, rect.h, t)];
    return () => controls.forEach((c) => c.stop());
  }, [rect.x, rect.y, rect.w, rect.h, reduced, x, y, w, h]);

  // Minimize flies the window toward its taskbar button; restore flies it back. Only a change of the `minimized`
  // flag animates, so re-renders (and React's double effects in development) never replay it.
  const targetRef = useRef(getTaskbarTarget);
  targetRef.current = getTaskbarTarget;
  const wasMinimized = useRef(minimized);
  useLayoutEffect(() => {
    const node = inner.current;
    const box = outer.current;
    if (!node || !box) return;
    if (wasMinimized.current === minimized) {
      node.style.visibility = minimized ? "hidden" : "visible";
      return;
    }
    wasMinimized.current = minimized;
    if (minimized) {
      const target = targetRef.current?.();
      const from = box.getBoundingClientRect();
      const dx = target ? target.left + target.width / 2 - (from.left + from.width / 2) : 0;
      const dy = target ? target.top + target.height / 2 - (from.top + from.height / 2) : 40;
      const run = animate(
        node,
        { x: dx, y: dy, scale: 0.15, opacity: 0 },
        { duration: reduced ? 0 : 0.24, ease: [0.4, 0, 0.8, 0.4] },
      );
      run.then(() => {
        node.style.visibility = "hidden";
        setHidden(true);
      });
      return () => run.stop();
    }
    node.style.visibility = "visible";
    setHidden(false);
    const run = animate(
      node,
      { x: 0, y: 0, scale: 1, opacity: 1 },
      { duration: reduced ? 0 : 0.22, ease: [0.2, 0.7, 0.2, 1] },
    );
    return () => run.stop();
  }, [minimized, reduced]);

  const onPointerDown = useCallback(
    (e: PointerEvent<HTMLDivElement>) => {
      if (e.button !== 0 || maximized) return;
      const handle = (e.target as HTMLElement).closest("[data-drag-handle]");
      if (!handle || (e.target as HTMLElement).closest("button")) return;
      e.currentTarget.setPointerCapture(e.pointerId);
      drag.current = { px: e.clientX, py: e.clientY, ox: x.get(), oy: y.get() };
      setDragging(true);
    },
    [maximized, x, y],
  );

  const onPointerMove = useCallback(
    (e: PointerEvent<HTMLDivElement>) => {
      const d = drag.current;
      if (!d || !bounds) return;
      x.set(clamp(d.ox + e.clientX - d.px, 0, bounds.w - w.get()));
      y.set(clamp(d.oy + e.clientY - d.py, 0, bounds.h - h.get()));
    },
    [bounds, x, y, w, h],
  );

  const endDrag = useCallback(() => {
    if (!drag.current) return;
    drag.current = null;
    setDragging(false);
    onMove?.(Math.round(x.get()), Math.round(y.get()));
  }, [onMove, x, y]);

  return (
    <motion.div
      ref={outer}
      id={windowDomId(id)}
      className={`xp-win xp-win--floating ${active ? "is-active" : ""} ${dragging ? "is-dragging" : ""} ${maximized ? "is-maximized" : ""} ${className}`}
      role="group"
      aria-labelledby={titleId}
      tabIndex={-1}
      data-window={id}
      style={{
        x,
        y,
        width: w,
        height: h,
        zIndex,
        pointerEvents: minimized ? "none" : undefined,
        visibility: hidden && minimized ? "hidden" : undefined,
      }}
      initial={reduced ? false : { opacity: 0, scale: 0.94 }}
      animate={{ opacity: 1, scale: 1 }}
      exit={reduced ? { opacity: 0 } : { opacity: 0, scale: 0.94 }}
      transition={{ duration: reduced ? 0 : 0.16 }}
      onPointerDownCapture={onFocus}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onLostPointerCapture={endDrag}
      inert={hidden && minimized ? true : undefined}
      aria-hidden={hidden && minimized ? true : undefined}
    >
      <div ref={inner} className="xp-win__fly">
        <div className="window xp-win__frame">{children}</div>
      </div>
    </motion.div>
  );
}
