/** The hero's "Try the demo" button asks the desktop to start the sample run. */
export const TRY_DEMO_EVENT = "legatus:try-demo";

export function requestTryDemo(): void {
  window.dispatchEvent(new CustomEvent(TRY_DEMO_EVENT));
}
