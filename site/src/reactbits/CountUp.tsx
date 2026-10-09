/*
 * Adapted from React Bits (https://github.com/DavidHDev/react-bits) by David Haz:
 *   src/ts-default/TextAnimations/CountUp/CountUp.tsx
 * License: MIT + Commons Clause, see ./LICENSE.md. This file is NOT covered by the MIT license of the rest of the
 * Legatus repository; it stays under its original terms.
 *
 * Changes from the original:
 *  - Counts toward `to` every time it changes, not once when scrolled into view, so it can show live progress.
 *  - Respects the visitor's reduced-motion setting by jumping straight to the value.
 *  - Drops the separator, delay, direction and callback options, which this site does not use.
 *  - Renders the number through React state so the current value is always in the DOM text.
 */
import { useEffect, useState } from "react";
import { useMotionValue, useReducedMotion, useSpring } from "motion/react";

interface CountUpProps {
  to: number;
  /** Seconds the count takes to settle. */
  duration?: number;
  className?: string;
}

export default function CountUp({ to, duration = 0.8, className = "" }: CountUpProps) {
  const reduced = useReducedMotion();
  const target = useMotionValue(to);
  const spring = useSpring(target, {
    damping: 20 + 40 * (1 / duration),
    stiffness: 100 * (1 / duration),
  });
  const [shown, setShown] = useState(to);

  useEffect(() => {
    target.set(to);
    if (reduced) {
      spring.jump(to);
      setShown(to);
    }
  }, [to, reduced, target, spring]);

  useEffect(() => spring.on("change", (latest: number) => setShown(Math.round(latest))), [spring]);

  return <span className={className}>{shown}</span>;
}
