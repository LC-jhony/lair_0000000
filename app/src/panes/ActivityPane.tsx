import { useCallback, useEffect, useState } from 'react';
import { Activity, FileText, Mail, Wrench } from 'lucide-react';
import type { DumpEvent, JobStatus, MailMessage } from '../api';
import { Badge, Button, Card, CardContent, CardDescription, CardHeader, CardTitle, EmptyState, Select } from '../components';
import { useT } from '../i18n';
import type { PaneProps } from './types';

interface ActivityItem {
  id: string;
  kind: 'jobs' | 'dumps' | 'mail' | 'logs';
  time: string;
  title: string;
  detail: string;
}

const KINDS = ['jobs', 'dumps', 'mail', 'logs'] as const;
type Kind = typeof KINDS[number];

const ICONS = {
  jobs: <Wrench className="size-4" />,
  dumps: <Activity className="size-4" />,
  mail: <Mail className="size-4" />,
  logs: <FileText className="size-4" />,
};

/** One chronological feed across the daemon's support surfaces. */
export function ActivityPane({ state }: PaneProps) {
  const t = useT();
  const [items, setItems] = useState<ActivityItem[]>([]);
  const [kind, setKind] = useState<Kind | ''>('');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    if (!state.client) return;
    try {
      const [jobs, dumps, mail, logs] = await Promise.all([
        state.client.jobs(),
        state.client.dumps({ op: 'list' }),
        state.client.mail({ op: 'list' }),
        state.client.logsList(),
      ]);
      const next: ActivityItem[] = [];
      for (const job of jobs as JobStatus[]) {
        next.push({
          id: `job-${job.id}`, kind: 'jobs', time: '',
          title: job.id ?? '—',
          detail: `${job.package ?? ''} ${job.version ?? ''} · ${job.phase ?? ''}${job.error ? ` · ${job.error}` : ''}`.trim(),
        });
      }
      for (const event of (dumps.events ?? []) as DumpEvent[]) {
        next.push({
          id: `dump-${event.id ?? event.time}`, kind: 'dumps', time: event.time ?? '',
          title: `${event.site ?? '—'} · ${event.type ?? 'other'}`,
          detail: event.request ?? (event.message ?? '').slice(0, 120),
        });
      }
      for (const message of (mail.messages ?? []) as MailMessage[]) {
        next.push({
          id: `mail-${message.id}`, kind: 'mail', time: message.date ?? '',
          title: message.subject ?? '—',
          detail: `${message.from ?? ''} → ${message.to ?? ''}`,
        });
      }
      for (const name of logs as string[]) {
        next.push({ id: `log-${name}`, kind: 'logs', time: '', title: name, detail: '' });
      }
      next.sort((a, b) => b.time.localeCompare(a.time));
      setItems(next);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [state.client]);

  useEffect(() => { void load(); }, [load]);

  const visible = kind ? items.filter((item) => item.kind === kind) : items;

  return (
    <div className="flex flex-col gap-5">
      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle>{t('activity.title')}</CardTitle>
            <CardDescription>{t('activity.subtitle')}</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Select
              label={t('activity.filter')}
              value={kind}
              options={[{ value: '', label: t('activity.all') }, ...KINDS.map((value) => ({ value, label: t(`activity.kind.${value}`) }))]}
              onChange={(value) => setKind(value as Kind | '')}
            />
            <Button variant="ghost" size="sm" onClick={() => void load()}>{t('dash.refresh')}</Button>
          </div>
        </CardHeader>
        <CardContent>
          {error && <p role="alert" className="mb-3 text-xs text-destructive">{error}</p>}
          {visible.length === 0 ? (
            <EmptyState title={t('activity.empty')} description={t('activity.emptyHelp')} />
          ) : (
            <ol className="flex flex-col gap-2">
              {visible.map((item) => (
                <li key={item.id} className="flex items-start gap-3 rounded-lg border border-border bg-muted/30 p-3">
                  <span className="mt-0.5 text-primary" aria-hidden="true">{ICONS[item.kind]}</span>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone="neutral">{t(`activity.kind.${item.kind}`)}</Badge>
                      <span className="truncate text-sm font-medium">{item.title}</span>
                    </div>
                    <p className="truncate text-xs text-muted-foreground">{item.detail || '—'}</p>
                  </div>
                  <span className="font-mono text-[11px] text-muted-foreground">{item.time || '—'}</span>
                </li>
              ))}
            </ol>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
