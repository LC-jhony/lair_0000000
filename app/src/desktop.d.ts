export {};

declare global {
  interface Window {
    /** Exposed by electron/preload.cjs with contextIsolation enabled. */
    lairDesktop?: {
      platform: string;
      electronVersion: string;
      /** Reads run/desktop.url and run/desktop.token in the main process. */
      connect: () => Promise<{ url: string; token: string } | null>;
      /** Runs `lair doctor --fix` from the main process after an install. */
      repairShims?: () => Promise<string>;
    };
  }
}
