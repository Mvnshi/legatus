/*
 * Adapted from React Bits (https://github.com/DavidHDev/react-bits) by David Haz:
 *   src/ts-default/TextAnimations/ShinyText/ShinyText.tsx
 * License: MIT + Commons Clause, see ./LICENSE.md. This file is NOT covered by the MIT license of the rest of the
 * Legatus repository; it stays under its original terms.
 *
 * Changes from the original:
 *  - Rewritten as a small one-shot sweep: a highlight crosses the text once when it appears, then stops.
 *    The original's looping, pointer following, bands, glow and canvas colour parsing are removed.
 *  - Driven by a Motion value instead of a requestAnimationFrame loop.
 *  - With reduced motion there is no sweep at all; the text is simply drawn in its final colour.
 */
import { useEffect, type ReactNode } from "react";
import { animate, motion, useMotionValue, useMotionTemplate, useReducedMotion } from "motion/react";

interface ShinyTextProps {
  children: ReactNode;
  color?: string;
  shineColor?: string;
  /** Seconds the sweep takes. */
  speed?: number;
  /** Seconds to wait before it starts. */
  delay?: number;
  className?: string;
}

export default function ShinyText({
  children,
  color = "currentColor",
  shineColor = "#ffffff",
  speed = 1.4,
  delay = 0.15,
  className = "",
}: ShinyTextProps) {
  const reduced = useReducedMotion();
  const position = useMotionValue(-30);
  const gradient = useMotionTemplate`linear-gradient(110deg, ${color} ${position}%, ${shineColor} calc(${position}% + 12%), ${color} calc(${position}% + 24%))`;

  useEffect(() => {
    if (reduced) return;
    const run = animate(position, 130, { duration: speed, delay, ease: "easeInOut" });
    return () => run.stop();
  }, [reduced, position, speed, delay]);

  if (reduced) return <span className={className}>{children}</span>;

  return (
    <motion.span
      className={`shiny-text ${className}`}
      style={{ backgroundImage: gradient, WebkitBackgroundClip: "text", backgroundClip: "text", color: "transparent" }}
    >
      {children}
    </motion.span>
  );
}
