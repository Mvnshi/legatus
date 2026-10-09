import type { Cue } from "../demo/engine";

/**
 * Small synthesized sound effects. They are original (plain sine and triangle tones), start muted, and nothing is
 * created until a person turns sound on, because browsers only allow audio after a click.
 */

export type SoundCue = Cue | "on";

let context: AudioContext | undefined;

function audio(): AudioContext | null {
  try {
    if (!context) {
      const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
      if (!Ctor) return null;
      context = new Ctor();
    }
    if (context.state === "suspended") void context.resume();
    return context;
  } catch {
    return null;
  }
}

function tone(ctx: AudioContext, freq: number, at: number, length: number, type: OscillatorType, peak: number) {
  const osc = ctx.createOscillator();
  const gain = ctx.createGain();
  osc.type = type;
  osc.frequency.setValueAtTime(freq, at);
  gain.gain.setValueAtTime(0.0001, at);
  gain.gain.exponentialRampToValueAtTime(peak, at + 0.015);
  gain.gain.exponentialRampToValueAtTime(0.0001, at + length);
  osc.connect(gain).connect(ctx.destination);
  osc.start(at);
  osc.stop(at + length + 0.02);
}

const notes: Record<SoundCue, [freq: number, offset: number, length: number][]> = {
  on: [
    [660, 0, 0.12],
    [880, 0.1, 0.16],
  ],
  start: [[520, 0, 0.09]],
  limit: [
    [440, 0, 0.16],
    [330, 0.15, 0.26],
  ],
  checkpoint: [[740, 0, 0.1]],
  check: [[620, 0, 0.08]],
  done: [
    [523.25, 0, 0.16],
    [659.25, 0.13, 0.16],
    [783.99, 0.26, 0.16],
    [1046.5, 0.39, 0.38],
  ],
};

export function playCue(cue: SoundCue): void {
  const ctx = audio();
  if (!ctx) return;
  const now = ctx.currentTime + 0.02;
  const type: OscillatorType = cue === "limit" ? "triangle" : "sine";
  for (const [freq, offset, length] of notes[cue]) tone(ctx, freq, now + offset, length, type, 0.05);
}
