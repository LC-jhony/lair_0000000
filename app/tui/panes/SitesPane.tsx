import { Box, Text, useInput } from 'ink';
import os from 'node:os';
import path from 'node:path';
import React, { useState } from 'react';
import { useT } from '../../src/i18n';
import { Badge } from '../components/ui/badge';
import { Hint, PromptLine, Section, useKeys, usePrompt } from './helpers';
import type { TerminalPaneProps } from './types';

export function SitesPane({ state }: TerminalPaneProps) {
  const t = useT();
  const sites = state.status?.sites ?? [];
  const [selected, setSelected] = useState(0);
  const site = sites[selected];

  useInput((_input, key) => {
    if (key.upArrow) setSelected((i) => Math.max(0, i - 1));
    if (key.downArrow) setSelected((i) => Math.min(sites.length - 1, i + 1));
  });

  const run = (value: string, mode: string) => {
    if (!value.trim()) return;
    if (mode === 'link') {
      void state.act(t('dash.linking', { name: value }), (c) => c.link(value.trim(), process.cwd()));
    } else if (mode === 'new') {
      void state.act(t('dash.installing', { name: value }), (c) => c.projectCreate({
        name: value.trim(), parent: path.join(os.homedir(), 'Lair'), secure: false,
        stack: 'blade', auth: false, testing: 'pest', boost: false,
        node_manager: 'npm', git: true, database: 'sqlite', create_db: false,
        start_service: false,
      }));
    }
  };
  const [mode, setMode] = useState('link');
  const prompt = usePrompt((value) => run(value, mode));

  useKeys({
    p: () => void state.act(t('dash.parking'), (c) => c.park(process.cwd())),
    l: () => { setMode('link'); prompt.start(); },
    u: () => { if (site) void state.act(t('dash.unlinking', { name: site.name ?? '' }), (c) => c.unlink(site.name ?? '')); },
    s: () => { if (site) void state.act(t('dash.securing', { name: site.name ?? '' }), (c) => c.secure(site.name ?? '')); },
    n: () => { setMode('new'); prompt.start(); },
    o: () => { if (site) void state.act(t('common.open'), (c) => c.siteOpen(site.name ?? '')); },
    e: () => { if (site) void state.act(t('common.switch'), (c) => c.siteSetEnabled(site.name ?? '', site.enabled === false)); },
    v: () => { if (site) void state.act(t('common.details'), (c) => c.site({ op: 'detail', name: site.name })); },
    f: () => { if (site) void state.act(t('common.details'), (c) => c.site({ op: 'config', name: site.name })); },
  }, !prompt.active);

  return (
    <Box flexDirection="column">
      <Section title={t('pane.sites')} subtitle={t('pane.sites.subtitle')}>
        {sites.length === 0 ? <Text dimColor>{t('dash.noSites')}</Text> : sites.map((item, index) => (
          <Box key={item.name}>
            <Text color={index === selected ? 'cyan' : undefined}>{index === selected ? '› ' : '  '}</Text>
            <Text bold={index === selected}>{item.name}</Text>
            <Text> </Text>
            <Text dimColor>{item.path ?? ''}</Text>
            {item.secure ? <Badge variant="success">HTTPS</Badge> : null}
          </Box>
        ))}
      </Section>
      {prompt.active ? <PromptLine label={mode === 'new' ? t('project.name') : t('dash.siteName')} value={prompt.value} /> : null}
      <Hint text="↑/↓: select · p: park · l: link · n: new project · u: unlink · s: secure · o: open · e: enable/disable · v: detail · f: vhost" />
    </Box>
  );
}
