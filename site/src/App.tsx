import { MotionConfig } from "motion/react";
import { XPIconDefs } from "./xp/icons";
import { XPTooltipProvider } from "./xp/controls";
import { SiteHeader } from "./sections/SiteHeader";
import { Hero } from "./sections/Hero";
import { DemoSection } from "./sections/DemoSection";
import { HowItWorks } from "./sections/HowItWorks";
import { Features } from "./sections/Features";
import { RealCockpit } from "./sections/RealCockpit";
import { StatusSection } from "./sections/StatusSection";
import { InstallSection } from "./sections/InstallSection";
import { SiteFooter } from "./sections/SiteFooter";

export function App() {
  return (
    // "user" makes Motion drop transform and layout animation when the visitor has asked their system for less motion.
    <MotionConfig reducedMotion="user">
      <XPTooltipProvider>
        <XPIconDefs />
        <a className="skip-link" href="#main">
          Skip to content
        </a>
        <SiteHeader />
        <main id="main">
          <Hero />
          <DemoSection />
          <HowItWorks />
          <Features />
          <RealCockpit />
          <StatusSection />
          <InstallSection />
        </main>
        <SiteFooter />
      </XPTooltipProvider>
    </MotionConfig>
  );
}
