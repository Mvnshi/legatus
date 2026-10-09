import { forwardRef, type ButtonHTMLAttributes, type ComponentPropsWithoutRef, type ReactNode } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import * as Menubar from "@radix-ui/react-menubar";
import * as Tooltip from "@radix-ui/react-tooltip";
import { XPIcon, type XPIconName } from "./icons";
import "./controls.css";

/* ---------- Tabs: Radix behaviour, XP.css tab anatomy ---------- */

export const XPTabs = Tabs.Root;

export function XPTabList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Tabs.List asChild aria-label={label}>
      <menu className="xp-tablist">{children}</menu>
    </Tabs.List>
  );
}

export function XPTab({ value, icon, children }: { value: string; icon?: XPIconName; children: ReactNode }) {
  return (
    <Tabs.Trigger value={value} className="xp-tab">
      {icon ? <XPIcon name={icon} size={16} /> : null}
      {children}
    </Tabs.Trigger>
  );
}

export function XPTabPanel({ value, className = "", children }: { value: string; className?: string; children: ReactNode }) {
  return (
    <Tabs.Content value={value} className={`xp-tabpanel ${className}`} tabIndex={-1}>
      {children}
    </Tabs.Content>
  );
}

/* ---------- Tooltip ---------- */

export const XPTooltipProvider = ({ children }: { children: ReactNode }) => (
  <Tooltip.Provider delayDuration={350} skipDelayDuration={200}>
    {children}
  </Tooltip.Provider>
);

export function XPTooltip({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>{children}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content className="xp-tooltip" side="top" sideOffset={6} collisionPadding={8}>
          {label}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}

/* ---------- Progress: the green block bar from XP.css ---------- */

export function XPProgress({ value, max = 100, label }: { value?: number; max?: number; label: string }) {
  return <progress className="xp-progress" value={value} max={max} aria-label={label} />;
}

/* ---------- Buttons ---------- */

interface XPButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "default" | "go" | "toolbar";
  icon?: XPIconName;
  /** A button that is unavailable right now but explains itself: it stays focusable and says why. */
  unavailable?: boolean;
}

export const XPButton = forwardRef<HTMLButtonElement, XPButtonProps>(function XPButton(
  { variant = "default", icon, unavailable, className = "", children, onClick, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type="button"
      className={`xp-button xp-button--${variant} ${className}`}
      aria-disabled={unavailable || undefined}
      onClick={(e) => {
        if (unavailable) {
          e.preventDefault();
          return;
        }
        onClick?.(e);
      }}
      {...rest}
    >
      {icon ? <XPIcon name={icon} size={variant === "toolbar" ? 24 : 18} /> : null}
      <span>{children}</span>
    </button>
  );
});

type AnchorProps = ComponentPropsWithoutRef<"a"> & { variant?: "default" | "go"; icon?: XPIconName };

/** A link that looks like an XP push button. Use for navigation (downloads, anchors); use XPButton for actions. */
export function XPLinkButton({ variant = "default", icon, className = "", children, ...rest }: AnchorProps) {
  return (
    <a className={`xp-button xp-button--${variant} ${className}`} {...rest}>
      {icon ? <XPIcon name={icon} size={18} /> : null}
      <span>{children}</span>
    </a>
  );
}

/* ---------- Menu bar (File, View, ...) ---------- */

export function XPMenuBar({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Menubar.Root className="xp-menubar" aria-label={label}>
      {children}
    </Menubar.Root>
  );
}

export function XPMenu({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Menubar.Menu>
      <Menubar.Trigger className="xp-menubar__trigger">{label}</Menubar.Trigger>
      <Menubar.Portal>
        <Menubar.Content className="xp-menu" align="start" sideOffset={2} loop collisionPadding={8}>
          {children}
        </Menubar.Content>
      </Menubar.Portal>
    </Menubar.Menu>
  );
}

export function XPMenuItem({
  onSelect,
  disabled,
  shortcut,
  icon,
  children,
}: {
  onSelect?: () => void;
  disabled?: boolean;
  shortcut?: string;
  icon?: XPIconName;
  children: ReactNode;
}) {
  return (
    <Menubar.Item className="xp-menu__item" disabled={disabled} onSelect={onSelect}>
      <span className="xp-menu__icon">{icon ? <XPIcon name={icon} size={16} /> : null}</span>
      <span className="xp-menu__label">{children}</span>
      {shortcut ? <span className="xp-menu__shortcut">{shortcut}</span> : null}
    </Menubar.Item>
  );
}

export function XPMenuLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Menubar.Item className="xp-menu__item" asChild>
      <a href={href} target="_blank" rel="noopener noreferrer">
        <span className="xp-menu__icon" />
        <span className="xp-menu__label">{children}</span>
        <span className="xp-menu__shortcut" aria-hidden="true">
          ↗
        </span>
      </a>
    </Menubar.Item>
  );
}

export function XPMenuRadioGroup({
  value,
  onValueChange,
  children,
}: {
  value: string;
  onValueChange: (v: string) => void;
  children: ReactNode;
}) {
  return (
    <Menubar.RadioGroup value={value} onValueChange={onValueChange}>
      {children}
    </Menubar.RadioGroup>
  );
}

export function XPMenuRadioItem({ value, children }: { value: string; children: ReactNode }) {
  return (
    <Menubar.RadioItem className="xp-menu__item" value={value}>
      <span className="xp-menu__icon">
        <Menubar.ItemIndicator className="xp-menu__dot" />
      </span>
      <span className="xp-menu__label">{children}</span>
    </Menubar.RadioItem>
  );
}

export function XPMenuSeparator() {
  return <Menubar.Separator className="xp-menu__sep" />;
}
