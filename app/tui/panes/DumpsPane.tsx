import { Box, Text } from 'ink';
import React from 'react';
import type { DumpsResult } from '../../src/api';
import { useT } from '../../src/i18n';
import { Facts, Hint, Section, useAsync, useKeys } from './helpers';
import type { TerminalPaneProps } from './types';

export function DumpsPane({ state }: TerminalPaneProps) {
  const t = useT();
  const client = state.client;
  const dumps = useAsync<DumpsResult>(() => client ? client.dumps({ op: 'status' }) : Promise.resolve({}), [client, state.status]);

  useKeys({
    s: () => void state.act(t('common.start'), (c) => c.dumps({ op: 'capture-on' })),
    x: () => void state.act(t('common.stop'), (c) => c.dumps({ op: 'capture-off' })),
    o: () => void state.act(t('dash.refresh'), (c) => c.dumps({ op: 'options' })),
    l: () => void state.act(t('dash.refresh'), (c) => c.dumps({ op: 'list' })),
    c: () => void state.act(t('common.clear'), (c) => c.dumps({ op: 'clear' })),
  });

  return (
    <Box flexDirection="column">
      <Section title={t('pane.dumps')} subtitle={t('pane.dumps.subtitle')}>
        <Facts items={[
          { key: t('common.running'), value: dumps?.capturing ? t('common.active') : t('common.stopped') },
          { key: t('dash.jobs'), value: String(dumps?.count ?? 0) },
          { key: t('dash.logs'), value: JSON.stringify(dumps?.options ?? {}) },
        ]} keyWidth={14} />
        <Box marginTop={1}>
          {(dumps?.events ?? []).slice(0, 10).map((e, i) => (
            <Text key={e.id ?? i} dimColor>{`${e.type ?? ''} ${e.site ?? ''}`.trim()}</Text>
          ))}
        </Box>
      </Section>
      <Hint text="s: capture on · x: capture off · o: options · l: list · c: clear" />
    </Box>
  );
}
