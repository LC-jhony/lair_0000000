import { Box, Text, useInput } from 'ink';
import React, { useState } from 'react';
import { activeVersion, installedVersions } from '../../src/api';
import { useT } from '../../src/i18n';
import { useVersions } from '../../src/panes/useVersions';
import { Badge } from '../components/ui/badge';
import { Toggle } from '../components/ui/toggle';
import { Hint, PromptLine, Section, useKeys, usePrompt } from './helpers';
import type { TerminalPaneProps } from './types';

const CATEGORY: Record<string, string> = {
  mariadb: 'services.category.database', mysql: 'services.category.database',
  postgres: 'services.category.database', mongodb: 'services.category.database',
  sqlite3: 'services.category.database', redis: 'services.category.cacheQueue',
  valkey: 'services.category.cacheQueue',
};

export function ServicesPane({ state }: TerminalPaneProps) {
  const t = useT();
  const services = state.status?.services ?? {};
  const ids = Object.keys(services).sort();
  const catalog = useVersions(state.client, ids);
  const [selected, setSelected] = useState(0);
  const id = ids[selected] ?? '';
  const svc = services[id];
  const installed = svc ? installedVersions(svc) : [];
  const active = activeVersion(svc);

  useInput((_input, key) => {
    if (key.upArrow) setSelected((i) => Math.max(0, i - 1));
    if (key.downArrow) setSelected((i) => Math.min(ids.length - 1, i + 1));
  });

  const create = (name: string) => {
    if (name.trim() && id) void state.act(t('dash.creatingDB', { name }), (c) => c.createDB(id, name.trim()));
  };
  const prompt = usePrompt(create);

  const handlers: Record<string, () => void> = {
    i: () => {
      const version = catalog[id]?.[0];
      if (version && id) void state.act(t('dash.installing', { name: id }), (c) => c.installService(id, version));
    },
    s: () => {
      if (!id) return;
      const action = svc?.running ? (c: import('../../src/api').DaemonClient) => c.stopService(id) : (c: import('../../src/api').DaemonClient) => c.restartService(id);
      void state.act(t(svc?.running ? 'dash.stopping' : 'dash.starting', { name: id }), action);
    },
    u: () => {
      const next = installed.find((v) => v !== active);
      if (next && id) void state.act(t('dash.switching', { name: next }), (c) => c.useService(id, next));
    },
    x: () => {
      const version = active;
      if (version && id) void state.act(t('dash.uninstalling', { name: version }), (c) => c.uninstallService(id, version));
    },
    c: prompt.start,
  };
  useKeys(handlers, !prompt.active);

  return (
    <Box flexDirection="column">
      <Section title={t('pane.services')} subtitle={t('pane.services.subtitle')}>
        {ids.map((candidate, index) => {
          const item = services[candidate];
          const itemActive = activeVersion(item);
          const unavailable = item?.available === false && !itemActive;
          return (
            <Box key={candidate}>
              <Text color={index === selected ? 'cyan' : undefined}>{index === selected ? '› ' : '  '}</Text>
              <Text bold={index === selected}>{candidate}</Text>
              <Text> </Text>
              <Badge variant={unavailable ? 'warning' : itemActive ? 'success' : 'secondary'}>
                {unavailable ? t('services.notAvailableOn', { platform: item?.platform ?? '—' }) : itemActive || t('services.card.notInstalled')}
              </Badge>
              {item?.running ? <Text color="green"> ●</Text> : null}
            </Box>
          );
        })}
      </Section>
      {svc ? (
        <Box flexDirection="column">
          <Text dimColor>{t(CATEGORY[id] ?? 'services.category.core')} · {t('dash.port')}: {svc.port ?? '—'}</Text>
          {id === 'sqlite3' ? (
            <Text dimColor>{t('services.sqlite.hint')}</Text>
          ) : (
            <Toggle checked={!!svc.running} label={active || t('dash.none')} onChange={() => handlers.s()} />
          )}
        </Box>
      ) : null}
      {prompt.active ? <PromptLine label={t('dash.dbName')} value={prompt.value} /> : null}
      <Hint text="↑/↓: select · i: install · s: start/stop · u: use version · c: create DB · x: uninstall" />
    </Box>
  );
}
