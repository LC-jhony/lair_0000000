export type Tone =
  | 'neutral'
  | 'gray'
  | 'blue'
  | 'indigo'
  | 'green'
  | 'orange'
  | 'brown'
  | 'cyan'
  | 'pink'
  | 'red'
  | 'purple'
  | 'teal';

export type IconName =
  | 'gear'
  | 'globe'
  | 'code'
  | 'hexagon'
  | 'server'
  | 'mail'
  | 'dumps'
  | 'bug'
  | 'sparkles'
  | 'info'
  | 'database'
  | 'mysql'
  | 'mariadb'
  | 'postgres'
  | 'mongodb'
  | 'redis'
  | 'search'
  | 'drive'
  | 'check'
  | 'checkOutline'
  | 'circle'
  | 'plus'
  | 'more'
  | 'download'
  | 'refresh'
  | 'log';

/** Mirrors ComponentJob from the daemon contract. */
export interface ComponentJob {
  action: 'install' | 'update';
  state: 'running' | 'failed';
  phase: 'downloading' | 'installing';
  /** The daemon sends -1 when the size is unknown; the UI treats it as null. */
  progress: number | null;
  error?: string;
  startedAt?: string;
}

export interface CatalogLineOption {
  id: string;
  label: string;
  latest?: string;
  installed?: boolean;
  active?: boolean;
}

/** -1, undefined and out-of-range values mean "unknown progress". */
export function normalizedProgress(job: Pick<ComponentJob, 'progress'>): number | null {
  const value = job.progress;
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0 || value > 1) return null;
  return value;
}
