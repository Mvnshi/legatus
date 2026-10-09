/**
 * The simulated run behind the desktop demo.
 *
 * It is a scripted replay of what `legatus demo` does with stand-in agents: the first Codex login runs out of
 * usage halfway through, the half-finished work is saved as a checkpoint, a second login continues, the checks
 * run, and a Claude Code login reviews the change read-only. Nothing here talks to Legatus or to an agent, and
 * every name, file and hash is sample data.
 *
 * The engine is a pure state machine: `advance` takes the milliseconds that have passed and returns the next
 * state. It owns no timers, which keeps it exact and easy to test.
 */

export type Phase = "idle" | "running" | "paused" | "done";
export type LoginId = "work-1" | "work-2" | "second-opinion";
export type StepId = "implement" | "checks" | "review";
export type StepStatus = "pending" | "running" | "succeeded";
export type LoginStatus = "ready" | "working" | "limited" | "reviewing";
export type JournalKind = "info" | "tool" | "say" | "warn" | "ok";
export type Cue = "start" | "limit" | "checkpoint" | "check" | "done";

export interface JournalLine {
  id: number;
  /** Wall-clock time of the line, in epoch milliseconds. */
  at: number;
  step?: StepId;
  kind: JournalKind;
  text: string;
  cue?: Cue;
}

export interface Notice {
  id: number;
  tone: "info" | "warning" | "success";
  title: string;
  text: string;
  /** A notice that asks for something stays until it is dismissed. */
  sticky?: boolean;
}

export interface StepState {
  status: StepStatus;
  /** The login that did the work, or the ones that did it in order. */
  logins: LoginId[];
  attempts: number;
  detail: string;
}

export interface LoginState {
  status: LoginStatus;
  note: string;
}

export interface CheckState {
  command: string;
  status: "pending" | "running" | "passed";
}

export interface FileChange {
  path: string;
  by: LoginId;
  kind: "added" | "modified";
}

export interface Checkpoint {
  sha: string;
  message: string;
  files: string[];
  login: LoginId;
}

export interface Verdict {
  verdict: "approve";
  by: LoginId;
  independence: "other-provider";
  summary: string;
}

export interface DemoState {
  phase: Phase;
  /** Epoch milliseconds when the run was started. */
  startedAt: number;
  /** Simulated milliseconds since the start, counting only while running. */
  elapsed: number;
  cursor: number;
  wait: number;
  injected: boolean;
  limitSource: "injected" | "scheduled" | null;
  journal: JournalLine[];
  notices: Notice[];
  steps: Record<StepId, StepState>;
  logins: Record<LoginId, LoginState>;
  checks: CheckState[];
  files: FileChange[];
  checkpoint: Checkpoint | null;
  commits: { sha: string; message: string }[];
  verdict: Verdict | null;
}

export const TASK = {
  title: "Add input validation to the signup form",
  repo: "my-app",
  base: "main",
  runId: "7c1f4a90",
  branch: "legatus/7c1f4a90",
  baseCommit: "e1b6638",
  checks: ["npm run lint", "npm test"],
} as const;

export const LOGINS: { id: LoginId; provider: "Codex" | "Claude Code"; role: string }[] = [
  { id: "work-1", provider: "Codex", role: "first choice for the work" },
  { id: "work-2", provider: "Codex", role: "second login" },
  { id: "second-opinion", provider: "Claude Code", role: "reviews other logins' work" },
];

export const STEP_ORDER: StepId[] = ["implement", "checks", "review"];

export const STEP_LABEL: Record<StepId, string> = {
  implement: "Implement",
  checks: "Checks",
  review: "Review",
};

/** How long after the user injects a limit it lands. */
export const INJECT_DELAY = 350;
/** The usage limit resets this long after it was hit. */
export const RESET_AFTER_MS = 3 * 60 * 60 * 1000;

type Draft = DemoState;
interface Beat {
  id: string;
  /** Milliseconds after the previous beat. */
  delay: number;
  apply: (s: Draft) => void;
}

function log(s: Draft, kind: JournalKind, text: string, step?: StepId, cue?: Cue) {
  const line: JournalLine = { id: s.journal.length + 1, at: s.startedAt + s.elapsed, kind, text };
  if (step) line.step = step;
  if (cue) line.cue = cue;
  s.journal.push(line);
}

function notify(s: Draft, tone: Notice["tone"], title: string, text: string, sticky = false) {
  const n: Notice = { id: s.notices.length + 1, tone, title, text };
  if (sticky) n.sticky = true;
  s.notices.push(n);
}

function touch(s: Draft, path: string, by: LoginId, kind: FileChange["kind"]) {
  if (!s.files.some((f) => f.path === path)) s.files.push({ path, by, kind });
}

export function formatClock(epoch: number): string {
  const d = new Date(epoch);
  const two = (n: number) => String(n).padStart(2, "0");
  return `${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`;
}

export function formatShortClock(epoch: number): string {
  return formatClock(epoch).slice(0, 5);
}

const beats: Beat[] = [
  {
    id: "created",
    delay: 500,
    apply: (s) => {
      s.steps.implement.detail = "waiting for a login";
      log(s, "info", `run ${TASK.runId} created on branch ${TASK.branch}`, undefined, "start");
    },
  },
  {
    id: "base",
    delay: 700,
    apply: (s) => log(s, "info", `worktree ready; base pinned at ${TASK.baseCommit} on ${TASK.base}`),
  },
  {
    id: "implement-start",
    delay: 600,
    apply: (s) => {
      s.steps.implement.status = "running";
      log(s, "info", "started (agent)", "implement");
    },
  },
  {
    id: "work1-start",
    delay: 700,
    apply: (s) => {
      s.logins["work-1"] = { status: "working", note: "writing the validation" };
      s.steps.implement.logins = ["work-1"];
      s.steps.implement.attempts = 1;
      s.steps.implement.detail = "working as work-1";
      log(s, "info", "working as work-1 (codex)", "implement");
    },
  },
  {
    id: "edit-1",
    delay: 1500,
    apply: (s) => {
      touch(s, "src/signup/validate.ts", "work-1", "added");
      log(s, "tool", "runs: write src/signup/validate.ts", "implement");
    },
  },
  {
    id: "edit-2",
    delay: 1700,
    apply: (s) => {
      touch(s, "src/signup/SignupForm.tsx", "work-1", "modified");
      log(s, "tool", "runs: edit src/signup/SignupForm.tsx", "implement");
    },
  },
  {
    id: "limit",
    delay: 1300,
    apply: (s) => {
      const resetAt = s.startedAt + s.elapsed + RESET_AFTER_MS;
      s.limitSource = s.injected ? "injected" : "scheduled";
      s.logins["work-1"] = { status: "limited", note: `resets about ${formatShortClock(resetAt)}` };
      s.steps.implement.detail = "work-1 hit its usage limit";
      log(
        s,
        "warn",
        `!! work-1 hit its usage limit; set aside until ${formatShortClock(resetAt)}. Continuing on another login.`,
        "implement",
        "limit",
      );
      notify(
        s,
        "warning",
        "work-1 reached its usage limit",
        "Legatus is saving the work so far and handing the task to work-2.",
      );
    },
  },
  {
    id: "checkpoint",
    delay: 900,
    apply: (s) => {
      const files = s.files.map((f) => f.path);
      const message = "legatus: implement (interrupted by a usage limit)";
      s.checkpoint = { sha: "9d0674d", message, files, login: "work-1" };
      s.commits.push({ sha: "9d0674d", message });
      s.steps.implement.detail = "checkpoint saved";
      log(
        s,
        "ok",
        `checkpoint saved on ${TASK.branch}: ${files.length} changed file${files.length === 1 ? "" : "s"} committed (9d0674d)`,
        "implement",
        "checkpoint",
      );
    },
  },
  {
    id: "work2-start",
    delay: 900,
    apply: (s) => {
      s.logins["work-2"] = { status: "working", note: "continuing from the checkpoint" };
      s.steps.implement.logins = ["work-1", "work-2"];
      s.steps.implement.attempts = 2;
      s.steps.implement.detail = "continuing as work-2";
      log(s, "info", "working as work-2 (codex)", "implement");
      log(s, "say", "told where things stand: the checkpoint's files are already on the branch", "implement");
    },
  },
  {
    id: "edit-3",
    delay: 1500,
    apply: (s) => {
      touch(s, "src/signup/validate.test.ts", "work-2", "added");
      log(s, "tool", "runs: write src/signup/validate.test.ts", "implement");
    },
  },
  {
    id: "edit-4",
    delay: 1400,
    apply: (s) => {
      touch(s, "src/signup/SignupForm.tsx", "work-2", "modified");
      log(s, "tool", "runs: edit src/signup/SignupForm.tsx", "implement");
    },
  },
  {
    id: "implement-done",
    delay: 900,
    apply: (s) => {
      s.commits.push({ sha: "595f857", message: "legatus: implement" });
      s.logins["work-2"] = { status: "ready", note: "finished the implementation" };
      s.steps.implement.status = "succeeded";
      s.steps.implement.detail = "added input validation to the signup form";
      log(s, "say", "Finished the task.", "implement");
      log(s, "ok", "done. added input validation to the signup form", "implement");
    },
  },
  {
    id: "checks-start",
    delay: 700,
    apply: (s) => {
      s.steps.checks.status = "running";
      s.steps.checks.detail = "running your checks";
      s.checks[0]!.status = "running";
      log(s, "info", "started (check)", "checks");
      log(s, "info", `check: ${TASK.checks[0]}`, "checks");
    },
  },
  {
    id: "check-1",
    delay: 1300,
    apply: (s) => {
      s.checks[0]!.status = "passed";
      s.checks[1]!.status = "running";
      log(s, "ok", "check passed", "checks", "check");
      log(s, "info", `check: ${TASK.checks[1]}`, "checks");
    },
  },
  {
    id: "check-2",
    delay: 1600,
    apply: (s) => {
      s.checks[1]!.status = "passed";
      s.steps.checks.status = "succeeded";
      s.steps.checks.detail = "2 command(s) passed";
      log(s, "ok", "check passed", "checks", "check");
      log(s, "ok", "done. 2 command(s) passed", "checks");
    },
  },
  {
    id: "review-start",
    delay: 700,
    apply: (s) => {
      s.steps.review.status = "running";
      s.steps.review.logins = ["second-opinion"];
      s.steps.review.attempts = 1;
      s.steps.review.detail = "reviewing read-only";
      s.logins["second-opinion"] = { status: "reviewing", note: "reading the diff, read-only" };
      log(s, "info", "started (review)", "review");
      log(s, "info", "working as second-opinion (claude), independent: other-provider", "review");
    },
  },
  {
    id: "review-verdict",
    delay: 2000,
    apply: (s) => {
      s.verdict = {
        verdict: "approve",
        by: "second-opinion",
        independence: "other-provider",
        summary: "matches the task",
      };
      s.steps.review.status = "succeeded";
      s.steps.review.detail = "approve (other-provider): matches the task";
      s.logins["second-opinion"] = { status: "ready", note: "approved the change" };
      log(s, "ok", "review: approve by second-opinion (other-provider): matches the task", "review");
      log(s, "ok", "done. approve (other-provider): matches the task", "review");
    },
  },
  {
    id: "done",
    delay: 700,
    apply: (s) => {
      s.phase = "done";
      log(s, "ok", "run succeeded", undefined, "done");
      notify(
        s,
        "success",
        "Ready for your review",
        `Branch ${TASK.branch} and its report are waiting. Nothing was pushed or published.`,
        true,
      );
    },
  },
];

const LIMIT_INDEX = beats.findIndex((b) => b.id === "limit");

/** The total scripted time with no interruption, in milliseconds. Used by tests and the progress readout. */
export const SCRIPT_MS = beats.reduce((sum, b) => sum + b.delay, 0);
export const BEAT_COUNT = beats.length;

export function initialState(): DemoState {
  return {
    phase: "idle",
    startedAt: 0,
    elapsed: 0,
    cursor: 0,
    wait: beats[0]!.delay,
    injected: false,
    limitSource: null,
    journal: [],
    notices: [],
    steps: {
      implement: { status: "pending", logins: [], attempts: 0, detail: "waiting to start" },
      checks: { status: "pending", logins: [], attempts: 0, detail: "after the implementation" },
      review: { status: "pending", logins: [], attempts: 0, detail: "after the checks" },
    },
    logins: {
      "work-1": { status: "ready", note: "has usage left" },
      "work-2": { status: "ready", note: "has usage left" },
      "second-opinion": { status: "ready", note: "has usage left" },
    },
    checks: TASK.checks.map((command) => ({ command, status: "pending" as const })),
    files: [],
    checkpoint: null,
    commits: [],
    verdict: null,
  };
}

export function start(state: DemoState, now: number): DemoState {
  if (state.phase !== "idle") return state;
  return { ...state, phase: "running", startedAt: now };
}

export function replay(now: number): DemoState {
  return start(initialState(), now);
}

export function pause(state: DemoState): DemoState {
  return state.phase === "running" ? { ...state, phase: "paused" } : state;
}

export function resume(state: DemoState): DemoState {
  return state.phase === "paused" ? { ...state, phase: "running" } : state;
}

export function limitHasHappened(state: DemoState): boolean {
  return state.cursor > LIMIT_INDEX;
}

/** A limit can be injected while the first login is still working: after the run starts, before the limit lands. */
export function canInjectLimit(state: DemoState): boolean {
  return state.phase === "running" && !state.injected && !limitHasHappened(state) && state.cursor >= 3;
}

export function injectLimit(state: DemoState): DemoState {
  if (!canInjectLimit(state)) return state;
  return normalize({ ...state, injected: true });
}

/**
 * When a limit has been injected and the first login has already written something, skip the rest of its
 * work and let the limit land almost at once.
 */
function normalize(state: DemoState): DemoState {
  if (!state.injected || limitHasHappened(state) || state.files.length === 0) return state;
  if (state.cursor < LIMIT_INDEX) return { ...state, cursor: LIMIT_INDEX, wait: Math.min(state.wait, INJECT_DELAY) };
  return { ...state, wait: Math.min(state.wait, INJECT_DELAY) };
}

export function advance(state: DemoState, dt: number): DemoState {
  if (state.phase !== "running" || dt <= 0) return state;
  const s: Draft = structuredClone(state);
  let left = dt;
  let elapsed = s.elapsed;
  while (s.cursor < beats.length) {
    if (left < s.wait) {
      s.wait -= left;
      elapsed += left;
      left = 0;
      break;
    }
    left -= s.wait;
    elapsed += s.wait;
    s.elapsed = elapsed;
    beats[s.cursor]!.apply(s);
    s.cursor += 1;
    if (s.cursor < beats.length) s.wait = beats[s.cursor]!.delay;
    else s.wait = 0;
    const next = normalize(s);
    s.cursor = next.cursor;
    s.wait = next.wait;
    if (s.phase === "done") break;
  }
  s.elapsed = elapsed;
  return s;
}

/** Progress through the scripted run, 0 to 1, for the progress bar. */
export function progress(state: DemoState): number {
  if (state.phase === "idle") return 0;
  if (state.phase === "done") return 1;
  return Math.min(0.99, state.cursor / beats.length);
}

/** How many different logins have done work or reviewed so far. */
export function loginsUsed(state: DemoState): number {
  return new Set(STEP_ORDER.flatMap((id) => state.steps[id].logins)).size;
}

export function stepsDone(state: DemoState): number {
  return STEP_ORDER.filter((id) => state.steps[id].status === "succeeded").length;
}

/** The step a person should be told about right now. */
export function currentStep(state: DemoState): StepId | null {
  return STEP_ORDER.find((id) => state.steps[id].status === "running") ?? null;
}

export function statusLabel(state: DemoState): string {
  switch (state.phase) {
    case "idle":
      return "Queued, not started";
    case "paused":
      return "Paused";
    case "done":
      return "Ready for your review";
    case "running": {
      if (limitHasHappened(state) && state.steps.implement.status === "running") {
        return state.checkpoint ? "Continuing on work-2" : "Saving a checkpoint";
      }
      const step = currentStep(state);
      if (step === "implement") return "Implementing with work-1";
      if (step === "checks") return "Running checks";
      if (step === "review") return "Independent review";
      return "Starting";
    }
  }
}

export function sampleDiff(state: DemoState): { path: string; by: LoginId; lines: string[] }[] {
  const wanted = new Map(state.files.map((f) => [f.path, f] as const));
  return diffs
    .filter((d) => wanted.has(d.path))
    .map((d) => ({ path: d.path, by: wanted.get(d.path)!.by, lines: d.lines }));
}

const diffs: { path: string; lines: string[] }[] = [
  {
    path: "src/signup/validate.ts",
    lines: [
      "@@ new file @@",
      "+export type SignupInput = { email: string; password: string };",
      "+export type FieldErrors = Partial<Record<keyof SignupInput, string>>;",
      "+",
      "+export function validateSignup(input: SignupInput): FieldErrors {",
      "+  const errors: FieldErrors = {};",
      "+  if (!/^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$/.test(input.email)) {",
      "+    errors.email = 'Enter a valid email address.';",
      "+  }",
      "+  if (input.password.length < 12) {",
      "+    errors.password = 'Use at least 12 characters.';",
      "+  }",
      "+  return errors;",
      "+}",
    ],
  },
  {
    path: "src/signup/SignupForm.tsx",
    lines: [
      "@@ SignupForm @@",
      " import { useState } from 'react';",
      "+import { validateSignup } from './validate';",
      " ",
      " export function SignupForm({ onSubmit }: Props) {",
      "+  const [errors, setErrors] = useState<FieldErrors>({});",
      "   function handle(e: FormEvent) {",
      "     e.preventDefault();",
      "-    onSubmit(readForm(e.currentTarget));",
      "+    const input = readForm(e.currentTarget);",
      "+    const found = validateSignup(input);",
      "+    setErrors(found);",
      "+    if (Object.keys(found).length === 0) onSubmit(input);",
      "   }",
    ],
  },
  {
    path: "src/signup/validate.test.ts",
    lines: [
      "@@ new file @@",
      "+import { validateSignup } from './validate';",
      "+",
      "+test('accepts a valid email and a long password', () => {",
      "+  expect(validateSignup({ email: 'a@b.co', password: 'twelve-chars!' })).toEqual({});",
      "+});",
      "+",
      "+test('rejects a short password', () => {",
      "+  expect(validateSignup({ email: 'a@b.co', password: 'short' }).password).toBeDefined();",
      "+});",
    ],
  },
];

/**
 * The report a person would read, laid out like the `evidence.md` that Legatus writes next to the branch.
 * It is built from the sample run, so it only claims what the simulation has shown.
 */
export function buildEvidence(state: DemoState): string {
  const rows = STEP_ORDER.map((id) => {
    const st = state.steps[id];
    const type = id === "implement" ? "agent" : id === "checks" ? "check" : "review";
    const login = st.logins.length ? st.logins[st.logins.length - 1] : "-";
    const result = st.status === "succeeded" ? "succeeded" : st.status;
    return `| ${id} | ${type} | ${result} | ${login} | ${st.attempts || 1} | ${st.detail} |`;
  });
  const status = state.phase === "done" ? "succeeded" : state.phase === "idle" ? "queued" : "running";
  const lines = [
    `# ${TASK.title}`,
    "",
    `- Run \`${TASK.runId}\`, **${status}**`,
    `- Branch \`${TASK.branch}\`, started from \`${TASK.baseCommit}\``,
    "",
    "## Task",
    "",
    TASK.title,
    "",
    "## Steps",
    "",
    "| Step | Type | Result | Login | Attempts | Detail |",
    "| --- | --- | --- | --- | --- | --- |",
    ...rows,
    "",
    "## Usage limits",
    "",
  ];
  const limit = state.journal.find((l) => l.cue === "limit");
  if (limit) {
    lines.push(`- \`work-1\` (codex) hit its usage limit during \`implement\`; ${limit.text.replace(/^!! work-1 hit its usage limit; /, "").replace(/\. Continuing.*$/, "")}`);
  } else {
    lines.push("No usage limit was hit.");
  }
  lines.push("", "## Review", "");
  if (state.verdict) {
    lines.push(
      `- \`review\`: **${state.verdict.verdict}** by \`${state.verdict.by}\` (${state.verdict.independence}): ${state.verdict.summary}`,
    );
  } else {
    lines.push("Not reviewed yet.");
  }
  lines.push("", "## Secrets removed from prompts", "", "Nothing needed removing.", "", "## Changes", "");
  if (state.commits.length) {
    for (const c of [...state.commits].reverse()) lines.push(`- ${c.sha} ${c.message}`);
    lines.push("");
  }
  for (const f of state.files) lines.push(`- ${f.kind === "added" ? "added" : "changed"}: ${f.path} (${f.by})`);
  if (!state.files.length) lines.push("No files changed yet.");
  lines.push("", "(Sample report from the simulated demo. Nothing was pushed or published.)");
  return lines.join("\n");
}
