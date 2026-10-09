import { MeshGradient } from "@paper-design/shaders-react";

/**
 * The page's only shader: a very slow, low-contrast mesh gradient in the sky colours, blended into the sky of
 * the SVG wallpaper. It is decoration, so it stays quiet: slow, soft, and masked away from the hills.
 */
export default function WallpaperShader() {
  return (
    <MeshGradient
      style={{ position: "absolute", inset: 0, width: "100%", height: "100%" }}
      colors={["#1f5fd6", "#5aa2f7", "#d7ebff", "#ffffff"]}
      distortion={0.85}
      swirl={0.2}
      grainMixer={0}
      grainOverlay={0}
      speed={0.12}
      minPixelRatio={1}
      maxPixelCount={1_000_000}
    />
  );
}
