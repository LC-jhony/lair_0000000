import { Box } from 'ink';
import React from 'react';
import { useT } from '../../src/i18n';
import { Badge } from '../components/ui/badge';
import { Facts, Hint, Section, useKeys } from './helpers';
import type { TerminalPaneProps } from './types';

export function OverviewPane({ state }: TerminalPaneProps) {
  const t = useT();
  const status = state.status;
  const services = status?.services ?? {};
  const running = Object.values(services).filter((s) => s.running).length;
  const defaultPHP = status?.php?.find((p) => p.default)?.minor ?? '—';
  const defaultNode = status?.node?.[0] ?? '—';

  useKeys({
    r: () => void state.refresh(),
    a: () => void state.act(t('dash.restartingAll'), (c) => c.restart()),
    u: () => void state.act(t('dash.updatingCatalog'), (c) => c.updateCatalog()),
    c: () => void state.act(t('common.installing', { name: 'Composer' }), (c) => c.installComposer()),
    s: () => void state.act(t('dash.refresh'), (c) => c.ensureShims()),
    j: () => {
      const job = status?.jobs?.find((item) => !item.done && item.id);
      if (job?.id) void state.act(t('dash.cancelJob'), (c) => c.cancelJob(job.id!));
    },
  });

  return (
    <Box flexDirection="column">
      <Section title={t('pane.overview')} subtitle={t('pane.overview.subtitle')}>
        <Facts
          items={[
            { key: t('dash.home'), value: status?.home ?? t('dash.waiting') },
            { key: t('dash.phpVersion'), value: defaultPHP },
            { key: t('dash.nodeVersion'), value: defaultNode },
            { key: t('dash.database'), value: status?.defaults?.database ?? t('dash.none') },
            { key: t('dash.services'), value: String(running) },
            { key: t('dash.sites'), value: String(status?.sites?.length ?? 0) },
            { key: t('dash.jobs'), value: String(status?.jobs?.length ?? 0) },
          ]}
          keyWidth={18}
        />
      </Section>
      <Box>
        <Badge variant="info">{t('dash.sameFunctions')}</Badge>
      </Box>
      <Hint text="r: refresh · a: restart services · u: update catalog · c: composer · s: shims · j: cancel job" />
    </Box>
  );
}
