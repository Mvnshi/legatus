import { useCallback, useEffect, useMemo, useReducer } from "react";
import {
  advance,
  initialState,
  injectLimit,
  pause,
  replay,
  resume,
  start,
  type DemoState,
} from "./engine";

type Action =
  | { type: "start"; now: number }
  | { type: "pause" }
  | { type: "resume" }
  | { type: "inject" }
  | { type: "replay"; now: number }
  | { type: "tick"; dt: number };

function reducer(state: DemoState, action: Action): DemoState {
  switch (action.type) {
    case "start":
      return start(state, action.now);
    case "pause":
      return pause(state);
    case "resume":
      return resume(state);
    case "inject":
      return injectLimit(state);
    case "replay":
      return replay(action.now);
    case "tick":
      return advance(state, action.dt);
  }
}

export interface DemoActions {
  /** Starts a queued run, or resumes a paused one. */
  play: () => void;
  pause: () => void;
  replay: () => void;
  injectLimit: () => void;
}

const TICK_MS = 50;
/** A tab that was hidden comes back with one huge gap; the demo treats it as a pause instead of racing ahead. */
const MAX_STEP_MS = 250;

export function useDemo(): { state: DemoState; actions: DemoActions } {
  const [state, dispatch] = useReducer(reducer, undefined, initialState);

  useEffect(() => {
    if (state.phase !== "running") return;
    let last = performance.now();
    const id = window.setInterval(() => {
      const now = performance.now();
      dispatch({ type: "tick", dt: Math.min(now - last, MAX_STEP_MS) });
      last = now;
    }, TICK_MS);
    return () => window.clearInterval(id);
  }, [state.phase]);

  const play = useCallback(() => {
    dispatch({ type: "start", now: Date.now() });
    dispatch({ type: "resume" });
  }, []);
  const stop = useCallback(() => dispatch({ type: "pause" }), []);
  const again = useCallback(() => dispatch({ type: "replay", now: Date.now() }), []);
  const inject = useCallback(() => dispatch({ type: "inject" }), []);

  const actions = useMemo(
    () => ({ play, pause: stop, replay: again, injectLimit: inject }),
    [play, stop, again, inject],
  );
  return { state, actions };
}
