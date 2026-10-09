import type { XPIconName } from "../xp/icons";

/**
 * The words on the page. Every claim here is either from the README, docs/DESIGN.md, docs/STATUS.md or the
 * CHANGELOG, or describes the simulated demo as a simulation. There are no customers, benchmarks or numbers about
 * performance, because the project has none to report.
 */

export const hero = {
  title: "Your agents. One command center.",
  lede: "Queue coding tasks, isolate every change, and review the checks and diff in one local workspace.",
  primary: "Try the demo",
  secondary: "Download Legatus",
  facts: ["Open source", "Early software (v0.1)", "Builds for Windows, macOS and Linux"],
};

export const demoIntro = {
  title: "Watch a usage limit get handled",
  lede: "A scripted run, played in your browser. It follows what `legatus demo` does with stand-in agents: the first login runs out halfway, the work is saved, another login continues, the checks run, and a different agent reviews it.",
  label: "Simulated demo",
  disclosure:
    "Nothing on this desktop runs Legatus or an agent. The names, files and hashes are sample data. To see the real engine do this, run legatus demo.",
  keyboard:
    "Keyboard: Tab moves between controls. Focus a title bar and use the arrow keys to move a window (Shift for bigger steps). Escape closes a menu. The taskbar buttons minimize and restore windows.",
};

export interface Move {
  id: string;
  icon: XPIconName;
  title: string;
  summary: string;
  body: string;
  command?: string;
  /** Where the simulated demo shows this step, so the explanation and the demo point at each other. */
  inDemo: string;
}

export const moves: Move[] = [
  {
    id: "queue",
    icon: "queue",
    title: "Queue a task",
    summary: "From a sentence or a GitHub issue.",
    body: "Describe the work, or point Legatus at a GitHub issue. Issue text is written by strangers, so it is fenced off and the agent is told not to take orders from it. `legatus serve` keeps a queue, runs several tasks at once, and picks up where it stopped after a restart.",
    command: 'legatus queue "add input validation to the signup form" --check "npm test" --review',
    inDemo: "The sample task is already queued. Press Start in the cockpit to run it.",
  },
  {
    id: "isolate",
    icon: "branch",
    title: "Isolate every change",
    summary: "One branch and worktree per run.",
    body: "Each run gets its own git branch, `legatus/<run id>`, in its own worktree, pinned to the commit it started from. Agents working in parallel never touch each other or your working copy.",
    inDemo: "The cockpit's address bar shows the run's branch, legatus/7c1f4a90, and the report lists the files changed on it.",
  },
  {
    id: "continue",
    icon: "limit",
    title: "Keep going at a limit",
    summary: "Save the work, hand it on, or wait.",
    body: "When a login hits its usage limit, whatever it wrote is committed on the run's branch and another login continues from there. If every login is out, the run waits for the first reset. A restart does not lose it.",
    inDemo: "Press Inject limit. work-1 runs out, a checkpoint is committed, and work-2 carries on from it.",
  },
  {
    id: "check",
    icon: "review",
    title: "Check and review",
    summary: "Your checks, then a second opinion.",
    body: "Your checks run, and a failure is sent back to the agent. A different provider, or at least a different login, reviews the change read-only; anything a reviewer touches is reverted. A review without a clear verdict goes to a person and is never treated as an approval.",
    inDemo: "The Checks tab runs two sample checks, and the Review tab shows an independent verdict from another provider.",
  },
  {
    id: "read",
    icon: "report",
    title: "Read it in one place",
    summary: "A branch and a report.",
    body: "You get the branch and a report, `evidence.md`: which login did what, the check results, the reviewer's verdict, every usage limit that was hit, anything removed from prompts, and the files that changed. Nothing is pushed or published unless you ask for that run.",
    inDemo: "When the run reaches “Ready for your review”, open evidence.md. It is laid out like the real report.",
  },
];

export interface Feature {
  icon: XPIconName;
  title: string;
  body: string;
}

export const features: Feature[] = [
  {
    icon: "logins",
    title: "A pool of your own logins",
    body: "Each login lives in its own folder and is signed in with the agent's own login command. Legatus never sees a password. Work goes to a login that still has usage.",
  },
  {
    icon: "review",
    title: "Independent review",
    body: "A different provider, or at least a different login, reviews read-only. Whatever a reviewer touches is reverted, and no verdict is never an approval.",
  },
  {
    icon: "lock",
    title: "Safe by default",
    body: "Secrets are removed before anything is stored. Agents get a scrubbed environment. The agent's own sandbox is checked before work is sent. Nothing is published unless you ask.",
  },
  {
    icon: "cockpit",
    title: "A cockpit that stays on your machine",
    body: "Runs, a live journal, reports, diffs, logins and reset countdowns, on a phone-sized screen too. It listens only on your own computer, and every request needs a secret key.",
  },
  {
    icon: "branch",
    title: "Issues in, pull requests out",
    body: "Start a task from a GitHub issue. A finished run can open a pull request with its report as the description, only when you ask for that run.",
  },
  {
    icon: "queue",
    title: "Tasks that start themselves",
    body: "Schedules, and a watcher for labelled issues that only accepts authors you list. Heavy checks run one at a time, so a queue of tasks does not use up your memory.",
  },
];

export const realCockpit = {
  title: "The real cockpit",
  lede: "These are screenshots of the local cockpit that `legatus serve` opens in your browser, driven by stand-in agents.",
};

export const links = {
  repo: "https://github.com/Mvnshi/legatus",
  status: "https://github.com/Mvnshi/legatus/blob/main/docs/STATUS.md",
  design: "https://github.com/Mvnshi/legatus/blob/main/docs/DESIGN.md",
  security: "https://github.com/Mvnshi/legatus/blob/main/SECURITY.md",
  contributing: "https://github.com/Mvnshi/legatus/blob/main/CONTRIBUTING.md",
  changelog: "https://github.com/Mvnshi/legatus/blob/main/CHANGELOG.md",
  license: "https://github.com/Mvnshi/legatus/blob/main/LICENSE",
  backlog: "https://github.com/Mvnshi/legatus/blob/main/docs/BACKLOG.md",
  thirdParty: "https://github.com/Mvnshi/legatus/blob/main/site/THIRD_PARTY.md",
  issues: "https://github.com/Mvnshi/legatus/issues",
};
