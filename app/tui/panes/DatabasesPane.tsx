import { Box, Text } from 'ink';
import React from 'react';
import { installedVersions } from '../../src/api';
import { useT } from '../../src/i18n';
import { useVersions } from '../../src/panes/useVersions';
import { Badge } from '../components/ui/badge';
import { Hint, PromptLine, Section, useKeys, usePrompt } from './helpers';
import type { TerminalPaneProps } from './types';

const DB_IDS = ['mariadb', 'postgres', 'mysql'];

export function DatabasesPane({ state }: TerminalPaneProps) {
  const t = useT();
  const services = state.status?.services ?? {};
  const catalog = useVersions(state.client, DB_IDS);
  const defaultDB = state.status?.defaults?.database ?? 'mariadb';

  const install = (id: string) => {
    const version = catalog[id]?.[0];
    if (version) void state.act(t('dash.installing', { name: id }), (c) => c.installService(id, version));
  };
  const create = (name: string) => {
    if (name.trim()) void state.act(t('dash.creatingDB', { name }), (c) => c.createDB(defaultDB, name.trim()));
  };
  const prompt = usePrompt(create);

  const handlers: Record<string, () => void> = {
    i: () => install(defaultDB),
    c: prompt.start,
    d: () => void state.act(t('common.installing', { name: defaultDB }), (c) => c.ensureDatabase()),
  };
  DB_IDS.forEach((id, index) => {
    handlers[String(index + 1)] = () => void state.act(t('common.makeDefault'), (c) => c.setDefaultDB(id));
  });
  useKeys(handlers, !prompt.active);

  return (
    <Box flexDirection="column">
      <Section title={t('pane.databases')} subtitle={t('pane.databases.subtitle')}>
        {DB_IDS.map((id) => {
          const svc = services[id];
          const active = svc?.active ?? '';
          const versions = svc ? installedVersions(svc) : [];
          return (
            <Box key={id} flexWrap="wrap">
              <Badge variant={id === defaultDB ? 'success' : 'secondary'}>{id}</Badge>
              <Text> </Text>
              <Text dimColor>
                {active || t('dash.none')} {versions.length > 1 ? `(${versions.join(', ')})` : ''}
              </Text>
            </Box>
          );
        })}
        <Box marginTop={1}>
          <Text dimColor>{t('dash.database')}: {defaultDB}</Text>
        </Box>
      </Section>
      {prompt.active ? <PromptLine label={t('dash.dbName')} value={prompt.value} /> : null}
      <Hint text="i: install · c: create database · d: ensure database · 1-3: set default" />
    </Box>
  );
}
