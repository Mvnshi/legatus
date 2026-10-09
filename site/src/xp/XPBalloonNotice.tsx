import { useEffect, useRef, useState, type ReactNode } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { XPIcon } from "./icons";
import "./balloon.css";

export interface BalloonNotice {
  id: number;
  tone: "info" | "warning" | "success";
  title: string;
  text: ReactNode;
  /** Stays until dismissed. Everything else goes away by itself after a while. */
  sticky?: boolean;
  actionLabel?: string;
  onAction?: () => void;
}

const AUTO_DISMISS_MS = 9000;

const toneIcon = { info: "info", warning: "warning", success: "check" } as const;

/**
 * A notification-area balloon. The region is always present so assistive technology announces what appears in
 * it; the balloon itself leaves on a timer unless it is sticky, and the timer waits while the pointer or the
 * keyboard focus is on it.
 */
export function XPBalloonNotice({ notice, onDismiss }: { notice: BalloonNotice | null; onDismiss: () => void }) {
  const reduced = useReducedMotion();
  const [paused, setPaused] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => {
    window.clearTimeout(timer.current);
    if (!notice || notice.sticky || paused) return;
    timer.current = window.setTimeout(onDismiss, AUTO_DISMISS_MS);
    return () => window.clearTimeout(timer.current);
  }, [notice, paused, onDismiss]);

  return (
    <div className="xp-balloon-region" role="status" aria-live="polite" aria-atomic="true">
      <AnimatePresence>
        {notice ? (
          <motion.div
            key={notice.id}
            className={`xp-balloon xp-balloon--${notice.tone}`}
            initial={reduced ? false : { opacity: 0, y: 12, scale: 0.96 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={reduced ? { opacity: 0 } : { opacity: 0, y: 8, scale: 0.97 }}
            transition={{ duration: reduced ? 0 : 0.2, ease: [0.2, 0.7, 0.2, 1] }}
            onMouseEnter={() => setPaused(true)}
            onMouseLeave={() => setPaused(false)}
            onFocus={() => setPaused(true)}
            onBlur={() => setPaused(false)}
            data-balloon={notice.tone}
          >
            <XPIcon name={toneIcon[notice.tone]} size={26} className="xp-balloon__icon" />
            <div className="xp-balloon__body">
              <p className="xp-balloon__title">{notice.title}</p>
              <p className="xp-balloon__text">{notice.text}</p>
              {notice.actionLabel && notice.onAction ? (
                <button type="button" className="xp-balloon__action" onClick={notice.onAction}>
                  {notice.actionLabel}
                </button>
              ) : null}
            </div>
            <button type="button" className="xp-balloon__close" aria-label="Dismiss notice" onClick={onDismiss}>
              <span aria-hidden="true">×</span>
            </button>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  );
}
