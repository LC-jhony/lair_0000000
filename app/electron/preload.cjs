const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('lairDesktop', {
  platform: process.platform,
  electronVersion: process.versions.electron,
  connect: () => ipcRenderer.invoke('lair:connect'),
  /** All daemon actions go through main, which owns the bridge token. */
  call: (action, payload) => ipcRenderer.invoke('lair:call', action, payload),
  repairShims: () => ipcRenderer.invoke('lair:repair-shims'),
});
