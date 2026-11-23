import { useEffect, useState } from 'react';
import { BookOpen, CircleHelp, DatabaseZap, FileText, Info, RefreshCw, Trash2 } from 'lucide-react';
import { toComponentJob } from '../api';
import type { JobStatus } from '../api';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  CodeBlock,
  ComponentJobFailure,
  ComponentJobProgress,
  Dialog,
  EmptyState,
  Progress,
  UninstallSheet,
} from '../components';
import { useT } from '../i18n';
import type { PaneProps } from './types';

const DOCS_URL = 'https://github.com/LC-jhony/lair/tree/main/docs';
const ISSUES_URL = 'https://github.com/LC-jhony/lair/issues/new';

/** About pane: versions, links, jobs and logs (the latter from logs-list). */
export function SystemPane({ state }: PaneProps) {
  const t = useT();
  const jobs: JobStatus[] = state.status?.jobs ?? [];
  const bridge = window.lairDesktop;
  const [logs, setLogs] = useState<string[]>([]);
  const [log, setLog] = useState<{ name: string; text: string } | null>(null);
  const [uninstallOpen, setUninstallOpen] = useState(false);
  const [catalogMessage, setCatalogMessage] = useState('');

  useEffect(() => {
    if (!state.client) return;
    let alive = true;
    state.client.logsList().then((items) => { if (alive) setLogs(items); }).catch(() => {});
    return () => { alive = false; };
  }, [state.client]);

  const showLog = (name: string) => {
    void state.act(t('dash.readingLog', { name }), async (c) => {
      const text = await c.log(name);
      setLog({ name, text });
    });
  };

  const linkClass = 'inline-flex h-8 items-center gap-2 rounded-lg border border-input bg-background px-3 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={!!state.busy}
          onClick={() => {
            let text = '';
            void state.act(t('dash.updatingCatalog'), async (c) => {
              text = await c.updateCatalog();
            }).then(() => { if (text) setCatalogMessage(text); });
          }}
        >
          <DatabaseZap className="size-3.5" aria-hidden="true" />
          {t('dash.updateCatalog')}
        </Button>
        {catalogMessage && <span role="status" className="text-xs text-muted-foreground">{catalogMessage}</span>}
        <Button variant="ghost" size="sm" disabled={!!state.busy} onClick={() => void state.refresh()}>
          <RefreshCw className="size-3.5" aria-hidden="true" />
          {t('dash.refresh')}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Info className="size-4 text-primary" aria-hidden="true" />
            {t('settings.pane.about')}
          </CardTitle>
          <CardDescription>{t('settings.pane.about.subtitle')}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <CodeBlock text={state.status?.home || '—'} />
          <div className="flex flex-wrap gap-2">
            <Badge tone="indigo">{t('about.version', { version: bridge?.electronVersion ?? '—' })}</Badge>
            <Badge tone="neutral">{bridge?.platform ?? '—'}</Badge>
            <Badge tone="green">{t('about.php', { version: state.status?.defaults?.php || '—' })}</Badge>
            <Badge tone="orange">{t('about.composer', { version: state.status?.composer || '—' })}</Badge>
          </div>
          <div className="flex flex-wrap gap-2">
            <a className={linkClass} href={DOCS_URL} target="_blank" rel="noreferrer">
              <BookOpen className="size-3.5" aria-hidden="true" />
              {t('about.docs')}
            </a>
            <a className={linkClass} href={ISSUES_URL} target="_blank" rel="noreferrer">
              <CircleHelp className="size-3.5" aria-hidden="true" />
              {t('about.reportIssue')}
            </a>
            <Button variant="ghost" size="sm" onClick={() => setUninstallOpen(true)}>
              <Trash2 className="size-3.5" aria-hidden="true" />
              {t('uninstall.action')}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('dash.connection')}</CardTitle>
          <CardDescription>{t('dash.sameFunctions')}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <CodeBlock text={state.status?.home || '—'} />
          <div className="flex flex-wrap gap-2">
            <Badge tone="indigo">Electron {bridge?.electronVersion ?? '—'}</Badge>
            <Badge tone="neutral">{bridge?.platform ?? '—'}</Badge>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t('dash.jobs')}</CardTitle>
            <CardDescription>{jobs.length ? `${jobs.length}` : t('dash.noJobs')}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {jobs.length === 0 ? (
              <EmptyState title={t('dash.noJobs')} />
            ) : (
              jobs.map((job, index) => {
                const component = toComponentJob(job);
                return (
                  <div key={job.id ?? index} className="flex flex-col gap-2 rounded-lg border border-border bg-muted/30 p-3">
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate text-sm font-medium">{job.id ?? '—'}</span>
                      <Badge tone={job.done ? 'green' : 'neutral'} dot>{job.phase ?? '—'}</Badge>
                    </div>
                    <Progress
                      value={job.downloaded ?? 0}
                      max={job.total && job.total > 0 ? job.total : 100}
                      label={job.id}
                    />
                    {job.error ? (
                      <ComponentJobFailure name={job.id ?? '—'} job={component} />
                    ) : (
                      <ComponentJobProgress title={job.id ?? '—'} job={component} />
                    )}
                    {!job.done && (
                      <Button variant="outline" size="sm" disabled={!!state.busy} onClick={() => void state.act(t('dash.cancelingJob', { name: job.id ?? '' }), (c) => c.cancelJob(job.id ?? ''))}>
                        <Trash2 className="size-3.5" aria-hidden="true" />
                        {t('dash.cancelJob')}
                      </Button>
                    )}
                  </div>
                );
              })
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('about.logFiles')}</CardTitle>
            <CardDescription>{t('pane.system.subtitle')}</CardDescription>
          </CardHeader>
          <CardContent>
            {logs.length === 0 ? (
              <EmptyState title={t('dash.noLogs')} />
            ) : (
              <ul className="flex flex-col gap-2">
                {logs.map((name) => (
                  <li key={name} className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
                    <span className="truncate text-sm font-medium">{name}</span>
                    <Button variant="ghost" size="sm" onClick={() => showLog(name)}>
                      <FileText className="size-3.5" aria-hidden="true" />
                      {t('dash.viewLog')}
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      <Dialog open={!!log} onClose={() => setLog(null)} title={log ? t('dash.logOf', { name: log.name }) : ''} size="lg">
        <pre className="max-h-[55vh] overflow-auto rounded-lg bg-muted/40 p-4 text-xs leading-relaxed">{log?.text || t('dash.noLog')}</pre>
      </Dialog>

      <UninstallSheet open={uninstallOpen} onClose={() => setUninstallOpen(false)} home={state.status?.home} />
    </div>
  );
}
