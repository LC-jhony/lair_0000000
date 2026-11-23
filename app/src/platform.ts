/** The OS the desktop shell runs on. Falls back to the user agent in tests and
 *  when the app is opened outside Electron, so platform rules still apply. */
function detect(): string {
  const desktop = window.lairDesktop?.platform;
  if (desktop) return desktop;
  const ua = navigator.userAgent;
  if (ua.includes('Mac')) return 'darwin';
  if (ua.includes('Win')) return 'win32';
  return 'linux';
}

export const platform = detect();
export const isMac = platform === 'darwin';
export const isWindows = platform === 'win32';
