import type { ReactElement, SVGProps } from "react";

/**
 * Original glossy icons for the Luna desktop. Every shape is drawn here; none is taken from Windows.
 * They share one set of gradients, declared once by <XPIconDefs /> near the top of the page.
 */

export function XPIconDefs() {
  return (
    <svg width="0" height="0" aria-hidden="true" focusable="false" style={{ position: "absolute" }}>
      <defs>
        <linearGradient id="lg-blue" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#9fcbff" />
          <stop offset=".5" stopColor="#2f78e6" />
          <stop offset="1" stopColor="#0a46b8" />
        </linearGradient>
        <linearGradient id="lg-green" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#c4f5aa" />
          <stop offset=".5" stopColor="#4fc23c" />
          <stop offset="1" stopColor="#1c861d" />
        </linearGradient>
        <linearGradient id="lg-red" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#ffb9a3" />
          <stop offset=".5" stopColor="#e8492b" />
          <stop offset="1" stopColor="#a31c10" />
        </linearGradient>
        <linearGradient id="lg-orange" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#ffd9a0" />
          <stop offset=".5" stopColor="#f58a1f" />
          <stop offset="1" stopColor="#b65208" />
        </linearGradient>
        <linearGradient id="lg-gold" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#ffeaa6" />
          <stop offset=".5" stopColor="#efb52e" />
          <stop offset="1" stopColor="#b5780a" />
        </linearGradient>
        <linearGradient id="lg-yellow" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#fff6b3" />
          <stop offset=".55" stopColor="#ffd13d" />
          <stop offset="1" stopColor="#e0a000" />
        </linearGradient>
        <linearGradient id="lg-cream" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#ffffff" />
          <stop offset=".6" stopColor="#efecdc" />
          <stop offset="1" stopColor="#cfc9b0" />
        </linearGradient>
        <linearGradient id="lg-silver" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#ffffff" />
          <stop offset=".5" stopColor="#d3dbea" />
          <stop offset="1" stopColor="#8e9cba" />
        </linearGradient>
        <linearGradient id="lg-dark" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#56627a" />
          <stop offset="1" stopColor="#161c2a" />
        </linearGradient>
        <linearGradient id="lg-gloss" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#fff" stopOpacity=".8" />
          <stop offset="1" stopColor="#fff" stopOpacity="0" />
        </linearGradient>
      </defs>
    </svg>
  );
}

const ink = "#1a1405";

type Draw = () => ReactElement;

const gloss = (x: number, y: number, w: number, h: number, r = 4) => (
  <rect x={x} y={y} width={w} height={h} rx={r} fill="url(#lg-gloss)" opacity=".7" />
);

const icons: Record<string, Draw> = {
  legatus: () => (
    <>
      <rect x="2.5" y="2.5" width="27" height="27" rx="7" fill="url(#lg-gold)" stroke="#8a5a06" />
      <path d="M11.5 8.5v14h9.5" fill="none" stroke={ink} strokeWidth="4.2" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="24.2" cy="22.5" r="2.3" fill={ink} />
      {gloss(4, 3.5, 24, 11, 5.5)}
    </>
  ),
  cockpit: () => (
    <>
      <rect x="3" y="4.5" width="26" height="19" rx="3" fill="url(#lg-blue)" stroke="#0a3a9a" />
      <rect x="5.8" y="7.3" width="20.4" height="13.4" rx="1.2" fill="#fff" stroke="#0a3a9a" strokeWidth=".6" />
      <rect x="8" y="14.5" width="3" height="4.2" fill="#2f78e6" />
      <rect x="12.6" y="11.5" width="3" height="7.2" fill="#4fc23c" />
      <rect x="17.2" y="13" width="3" height="5.7" fill="#f58a1f" />
      <rect x="21.8" y="10" width="2.4" height="8.7" fill="#2f78e6" />
      <path d="M12 27h8M16 23.5V27" stroke="#46526b" strokeWidth="2.4" strokeLinecap="round" />
      {gloss(3.6, 5.2, 24.8, 7)}
    </>
  ),
  logins: () => (
    <>
      <rect x="3.5" y="6.5" width="25" height="19" rx="3" fill="url(#lg-cream)" stroke="#8a8467" />
      <path d="M3.5 9.5a3 3 0 0 1 3-3h19a3 3 0 0 1 3 3v2.5h-25z" fill="url(#lg-blue)" />
      <circle cx="11.5" cy="16.5" r="3" fill="url(#lg-blue)" stroke="#0a3a9a" strokeWidth=".6" />
      <path d="M6.6 23.2c.4-3 2.4-4.4 4.9-4.4s4.5 1.4 4.9 4.4z" fill="url(#lg-blue)" stroke="#0a3a9a" strokeWidth=".6" />
      <path d="M19 15.5h6.5M19 19h6.5M19 22.5h4" stroke="#8d9ab5" strokeWidth="1.8" strokeLinecap="round" />
    </>
  ),
  report: () => (
    <>
      <path d="M7.5 3.5h13l5 5v20h-18z" fill="url(#lg-cream)" stroke="#8a8467" strokeLinejoin="round" />
      <path d="M20.5 3.5v5h5" fill="#d9d4bd" stroke="#8a8467" strokeLinejoin="round" />
      <path d="M11 13h10M11 16.5h10M11 20h6" stroke="#9aa6c0" strokeWidth="1.7" strokeLinecap="round" />
      <circle cx="22.5" cy="23.5" r="6" fill="url(#lg-green)" stroke="#146014" />
      <path d="m19.6 23.6 2.0 2.1 3.6-4.3" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </>
  ),
  status: () => (
    <>
      <path d="M16 2.8 27 6.8v8.4c0 6.6-4.6 10.8-11 13.9C9.6 26 5 21.800 5 15.200V6.800z" fill="url(#lg-blue)" stroke="#0a3a9a" strokeLinejoin="round" />
      <path d="m10.600 15.800 3.800 3.800 7.200-8.200" fill="none" stroke="#fff" strokeWidth="2.800" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M16 3.600 26 7.200v3.500C22 8.800 19 8.600 16 8.600z" fill="url(#lg-gloss)" opacity=".6" />
    </>
  ),
  download: () => (
    <>
      <circle cx="16" cy="16" r="13" fill="url(#lg-green)" stroke="#146014" />
      <path d="M16 7.500v10.500m-5-4.800 5 5.200 5-5.200" fill="none" stroke="#fff" strokeWidth="3.200" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M9.500 23.500h13" stroke="#fff" strokeWidth="3" strokeLinecap="round" />
      <ellipse cx="16" cy="9" rx="10" ry="6.500" fill="url(#lg-gloss)" opacity=".6" />
    </>
  ),
  branch: () => (
    <>
      <rect x="2.500" y="2.500" width="27" height="27" rx="7" fill="url(#lg-green)" stroke="#146014" />
      <path d="M10 11c0 6 6 5 6 10M22 11c0 6-6 5-6 10" fill="none" stroke="#fff" strokeWidth="2.400" strokeLinecap="round" />
      <circle cx="10" cy="9" r="3.200" fill="#fff" stroke="#146014" strokeWidth=".8" />
      <circle cx="22" cy="9" r="3.200" fill="#fff" stroke="#146014" strokeWidth=".8" />
      <circle cx="16" cy="23" r="3.200" fill="#fff" stroke="#146014" strokeWidth=".8" />
      {gloss(4, 3.500, 24, 11, 5.500)}
    </>
  ),
  play: () => (
    <>
      <circle cx="16" cy="16" r="13" fill="url(#lg-green)" stroke="#146014" />
      <path d="M12.500 9.500v13l11-6.500z" fill="#fff" />
      <ellipse cx="16" cy="9" rx="10" ry="6.500" fill="url(#lg-gloss)" opacity=".6" />
    </>
  ),
  pause: () => (
    <>
      <circle cx="16" cy="16" r="13" fill="url(#lg-blue)" stroke="#0a3a9a" />
      <rect x="10.500" y="9.500" width="4" height="13" rx="1" fill="#fff" />
      <rect x="17.500" y="9.500" width="4" height="13" rx="1" fill="#fff" />
      <ellipse cx="16" cy="9" rx="10" ry="6.500" fill="url(#lg-gloss)" opacity=".6" />
    </>
  ),
  replay: () => (
    <>
      <circle cx="16" cy="16" r="13" fill="url(#lg-blue)" stroke="#0a3a9a" />
      <path d="M22.200 11.200A7.800 7.800 0 1 0 23.800 17" fill="none" stroke="#fff" strokeWidth="2.800" strokeLinecap="round" />
      <path d="m23.500 6.800.2 6.200-6-1z" fill="#fff" />
      <ellipse cx="16" cy="9" rx="10" ry="6.500" fill="url(#lg-gloss)" opacity=".6" />
    </>
  ),
  limit: () => (
    <>
      <circle cx="16" cy="16" r="13" fill="url(#lg-orange)" stroke="#9a4606" />
      <path d="M17.800 6.500 10.500 17.600h4.800l-1.200 7.900 7.600-11.300h-4.900z" fill="#fff" stroke="#9a4606" strokeWidth=".6" strokeLinejoin="round" />
      <ellipse cx="16" cy="9" rx="10" ry="6.500" fill="url(#lg-gloss)" opacity=".55" />
    </>
  ),
  checkpoint: () => (
    <>
      <path d="M4 6a2 2 0 0 1 2-2h17l5 5v17a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" fill="url(#lg-blue)" stroke="#0a3a9a" strokeLinejoin="round" />
      <rect x="9" y="4.500" width="12" height="7.500" rx="1" fill="url(#lg-silver)" stroke="#6b7a99" strokeWidth=".7" />
      <rect x="17.200" y="5.800" width="2.200" height="4.600" rx=".5" fill="#46526b" />
      <rect x="8" y="16.500" width="16" height="11" rx="1.200" fill="#fff" stroke="#6b7a99" strokeWidth=".7" />
      <path d="M10.500 20h11M10.500 23.200h8" stroke="#9aa6c0" strokeWidth="1.500" strokeLinecap="round" />
    </>
  ),
  check: () => (
    <>
      <circle cx="16" cy="16" r="13" fill="url(#lg-green)" stroke="#146014" />
      <path d="m9.500 16.500 4.500 4.600 8.500-9.800" fill="none" stroke="#fff" strokeWidth="3.200" strokeLinecap="round" strokeLinejoin="round" />
      <ellipse cx="16" cy="9" rx="10" ry="6.500" fill="url(#lg-gloss)" opacity=".6" />
    </>
  ),
  warning: () => (
    <>
      <path d="M16 3.800 29.200 27a1.500 1.500 0 0 1-1.300 2.200H4.100A1.500 1.500 0 0 1 2.800 27z" fill="url(#lg-yellow)" stroke="#8a6200" strokeLinejoin="round" />
      <path d="M16 11.500v8" stroke={ink} strokeWidth="3" strokeLinecap="round" />
      <circle cx="16" cy="24" r="1.800" fill={ink} />
    </>
  ),
  info: () => (
    <>
      <circle cx="16" cy="16" r="13" fill="url(#lg-blue)" stroke="#0a3a9a" />
      <circle cx="16" cy="9.800" r="2" fill="#fff" />
      <path d="M16 14.500v9" stroke="#fff" strokeWidth="3.200" strokeLinecap="round" />
      <ellipse cx="16" cy="9" rx="10" ry="6.500" fill="url(#lg-gloss)" opacity=".5" />
    </>
  ),
  folder: () => (
    <>
      <path d="M3 8a2 2 0 0 1 2-2h7l3 3h12a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" fill="url(#lg-yellow)" stroke="#8a6200" strokeLinejoin="round" />
      <path d="M3 13h26" stroke="#fff7c0" strokeWidth="1.500" />
      {gloss(4, 14, 24, 6, 2)}
    </>
  ),
  terminal: () => (
    <>
      <rect x="3" y="5" width="26" height="22" rx="3" fill="url(#lg-dark)" stroke="#0b0f18" />
      <rect x="3" y="5" width="26" height="5" rx="3" fill="url(#lg-blue)" />
      <path d="m8 15 4 3.200L8 21.500" fill="none" stroke="#7cf56a" strokeWidth="2.200" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M15 22h7" stroke="#cfe6ff" strokeWidth="2.200" strokeLinecap="round" />
    </>
  ),
  speaker: () => (
    <>
      <path d="M4 12h5l7-5.500v19L9 20H4z" fill="url(#lg-silver)" stroke="#5a6785" strokeLinejoin="round" />
      <path d="M20 11.500a6 6 0 0 1 0 9M23.500 8a11 11 0 0 1 0 16" fill="none" stroke="#2f78e6" strokeWidth="2.200" strokeLinecap="round" />
    </>
  ),
  "speaker-off": () => (
    <>
      <path d="M4 12h5l7-5.500v19L9 20H4z" fill="url(#lg-silver)" stroke="#5a6785" strokeLinejoin="round" />
      <path d="m20.500 12 7 8m0-8-7 8" stroke="#d6371c" strokeWidth="2.600" strokeLinecap="round" />
    </>
  ),
  review: () => (
    <>
      <path d="M5.500 3.500h12l4.500 4.500v8" fill="url(#lg-cream)" stroke="#8a8467" strokeLinejoin="round" />
      <path d="M5.500 3.500v22h8" fill="url(#lg-cream)" stroke="#8a8467" strokeLinejoin="round" />
      <path d="M9 10h8M9 14h8M9 18h4" stroke="#9aa6c0" strokeWidth="1.600" strokeLinecap="round" />
      <circle cx="20.500" cy="20.500" r="6" fill="#e8f2ff" fillOpacity=".7" stroke="#2f78e6" strokeWidth="2.600" />
      <path d="m25 25 4.200 4.200" stroke="#46526b" strokeWidth="3.400" strokeLinecap="round" />
    </>
  ),
  queue: () => (
    <>
      <rect x="6" y="3.500" width="20" height="7.500" rx="2" fill="url(#lg-blue)" stroke="#0a3a9a" />
      <rect x="4.500" y="12.200" width="23" height="7.500" rx="2" fill="url(#lg-cream)" stroke="#8a8467" />
      <rect x="3" y="21" width="26" height="7.500" rx="2" fill="url(#lg-cream)" stroke="#8a8467" />
      <path d="M8 7.200h10M7 16h12M6 24.800h14" stroke="#fff" strokeWidth="1.600" strokeLinecap="round" opacity=".9" />
      <path d="M8 16h12M6 24.800h14" stroke="#9aa6c0" strokeWidth="1.600" strokeLinecap="round" />
    </>
  ),
  book: () => (
    <>
      <path d="M5 5.500a2 2 0 0 1 2-2h18v22H7a2 2 0 0 0-2 2z" fill="url(#lg-blue)" stroke="#0a3a9a" strokeLinejoin="round" />
      <path d="M5 27.500a2 2 0 0 1 2-2h18v3H7a2 2 0 0 1-2-1z" fill="url(#lg-cream)" stroke="#8a8467" strokeLinejoin="round" />
      <path d="M10 9.500h10M10 13h10" stroke="#fff" strokeWidth="1.800" strokeLinecap="round" />
    </>
  ),
  lock: () => (
    <>
      <path d="M10 14v-3.500a6 6 0 0 1 12 0V14" fill="none" stroke="#7a8397" strokeWidth="3" strokeLinecap="round" />
      <rect x="6" y="13.500" width="20" height="15" rx="3" fill="url(#lg-gold)" stroke="#8a5a06" />
      <circle cx="16" cy="20" r="2.200" fill={ink} />
      <path d="M16 21v3.500" stroke={ink} strokeWidth="2" strokeLinecap="round" />
      {gloss(7, 14.500, 18, 6, 2)}
    </>
  ),
  globe: () => (
    <>
      <circle cx="16" cy="16" r="13" fill="url(#lg-blue)" stroke="#0a3a9a" />
      <path d="M8 11c3 0 4 2.500 7 2s2-4 5-5.500M7 19c3-1 4 1 6 3s1 5 1 5M20 16c2-1 4 0 6 2" fill="none" stroke="#7de26a" strokeWidth="2.800" strokeLinecap="round" />
      <ellipse cx="16" cy="9" rx="10" ry="6.500" fill="url(#lg-gloss)" opacity=".55" />
    </>
  ),
  "all-programs": () => (
    <>
      <rect x="3.500" y="3.500" width="11" height="11" rx="2.500" fill="url(#lg-blue)" stroke="#0a3a9a" />
      <rect x="17.500" y="3.500" width="11" height="11" rx="2.500" fill="url(#lg-green)" stroke="#146014" />
      <rect x="3.500" y="17.500" width="11" height="11" rx="2.500" fill="url(#lg-orange)" stroke="#9a4606" />
      <rect x="17.500" y="17.500" width="11" height="11" rx="2.500" fill="url(#lg-gold)" stroke="#8a5a06" />
    </>
  ),
  power: () => (
    <>
      <rect x="3" y="3" width="26" height="26" rx="6" fill="url(#lg-red)" stroke="#8a1a0e" />
      <path d="M16 8v8.500M10.500 11.200a7.600 7.600 0 1 0 11 0" fill="none" stroke="#fff" strokeWidth="2.800" strokeLinecap="round" />
      {gloss(4, 4, 24, 11, 5)}
    </>
  ),
};

export type XPIconName = keyof typeof icons;

export const iconNames = Object.keys(icons) as XPIconName[];

interface XPIconProps extends Omit<SVGProps<SVGSVGElement>, "name"> {
  name: XPIconName;
  size?: number;
}

/** A decorative glossy icon. The label next to it carries the meaning, so it is hidden from screen readers. */
export function XPIcon({ name, size = 16, ...rest }: XPIconProps) {
  const draw = icons[name];
  if (!draw) return null;
  return (
    <svg
      viewBox="0 0 32 32"
      width={size}
      height={size}
      aria-hidden="true"
      focusable="false"
      className="xp-icon"
      {...rest}
    >
      {draw()}
    </svg>
  );
}
