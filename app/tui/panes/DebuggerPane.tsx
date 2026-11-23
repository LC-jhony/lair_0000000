import { Box, Text } from 'ink';
import React from 'react';
import type { DebuggerResult } from '../../src/api';
import { useT } from '../../src/i18n';
import { Facts, Hint, Section, useAsync, useKeys } from './helpers';
import type { TerminalPaneProps } from './types';

export function DebuggerPane({ state }: TerminalPaneProps) {
  const t = useT();
  const client = state.client;
  const debug = useAsync<DebuggerResult>(() => client ? client.debugger({ op: 'status' }) : Promise.resolve({}), [client, state.status]);

  useKeys({
    d: () => void state.act(t('common.switch'), (c) => c.debugger({ op: debug?.detect ? 'detect-off' : 'detect-on' })),
    b: () => void state.act(t('common.switch'), (c) => c.debugger({ op: debug?.breakpoints ? 'breakpoints-off' : 'breakpoints-on' })),
  });

  return (
    <Box flexDirection="column">
      <Section title={t('pane.debugger')} subtitle={t('pane.debugger.subtitle')}>
        <Facts items={[
          { key: 'Xdebug', value: debug?.detect ? t('common.active') : t('common.stopped') },
          { key: t('dash.version'), value: debug?.versions?.map((v) => v.version).filter(Boolean).join(', ') || t('dash.none') },
          { key: t('dash.port'), value: String(debug?.port ?? '—') },
          { key: t('common.details'), value: debug?.mode ?? t('dash.none') },
          { key: t('dash.sites'), value: (debug?.domains ?? []).join(', ') || t('dash.none') },
        ]} keyWidth={14} />
      </Section>
      <Hint text="d: toggle detect · b: toggle breakpoints" />
    </Box>
  );
}
