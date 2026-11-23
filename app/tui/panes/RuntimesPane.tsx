import { Box, Text } from 'ink';
import React from 'react';
import { useT } from '../../src/i18n';
import { useVersions } from '../../src/panes/useVersions';
import { Badge } from '../components/ui/badge';
import { Facts, Hint, Section, useKeys } from './helpers';
import type { TerminalPaneProps } from './types';

export function RuntimesPane({ state }: TerminalPaneProps) {
  const t = useT();
  const catalog = useVersions(state.client, ['php', 'node', 'composer']);
  const status = state.status;
  const php = status?.php ?? [];
  const node = status?.node ?? [];
  const phpVersions = catalog.php ?? [];
  const nodeVersions = catalog.node ?? [];

  const handlers: Record<string, () => void> = {
    i: () => {
      const version = phpVersions[0];
      if (version) void state.act(t('dash.installing', { name: 'PHP' }), (c) => c.installPHP(version));
    },
    p: () => {
      const version = nodeVersions[0];
      if (version) void state.act(t('dash.installing', { name: 'Node' }), (c) => c.installNode(version));
    },
    m: () => void state.act(t('common.installing', { name: 'Composer' }), (c) => c.installComposer()),
    a: () => {
      const minor = php.find((p) => p.default)?.minor ?? php[0]?.minor;
      if (minor) void state.act(t('dash.pin'), (c) => c.pin('php', minor!, process.cwd()));
    },
    o: () => {
      const version = node[0];
      if (version) void state.act(t('dash.isolate'), (c) => c.isolate('node', version, process.cwd()));
    },
  };
  php.forEach((info, index) => {
    if (index < 9 && info.minor) {
      handlers[String(index + 1)] = () => void state.act(t('dash.switching', { name: info.minor! }), (c) => c.usePHP(info.minor!));
    }
  });
  node.forEach((version, index) => {
    if (index < 9 && version) {
      handlers['qwertyuiop'[index]!] = () => void state.act(t('dash.switching', { name: version }), (c) => c.useNode(version));
    }
  });
  useKeys(handlers);

  return (
    <Box flexDirection="column">
      <Section title={t('pane.runtimes')} subtitle={t('pane.runtimes.subtitle')}>
        <Facts
          items={[
            { key: 'PHP', value: php.length ? php.map((p) => `${p.minor}${p.default ? '*' : ''}`).join('  ') : t('dash.noneInstalled') },
            { key: 'Node', value: node.length ? node.join('  ') : t('dash.noneInstalled') },
            { key: 'Composer', value: status?.composer ?? t('dash.none') },
          ]}
          keyWidth={12}
        />
        <Box marginTop={1} flexWrap="wrap">
          {phpVersions.slice(0, 4).map((v) => <Badge key={v} variant="secondary">{`PHP ${v}`}</Badge>)}
          {nodeVersions.slice(0, 4).map((v) => <Badge key={v} variant="secondary">{`Node ${v}`}</Badge>)}
        </Box>
      </Section>
      <Hint text="i: install PHP · p: install Node · m: composer · 1-9: use PHP · q-w-e…: use Node · a: pin · o: isolate" />
      <Text dimColor>{t('dash.sameFunctions')}</Text>
    </Box>
  );
}
