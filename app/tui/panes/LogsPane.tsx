import { Box, Text, useInput } from 'ink';
import React, { useEffect, useState } from 'react';
import { useT } from '../../src/i18n';
import { Hint, Section, useAsync, useKeys } from './helpers';
import type { TerminalPaneProps } from './types';

export function LogsPane({ state }: TerminalPaneProps) {
  const t = useT();
  const client = state.client;
  const names = useAsync<string[]>(() => client ? client.logsList() : Promise.resolve([]), [client, state.status]);
  const [selected, setSelected] = useState(0);
  const [text, setText] = useState('');
  const name = names?.[selected];

  useEffect(() => {
    let alive = true;
    if (name && client) {
      void client.log(name).then((value) => { if (alive) setText(value); }).catch(() => { if (alive) setText(''); });
    } else {
      setText('');
    }
    return () => { alive = false; };
  }, [client, name]);

  useInput((_input, key) => {
    if (key.upArrow) setSelected((i) => Math.max(0, i - 1));
    if (key.downArrow) setSelected((i) => Math.min((names?.length ?? 1) - 1, i + 1));
  });
  useKeys({ r: () => { if (name) void state.act(t('dash.readingLog', { name }), (c) => c.log(name)); } });

  return (
    <Box flexDirection="column">
      <Section title={t('pane.logs')} subtitle={t('pane.logs.subtitle')}>
        <Box>
          {(names ?? []).map((item, i) => (
            <Text key={item} color={i === selected ? 'cyan' : undefined}>
              {i === selected ? '› ' : '  '}{item}
            </Text>
          ))}
          {(names ?? []).length === 0 ? <Text dimColor>{t('dash.noLogs')}</Text> : null}
        </Box>
        <Box marginTop={1}>
          <Text dimColor>{text ? text.slice(-2000) : t('dash.noLog')}</Text>
        </Box>
      </Section>
      <Hint text="↑/↓: select log · r: refresh" />
    </Box>
  );
}
