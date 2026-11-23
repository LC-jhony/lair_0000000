const { app, BrowserWindow, ipcMain, Menu, nativeTheme, shell } = require('electron');
const { execFile } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');

function lairHome() {
  if (process.env.LAIR_HOME) return path.resolve(process.env.LAIR_HOME);
  if (process.env.XDG_DATA_HOME) return path.join(process.env.XDG_DATA_HOME, 'lair');
  return path.join(os.homedir(), '.local', 'share', 'lair');
}

function readTrimmed(file) {
  try {
    return fs.readFileSync(file, 'utf8').trim();
  } catch {
    return '';
  }
}

// The main process owns the filesystem access. The renderer only receives the
// connection details through a narrow IPC call, never a raw fs API.
function connection() {
  const runDir = path.join(lairHome(), 'run');
  const url = readTrimmed(path.join(runDir, 'desktop.url'));
  const token = readTrimmed(path.join(runDir, 'desktop.token'));
  if (!url || !token) return null;
  return { url, token };
}

ipcMain.handle('lair:connect', () => connection());

const isMac = process.platform === 'darwin';
const isWindows = process.platform === 'win32';
let mainWindow = null;

// Window chrome is delegated to each OS so the app behaves like a native
// window everywhere: the Linux WM draws the title bar, Windows draws its
// caption buttons, and macOS keeps its traffic lights.
// Colors match --background in src/styles/globals.css.
function windowColors() {
  if (nativeTheme.shouldUseDarkColors) return { background: '#0a0a0a', foreground: '#fafafa' };
  return { background: '#ffffff', foreground: '#0a0a0a' };
}

function titleBarOptions() {
  const colors = windowColors();
  if (isMac) {
    // Native traffic lights, inset over the sidebar. The app reserves room
    // for them in styles/app.css.
    return {
      titleBarStyle: 'hiddenInset',
      trafficLightPosition: { x: 18, y: 18 },
    };
  }
  if (isWindows) {
    // Native minimize/maximize/close drawn by Windows over the app header.
    return {
      titleBarStyle: 'hidden',
      titleBarOverlay: { color: colors.background, symbolColor: colors.foreground, height: 64 },
    };
  }
  // Linux: standard server-side decorations from the window manager.
  return {};
}

function applyTitleBarOverlay() {
  if (!isWindows || !mainWindow || mainWindow.isDestroyed()) return;
  const colors = windowColors();
  mainWindow.setTitleBarOverlay({
    color: colors.background,
    symbolColor: colors.foreground,
    height: 64,
  });
}

nativeTheme.on('updated', applyTitleBarOverlay);

/**
 * Only well-formed http(s) links may be handed to the OS. LAIR_EXTERNAL_ALLOWLIST
 * is an optional comma-separated list of hostnames to further restrict them.
 */
function allowedExternalUrl(rawUrl) {
  let url;
  try {
    url = new URL(rawUrl);
  } catch {
    return false;
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return false;
  const allowlist = process.env.LAIR_EXTERNAL_ALLOWLIST;
  if (!allowlist) return true;
  const hosts = allowlist.split(',').map((host) => host.trim().toLowerCase()).filter(Boolean);
  if (hosts.length === 0) return true;
  const hostname = url.hostname.toLowerCase();
  return hosts.some((host) => hostname === host || hostname.endsWith(`.${host}`));
}

/** Resolves and validates the binary used by the shim repair path. */
function lairBinary() {
  const configured = process.env.LAIR_BIN;
  if (configured) {
    if (!path.isAbsolute(configured)) throw new Error('LAIR_BIN must be an absolute path');
    let stat;
    try {
      stat = fs.statSync(configured);
    } catch {
      throw new Error(`LAIR_BIN does not exist: ${configured}`);
    }
    if (!stat.isFile()) throw new Error(`LAIR_BIN is not a file: ${configured}`);
    try {
      fs.accessSync(configured, fs.constants.X_OK);
    } catch {
      throw new Error(`LAIR_BIN is not executable: ${configured}`);
    }
    return configured;
  }
  const local = path.join(os.homedir(), '.local', 'bin', 'lair');
  return fs.existsSync(local) ? local : 'lair';
}

// Runs the same repair path as `lair doctor --fix`. The desktop uses it after
// an install so node/npm shims exist even when the running daemon predates the
// ensure-shims action.
ipcMain.handle('lair:repair-shims', () => {
  // Throws (rejecting the renderer's invoke) when LAIR_BIN is misconfigured,
  // so no arbitrary path is ever passed to execFile.
  const binary = lairBinary();
  return new Promise((resolve, reject) => {
    execFile(binary, ['doctor', '--fix'], { timeout: 30000 }, (error, stdout) => {
      if (error) reject(new Error(stdout || error.message));
      else resolve(stdout);
    });
  });
});

function createWindow() {
  const devURL = process.env.VITE_DEV_SERVER_URL;
  const localFile = path.join(__dirname, '..', 'dist-app', 'index.html');

  const window = new BrowserWindow({
    width: 1120,
    height: 740,
    minWidth: 830,
    minHeight: 480,
    title: 'Lair',
    icon: path.join(__dirname, 'icon.png'),
    backgroundColor: windowColors().background,
    ...titleBarOptions(),
    // No native File/Edit/View/Window menu bar: the app has its own UI.
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webSecurity: true,
    },
  });
  mainWindow = window;
  window.on('closed', () => {
    if (mainWindow === window) mainWindow = null;
  });

  // In-page navigation is limited to the app itself; everything else is
  // handed to the OS after scheme/host validation.
  const expectedNavigation = (url) => {
    if (devURL) {
      try {
        return new URL(url).origin === new URL(devURL).origin;
      } catch {
        return false;
      }
    }
    return url === pathToFileURL(localFile).href;
  };

  window.webContents.on('will-navigate', (event, url) => {
    if (expectedNavigation(url)) return;
    event.preventDefault();
    if (allowedExternalUrl(url)) void shell.openExternal(url);
  });

  window.webContents.setWindowOpenHandler(({ url }) => {
    if (allowedExternalUrl(url)) void shell.openExternal(url);
    return { action: 'deny' };
  });

  if (devURL) {
    void window.loadURL(devURL);
  } else {
    void window.loadFile(localFile);
  }
}

// macOS needs an application menu for standard shortcuts (Cmd+Q, Cmd+C, …).
// Every other platform hides the menu entirely: the app draws its own UI.
function installMenu() {
  if (!isMac) {
    Menu.setApplicationMenu(null);
    return;
  }
  Menu.setApplicationMenu(
    Menu.buildFromTemplate([
      { role: 'appMenu' },
      { role: 'editMenu' },
      { role: 'viewMenu' },
      { role: 'windowMenu' },
    ]),
  );
}

void app.whenReady().then(() => {
  // Groups the window under the Lair icon in the Windows taskbar.
  if (isWindows) app.setAppUserModelId('sh.lair.desktop');
  installMenu();
  createWindow();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});
