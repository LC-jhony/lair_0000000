import { useCallback, useEffect, useState } from 'react';
import { RefreshCw, Trash2 } from 'lucide-react';
import type { JobStatus } from '../api';
import { jobProgress, projectProgress } from '../api';
import { Badge, Button, Card, CardContent, CardDescription, CardHeader, CardTitle, EmptyState, Progress } from '../components';
import { LogViewer } from '../components/LogViewer';
import { useT } from '../i18n';
import type { PaneProps } from './types';

const REFRESH_MS = 3000;

function tone(job: JobStatus, log: string): 'danger' | 'warning' | 'success' | undefined {
  if (job.error || job.canceled) return 'danger';
  if (job.done) return 'success';
  const value = jobProgress(job);
  if (value === null ? projectProgress(log, job) >= 50 : value >= 0.5) return 'warning';
  return 'danger';
}

function percent(job: JobStatus, log: string): number {
  if (job.done || job.error || job.canceled) return 100;
  const ratio = jobProgress(job);
  if (ratio !== null) return Math.round(ratio * 100);
  return projectProgress(log, job);
}

/** Live view of the daemon's installer/supervisor jobs with logs and retry. */
export function JobsPane({ state }: PaneProps) {
  const t = useT();
  const [jobs, setJobs] = useState<JobStatus[]>([]);
  const [logs, setLogs] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<JobStatus | null>(null);
  const [text, setText] = useState('');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    if (!state.client) return;
    try {
      const list = await state.client.jobs();
      const entries = await Promise.all(list.map(async (job) => {
        const raw = await state.client!.log(`${job.id}-install.log`).catch(() => '');
        return [job.id ?? '', raw] as const;
      }));
      setJobs(list);
      setLogs(Object.fromEntries(entries));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [state.client]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), REFRESH_MS);
    return () => window.clearInterval(timer);
  }, [load]);

  const showLog = (job: JobStatus) => {
    setSelected(job);
    void (async () => {
      try {
        const raw = await state.client?.log(`${job.id}-install.log`);
        setText(raw ?? t('jobs.noLog'));
      } catch (cause) {
        setText(cause instanceof Error ? cause.message : String(cause));
      }
    })();
  };

  const act = (label: string, fn: (client: NonNullable<typeof state.client>) => Promise<unknown>) => {
    void state.act(label, async (client) => { await fn(client); await load(); });
  };

  return (
    <div className="flex flex-col gap-5">
      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle>{t('jobs.title')}</CardTitle>
            <CardDescription>{t('jobs.subtitle')}</CardDescription>
          </div>
          <Button variant="ghost" size="sm" onClick={() => void load()}>
            <RefreshCw className="size-4" aria-hidden="true" />
            {t('dash.refresh')}
          </Button>
        </CardHeader>
        <CardContent>
          {error && <p role="alert" className="mb-3 text-xs text-red-600 dark:text-red-400">{error}</p>}
          {jobs.length === 0 ? (
            <EmptyState title={t('jobs.empty')} description={t('jobs.emptyHelp')} />
          ) : (
            <div className="flex flex-col gap-2">
              {jobs.map((job) => {
                const failed = Boolean(job.error || job.canceled);
                return (
                  <div key={job.id} className="flex flex-col gap-2 rounded-lg border border-border bg-muted/30 p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">{job.id}</p>
                        <p className="text-xs text-muted-foreground">{job.package ?? '—'} {job.version ?? ''} · {job.phase ?? '—'}</p>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5">
                        <Badge tone={job.done && !failed ? 'green' : failed ? 'red' : 'blue'} dot>
                          {job.done && !failed ? t('jobs.done') : failed ? t('jobs.failed') : t('jobs.running')}
                        </Badge>
                        <Button variant="outline" size="sm" onClick={() => showLog(job)}>{t('jobs.viewLog')}</Button>
                        {!job.done && !failed && (
                          <Button variant="ghost" size="sm" onClick={() => act(t('jobs.canceling'), (c) => c.cancelJob(job.id!))}>
                            <Trash2 className="size-3.5" aria-hidden="true" />{t('jobs.cancel')}
                          </Button>
                        )}
                        {failed && (
                          <Button variant="outline" size="sm" onClick={() => act(t('jobs.retrying'), (c) => c.retryJob(job.id!))}>
                            {t('jobs.retry')}
                          </Button>
                        )}
                      </div>
                    </div>
                    <Progress value={percent(job, logs[job.id ?? ''] ?? '')} tone={tone(job, logs[job.id ?? ''] ?? '')} label={job.id} />
                    {job.error && <p className="text-xs text-red-600 dark:text-red-400">{job.error}</p>}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {selected && <LogViewer title={`${selected.id}-install.log`} text={text} onClose={() => setSelected(null)} />}
    </div>
  );
}
