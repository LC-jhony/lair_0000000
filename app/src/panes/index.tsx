import type { IconName, Tone } from '../components';
import type { LairController } from '../useLair';
import { DatabasesPane } from './DatabasesPane';
import { DebuggerPane } from './DebuggerPane';
import { DumpsPane } from './DumpsPane';
import { LogsPane } from './LogsPane';
import { MailPane } from './MailPane';
import { OverviewPane } from './OverviewPane';
import { RuntimesPane } from './RuntimesPane';
import { ServicesPane } from './ServicesPane';
import { SitesPane } from './SitesPane';
import { SystemPane } from './SystemPane';
import type { PaneId, PaneProps } from './types';

export type { PaneId, PaneProps } from './types';

export interface PaneDefinition {
  id: PaneId;
  icon: IconName;
  color: Tone;
  titleKey: string;
  subtitleKey: string;
}

/** Mirrors the TUI panes so both interfaces expose the same functionality. */
export const PANES: PaneDefinition[] = [
  { id: 'overview', icon: 'gear', color: 'gray', titleKey: 'pane.overview', subtitleKey: 'pane.overview.subtitle' },
  { id: 'runtimes', icon: 'code', color: 'indigo', titleKey: 'pane.runtimes', subtitleKey: 'pane.runtimes.subtitle' },
  { id: 'databases', icon: 'database', color: 'orange', titleKey: 'pane.databases', subtitleKey: 'pane.databases.subtitle' },
  { id: 'services', icon: 'server', color: 'green', titleKey: 'pane.services', subtitleKey: 'pane.services.subtitle' },
  { id: 'sites', icon: 'globe', color: 'blue', titleKey: 'pane.sites', subtitleKey: 'pane.sites.subtitle' },
  { id: 'mail', icon: 'mail', color: 'brown', titleKey: 'pane.mail', subtitleKey: 'pane.mail.subtitle' },
  { id: 'dumps', icon: 'dumps', color: 'teal', titleKey: 'pane.dumps', subtitleKey: 'pane.dumps.subtitle' },
  { id: 'debugger', icon: 'bug', color: 'purple', titleKey: 'pane.debugger', subtitleKey: 'pane.debugger.subtitle' },
  { id: 'logs', icon: 'log', color: 'cyan', titleKey: 'pane.logs', subtitleKey: 'pane.logs.subtitle' },
  { id: 'system', icon: 'info', color: 'neutral', titleKey: 'pane.system', subtitleKey: 'pane.system.subtitle' },
];

export function PaneContent({ id, state, onNavigate }: { id: PaneId; state: LairController; onNavigate?: (id: PaneId) => void }) {
  const props: PaneProps = { state, onNavigate };
  switch (id) {
    case 'runtimes':
      return <RuntimesPane {...props} />;
    case 'databases':
      return <DatabasesPane {...props} />;
    case 'services':
      return <ServicesPane {...props} />;
    case 'sites':
      return <SitesPane {...props} />;
    case 'mail':
      return <MailPane {...props} />;
    case 'dumps':
      return <DumpsPane {...props} />;
    case 'debugger':
      return <DebuggerPane {...props} />;
    case 'logs':
      return <LogsPane {...props} />;
    case 'system':
      return <SystemPane {...props} />;
    default:
      return <OverviewPane {...props} />;
  }
}
