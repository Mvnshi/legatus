import { describe, expect, it } from "vitest";
import {
  BEAT_COUNT,
  INJECT_DELAY,
  SCRIPT_MS,
  TASK,
  advance,
  buildEvidence,
  canInjectLimit,
  initialState,
  injectLimit,
  limitHasHappened,
  pause,
  progress,
  replay,
  resume,
  sampleDiff,
  start,
  statusLabel,
  stepsDone,
  type DemoState,
} from "./engine";

const T0 = Date.UTC(2026, 9, 9, 9, 41, 0);

function runToEnd(state: DemoState): DemoState {
  return advance(state, SCRIPT_MS + 1000);
}

/** Advance until a journal line containing `text` exists, in small steps. */
function advanceUntil(state: DemoState, text: string, step = 50): DemoState {
  let s = state;
  for (let i = 0; i < 2000; i++) {
    if (s.journal.some((l) => l.text.includes(text))) return s;
    s = advance(s, step);
  }
  throw new Error(`never saw "${text}"; last line: ${s.journal.at(-1)?.text}`);
}

describe("the scripted run", () => {
  it("does nothing until it is started", () => {
    const idle = initialState();
    expect(advance(idle, 10_000)).toBe(idle);
    expect(idle.phase).toBe("idle");
    expect(statusLabel(idle)).toBe("Queued, not started");
    expect(progress(idle)).toBe(0);
  });

  it("is long enough to follow and short enough to finish", () => {
    expect(SCRIPT_MS).toBeGreaterThan(15_000);
    expect(SCRIPT_MS).toBeLessThan(30_000);
  });

  it("tells the whole story: limit, checkpoint, continuation, checks, review, ready", () => {
    const done = runToEnd(start(initialState(), T0));
    expect(done.phase).toBe("done");
    expect(done.cursor).toBe(BEAT_COUNT);
    expect(statusLabel(done)).toBe("Ready for your review");
    expect(progress(done)).toBe(1);
    expect(stepsDone(done)).toBe(3);

    const texts = done.journal.map((l) => l.text);
    const order = [
      "created on branch",
      "working as work-1",
      "hit its usage limit",
      "checkpoint saved",
      "working as work-2",
      "Finished the task.",
      `check: ${TASK.checks[0]}`,
      `check: ${TASK.checks[1]}`,
      "working as second-opinion (claude), independent: other-provider",
      "review: approve by second-opinion",
      "run succeeded",
    ].map((needle) => texts.findIndex((t) => t.includes(needle)));
    expect(order.every((i) => i >= 0)).toBe(true);
    expect([...order].sort((a, b) => a - b)).toEqual(order);

    expect(done.logins["work-1"].status).toBe("limited");
    expect(done.logins["work-2"].status).toBe("ready");
    expect(done.checks.every((c) => c.status === "passed")).toBe(true);
    expect(done.verdict?.verdict).toBe("approve");
    expect(done.checkpoint?.message).toBe("legatus: implement (interrupted by a usage limit)");
    expect(done.commits.map((c) => c.message)).toEqual([
      "legatus: implement (interrupted by a usage limit)",
      "legatus: implement",
    ]);
  });

  it("ends with a sticky 'Ready for your review' notice and a warning at the limit", () => {
    const done = runToEnd(start(initialState(), T0));
    const last = done.notices.at(-1)!;
    expect(last.title).toBe("Ready for your review");
    expect(last.sticky).toBe(true);
    expect(done.notices.some((n) => n.tone === "warning" && n.title.includes("usage limit"))).toBe(true);
  });

  it("gives the same result however the time is sliced", () => {
    const whole = runToEnd(start(initialState(), T0));
    let sliced = start(initialState(), T0);
    for (let i = 0; i < 4000; i++) sliced = advance(sliced, 7);
    expect(sliced.journal.map((l) => [l.text, l.at])).toEqual(whole.journal.map((l) => [l.text, l.at]));
    expect(sliced.phase).toBe("done");
  });

  it("stamps journal lines with the wall clock of the run", () => {
    const s = advance(start(initialState(), T0), 600);
    expect(s.journal[0]!.at).toBe(T0 + 500);
  });
});

describe("pause and resume", () => {
  it("freezes the run while paused and carries on from the same place", () => {
    let s = advance(start(initialState(), T0), 3000);
    const frozen = pause(s);
    expect(frozen.phase).toBe("paused");
    expect(advance(frozen, 60_000)).toBe(frozen);
    s = resume(frozen);
    expect(s.phase).toBe("running");
    expect(s.journal.length).toBe(frozen.journal.length);
    expect(runToEnd(s).phase).toBe("done");
  });

  it("ignores pause when there is nothing running", () => {
    const idle = initialState();
    expect(pause(idle)).toBe(idle);
    const done = runToEnd(start(idle, T0));
    expect(pause(done)).toBe(done);
    expect(resume(done)).toBe(done);
  });
});

describe("injecting a usage limit", () => {
  it("is not available before the run starts, or once the limit has happened", () => {
    const idle = initialState();
    expect(canInjectLimit(idle)).toBe(false);
    expect(injectLimit(idle)).toBe(idle);

    const after = advanceUntil(start(idle, T0), "hit its usage limit");
    expect(limitHasHappened(after)).toBe(true);
    expect(canInjectLimit(after)).toBe(false);
    expect(injectLimit(after)).toBe(after);
  });

  it("cuts the first login off almost at once after it has written something", () => {
    const working = advanceUntil(start(initialState(), T0), "runs: write src/signup/validate.ts");
    expect(canInjectLimit(working)).toBe(true);
    const injected = injectLimit(working);
    expect(injected.injected).toBe(true);
    expect(canInjectLimit(injected)).toBe(false);

    const landed = advance(injected, INJECT_DELAY + 1);
    expect(landed.limitSource).toBe("injected");
    expect(landed.logins["work-1"].status).toBe("limited");
    // The second edit of the first login never happened.
    expect(landed.journal.some((l) => l.text.includes("SignupForm.tsx"))).toBe(false);

    const finished = runToEnd(landed);
    expect(finished.phase).toBe("done");
    expect(finished.checkpoint?.files).toEqual(["src/signup/validate.ts"]);
    expect(finished.files.map((f) => [f.path, f.by])).toEqual([
      ["src/signup/validate.ts", "work-1"],
      ["src/signup/validate.test.ts", "work-2"],
      ["src/signup/SignupForm.tsx", "work-2"],
    ]);
  });

  it("waits for the first file when injected the moment work starts", () => {
    let s = advanceUntil(start(initialState(), T0), "started (agent)");
    expect(canInjectLimit(s)).toBe(true);
    s = injectLimit(s);
    // Nothing has been written yet, so the limit cannot land yet: a checkpoint needs something to save.
    s = advance(s, 400);
    expect(limitHasHappened(s)).toBe(false);
    const landed = advanceUntil(s, "hit its usage limit");
    expect(landed.limitSource).toBe("injected");
    expect(landed.files.length).toBeGreaterThan(0);
    expect(runToEnd(landed).phase).toBe("done");
  });

  it("lets the limit arrive on its own when nobody injects one", () => {
    const done = runToEnd(start(initialState(), T0));
    expect(done.limitSource).toBe("scheduled");
    expect(done.injected).toBe(false);
  });

  it("cannot be injected while paused", () => {
    const paused = pause(advance(start(initialState(), T0), 3500));
    expect(canInjectLimit(paused)).toBe(false);
    expect(injectLimit(paused)).toBe(paused);
  });
});

describe("replay", () => {
  it("starts over from the beginning with a fresh clock", () => {
    const first = runToEnd(start(initialState(), T0));
    const later = replay(T0 + 60_000);
    expect(later.phase).toBe("running");
    expect(later.journal).toEqual([]);
    expect(later.injected).toBe(false);
    expect(later.startedAt).toBe(T0 + 60_000);
    // Same clock, same story, line for line.
    const second = runToEnd(replay(T0));
    expect(second.journal.map((l) => l.text)).toEqual(first.journal.map((l) => l.text));
    // A later start moves the timestamps with it.
    expect(runToEnd(later).journal[0]!.at).toBe(first.journal[0]!.at + 60_000);
  });
});

describe("what a person reads at the end", () => {
  it("builds a report in the shape of Legatus's evidence.md, from what actually happened", () => {
    const done = runToEnd(start(initialState(), T0));
    const md = buildEvidence(done);
    expect(md).toContain(`# ${TASK.title}`);
    expect(md).toContain(`Run \`${TASK.runId}\`, **succeeded**`);
    expect(md).toContain("| implement | agent | succeeded | work-2 | 2 |");
    expect(md).toContain("| checks | check | succeeded |");
    expect(md).toContain("## Usage limits");
    expect(md).toContain("`work-1` (codex) hit its usage limit during `implement`");
    expect(md).toContain("**approve** by `second-opinion` (other-provider)");
    expect(md).toContain("Nothing needed removing.");
    expect(md).toContain("Nothing was pushed or published");
  });

  it("does not claim a review before there is one", () => {
    const mid = advanceUntil(start(initialState(), T0), "checkpoint saved");
    const md = buildEvidence(mid);
    expect(md).toContain("Not reviewed yet.");
    expect(md).toContain("**running**");
  });

  it("shows a diff only for files the run has touched", () => {
    const mid = advanceUntil(start(initialState(), T0), "runs: write src/signup/validate.ts");
    expect(sampleDiff(mid).map((d) => d.path)).toEqual(["src/signup/validate.ts"]);
    const done = runToEnd(start(initialState(), T0));
    expect(sampleDiff(done).map((d) => d.path).sort()).toEqual([
      "src/signup/SignupForm.tsx",
      "src/signup/validate.test.ts",
      "src/signup/validate.ts",
    ]);
  });
});
