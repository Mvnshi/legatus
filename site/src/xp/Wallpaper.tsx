import { Component, lazy, Suspense, useEffect, useRef, useState, type ReactNode } from "react";
import { useInView, useReducedMotion } from "motion/react";
import "./wallpaper.css";

// The one optional effect on the page: a slow Paper Shaders gradient drifting through the sky. It is split into
// its own chunk and only loaded when the browser can run it and the person has not asked for less motion.
const WallpaperShader = lazy(() => import("./WallpaperShader"));

/** The shader is a bonus. If its chunk cannot load or WebGL fails, draw nothing rather than take the page down. */
class ShaderBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? null : this.props.children;
  }
}

let webglSupport: boolean | undefined;
function hasWebGL(): boolean {
  if (webglSupport !== undefined) return webglSupport;
  try {
    const canvas = document.createElement("canvas");
    webglSupport = Boolean(canvas.getContext("webgl2") ?? canvas.getContext("webgl"));
  } catch {
    webglSupport = false;
  }
  return webglSupport;
}

function savingData(): boolean {
  const connection = (navigator as Navigator & { connection?: { saveData?: boolean } }).connection;
  return Boolean(connection?.saveData);
}

/**
 * An original wallpaper: a bright sky, layered hills, a road that winds toward the horizon and a small banner on
 * the crest. The static SVG is always drawn; the shader is a bonus on top.
 */
export function Wallpaper({ shader = true }: { shader?: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  const reduced = useReducedMotion();
  const visible = useInView(ref, { margin: "120px" });
  const [capable, setCapable] = useState(false);

  useEffect(() => {
    if (!shader || reduced || savingData() || !hasWebGL()) {
      setCapable(false);
      return;
    }
    const idle = window.requestIdleCallback ?? ((cb: () => void) => window.setTimeout(cb, 400));
    const cancel = window.cancelIdleCallback ?? window.clearTimeout;
    const handle = idle(() => setCapable(true));
    return () => cancel(handle as number);
  }, [shader, reduced]);

  return (
    <div className="wallpaper" ref={ref} aria-hidden="true" data-shader={capable && visible ? "on" : "off"}>
      <svg
        className="wallpaper__art"
        viewBox="0 0 1600 1000"
        preserveAspectRatio="xMidYMax slice"
        focusable="false"
      >
        <defs>
          <linearGradient id="wp-sky" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#1748b8" />
            <stop offset=".34" stopColor="#2f78e3" />
            <stop offset=".6" stopColor="#74b0f3" />
            <stop offset=".74" stopColor="#bfdcfa" />
            <stop offset="1" stopColor="#e9f4ff" />
          </linearGradient>
          <radialGradient id="wp-glow" cx=".72" cy=".66" r=".55">
            <stop offset="0" stopColor="#fff" stopOpacity=".7" />
            <stop offset="1" stopColor="#fff" stopOpacity="0" />
          </radialGradient>
          <radialGradient id="wp-cloud">
            <stop offset="0" stopColor="#fff" stopOpacity=".95" />
            <stop offset=".55" stopColor="#fff" stopOpacity=".55" />
            <stop offset="1" stopColor="#fff" stopOpacity="0" />
          </radialGradient>
          <linearGradient id="wp-far" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#8db4e2" />
            <stop offset="1" stopColor="#b5d0ee" />
          </linearGradient>
          <linearGradient id="wp-mid" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#6dbd74" />
            <stop offset="1" stopColor="#3d9552" />
          </linearGradient>
          <linearGradient id="wp-near" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#9bdb52" />
            <stop offset=".55" stopColor="#58b032" />
            <stop offset="1" stopColor="#2f8a28" />
          </linearGradient>
          <linearGradient id="wp-front" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#7cc93f" />
            <stop offset="1" stopColor="#1f6e1f" />
          </linearGradient>
          <linearGradient id="wp-road" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#f6ecc4" />
            <stop offset="1" stopColor="#cfb878" />
          </linearGradient>
        </defs>

        <rect width="1600" height="1000" fill="url(#wp-sky)" />
        <rect width="1600" height="1000" fill="url(#wp-glow)" />

        <g>
          <ellipse cx="260" cy="250" rx="230" ry="46" fill="url(#wp-cloud)" />
          <ellipse cx="400" cy="226" rx="170" ry="52" fill="url(#wp-cloud)" />
          <ellipse cx="140" cy="236" rx="140" ry="36" fill="url(#wp-cloud)" />
          <ellipse cx="1180" cy="170" rx="250" ry="42" fill="url(#wp-cloud)" />
          <ellipse cx="1320" cy="150" rx="170" ry="46" fill="url(#wp-cloud)" />
          <ellipse cx="860" cy="360" rx="210" ry="30" fill="url(#wp-cloud)" opacity=".8" />
          <ellipse cx="1480" cy="400" rx="180" ry="28" fill="url(#wp-cloud)" opacity=".75" />
          <ellipse cx="520" cy="470" rx="240" ry="28" fill="url(#wp-cloud)" opacity=".7" />
        </g>

        <path
          d="M0 640 C150 590 290 560 450 598 S740 662 920 604 S1250 540 1600 612 V1000 H0Z"
          fill="url(#wp-far)"
        />
        <path
          d="M0 700 C210 640 380 640 560 684 S900 730 1100 668 S1420 630 1600 676 V1000 H0Z"
          fill="url(#wp-mid)"
        />
        <g fill="#2f7a42" opacity=".55">
          <circle cx="330" cy="668" r="11" />
          <circle cx="352" cy="672" r="8" />
          <circle cx="388" cy="676" r="10" />
          <circle cx="1042" cy="688" r="9" />
          <circle cx="1066" cy="690" r="12" />
          <circle cx="1394" cy="656" r="10" />
        </g>

        <path
          d="M-100 790 C220 690 600 650 960 722 S1460 800 1700 730 V1000 H-100Z"
          fill="url(#wp-near)"
        />
        <path
          d="M-100 790 C220 690 600 650 960 722 S1460 800 1700 730"
          fill="none"
          stroke="#e6ffb8"
          strokeWidth="3"
          opacity=".55"
        />

        {/* The road to the horizon. */}
        <path
          d="M690 1000 C770 940 560 900 650 842 S860 800 836 758 S800 732 812 716 L822 716 C816 734 856 752 880 760 C940 790 760 840 820 880 S1080 920 1020 1000Z"
          fill="url(#wp-road)"
          opacity=".95"
        />

        {/* A banner on the crest: a standard, as a legate would carry. */}
        <g transform="translate(1190 640)">
          <path d="M0 96V0" stroke="#5b3f12" strokeWidth="4" strokeLinecap="round" />
          <rect x="4" y="6" width="54" height="40" rx="4" fill="#e4b13a" stroke="#8a5a06" strokeWidth="2" />
          <path d="M22 16v22h20" fill="none" stroke="#1a1405" strokeWidth="6" strokeLinecap="round" strokeLinejoin="round" />
          <circle cx="47" cy="37" r="3.4" fill="#1a1405" />
          <circle cx="0" cy="-2" r="5" fill="#f3cd62" stroke="#8a5a06" strokeWidth="1.5" />
        </g>

        <path
          d="M-100 900 C260 810 640 830 1000 880 S1500 920 1700 870 V1000 H-100Z"
          fill="url(#wp-front)"
        />
        <path
          d="M-100 900 C260 810 640 830 1000 880 S1500 920 1700 870"
          fill="none"
          stroke="#d8ff9a"
          strokeWidth="3"
          opacity=".5"
        />
      </svg>
      {capable && visible ? (
        <div className="wallpaper__shader">
          <ShaderBoundary>
            <Suspense fallback={null}>
              <WallpaperShader />
            </Suspense>
          </ShaderBoundary>
        </div>
      ) : null}
    </div>
  );
}
