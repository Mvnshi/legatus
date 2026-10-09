/**
 * Window management for the demo desktop, kept free of React so the rules can be tested exactly.
 *
 * Positions are in desktop pixels, measured from the top-left of the usable desktop (the area above the
 * taskbar). A window can never leave that area: dragging is clamped, and a desktop that gets smaller pulls
 * its windows back in.
 */

export type WinId = "cockpit" | "logins" | "guide" | "evidence" | "notes";

export const WIN_IDS: WinId[] = ["cockpit", "logins", "guide", "evidence", "notes"];

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface Size {
  w: number;
  h: number;
}

export interface Win {
  id: WinId;
  open: boolean;
  minimized: boolean;
  maximized: boolean;
  rect: Rect;
  /** Where the window goes back to when it stops being maximized. */
  restoreRect: Rect | null;
  z: number;
}

export type Mode = "floating" | "stacked";

export interface DesktopState {
  mode: Mode;
  /** The usable desktop, excluding the taskbar. */
  bounds: Size;
  wins: Record<WinId, Win>;
  active: WinId | null;
  zTop: number;
}

export const TASKBAR_HEIGHT = 44;
/** Below this width the windows stack in a column and cannot be dragged. */
export const STACK_BREAKPOINT = 900;
const MARGIN = 16;
const LEFT_COLUMN = 296;
const ICON_COLUMN = 104;

export interface Layout {
  mode: Mode;
  bounds: Size;
  rects: Record<WinId, Rect>;
  open: WinId[];
  minimized: WinId[];
}

const clamp = (n: number, lo: number, hi: number) => Math.min(Math.max(n, lo), Math.max(lo, hi));

export function clampRect(r: Rect, bounds: Size): Rect {
  const w = Math.min(r.w, bounds.w);
  const h = Math.min(r.h, bounds.h);
  return { w, h, x: clamp(r.x, 0, bounds.w - w), y: clamp(r.y, 0, bounds.h - h) };
}

/** Where every window starts for a desktop of this size. `height` includes the taskbar. */
export function computeLayout(width: number, height: number): Layout {
  const bounds: Size = { w: Math.max(0, width), h: Math.max(0, height - TASKBAR_HEIGHT) };
  const zero: Rect = { x: 0, y: 0, w: 0, h: 0 };
  if (width < STACK_BREAKPOINT) {
    return {
      mode: "stacked",
      bounds,
      rects: { cockpit: zero, logins: zero, guide: zero, evidence: zero, notes: zero },
      open: ["cockpit", "logins", "guide"],
      minimized: [],
    };
  }

  const wide = width >= 1100;
  const inner = bounds.h - MARGIN * 2;
  const leftX = (wide ? ICON_COLUMN : 0) + MARGIN;
  let rects: Record<WinId, Rect>;
  let minimized: WinId[] = [];

  if (wide) {
    const cockpitX = leftX + LEFT_COLUMN + MARGIN;
    const loginsH = Math.min(318, Math.max(260, Math.round(inner * 0.5)));
    rects = {
      logins: { x: leftX, y: MARGIN, w: LEFT_COLUMN, h: loginsH },
      guide: { x: leftX, y: MARGIN * 2 + loginsH, w: LEFT_COLUMN, h: inner - loginsH - MARGIN },
      cockpit: { x: cockpitX, y: MARGIN, w: Math.min(width - cockpitX - MARGIN, 820), h: inner },
      evidence: { x: 0, y: 0, w: 580, h: Math.min(480, inner) },
      notes: { x: 0, y: 0, w: 540, h: Math.min(440, inner) },
    };
  } else {
    // A medium desktop has room for the cockpit and little else; the other two windows wait on the taskbar.
    rects = {
      cockpit: { x: MARGIN, y: MARGIN, w: width - MARGIN * 2, h: inner },
      logins: { x: MARGIN * 3, y: MARGIN * 3, w: LEFT_COLUMN, h: Math.min(262, inner) },
      guide: { x: MARGIN * 5, y: MARGIN * 5, w: LEFT_COLUMN, h: Math.min(300, inner) },
      evidence: { x: 0, y: 0, w: 580, h: Math.min(480, inner) },
      notes: { x: 0, y: 0, w: 540, h: Math.min(440, inner) },
    };
    minimized = ["logins", "guide"];
  }

  // Windows that open later arrive centred.
  for (const id of ["evidence", "notes"] as const) {
    const r = rects[id];
    const w = Math.min(r.w, bounds.w - MARGIN * 2);
    rects[id] = { ...r, w, x: Math.round((bounds.w - w) / 2) + (id === "notes" ? -MARGIN : MARGIN), y: Math.round((bounds.h - r.h) / 2) };
  }
  for (const id of WIN_IDS) rects[id] = clampRect(rects[id], bounds);

  return { mode: "floating", bounds, rects, open: ["cockpit", "logins", "guide"], minimized };
}

export function fromLayout(layout: Layout): DesktopState {
  const wins = {} as Record<WinId, Win>;
  let z = 0;
  // Draw order: the cockpit ends up on top.
  const order: WinId[] = ["evidence", "notes", "guide", "logins", "cockpit"];
  for (const id of order) {
    z += 1;
    wins[id] = {
      id,
      open: layout.open.includes(id),
      minimized: layout.minimized.includes(id),
      maximized: false,
      rect: layout.rects[id],
      restoreRect: null,
      z,
    };
  }
  return { mode: layout.mode, bounds: layout.bounds, wins, active: "cockpit", zTop: z };
}

export type Action =
  | { type: "layout"; layout: Layout }
  | { type: "bounds"; bounds: Size }
  | { type: "open"; id: WinId }
  | { type: "close"; id: WinId }
  | { type: "minimize"; id: WinId }
  | { type: "restore"; id: WinId }
  | { type: "focus"; id: WinId }
  | { type: "taskbar"; id: WinId }
  | { type: "move"; id: WinId; x: number; y: number }
  | { type: "maximize"; id: WinId };

/** The visible window that is highest in the stack, if any. */
export function topVisible(state: DesktopState, except?: WinId): WinId | null {
  let best: Win | null = null;
  for (const id of WIN_IDS) {
    const w = state.wins[id];
    if (id === except || !w.open || w.minimized) continue;
    if (!best || w.z > best.z) best = w;
  }
  return best ? best.id : null;
}

function raise(state: DesktopState, id: WinId): DesktopState {
  const z = state.zTop + 1;
  return { ...state, zTop: z, active: id, wins: { ...state.wins, [id]: { ...state.wins[id], z } } };
}

function patch(state: DesktopState, id: WinId, p: Partial<Win>): DesktopState {
  return { ...state, wins: { ...state.wins, [id]: { ...state.wins[id], ...p } } };
}

function release(state: DesktopState, id: WinId): DesktopState {
  return state.active === id ? { ...state, active: topVisible(state, id) } : state;
}

export function reduce(state: DesktopState, action: Action): DesktopState {
  switch (action.type) {
    case "layout":
      return fromLayout(action.layout);

    case "bounds": {
      const bounds = action.bounds;
      let next: DesktopState = { ...state, bounds };
      for (const id of WIN_IDS) {
        const w = next.wins[id];
        const rect = w.maximized ? { x: 0, y: 0, w: bounds.w, h: bounds.h } : clampRect(w.rect, bounds);
        next = patch(next, id, { rect });
      }
      return next;
    }

    case "open": {
      const w = state.wins[action.id];
      const opened = patch(state, action.id, {
        open: true,
        minimized: false,
        rect: w.maximized ? { x: 0, y: 0, w: state.bounds.w, h: state.bounds.h } : clampRect(w.rect, state.bounds),
      });
      return raise(opened, action.id);
    }

    case "close": {
      const closed = patch(state, action.id, { open: false, minimized: false });
      return release(closed, action.id);
    }

    case "minimize": {
      const w = state.wins[action.id];
      if (!w.open || w.minimized) return state;
      return release(patch(state, action.id, { minimized: true }), action.id);
    }

    case "restore": {
      const w = state.wins[action.id];
      if (!w.open) return state;
      return raise(patch(state, action.id, { minimized: false }), action.id);
    }

    case "focus": {
      const w = state.wins[action.id];
      if (!w.open || w.minimized) return state;
      return state.active === action.id && w.z === state.zTop ? state : raise(state, action.id);
    }

    case "taskbar": {
      const w = state.wins[action.id];
      if (!w.open) return reduce(state, { type: "open", id: action.id });
      if (w.minimized) return reduce(state, { type: "restore", id: action.id });
      if (state.active === action.id) return reduce(state, { type: "minimize", id: action.id });
      return raise(state, action.id);
    }

    case "move": {
      const w = state.wins[action.id];
      if (w.maximized) return state;
      const rect = clampRect({ ...w.rect, x: action.x, y: action.y }, state.bounds);
      return rect.x === w.rect.x && rect.y === w.rect.y ? state : patch(state, action.id, { rect });
    }

    case "maximize": {
      const w = state.wins[action.id];
      if (w.maximized) {
        return patch(state, action.id, {
          maximized: false,
          rect: clampRect(w.restoreRect ?? w.rect, state.bounds),
          restoreRect: null,
        });
      }
      return patch(state, action.id, {
        maximized: true,
        restoreRect: w.rect,
        rect: { x: 0, y: 0, w: state.bounds.w, h: state.bounds.h },
      });
    }
  }
}

/** Windows that appear on the taskbar: every window that is open, in the order they were first opened. */
export function taskbarWindows(state: DesktopState): WinId[] {
  return WIN_IDS.filter((id) => state.wins[id].open);
}
