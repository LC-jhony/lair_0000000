import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { HttpDaemonClient, type DaemonClient } from '../src/api';

/** Same resolution rules as the Electron main process and the Go daemon. */
export function lairHome(): string {
  if (process.env.LAIR_HOME) return path.resolve(process.env.LAIR_HOME);
  if (process.env.XDG_DATA_HOME) return path.join(process.env.XDG_DATA_HOME, 'lair');
  return path.join(os.homedir(), '.local', 'share', 'lair');
}

function readTrimmed(file: string): string {
  try {
    return fs.readFileSync(file, 'utf8').trim();
  } catch {
    return '';
  }
}

/** Connects to the running daemon using the same bridge as the desktop app. */
export async function connect(): Promise<DaemonClient> {
  const run = path.join(lairHome(), 'run');
  const url = readTrimmed(path.join(run, 'desktop.url')) || 'http://127.0.0.1:9797';
  const token = readTrimmed(path.join(run, 'desktop.token'));
  if (!token) {
    throw new Error('Daemon no disponible; inicia Lair con `lair daemon --foreground`.');
  }
  return new HttpDaemonClient(url, token);
}
