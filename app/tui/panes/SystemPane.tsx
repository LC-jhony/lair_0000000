import { Box, Text } from 'ink';
import React from 'react';
import { jobProgress } from '../../src/api';
import { useT } from '../../src/i18n';
import { Facts, Hint, Section, useKeys } from './helpers';
import type { TerminalPaneProps } from './types';

export function SystemPane({ state }: TerminalPaneProps) {
  const t = useT();
  const status = state.status;
  const jobs = status?.jobs ?? [];

  useKeys({
    u: () => void state.act(t('dash.updatingCatalog'), (c) => c.updateCatalog()),
    r: () => void state.act(t('dash.restartingAll'), (c) => c.restart()),
    s: () => void state.act(t('dash.refresh'), (c) => c.ensureShims()),
    i: () => void state.act(t('dash.refresh'), (c) => c.projectInfo()),
    d: () => void state.act(t('dash.refresh'), (c) => c.projectDirs()),
    y: () => void state.act(t('dash.refresh'), (c) => c.projectSync()),
    w: () => void state.act(t('dash.refresh'), (c) => c.web('status')),
    h: () => void state.act(t('dash.secure'), (c) => c.web(status?.web?.https ? 'https-off' : 'https-on')),
    g: () => void state.act(t('dash.refresh'), (c) => c.web('configure', {
      http_port: status?.web?.http_port ?? 80,
      https_port: status?.web?.https_port ?? 443,
      https: status?.web?.https ?? false,
      redirect_http: status?.web?.redirect_http ?? false,
      bind: status?.web?.bind ?? '127.0.0.1',
      domain: status?.web?.domain ?? 'test',
    })),
  });

  return (
    <Box flexDirection="column">
      <Section title={t('pane.system')} subtitle={t('pane.system.subtitle')}>
        <Facts items={[
          { key: t('dash.home'), value: status?.home ?? t('dash.waiting') },
          { key: t('dash.connection'), value: state.client ? t('common.active') : t('common.stopped') },
          { key: t('dash.jobs'), value: String(jobs.length) },
          { key: t('dash.database'), value: status?.defaults?.database ?? t('dash.none') },
        ]} keyWidth={16} />
        <Box marginTop={1} flexDirection="column">
          {jobs.length === 0 ? <Text dimColor>{t('dash.noJobs')}</Text> : jobs.map((job, i) => {
            const progress = jobProgress(job);
            return (
              <Text key={job.id ?? i} dimColor>
                {`${job.id ?? ''} ${progress ?? job.phase ?? ''}`}
              </Text>
            );
          })}
        </Box>
      </Section>
      <Hint text="u: catalog · r: restart · s: shims · i: info · d: dirs · y: sync · w: web · h: HTTPS · g: configure" />
    </Box>
  );
}
