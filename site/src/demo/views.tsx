import { useEffect, useRef, type ReactNode } from "react";
import {
  LOGINS,
  STEP_LABEL,
  STEP_ORDER,
  TASK,
  buildEvidence,
  canInjectLimit,
  formatClock,
  limitHasHappened,
  loginsUsed,
  progress,
  sampleDiff,
  statusLabel,
  stepsDone,
  type DemoState,
  type LoginStatus,
  type StepId,
} from "./engine";
import type { DemoActions } from "./useDemo";
import { XPIcon, type XPIconName } from "../xp/icons";
import { XPButton, XPProgress, XPTab, XPTabList, XPTabPanel, XPTabs, XPTooltip } from "../xp/controls";
import { XPExplorerPane, XPTaskGroup } from "../xp/XPExplorerPane";
import CountUp from "../reactbits/CountUp";
import ShinyText from "../reactbits/ShinyText";
import { statusGroups } from "../content/status";
import { links } from "../content/copy";
import type { WinId } from "../desktop/windows";
import "./demo.css";

export type CockpitTab = "overview" | "journal" | "checks" | "review";
export const COCKPIT_TABS: { id: CockpitTab; label: string; icon: XPIconName }[] = [
  { id: "overview", label: "Overview", icon: "cockpit" },
  { id: "journal", label: "Journal", icon: "terminal" },
  { id: "checks", label: "Checks", icon: "check" },
  { id: "review", label: "Review", icon: "review" },
];

const loginStatusText: Record<LoginStatus, string> = {
  ready: "Ready",
  working: "Working",
  limited: "At its usage limit",
  reviewing: "Reviewing",
};

const loginStatusIcon: Record<LoginStatus, XPIconName> = {
  ready: "check",
  working: "play",
  limited: "limit",
  reviewing: "review",
};

export function LoginBadge({ status }: { status: LoginStatus }) {
  return (
    <span className={`login-badge login-badge--${status}`}>
      <XPIcon name={loginStatusIcon[status]} size={16} />
      {loginStatusText[status]}
    </span>
  );
}

/* ---------- Run toolbar ---------- */

function injectReason(state: DemoState): string {
  if (canInjectLimit(state)) return "Make work-1 run out of usage right now.";
  if (state.phase === "idle") return "Start the run first. Then you can cut work-1 off with a usage limit.";
  if (state.phase === "paused") return "Resume the run to inject a limit.";
  if (state.phase === "done") return "This run is finished. Replay it to inject the limit yourself.";
  if (state.injected) return "The limit is on its way.";
  return "The limit has already been hit in this run. Replay to try again.";
}

export function RunToolbar({ state, actions }: { state: DemoState; actions: DemoActions }) {
  const canPlay = state.phase === "idle" || state.phase === "paused";
  const canInject = canInjectLimit(state);
  return (
    <div className="run-toolbar" role="group" aria-label="Run controls">
      <XPButton variant="toolbar" icon="play" unavailable={!canPlay} onClick={actions.play}>
        {state.phase === "paused" ? "Resume" : "Start"}
      </XPButton>
      <XPButton variant="toolbar" icon="pause" unavailable={state.phase !== "running"} onClick={actions.pause}>
        Pause
      </XPButton>
      <XPButton variant="toolbar" icon="replay" unavailable={state.phase === "idle"} onClick={actions.replay}>
        Replay
      </XPButton>
      <span className="run-toolbar__sep" aria-hidden="true" />
      <XPTooltip label={injectReason(state)}>
        <XPButton variant="toolbar" icon="limit" unavailable={!canInject} onClick={actions.injectLimit}>
          Inject limit
        </XPButton>
      </XPTooltip>
    </div>
  );
}

/* ---------- Cockpit ---------- */

function StepGlyph({ status }: { status: string }) {
  if (status === "succeeded") return <XPIcon name="check" size={22} />;
  if (status === "running") return <span className="step-spinner" aria-hidden="true" />;
  return <span className="step-dot" aria-hidden="true" />;
}

const stepType: Record<StepId, string> = { implement: "agent", checks: "your checks", review: "independent review" };

function Overview({ state, goto, openWindow }: { state: DemoState; goto: (t: CockpitTab) => void; openWindow: (id: WinId) => void }) {
  const done = state.phase === "done";
  return (
    <div className="ov">
      {done ? (
        <div className="ov-ready" data-testid="ready">
          <XPIcon name="check" size={36} />
          <div>
            <p className="ov-ready__title">
              <ShinyText color="#0b5d0b" shineColor="#ffffff">
                Ready for your review
              </ShinyText>
            </p>
            <p className="ov-ready__text">
              Branch <code>{TASK.branch}</code> and its report are waiting. Nothing was pushed or published.
            </p>
            <div className="ov-ready__actions">
              <XPButton variant="go" icon="report" onClick={() => openWindow("evidence")}>
                Open the report
              </XPButton>
              <XPButton icon="review" onClick={() => goto("review")}>
                See the diff
              </XPButton>
            </div>
          </div>
        </div>
      ) : null}

      <h4 className="section-label">Steps</h4>
      <ol className="steps">
        {STEP_ORDER.map((id) => {
          const step = state.steps[id];
          return (
            <li key={id} className={`step step--${step.status}`} data-step={id} data-status={step.status}>
              <StepGlyph status={step.status} />
              <div className="step__main">
                <p className="step__title">
                  {STEP_LABEL[id]} <span className="step__type">{stepType[id]}</span>
                </p>
                <p className="step__detail">{step.detail}</p>
              </div>
              <div className="step__logins">
                {step.logins.map((l, i) => (
                  <span key={l} className={`chip ${l === "work-1" && limitHasHappened(state) ? "chip--limit" : ""}`}>
                    {i > 0 ? <span aria-label="then">→ </span> : null}
                    {l}
                  </span>
                ))}
                {step.attempts > 1 ? <span className="chip chip--plain">{step.attempts} attempts</span> : null}
              </div>
            </li>
          );
        })}
      </ol>

      {state.checkpoint ? (
        <div className="checkpoint" data-testid="checkpoint">
          <XPIcon name="checkpoint" size={34} />
          <div>
            <p className="checkpoint__title">Checkpoint saved</p>
            <p className="checkpoint__text">
              <code>{state.checkpoint.sha}</code> {state.checkpoint.message}
            </p>
            <p className="checkpoint__text">
              {state.checkpoint.files.length} file{state.checkpoint.files.length === 1 ? "" : "s"} from{" "}
              {state.checkpoint.login}:{" "}
              {state.checkpoint.files.map((f) => (
                <code key={f} className="checkpoint__file">
                  {f}
                </code>
              ))}
            </p>
            <p className="checkpoint__text checkpoint__text--quiet">
              The next login is told where things stand and continues from this commit.
            </p>
          </div>
        </div>
      ) : null}

      <h4 className="section-label">So far</h4>
      <dl className="tally">
        <div>
          <dt>Steps done</dt>
          <dd>
            <CountUp to={stepsDone(state)} /> <span className="tally__of">of 3</span>
          </dd>
        </div>
        <div>
          <dt>Files changed</dt>
          <dd>
            <CountUp to={state.files.length} />
          </dd>
        </div>
        <div>
          <dt>Commits</dt>
          <dd>
            <CountUp to={state.commits.length} />
          </dd>
        </div>
        <div>
          <dt>Logins used</dt>
          <dd>
            <CountUp to={loginsUsed(state)} />
          </dd>
        </div>
      </dl>
    </div>
  );
}

function Journal({ state }: { state: DemoState }) {
  const ref = useRef<HTMLOListElement>(null);
  const stick = useRef(true);
  useEffect(() => {
    const el = ref.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [state.journal.length]);

  return (
    <ol
      className="journal"
      ref={ref}
      role="log"
      aria-live="off"
      aria-label="Run journal"
      tabIndex={0}
      onScroll={(e) => {
        const el = e.currentTarget;
        stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
      }}
    >
      {state.journal.length === 0 ? <li className="journal__empty">Nothing yet. Press Start to begin the run.</li> : null}
      {state.journal.map((l) => (
        <li key={l.id} className={`journal__line journal__line--${l.kind}`}>
          <time>{formatClock(l.at)}</time>
          <span className="journal__step">{l.step ? `[${l.step}]` : ""}</span>
          <span className="journal__text">{l.text}</span>
        </li>
      ))}
    </ol>
  );
}

function Checks({ state }: { state: DemoState }) {
  const running = state.checks.find((c) => c.status === "running");
  return (
    <div className="checks">
      <p className="checks__lead">
        Your checks run after the implementation. A failing check is sent back to the agent; only one heavy check runs at a
        time across runs.
      </p>
      <ul className="checks__list">
        {state.checks.map((c) => (
          <li key={c.command} className={`check check--${c.status}`} data-status={c.status}>
            {c.status === "passed" ? (
              <XPIcon name="check" size={22} />
            ) : c.status === "running" ? (
              <span className="step-spinner" aria-hidden="true" />
            ) : (
              <span className="step-dot" aria-hidden="true" />
            )}
            <code>{c.command}</code>
            <span className="check__state">{c.status === "passed" ? "passed" : c.status === "running" ? "running" : "waiting"}</span>
          </li>
        ))}
      </ul>
      {running ? <XPProgress label={`Running ${running.command}`} /> : null}
      {state.steps.checks.status === "succeeded" ? (
        <p className="checks__result">
          <XPIcon name="check" size={18} /> 2 command(s) passed.
        </p>
      ) : null}
    </div>
  );
}

function Review({ state }: { state: DemoState }) {
  const diff = sampleDiff(state);
  return (
    <div className="review">
      {state.verdict ? (
        <div className="verdict" data-testid="verdict">
          <XPIcon name="check" size={30} />
          <div>
            <p className="verdict__title">
              Approved by <strong>{state.verdict.by}</strong>
            </p>
            <p className="verdict__text">
              {state.verdict.independence === "other-provider"
                ? "A different provider from the author, so the review is independent."
                : ""}{" "}
              Verdict: “{state.verdict.summary}”. The reviewer ran read-only; anything it had touched would have been reverted.
            </p>
          </div>
        </div>
      ) : (
        <p className="review__wait">
          {state.steps.review.status === "running"
            ? "The reviewer is reading the diff, read-only."
            : "The review comes last, after the checks. The reviewer is a different provider from the author when you have one, and a review with no clear verdict goes to a person."}
        </p>
      )}
      <h4 className="section-label">Changes on {TASK.branch}</h4>
      {diff.length === 0 ? (
        <p className="review__wait">No files changed yet.</p>
      ) : (
        diff.map((d) => (
          <figure className="diff" key={d.path}>
            <figcaption>
              <XPIcon name="report" size={16} /> <code>{d.path}</code>
              <span className="diff__by">{d.by}</span>
            </figcaption>
            <pre aria-label={`Diff of ${d.path}`} tabIndex={0}>
              {d.lines.map((line, i) => (
                <span key={i} className={`diff__line diff__line--${line.startsWith("+") ? "add" : line.startsWith("-") ? "del" : line.startsWith("@@") ? "hunk" : "ctx"}`}>
                  {line}
                  {"\n"}
                </span>
              ))}
            </pre>
          </figure>
        ))
      )}
    </div>
  );
}

export function CockpitBody({
  state,
  tab,
  onTab,
  openWindow,
}: {
  state: DemoState;
  tab: CockpitTab;
  onTab: (t: CockpitTab) => void;
  openWindow: (id: WinId) => void;
}) {
  const pct = Math.round(progress(state) * 100);
  return (
    <XPExplorerPane
      address={{ icon: "branch", text: `${TASK.repo} › ${TASK.branch}` }}
      sidebar={
        <>
          <XPTaskGroup title="This run" icon="cockpit">
            <p className="task-title">{TASK.title}</p>
            <p className="task-status" data-testid="run-status">
              {statusLabel(state)}
            </p>
            <XPProgress value={pct} max={100} label={`Run progress ${pct} percent`} />
            <p className="task-meta">
              Base <code>{TASK.base}</code> at <code>{TASK.baseCommit}</code>
            </p>
          </XPTaskGroup>
          <XPTaskGroup title="Logins" icon="logins" optional>
            <ul className="mini-logins">
              {LOGINS.map((l) => (
                <li key={l.id} title={state.logins[l.id].note}>
                  <span className="mini-logins__name">{l.id}</span>
                  <LoginBadge status={state.logins[l.id].status} />
                </li>
              ))}
            </ul>
          </XPTaskGroup>
        </>
      }
    >
      <XPTabs value={tab} onValueChange={(v) => onTab(v as CockpitTab)} className="cockpit-tabs">
        <XPTabList label="Run details">
          {COCKPIT_TABS.map((t) => (
            <XPTab key={t.id} value={t.id} icon={t.icon}>
              {t.label}
            </XPTab>
          ))}
        </XPTabList>
        <XPTabPanel value="overview">
          <Overview state={state} goto={onTab} openWindow={openWindow} />
        </XPTabPanel>
        <XPTabPanel value="journal">
          <Journal state={state} />
        </XPTabPanel>
        <XPTabPanel value="checks">
          <Checks state={state} />
        </XPTabPanel>
        <XPTabPanel value="review">
          <Review state={state} />
        </XPTabPanel>
      </XPTabs>
    </XPExplorerPane>
  );
}

/* ---------- Logins window ---------- */

export function LoginsBody({ state }: { state: DemoState }) {
  return (
    <ul className="logins">
      {LOGINS.map((l) => {
        const s = state.logins[l.id];
        return (
          <li key={l.id} className={`login login--${s.status}`} data-login={l.id} data-status={s.status}>
            <XPIcon name="logins" size={28} className="login__icon" />
            <p className="login__name">
              {l.id} <span className="login__provider">{l.provider}</span>
            </p>
            <LoginBadge status={s.status} />
            <p className="login__note">{s.note}</p>
          </li>
        );
      })}
    </ul>
  );
}

/* ---------- Getting started ---------- */

export function GuideBody({ state, reportOpened }: { state: DemoState; reportOpened: boolean }) {
  const started = state.phase !== "idle";
  const limit = limitHasHappened(state);
  const finished = state.phase === "done";
  const items: { done: boolean; title: string; text: ReactNode }[] = [
    { done: started, title: "Start the sample task", text: "Press Start in the cockpit toolbar." },
    {
      done: limit,
      title: "Hit a usage limit",
      text: limit
        ? `${state.limitSource === "injected" ? "You cut work-1 off" : "work-1 ran out"}; work-2 continues from a checkpoint.`
        : "Press Inject limit while work-1 works, or let it run out.",
    },
    { done: finished, title: "Wait for checks and review", text: "A different agent reviews, read-only." },
    { done: reportOpened, title: "Open the report", text: "evidence.md is what you read before merging." },
  ];
  return (
    <div className="guide">
      <ol className="guide__list">
        {items.map((it, i) => (
          <li key={i} className={it.done ? "is-done" : ""} data-done={it.done}>
            <span className="guide__mark" aria-hidden="true">
              {it.done ? <XPIcon name="check" size={20} /> : <span>{i + 1}</span>}
            </span>
            <div>
              <p className="guide__title">
                {it.title}
                {it.done ? <span className="visually-hidden"> (done)</span> : null}
              </p>
              <p className="guide__text">{it.text}</p>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}

/* ---------- evidence.md and verification notes ---------- */

export function EvidenceBody({ state }: { state: DemoState }) {
  return (
    <pre className="evidence" tabIndex={0} aria-label="evidence.md">
      {buildEvidence(state)}
    </pre>
  );
}

export function NotesBody() {
  return (
    <div className="notes">
      <p className="notes__lead">
        A short version of <a href={links.status} target="_blank" rel="noopener noreferrer">docs/STATUS.md</a>. If something is
        not listed as verified, assume it has not been run for real.
      </p>
      {statusGroups
        .filter((g) => g.id === "real" || g.id === "not")
        .map((g) => (
          <section key={g.id}>
            <h4 className="section-label">
              <XPIcon name={g.icon} size={16} /> {g.title}
            </h4>
            <ul>
              {g.items.map((i) => (
                <li key={i.lead}>
                  <strong>{i.lead}.</strong> {i.text}
                </li>
              ))}
            </ul>
          </section>
        ))}
    </div>
  );
}

export function statusFields(state: DemoState): ReactNode[] {
  return [
    <>
      <XPIcon name="info" size={14} /> Simulated demo
    </>,
    statusLabel(state),
    `Step ${state.phase === "done" ? 3 : Math.min(3, stepsDone(state) + 1)} of 3`,
  ];
}
