import type { LairController } from '../useLair';

export type PaneId =
  | 'overview'
  | 'runtimes'
  | 'databases'
  | 'services'
  | 'sites'
  | 'mail'
  | 'dumps'
  | 'debugger'
  | 'logs'
  | 'jobs'
  | 'activity'
  | 'system';

export interface PaneProps {
  state: LairController;
  /** Lets panes link to each other without a native menu or window navigation. */
  onNavigate?: (id: PaneId) => void;
}
