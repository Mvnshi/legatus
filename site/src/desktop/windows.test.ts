import { describe, expect, it } from "vitest";
import {
  STACK_BREAKPOINT,
  TASKBAR_HEIGHT,
  WIN_IDS,
  clampRect,
  computeLayout,
  fromLayout,
  reduce,
  taskbarWindows,
  topVisible,
  type DesktopState,
} from "./windows";

const desktop = (w = 1240, h = 660): DesktopState => fromLayout(computeLayout(w, h));

describe("layout", () => {
  it("stacks the windows in a column on a phone and never lets them be dragged", () => {
    const l = computeLayout(390, 600);
    expect(l.mode).toBe("stacked");
    expect(computeLayout(STACK_BREAKPOINT - 1, 700).mode).toBe("stacked");
    expect(computeLayout(STACK_BREAKPOINT, 700).mode).toBe("floating");
  });

  it("opens the cockpit, logins and guide, and keeps the others closed until asked", () => {
    const s = desktop();
    expect(WIN_IDS.filter((id) => s.wins[id].open)).toEqual(["cockpit", "logins", "guide"]);
    expect(s.active).toBe("cockpit");
    expect(topVisible(s)).toBe("cockpit");
  });

  it("keeps every window inside the desktop at every supported size", () => {
    for (const [w, h] of [
      [900, 560],
      [1024, 600],
      [1100, 640],
      [1240, 660],
      [1600, 760],
      [2200, 900],
    ] as const) {
      const l = computeLayout(w, h);
      for (const id of WIN_IDS) {
        const r = l.rects[id];
        expect(r.x, `${id} x at ${w}x${h}`).toBeGreaterThanOrEqual(0);
        expect(r.y, `${id} y at ${w}x${h}`).toBeGreaterThanOrEqual(0);
        expect(r.x + r.w, `${id} right edge at ${w}x${h}`).toBeLessThanOrEqual(l.bounds.w);
        expect(r.y + r.h, `${id} bottom edge at ${w}x${h}`).toBeLessThanOrEqual(l.bounds.h);
      }
      expect(l.bounds.h).toBe(h - TASKBAR_HEIGHT);
    }
  });

  it("does not overlap the three opening windows on a wide desktop", () => {
    const l = computeLayout(1240, 660);
    const overlap = (a: (typeof l.rects)["cockpit"], b: (typeof l.rects)["cockpit"]) =>
      a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
    expect(overlap(l.rects.cockpit, l.rects.logins)).toBe(false);
    expect(overlap(l.rects.cockpit, l.rects.guide)).toBe(false);
    expect(overlap(l.rects.logins, l.rects.guide)).toBe(false);
  });

  it("puts the secondary windows on the taskbar when the desktop is too small for them", () => {
    const l = computeLayout(1000, 640);
    expect(l.minimized).toEqual(["logins", "guide"]);
  });
});

describe("dragging is constrained to the desktop", () => {
  it("clamps a window that is dragged past any edge", () => {
    let s = desktop();
    const r = s.wins.cockpit.rect;
    s = reduce(s, { type: "move", id: "cockpit", x: -500, y: -500 });
    expect(s.wins.cockpit.rect).toMatchObject({ x: 0, y: 0, w: r.w, h: r.h });
    s = reduce(s, { type: "move", id: "cockpit", x: 99_999, y: 99_999 });
    expect(s.wins.cockpit.rect.x + r.w).toBe(s.bounds.w);
    expect(s.wins.cockpit.rect.y + r.h).toBe(s.bounds.h);
  });

  it("pulls windows back in when the desktop shrinks", () => {
    let s = desktop(1600, 760);
    s = reduce(s, { type: "move", id: "cockpit", x: 800, y: 40 });
    s = reduce(s, { type: "bounds", bounds: { w: 1000, h: 500 } });
    for (const id of WIN_IDS) {
      const r = s.wins[id].rect;
      expect(r.x + r.w).toBeLessThanOrEqual(1000);
      expect(r.y + r.h).toBeLessThanOrEqual(500);
    }
  });

  it("never makes a window larger than the desktop", () => {
    expect(clampRect({ x: 10, y: 10, w: 5000, h: 5000 }, { w: 800, h: 500 })).toEqual({ x: 0, y: 0, w: 800, h: 500 });
  });
});

describe("minimize, restore, close and the taskbar", () => {
  it("minimizing the active window hands focus to the next visible one", () => {
    let s = desktop();
    s = reduce(s, { type: "minimize", id: "cockpit" });
    expect(s.wins.cockpit.minimized).toBe(true);
    expect(s.active).toBe("logins");
    s = reduce(s, { type: "minimize", id: "logins" });
    s = reduce(s, { type: "minimize", id: "guide" });
    expect(s.active).toBeNull();
  });

  it("restoring a window raises it and makes it active", () => {
    let s = desktop();
    s = reduce(s, { type: "minimize", id: "cockpit" });
    s = reduce(s, { type: "restore", id: "cockpit" });
    expect(s.wins.cockpit.minimized).toBe(false);
    expect(s.active).toBe("cockpit");
    expect(topVisible(s)).toBe("cockpit");
  });

  it("a taskbar button restores a minimized window, minimizes the active one and raises a background one", () => {
    let s = desktop();
    s = reduce(s, { type: "taskbar", id: "cockpit" }); // active -> minimize
    expect(s.wins.cockpit.minimized).toBe(true);
    s = reduce(s, { type: "taskbar", id: "cockpit" }); // minimized -> restore
    expect(s.wins.cockpit.minimized).toBe(false);
    expect(s.active).toBe("cockpit");
    s = reduce(s, { type: "taskbar", id: "guide" }); // background -> raise
    expect(s.active).toBe("guide");
    expect(s.wins.guide.minimized).toBe(false);
    expect(s.wins.guide.z).toBeGreaterThan(s.wins.cockpit.z);
  });

  it("a taskbar entry for a closed window opens it", () => {
    let s = desktop();
    expect(taskbarWindows(s)).not.toContain("evidence");
    s = reduce(s, { type: "open", id: "evidence" });
    expect(taskbarWindows(s)).toContain("evidence");
    expect(s.active).toBe("evidence");
  });

  it("closing removes the window from the taskbar and keeps its position for next time", () => {
    let s = desktop();
    s = reduce(s, { type: "move", id: "logins", x: 300, y: 120 });
    const where = s.wins.logins.rect;
    s = reduce(s, { type: "close", id: "logins" });
    expect(taskbarWindows(s)).not.toContain("logins");
    s = reduce(s, { type: "open", id: "logins" });
    expect(s.wins.logins.rect).toEqual(where);
  });

  it("focus ignores windows that are closed or minimized", () => {
    let s = desktop();
    s = reduce(s, { type: "minimize", id: "guide" });
    const before = s;
    expect(reduce(s, { type: "focus", id: "guide" })).toBe(before);
    expect(reduce(s, { type: "focus", id: "evidence" })).toBe(before);
  });

  it("minimizing a window that is already minimized changes nothing", () => {
    let s = desktop();
    s = reduce(s, { type: "minimize", id: "guide" });
    expect(reduce(s, { type: "minimize", id: "guide" })).toBe(s);
  });
});

describe("maximize", () => {
  it("fills the desktop and goes back to where it was", () => {
    let s = desktop();
    const before = s.wins.cockpit.rect;
    s = reduce(s, { type: "maximize", id: "cockpit" });
    expect(s.wins.cockpit.rect).toEqual({ x: 0, y: 0, w: s.bounds.w, h: s.bounds.h });
    expect(reduce(s, { type: "move", id: "cockpit", x: 50, y: 50 })).toBe(s);
    s = reduce(s, { type: "maximize", id: "cockpit" });
    expect(s.wins.cockpit.rect).toEqual(before);
  });

  it("follows the desktop when it is resized while maximized", () => {
    let s = reduce(desktop(), { type: "maximize", id: "cockpit" });
    s = reduce(s, { type: "bounds", bounds: { w: 1000, h: 480 } });
    expect(s.wins.cockpit.rect).toEqual({ x: 0, y: 0, w: 1000, h: 480 });
  });
});
