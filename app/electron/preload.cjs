const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('lairDesktop', {
  platform: process.platform,
  electronVersion: process.versions.electron,
  connect: () => ipcRenderer.invoke('lair:connect'),
  repairShims: () => ipcRenderer.invoke('lair:repair-shims'),
});
