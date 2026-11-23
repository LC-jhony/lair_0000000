import { useCallback, useEffect, useRef, useState } from 'react';
import { Activity, Database, FileCode2, ListChecks, Trash2, Webhook } from 'lucide-react';
import type { DumpsPayload, DumpsResult, DumpsOptions } from '../api';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DumpsTimeline,
  EmptyState,
  Input,
  Select,
  StatCard,
  StatusDot,
  Switch,
} from '../components';
import { useT } from '../i18n';
import type { PaneProps } from './types';

const OPTION_TYPES = ['queries', 'logs', 'views', 'jobs', 'requests'] as const;
const TYPE_ICONS = {
  queries: <Database />,
  logs: <FileCode2 />,
  views: <Activity />,
  jobs: <ListChecks />,
  requests: <Webhook />,
};

function enabledTypes(options: DumpsOptions | undefined): string[] {
  return OPTION_TYPES.filter((type) => Boolean(options?.[type]));
}

/** Captured query/log/view/job/request dumps with type filters and search. */
export function DumpsPane({ state }: PaneProps) {
  const t = useT();
  const [result, setResult] = useState<DumpsResult | null>(null);
  const [query, setQuery] = useState('');
  const [latestOnly, setLatestOnly] = useState(false);
  const [site, setSite] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [confirmClear, setConfirmClear] = useState(false);
  const aliveRef = useRef(true);

  const load = useCallback(async () => {
    if (!state.client) return;
    try {
      const [status, list] = await Promise.all([
        state.client.dumps({ op: 'status' }),
        state.client.dumps({ op: 'list' }),
      ]);
      if (!aliveRef.current) return;
      setResult({ ...status, ...list, count: list.events?.length ?? status.count ?? 0 });
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [state.client]);

  useEffect(() => {
    aliveRef.current = true;
    void load();
    return () => { aliveRef.current = false; };
  }, [load]);

  const run = async (payload: DumpsPayload) => {
    if (!state.client) return;
    setBusy(true);
    setError('');
    try {
      const next = await state.client.dumps(payload);
      if (!aliveRef.current) return;
      setResult((previous) => {
        const merged = { ...(previous ?? {}), ...next };
        if (payload.op === 'clear') return { ...merged, events: [], count: 0 };
        return {
          ...merged,
          events: next.events ?? previous?.events,
          count: next.count ?? next.events?.length ?? previous?.count ?? 0,
        };
      });
      if (payload.op === 'clear') setConfirmClear(false);
      await state.refresh();
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      if (aliveRef.current) setBusy(false);
    }
  };

  const events = result?.events ?? [];
  const options = result?.options ?? {};
  const busyAll = busy || !!state.busy;
  const sites = state.status?.sites ?? [];
  const siteOptions = [
    { value: '', label: t('dumps.allSites') },
    ...sites.map((item) => ({ value: item.name ?? '', label: item.name ?? '' })),
  ];

  const setOption = (type: string, checked: boolean) => {
    const next = new Set(enabledTypes(options));
    if (checked) next.add(type);
    else next.delete(type);
    void run({ op: 'options', types: OPTION_TYPES.filter((item) => next.has(item)), site: site || undefined });
  };

  const changeSite = (value: string) => {
    setSite(value);
    void run({ op: 'options', types: enabledTypes(options), site: value || undefined });
  };

  return (
    <div className="flex flex-col gap-5">
      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div className="flex items-start gap-3">
            <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary">
              <Activity className="size-5" aria-hidden="true" />
            </span>
            <div>
              <CardTitle className="flex items-center gap-2">
                {t('pane.dumps')}
                <StatusDot on={Boolean(result?.capturing)} />
                <Badge tone={result?.capturing ? 'green' : 'neutral'} dot>
                  {result?.capturing ? t('dumps.capturing') : t('dumps.notCapturing')}
                </Badge>
              </CardTitle>
              <CardDescription>{t('pane.dumps.subtitle')}</CardDescription>
            </div>
          </div>
          <Button
            variant="ghost"
            size="sm"
            disabled={busyAll || events.length === 0}
            onClick={() => setConfirmClear(true)}
          >
            <Trash2 className="size-3.5" aria-hidden="true" />
            {t('common.clear')}
          </Button>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <StatCard label={t('dumps.total')} value={String(events.length)} icon={<Activity />} />
            <StatCard label={t('dumps.sites')} value={String(new Set(events.map((event) => event.site).filter(Boolean)).size)} />
            <StatCard label={t('dumps.queries')} value={String(events.filter((event) => event.type === 'queries').length)} />
          </div>
          <div className="rounded-xl border border-border bg-muted/30 p-4">
            <Switch
              checked={Boolean(result?.capturing)}
              onChange={(checked) => void run({ op: checked ? 'capture-on' : 'capture-off' })}
              label={t('dumps.capture')}
              description={t('dumps.captureHelp')}
              disabled={busyAll}
            />
          </div>
          {result?.output && <p className="text-xs text-muted-foreground">{result.output}</p>}
          {error && <p role="alert" className="text-xs text-red-600 dark:text-red-400">{error}</p>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('dumps.options.title')}</CardTitle>
          <CardDescription>{t('dumps.options.help')}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {OPTION_TYPES.map((type) => (
              <div key={type} className="flex items-center gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
                <span className="text-primary/70 [&>svg]:size-4">{TYPE_ICONS[type]}</span>
                <Switch
                  className="flex-1"
                  checked={Boolean(options[type])}
                  onChange={(checked) => setOption(type, checked)}
                  label={t(`dumps.type.${type}`)}
                  disabled={busyAll}
                />
              </div>
            ))}
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <Switch
              checked={Boolean(options.show_in_page)}
              onChange={(checked) => void run({ op: 'options', types: enabledTypes(options), show_in_page: checked })}
              label={t('dumps.showInPage')}
              description={t('dumps.showInPageHelp')}
              disabled={busyAll}
            />
            <Select
              label={t('dumps.site')}
              value={site}
              options={siteOptions}
              onChange={changeSite}
              disabled={busyAll}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle>{t('dumps.timeline')}</CardTitle>
            <CardDescription>{t('dumps.timelineHelp')}</CardDescription>
          </div>
          <Switch
            checked={latestOnly}
            onChange={setLatestOnly}
            label={t('dumps.latestOnly')}
            description={t('dumps.latestOnlyHelp')}
            disabled={busyAll}
          />
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">{t('dumps.search')}</span>
            <Input
              type="search"
              value={query}
              aria-label={t('dumps.search')}
              placeholder={t('dumps.searchPlaceholder')}
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          {events.length === 0 ? (
            <EmptyState icon={<Activity />} title={t('dumps.noEvents')} description={t('dumps.noEventsHelp')} />
          ) : (
            <DumpsTimeline events={events} query={query} latestOnly={latestOnly} />
          )}
        </CardContent>
      </Card>

      <Dialog
        open={confirmClear}
        onClose={() => setConfirmClear(false)}
        title={t('dumps.clear.title')}
        description={t('dumps.clear.message')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirmClear(false)}>{t('common.cancel')}</Button>
            <Button variant="danger" disabled={busyAll} onClick={() => void run({ op: 'clear' })}>
              <Trash2 className="size-3.5" aria-hidden="true" />
              {t('common.clear')}
            </Button>
          </>
        }
      />
    </div>
  );
}
