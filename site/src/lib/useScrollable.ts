import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

/**
 * Tells whether an element currently scrolls. A region that scrolls must be reachable by keyboard, or people who
 * cannot use a mouse cannot read what is off-screen. Most of the time it does not scroll, and then it is better
 * not to add a Tab stop, so this reports the real state and the caller adds `tabIndex` only while it is true.
 */
export function useScrollable<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [scrollable, setScrollable] = useState(false);

  const measure = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    const style = getComputedStyle(el);
    const scrollsY = /auto|scroll/.test(style.overflowY) && el.scrollHeight > el.clientHeight + 1;
    const scrollsX = /auto|scroll/.test(style.overflowX) && el.scrollWidth > el.clientWidth + 1;
    const next = scrollsY || scrollsX;
    setScrollable((prev) => (prev === next ? prev : next));
  }, []);

  // After every render: the content may have changed even when the box did not.
  useLayoutEffect(measure);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    for (const child of Array.from(el.children)) observer.observe(child);
    return () => observer.disconnect();
  }, [measure]);

  return { ref, scrollable };
}
