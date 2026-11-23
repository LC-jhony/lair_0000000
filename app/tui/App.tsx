import { Box, Text, useApp, useInput } from 'ink';
import React, { useState } from 'react';
import { useT } from '../src/i18n';
import { useLair } from '../src/useLair';
import type { PaneId } from '../src/panes/types';
import { Badge } from './components/ui/badge';
import { Sidebar } from './components/ui/sidebar';
import { StatusMessage } from './components/ui/status-message';
import { DumpsPane } from './panes/DumpsPane';
import type { TerminalPaneProps } from './panes/types';
import { DatabasesPane } from './panes/DatabasesPane';
import { DebuggerPane } from './panes/DebuggerPane';
import { LogsPane } from './panes/LogsPane';
import { MailPane } from './panes/MailPane';
import { OverviewPane } from './panes/OverviewPane';
import { RuntimesPane } from './panes/RuntimesPane';
import { ServicesPane } from './panes/ServicesPane';
import { SitesPane } from './panes/SitesPane';
import { SystemPane } from './panes/SystemPane';

/** Same ten panes, keys and labels as the Electron app. */
const PANES: { id: PaneId; labelKey: string; subtitleKey: string }[] = [
  { id: 'overview', labelKey: 'pane.overview', subtitleKey: 'pane.overview.subtitle' },
  { id: 'runtimes', labelKey: 'pane.runtimes', subtitleKey: 'pane.runtimes.subtitle' },
  { id: 'databases', labelKey: 'pane.databases', subtitleKey: 'pane.databases.subtitle' },
  { id: 'services', labelKey: 'pane.services', subtitleKey: 'pane.services.subtitle' },
  { id: 'sites', labelKey: 'pane.sites', subtitleKey: 'pane.sites.subtitle' },
  { id: 'mail', labelKey: 'pane.mail', subtitleKey: 'pane.mail.subtitle' },
  { id: 'dumps', labelKey: 'pane.dumps', subtitleKey: 'pane.dumps.subtitle' },
  { id: 'debugger', labelKey: 'pane.debugger', subtitleKey: 'pane.debugger.subtitle' },
  { id: 'logs', labelKey: 'pane.logs', subtitleKey: 'pane.logs.subtitle' },
  { id: 'system', labelKey: 'pane.system', subtitleKey: 'pane.system.subtitle' },
];

export interface AppProps {
  clientFactory: () => Promise<import('../src/api').DaemonClient>;
}

export function App({ clientFactory }: AppProps) {
  const t = useT();
  const { exit } = useApp();
  const state = useLair(clientFactory);
  const [pane, setPane] = useState<PaneId>('overview');

  useInput((input, key) => {
    if (key.ctrl && input === 'c') {
      exit();
      return;
    }
    if (key.tab || key.rightArrow) {
      const index = PANES.findIndex((p) => p.id === pane);
      setPane(PANES[(index + 1) % PANES.length]!.id);
      return;
    }
    if (key.leftArrow) {
      const index = PANES.findIndex((p) => p.id === pane);
      setPane(PANES[(index - 1 + PANES.length) % PANES.length]!.id);
      return;
    }
    if (input === 'r') void state.refresh();
  });

  const current = PANES.find((p) => p.id === pane)!;
  const props: TerminalPaneProps = { state, onNavigate: setPane };
  const content = (() => {
    switch (pane) {
      case 'runtimes': return <RuntimesPane {...props} />;
      case 'databases': return <DatabasesPane {...props} />;
      case 'services': return <ServicesPane {...props} />;
      case 'sites': return <SitesPane {...props} />;
      case 'mail': return <MailPane {...props} />;
      case 'dumps': return <DumpsPane {...props} />;
      case 'debugger': return <DebuggerPane {...props} />;
      case 'logs': return <LogsPane {...props} />;
      case 'system': return <SystemPane {...props} />;
      default: return <OverviewPane {...props} />;
    }
  })();

  return (
    <Box flexDirection="column" height="100%">
      <Box borderStyle="round" paddingX={1}>
        <Text bold color="cyan">Lair</Text>
        <Text> · </Text>
        <Text>{t(current.labelKey)}</Text>
        <Text> </Text>
        {state.status?.home ? <Text dimColor>{state.status.home}</Text> : null}
      </Box>
      <Box flexDirection="row" flexGrow={1} minHeight={10}>
        <Sidebar
          width={22}
          activeKey={pane}
          onSelect={(key) => setPane(key as PaneId)}
          title="Lair"
          items={PANES.map((p) => ({ key: p.id, label: t(p.labelKey) }))}
        />
        <Box flexDirection="column" flexGrow={1} paddingX={1} marginLeft={1}>
          <Text bold>{t(current.labelKey)}</Text>
          <Text dimColor>{t(current.subtitleKey)}</Text>
          <Box marginY={1}>
            {state.error ? (
              <StatusMessage variant="error">{state.error}</StatusMessage>
            ) : state.busy ? (
              <StatusMessage variant="info">{state.busy}</StatusMessage>
            ) : (
              <Badge variant={state.status ? 'success' : 'warning'}>
                {state.status ? t('common.running') : t('dash.waiting')}
              </Badge>
            )}
          </Box>
          <Box flexGrow={1}>{content}</Box>
          <Box marginTop={1}>
            <Text dimColor>Tab/←→: {t('common.switch')} · r: {t('dash.refresh')} · Ctrl+C: {t('common.close')}</Text>
          </Box>
        </Box>
      </Box>
    </Box>
  );
}
