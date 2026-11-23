export {};

declare global {
  interface Window {
    /** Exposed by electron/preload.cjs with contextIsolation enabled. */
    lairDesktop?: {
      platform: string;
      electronVersion: string;
      /** Checks that the daemon published its connection details. */
      connect: () => Promise<{ url: string } | null>;
      /** Invokes a daemon action in main; the token never reaches the renderer. */
      call?: (action: string, payload: unknown) => Promise<unknown>;
      /** Runs `lair doctor --fix` from the main process after an install. */
      repairShims?: () => Promise<string>;
    };
  }
}
