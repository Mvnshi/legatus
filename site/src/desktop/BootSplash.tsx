import { useEffect, useRef } from "react";
import { motion, useReducedMotion } from "motion/react";
import { XPIcon } from "../xp/icons";
import "./boot.css";

const BOOT_MS = 2600;

/**
 * An optional start-up screen. It only ever appears when a person asks for it from the Start menu, it can be
 * skipped by clicking or pressing any key, and with reduced motion it is a short static card.
 */
export function BootSplash({ onDone }: { onDone: () => void }) {
  const reduced = useReducedMotion();
  const skip = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    skip.current?.focus();
    const t = window.setTimeout(onDone, reduced ? 900 : BOOT_MS);
    return () => window.clearTimeout(t);
  }, [onDone, reduced]);

  return (
    <motion.div
      className="boot"
      role="dialog"
      aria-modal="true"
      aria-label="Starting the Legatus desktop"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: reduced ? 0 : 0.3 }}
      onKeyDown={(e) => {
        if (e.key === "Tab") e.preventDefault();
        else onDone();
      }}
      onClick={onDone}
    >
      <div className="boot__center">
        <XPIcon name="legatus" size={96} />
        <p className="boot__name">Legatus</p>
        <p className="boot__tag">Command center</p>
        <div className="boot__bar" aria-hidden="true">
          <span className={reduced ? "is-still" : ""} />
        </div>
      </div>
      <button ref={skip} type="button" className="boot__skip" onClick={onDone}>
        Skip
      </button>
    </motion.div>
  );
}
