import { Box, Text } from 'ink';
import React from 'react';
import type { MailResult } from '../../src/api';
import { useT } from '../../src/i18n';
import { Badge } from '../components/ui/badge';
import { Facts, Hint, Section, useAsync, useKeys } from './helpers';
import type { TerminalPaneProps } from './types';

export function MailPane({ state }: TerminalPaneProps) {
  const t = useT();
  const client = state.client;
  const mail = useAsync<MailResult>(() => client ? client.mail({ op: 'status' }) : Promise.resolve({}), [client, state.status]);
  const [selected, setSelected] = React.useState(0);
  const messageId = mail?.messages?.[selected]?.id;

  useKeys({
    s: () => void state.act(t('common.start'), (c) => c.mail({ op: 'start' })),
    x: () => void state.act(t('common.stop'), (c) => c.mail({ op: 'stop' })),
    l: () => void state.act(t('dash.refresh'), (c) => c.mail({ op: 'list' })),
    m: () => { if (messageId) void state.act(t('common.open'), (c) => c.mail({ op: 'message', id: messageId })); },
    c: () => void state.act(t('common.clear'), (c) => c.mail({ op: 'clear' })),
    e: () => void state.act(t('common.copy'), (c) => c.mail({ op: 'status' }).then(() => c.call('mail', { op: 'env' }))),
  });

  return (
    <Box flexDirection="column">
      <Section title={t('pane.mail')} subtitle={t('pane.mail.subtitle')}>
        <Facts items={[
          { key: t('common.running'), value: mail?.running ? t('common.active') : t('common.stopped') },
          { key: t('dash.port'), value: String(mail?.port ?? '—') },
          { key: t('dash.jobs'), value: String(mail?.count ?? 0) },
        ]} keyWidth={14} />
        <Box marginTop={1}>
          {(mail?.messages ?? []).slice(0, 10).map((m, i) => (
            <Box key={m.id ?? i}>
              <Text color={i === selected ? 'cyan' : undefined}>{i === selected ? '› ' : '  '}</Text>
              <Text>{m.subject ?? m.id ?? '—'}</Text>
            </Box>
          ))}
        </Box>
        {mail?.message ? <Text dimColor>{String(mail.message.body ?? '').slice(0, 200)}</Text> : null}
      </Section>
      <Box><Badge variant="secondary">{t('mail.inboxHint')}</Badge></Box>
      <Hint text="s: start · x: stop · l: list · m: read message · c: clear · e: env" />
    </Box>
  );
}
