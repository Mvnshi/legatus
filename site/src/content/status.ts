import type { XPIconName } from "../xp/icons";

/**
 * What is and is not verified, condensed from docs/STATUS.md. The wording is shorter but the claims are the same.
 * `status.test.ts` compares the "Not verified yet" list with STATUS.md, so a new gap documented there cannot be
 * left off this page without a test failing.
 */

export interface StatusItem {
  /** The bold lead-in used in STATUS.md, which the test matches against. */
  lead: string;
  text: string;
}

export interface StatusGroup {
  id: "tested" | "real" | "not" | "limits";
  title: string;
  icon: XPIconName;
  lead: string;
  items: StatusItem[];
}

export const statusGroups: StatusGroup[] = [
  {
    id: "tested",
    title: "Tested automatically",
    icon: "check",
    lead: "These run on every change, on Linux, macOS and Windows, with stand-in agents and real git.",
    items: [
      { lead: "Runs and workflows", text: "and worktrees: twelve created at the same moment without lock errors." },
      {
        lead: "The account pool and scheduler",
        text: "limit handling, waiting for a reset, checks and their feedback, independent review and its fallbacks, redaction and the scrubbed environment, cancel and resume. Nine runs at once over three logins, and one heavy check at a time across runs.",
      },
      { lead: "The queue", text: "worker limits, cancel, retry, and resuming after a restart." },
      {
        lead: "The HTTP API and its protections",
        text: "host name, origin, key and content policy, and the live streams.",
      },
      {
        lead: "Schedules and the GitHub watcher",
        text: "with a fake clock: first sighting, catching up once, weekends, active limits, author checks, restarts and hot reload.",
      },
      {
        lead: "Pull requests",
        text: "against a scripted gh and a real local git remote: the push really happens, and the arguments and body are checked.",
      },
      { lead: "The command line", text: "including that the sample automations.yaml stays valid." },
    ],
  },
  {
    id: "real",
    title: "Run for real",
    icon: "status",
    lead: "On a Windows PC and on Linux.",
    items: [
      {
        lead: "Codex CLI 0.162 (Windows)",
        text: "completed a small task end to end through Legatus using an existing login: branch, commit, check and report. The event shapes the parser relies on were captured from that CLI.",
      },
      {
        lead: "Codex has also been used to work on Legatus itself",
        text: "one task at a time, with the output reviewed by a person before merging.",
      },
      { lead: "Reading a GitHub issue", text: "through the real gh was checked once, read-only." },
      {
        lead: "The cockpit (Windows, Chromium)",
        text: "was used in a real browser against the daemon with stand-in agents: the run list, a run's live journal, report and diff, adding and switching logins, submitting a task, the automations page, and a phone-sized screen.",
      },
      {
        lead: "Claude Code 2.1.295 (Linux)",
        text: "ran an agent step, a check that failed and sent the work back, a review, and the daemon working two logins at once. The logins used the test machine's credentials, not a personal subscription.",
      },
      {
        lead: "A usage limit (Linux)",
        text: "A local stand-in for the Anthropic API refused a request the way an exhausted login is refused, and the real Claude Code answered. Legatus set that login aside until the reset in its event and carried on with the second login, also when the refusal came mid-task.",
      },
      {
        lead: "Pushing a run's branch (Linux)",
        text: "legatus pr pushed a branch to a real GitHub repository. Opening the pull request itself was refused by the test machine's GitHub proxy, so that step is still untried.",
      },
      {
        lead: "Cancel (Linux)",
        text: "stops the agent and the shell command it started. The first real try found a command left running; that is fixed.",
      },
      {
        lead: "The cockpit in three engines (Linux)",
        text: "was driven in Chromium, Firefox and Playwright's WebKit: submitting a task, the live journal, report and diff, adding a login, automations, and a phone-sized screen. The site's own browser tests pass in all three (two WebGL tests skip in Firefox).",
      },
    ],
  },
  {
    id: "not",
    title: "Not verified yet",
    icon: "warning",
    lead: "If something is not listed as verified, assume it has not been run for real.",
    items: [
      {
        lead: "A real usage limit",
        text: "Claude Code was run against a refusal simulated by a local server, because a login cannot be run out of usage on demand: how it reacts is real, what the Anthropic service sends for a real limit is not captured. Codex's limit messages were written from documented and remembered wordings and never captured. If the reset time cannot be read, the login is set aside for 30 minutes.",
      },
      {
        lead: "A personal subscription login for Claude Code",
        text: "The Linux runs used the test machine's credentials, with a separate configuration folder per login. legatus accounts login and logins kept in the system keychain are untried.",
      },
      {
        lead: "A real pull request opened by Legatus",
        text: "The push to a real repository worked. gh pr create has never succeeded against one; the rest of the flow is tested against a scripted gh.",
      },
      {
        lead: "macOS by hand, and Claude Code on Windows and macOS",
        text: "The tests run there in CI; nothing has been driven by a person. Codex was run for real only on Windows.",
      },
      { lead: "Safari itself", text: "The cockpit and the site ran in Firefox and in Playwright's WebKit build on Linux, not in Apple's Safari." },
      {
        lead: "Codex's own Windows sandbox",
        text: "On the PC Legatus was developed on, it could not start, so Legatus probes first and offers --no-sandbox, which is never the default. Whether it starts on other Windows machines is untested, and so is the path where it does.",
      },
    ],
  },
  {
    id: "limits",
    title: "Known limits",
    icon: "info",
    lead: "Limits today.",
    items: [
      {
        lead: "Local only",
        text: "The daemon listens only on the local computer. There is no login or encryption for reaching it from another device, so it is not for sharing or for a server yet.",
      },
      { lead: "Two agents", text: "Only Codex and Claude Code are supported." },
      {
        lead: "Limits are read from the agent's output",
        text: "For Claude Code that is its rate-limit event, then its message. A change in an agent's wording can make a limit look like an ordinary failure until the parser is updated. Only Claude Code 2.1.295 was run for real.",
      },
      {
        lead: "Not an operating-system sandbox",
        text: "Legatus turns on the agent's own sandbox and checks that it can start. With --no-sandbox the agent can reach anything your user account can.",
      },
      {
        lead: "Unsigned builds",
        text: "Release builds are not code-signed yet (signed releases are on the backlog), so your system may warn the first time you run one.",
      },
    ],
  },
];

/** A shorter version for the compatibility box beside the install dialog. */
export const compatibility: string[] = [
  "Run for real: Codex CLI 0.162 on a Windows PC, Claude Code 2.1.295 on Linux.",
  "Not run for real: a usage limit from Anthropic itself, a personal Claude Code login, a pull request opened by Legatus itself, macOS by hand, and Safari itself. Tests cover them with stand-ins on all three systems.",
  "Release builds are not code-signed yet.",
  "Legatus is not an operating-system sandbox, and the cockpit is local only.",
];
