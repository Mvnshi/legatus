import { useEffect, useRef, useState } from "react";
import "./codeline.css";

/** A command or value in a monospace well with a Copy button. The result is announced politely. */
export function CodeLine({ text, label, wrap = false }: { text: string; label: string; wrap?: boolean }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setState("copied");
    } catch {
      setState("failed");
    }
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setState("idle"), 2200);
  };

  return (
    <div className="xp-codeline">
      <code className={wrap ? "is-wrap" : undefined} tabIndex={0} aria-label={`${label}: ${text}`}>
        {text}
      </code>
      <button type="button" className="xp-codeline__copy" onClick={copy}>
        {state === "copied" ? "Copied" : state === "failed" ? "Select it" : "Copy"}
        <span className="visually-hidden"> {label}</span>
      </button>
      <span className="visually-hidden" role="status">
        {state === "copied" ? `${label} copied to the clipboard.` : state === "failed" ? "Copying is not allowed here. Select the text instead." : ""}
      </span>
    </div>
  );
}
