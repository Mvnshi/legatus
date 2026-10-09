import type { ReactNode } from "react";
import { XPIcon, type XPIconName } from "./icons";
import "./explorer.css";

/**
 * The Explorer layout: a blue task pane of collapsible-looking groups on the left, an inset white content area
 * on the right. It collapses to one column when its own width is small (container query), so it behaves inside a
 * narrow window and on a phone alike.
 */
export function XPExplorerPane({
  sidebar,
  toolbar,
  address,
  children,
  className = "",
}: {
  sidebar: ReactNode;
  toolbar?: ReactNode;
  /** The address bar text, for example `legatus/7c1f4a90`. */
  address?: { icon: XPIconName; text: string };
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={`xp-explorer ${className}`}>
      {toolbar ? <div className="xp-explorer__toolbar">{toolbar}</div> : null}
      {address ? (
        <div className="xp-explorer__address">
          <span className="xp-explorer__address-label">Address</span>
          <span className="xp-explorer__address-box">
            <XPIcon name={address.icon} size={16} />
            <span>{address.text}</span>
          </span>
        </div>
      ) : null}
      <div className="xp-explorer__split">
        <aside className="xp-explorer__tasks">{sidebar}</aside>
        <div className="xp-explorer__content">{children}</div>
      </div>
    </div>
  );
}

/** One group in the task pane: a rounded header and a pale body. */
export function XPTaskGroup({
  title,
  icon,
  children,
  tone = "primary",
  optional = false,
}: {
  title: string;
  icon?: XPIconName;
  children: ReactNode;
  tone?: "primary" | "plain";
  /** Dropped when the pane is too narrow for a side column, to keep the main content near the top. */
  optional?: boolean;
}) {
  return (
    <section className={`xp-taskgroup xp-taskgroup--${tone} ${optional ? "xp-taskgroup--optional" : ""}`}>
      <h3 className="xp-taskgroup__head">
        {icon ? <XPIcon name={icon} size={18} /> : null}
        <span>{title}</span>
      </h3>
      <div className="xp-taskgroup__body">{children}</div>
    </section>
  );
}
