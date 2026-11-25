import { useEffect, useState } from 'react';
import { Database, Eye, EyeOff, ExternalLink, FileText, Mail, Server } from 'lucide-react';
import type { LogSource, MailResult, QueueInfo, SiteData, SiteDetail, SiteEnvEntry, SiteStack, SiteView } from '../api';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  CodeBlock,
  Dialog,
  EmptyState,
  Input,
  LogViewer,
  Select,
  StatCard,
  StatusDot,
  Switch,
  Tabs,
  TabsList,
  TabsPanel,
  TabsPanels,
  TabsTrigger,
} from '../components';
import { useT } from '../i18n';
import type { LairController } from '../useLair';
import type { PaneId } from './types';

const DATABASES = ['sqlite', 'mysql', 'mariadb', 'pgsql', 'sqlsrv'];

export interface SiteDetailDialogProps {
  open: boolean;
  site: SiteView | null;
  state: LairController;
  onClose: () => void;
  /** Reuses the app-level navigation so the dialog can link to Mail and Logs. */
  onNavigate?: (id: PaneId) => void;
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/30 px-3 py-2">
      <span className="shrink-0 text-xs text-muted-foreground">{label}</span>
      <span className="truncate text-right text-sm font-medium">{value || '—'}</span>
    </div>
  );
}

/** Full inspector for one parked site: seven tabs backed by the `site` action. */
export function SiteDetailDialog({ open, site, state, onClose, onNavigate }: SiteDetailDialogProps) {
  const t = useT();
  const name = site?.name ?? site?.host ?? '';
  const root = site?.root ?? site?.path ?? '';
  const domain = state.status?.web?.domain ?? 'test';

  const [detail, setDetail] = useState<SiteDetail | null>(null);
  const [env, setEnv] = useState<SiteEnvEntry[]>([]);
  const [mail, setMail] = useState<MailResult | null>(null);
  const [queueInfo, setQueueInfo] = useState<QueueInfo | null>(null);
  const [siteData, setSiteData] = useState<SiteData | null>(null);
  const [stack, setStack] = useState<SiteStack | null>(null);
  const [envDrafts, setEnvDrafts] = useState<Record<string, string>>({});
  const [dbPassword, setDbPassword] = useState<string | null>(null);
  const [showPassword, setShowPassword] = useState(false);
  const [logs, setLogs] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [viewerOpen, setViewerOpen] = useState(false);
  const [vhostOpen, setVhostOpen] = useState(false);
  const [vhost, setVhost] = useState('');
  const [vhostLoading, setVhostLoading] = useState(false);
  const [vhostError, setVhostError] = useState('');
  const expectedRoot = detail?.root || root || detail?.root_suggestions?.[0] || '—';

  const [driver, setDriver] = useState('');
  const [dbName, setDbName] = useState('');
  const [phpVersion, setPhpVersion] = useState('');
  const [nodeVersion, setNodeVersion] = useState('');

  useEffect(() => {
    if (!open || !name || !state.client) return;
    let alive = true;
    setLoading(true);
    setError('');
    void (async () => {
      try {
        const client = state.client!;
        const [detailResult, envResult, mailResult] = await Promise.all([
          client.site({ op: 'detail', name }),
          client.site({ op: 'env', name }),
          client.mail({ op: 'status' }),
        ]);
        if (!alive) return;
        setDetail(detailResult.detail ?? null);
        setDbPassword(null);
        setShowPassword(false);
        const entries = envResult.env ?? [];
        setEnv(entries);
        // Show the real value in the editable input for non-secret keys.
        // Secrets arrive empty from the daemon and must never be prefilled.
        setEnvDrafts(Object.fromEntries(
          entries.filter((item) => item.key && !item.secret).map((item) => [item.key!, item.value ?? '']),
        ));
        setMail(mailResult);

        // Logs are grouped by source; pick this site's files, falling back to
        // every site source when the daemon does not name them consistently.
        try {
          setQueueInfo(await client.queue(name));
        } catch {
          setQueueInfo(null); // older daemon without the queue action
        }
        try {
          setSiteData(await client.siteData(name));
        } catch {
          setSiteData(null);
        }
        try {
          setStack(await client.siteStack(name));
        } catch {
          setStack(null);
        }
        const sources = await client.logSources();
        const short = name.split('.')[0] || name;
        const matches = (s: LogSource) =>
          s.kind === 'site' &&
          (s.id === name || s.name === name || s.id === short || s.name?.split('.')[0] === short);
        const siteSources = sources.filter(matches);
        const chosen = siteSources.length > 0 ? siteSources : sources.filter((s) => s.kind === 'site');
        const files = chosen.flatMap((s) => (s.files ?? []).map((file) =>
          s.kind === 'site' ? `site:${s.id}:${file}` : file));
        const texts = await Promise.all(files.map((file) => client.log(file)));
        if (alive) setLogs(texts.filter(Boolean).join('\n').trim());
      } catch (cause) {
        if (alive) setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => { alive = false; };
  }, [open, name, state.client]);

  useEffect(() => {
    if (!detail) return;
    setDriver(detail.database ?? '');
    setDbName(detail.database_name ?? '');
    setPhpVersion(detail.php ?? '');
    setNodeVersion(detail.node ?? '');
  }, [detail]);

  const run = (label: string, fn: Parameters<LairController['act']>[1]) => {
    void state.act(label, fn);
  };

  /** Toggles serving the vhost, then reflects the new state in the inspector. */
  const toggleEnabled = (enabled: boolean) => {
    if (!name || !detail || !!state.busy) return;
    void state.act(t(enabled ? 'site.enabling' : 'site.disabling', { name }), async (c) => {
      const result = await c.siteSetEnabled(name, enabled);
      const refreshed = result.detail ?? (await c.site({ op: 'detail', name })).detail;
      if (refreshed) setDetail(refreshed);
      // Keep the site list (and any other pane) in sync with the new state.
      await state.refresh();
    });
  };

  /** Reads the generated vhost on demand; disabled sites are readable too. */
  const openVhost = () => {
    if (!name || !state.client) return;
    const client = state.client;
    setVhostOpen(true);
    setVhostLoading(true);
    setVhostError('');
    setVhost('');
    void (async () => {
      try {
        const result = await client.siteConfig(name);
        setVhost(result.config ?? '');
      } catch (cause) {
        setVhostError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        setVhostLoading(false);
      }
    })();
  };

  const saveEnv = (key: string) => {
    const value = envDrafts[key] ?? '';
    if (!name || !key || !value.trim()) return;
    run(t('site.env.saving', { key }), async (c) => {
      await c.site({ op: 'env-set', name, key, value });
      const next = await c.site({ op: 'env', name });
      setEnv(next.env ?? []);
      setEnvDrafts((previous) => ({ ...previous, [key]: '' }));
    });
  };

  const togglePassword = () => {
    if (!name || !state.client) return;
    if (dbPassword !== null) {
      setShowPassword((value) => !value);
      return;
    }
    void (async () => {
      try {
        const result = await state.client!.site({ op: 'env', name, key: 'DB_PASSWORD', reveal: true });
        setDbPassword(result.value ?? '');
        setShowPassword(true);
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : String(cause));
      }
    })();
  };

  const phpOptions = [
    { value: '', label: t('site.runtime.default') },
    ...(state.status?.php ?? []).map((item) => ({
      value: item.minor ?? '',
      label: `PHP ${item.minor ?? '—'}`,
    })),
  ];
  const nodeOptions = [
    { value: '', label: t('site.runtime.default') },
    ...(state.status?.node ?? []).map((version) => ({ value: version, label: `Node ${version}` })),
  ];
  const databaseOptions = DATABASES.map((value) => ({ value, label: value }));

  const mailEnv = [
    'MAIL_MAILER=smtp',
    'MAIL_HOST=127.0.0.1',
    `MAIL_PORT=${mail?.port ?? 2525}`,
    'MAIL_ENCRYPTION=null',
    `MAIL_FROM_ADDRESS=no-reply@${domain}`,
  ].join('\n');

  const services = Object.entries(state.status?.services ?? {});
  const envValue = (key: string) => env.find((entry) => entry.key === key)?.value ?? '';

  return (
    <>
      <Dialog
        open={open}
        onClose={onClose}
        title={t('site.detail.title', { name })}
        description={t('site.detail.subtitle')}
        size="lg"
      >
        {loading && <p className="text-xs text-muted-foreground">{t('site.overview.loading')}</p>}
        {error && <p role="alert" className="mb-3 text-xs text-red-600 dark:text-red-400">{error}</p>}
        <Tabs>
          <TabsList className="flex-wrap">
            <TabsTrigger>{t('site.tab.overview')}</TabsTrigger>
            <TabsTrigger>{t('site.tab.environment')}</TabsTrigger>
            <TabsTrigger>{t('site.tab.database')}</TabsTrigger>
            <TabsTrigger>{t('site.tab.mail')}</TabsTrigger>
            <TabsTrigger>{t('site.tab.queues')}</TabsTrigger>
            <TabsTrigger>{t('site.tab.stack')}</TabsTrigger>
            <TabsTrigger>{t('site.tab.data')}</TabsTrigger>
            <TabsTrigger>{t('site.tab.phpNode')}</TabsTrigger>
            <TabsTrigger>{t('site.tab.logs')}</TabsTrigger>
          </TabsList>
          <TabsPanels>
            {/* 1. Overview */}
            <TabsPanel className="flex flex-col gap-4">
              <div className="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 p-3 sm:flex-row sm:items-center sm:justify-between">
                <Switch
                  checked={Boolean(detail?.enabled ?? site?.enabled ?? false)}
                  disabled={!!state.busy || loading || !name || !detail}
                  onChange={toggleEnabled}
                  label={t('site.enabled')}
                  description={t('site.enabledHelp')}
                />
                <Button
                  variant="outline"
                  size="sm"
                  className="shrink-0"
                  disabled={!name || !state.client}
                  onClick={openVhost}
                >
                  <FileText className="size-3.5" aria-hidden="true" />
                  {t('site.viewVhost')}
                </Button>
              </div>
              {detail?.root_state && detail.root_state !== 'ok' && (
                <div role="alert" className="rounded-lg border border-amber-500/50 bg-amber-500/10 p-3 text-xs text-amber-800 dark:text-amber-300">
                  <p className="font-medium">
                    {t('site.notConfigured', { name, state: detail.root_state })}
                  </p>
                  <p className="mt-1">{t('site.expectedRoot', { path: expectedRoot })}</p>
                </div>
              )}
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <StatCard label={t('site.overview.framework')} value={detail?.framework || '—'} icon={<Server />} />
                <StatCard label={t('site.overview.php')} value={detail?.php_version || detail?.php || '—'} />
                <StatCard label={t('site.overview.database')} value={detail?.database || '—'} icon={<Database />} />
                <StatCard label={t('site.overview.https')} value={detail?.secure ? t('common.running') : t('common.stopped')} />
              </div>
              <div className="flex flex-col gap-2">
                <Metric label={t('site.overview.type')} value={detail?.type ?? ''} />
                <Metric label={t('site.overview.url')} value={detail?.url ?? site?.url ?? ''} />
                <Metric label={t('site.overview.root')} value={detail?.root ?? root} />
                <Metric label={t('site.overview.rootState')} value={detail?.root_state ?? ''} />
                <Metric label={t('site.overview.domains')} value={(detail?.domains ?? []).join(', ')} />
              </div>
              {detail?.problem ? (
                <p role="alert" className="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">
                  {t('site.overview.problem')}: {detail.problem}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">{t('site.overview.noProblem')}</p>
              )}
              {detail?.notice && (
                <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
                  {t('site.overview.notice')}: {detail.notice}
                </p>
              )}
              {detail?.root_suggestions && detail.root_suggestions.length > 0 && (
                <ul className="flex flex-col gap-1 text-xs text-muted-foreground">
                  {detail.root_suggestions.map((suggestion) => <li key={suggestion}>{suggestion}</li>)}
                </ul>
              )}
            </TabsPanel>

            {/* 2. Environment — non-secret values are shown; secrets stay masked. */}
            <TabsPanel className="flex flex-col gap-4">
              <p className="text-xs text-muted-foreground">{t('site.env.help')}</p>
              {env.length === 0 ? (
                <EmptyState title={t('site.env.empty')} />
              ) : (
                <div className="flex flex-col gap-2">
                  {env.map((entry) => {
                    const key = entry.key ?? '';
                    return (
                      <div key={key} className="flex flex-col gap-2 rounded-lg border border-border bg-muted/30 p-3 sm:flex-row sm:items-center sm:justify-between">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="truncate text-sm font-medium">{key}</span>
                            {entry.secret && <Badge tone="red">{t('site.env.secret')}</Badge>}
                            {entry.set && <Badge tone="green">{t('site.env.set')}</Badge>}
                          </div>
                          <p className="mt-0.5 text-xs text-muted-foreground">
                            {t('site.env.value')}:{' '}
                            {entry.secret ? t('site.env.masked') : (entry.value || '—')}
                          </p>
                        </div>
                        <div className="flex items-center gap-2">
                          <Input
                            type={entry.secret ? 'password' : 'text'}
                            aria-label={`${t('site.env.newValue')} ${key}`}
                            placeholder={t('site.env.newValue')}
                            autoComplete="off"
                            value={envDrafts[key] ?? ''}
                            className="sm:w-48"
                            onChange={(event) => setEnvDrafts((previous) => ({ ...previous, [key]: event.target.value }))}
                          />
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={!!state.busy || !(envDrafts[key] ?? '').trim()}
                            aria-label={`${t('site.env.save')} ${key}`}
                            onClick={() => saveEnv(key)}
                          >
                            {t('site.env.save')}
                          </Button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
              <p className="text-xs text-muted-foreground">{t('site.env.secretHelp')}</p>
            </TabsPanel>

            {/* 3. Database */}
            <TabsPanel className="flex flex-col gap-4">
              <div className="grid gap-2 sm:grid-cols-2">
                <Metric label={t('site.db.driver')} value={detail?.database ?? ''} />
                <Metric label={t('site.db.host')} value={detail?.database_host ?? ''} />
                <Metric label={t('site.db.port')} value={String(detail?.database_port ?? '')} />
                <Metric label={t('site.db.name')} value={detail?.database_name ?? ''} />
                <Metric label={t('site.db.user')} value={envValue('DB_USERNAME') || '—'} />
                <div className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
                  <span className="shrink-0 text-xs text-muted-foreground">{t('site.db.password')}</span>
                  <div className="flex min-w-0 items-center gap-1.5">
                    <Input
                      readOnly
                      type={showPassword ? 'text' : 'password'}
                      aria-label={t('site.db.password')}
                      value={showPassword ? (dbPassword ?? '') : t('site.env.masked')}
                      className="h-7 w-32 text-right text-sm"
                    />
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      aria-label={showPassword ? t('site.db.hide') : t('site.db.reveal')}
                      disabled={!env.some((e) => e.key === 'DB_PASSWORD') || !!state.busy}
                      onClick={togglePassword}
                    >
                      {showPassword ? <EyeOff className="size-4" aria-hidden="true" /> : <Eye className="size-4" aria-hidden="true" />}
                    </Button>
                  </div>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">{t('site.db.driverHelp')}</p>
              <div className="flex flex-wrap items-end gap-2">
                <Select label={t('site.db.driver')} value={driver} options={databaseOptions} onChange={setDriver} className="min-w-40" />
                <Button variant="outline" size="sm" disabled={!!state.busy || !driver || driver === detail?.database} onClick={() =>
                  run(t('site.saved'), (c) => c.projectSetDB(name, driver))
                }>
                  {t('site.db.saveDriver')}
                </Button>
              </div>
              <div className="flex flex-wrap items-end gap-2">
                <label className="flex min-w-48 flex-col gap-1.5">
                  <span className="text-xs font-medium text-muted-foreground">{t('site.db.name')}</span>
                  <Input value={dbName} placeholder="acme_dev" onChange={(event) => setDbName(event.target.value)} />
                </label>
                <Button variant="outline" size="sm" disabled={!!state.busy || !dbName.trim()} onClick={() =>
                  run(t('site.db.creating', { name: dbName }), (c) => c.createDB(driver || detail?.database || 'mariadb', dbName.trim()))
                }>
                  <Database className="size-3.5" aria-hidden="true" />
                  {t('site.db.create')}
                </Button>
              </div>
              {!detail?.database && <p className="text-xs text-muted-foreground">{t('site.db.noDriver')}</p>}
            </TabsPanel>

            {/* 4. Mail */}
            <TabsPanel className="flex flex-col gap-4">
              <p className="text-xs text-muted-foreground">{t('site.mail.help')}</p>
              {mail ? (
                <CodeBlock text={mailEnv} />
              ) : (
                <p className="text-xs text-muted-foreground">{t('site.mail.missing')}</p>
              )}
              <Button variant="outline" size="sm" disabled={!onNavigate} onClick={() => onNavigate?.('mail')}>
                <Mail className="size-3.5" aria-hidden="true" />
                {t('site.mail.open')}
              </Button>
            </TabsPanel>

            {/* 5. Queues — read-only */}
            <TabsPanel className="flex flex-col gap-4">
              <p className="text-xs text-muted-foreground">{t('site.queues.help')}</p>
              <Metric label={t('site.queues.driver')} value={queueInfo?.driver ?? detail?.database ?? ''} />
              <div className="flex flex-wrap gap-2">
                <Badge tone="blue">{t('site.queues.pending', { count: queueInfo?.pending ?? 0 })}</Badge>
                <Badge tone={queueInfo?.failed ? 'red' : 'neutral'}>{t('site.queues.failed', { count: queueInfo?.failed ?? 0 })}</Badge>
              </div>
              {queueInfo?.error && <p className="text-xs text-destructive">{queueInfo.error}</p>}
              {queueInfo?.hint && <p className="text-xs text-muted-foreground">{queueInfo.hint}</p>}
              {services.length === 0 ? (
                <p className="text-xs text-muted-foreground">{t('site.queues.noServices')}</p>
              ) : (
                <div className="flex flex-col gap-2">
                  {services.map(([id, service]) => (
                    <div key={id} className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/30 px-3 py-2">
                      <span className="flex items-center gap-2 text-sm font-medium">
                        <StatusDot on={Boolean(service.running)} />
                        {id}
                      </span>
                      <Badge tone={service.running ? 'green' : 'neutral'} dot>
                        {service.running ? t('common.running') : t('common.stopped')}
                      </Badge>
                    </div>
                  ))}
                </div>
              )}
              <Button variant="outline" size="sm" disabled={!onNavigate} onClick={() => onNavigate?.('logs')}>
                <FileText className="size-3.5" aria-hidden="true" />
                {t('site.queues.openLogs')}
              </Button>
            </TabsPanel>

            {/* 6. Stack — detected technologies, no credentials */}
            <TabsPanel className="flex flex-col gap-4">
              <p className="text-xs text-muted-foreground">{t('site.stack.help')}</p>
              {stack?.error && <p role="alert" className="text-xs text-destructive">{stack.error}</p>}
              <div className="grid gap-2 sm:grid-cols-2">
                <Metric label={t('site.stack.framework')} value={`${stack?.framework ?? '—'} ${stack?.framework_version ?? ''}`.trim()} />
                <Metric label={t('site.stack.database')} value={`${stack?.database ?? '—'}${stack?.database_name ? ` · ${stack.database_name}` : ''}`} />
                {stack?.database_host && <Metric label={t('site.stack.dbHost')} value={`${stack.database_host}${stack.database_port ? `:${stack.database_port}` : ''}`} />}
                <Metric label={t('site.stack.cache')} value={stack?.cache ?? '—'} />
                <Metric label={t('site.stack.webRoot')} value={stack?.web_root ?? '—'} />
                <Metric label={t('site.stack.front')} value={stack?.front_controller ?? '—'} />
                <Metric label={t('site.stack.server')} value={stack?.server ?? '—'} />
                <Metric label={t('site.stack.php')} value={`${stack?.php ?? '—'} · ${stack?.node ?? '—'}`} />
              </div>
              <div className="flex flex-wrap gap-2">
                {(stack?.components ?? []).map((component) => (
                  <Badge key={component} tone="neutral">{component}</Badge>
                ))}
              </div>
            </TabsPanel>

            {/* 7. Data — read-only counts, no values */}
            <TabsPanel className="flex flex-col gap-4">
              <p className="text-xs text-muted-foreground">{t('site.data.help')}</p>
              {siteData?.error && <p role="alert" className="text-xs text-destructive">{siteData.error}</p>}
              <div className="grid gap-2 sm:grid-cols-2">
                {(siteData?.tables ?? []).map((table) => (
                  <Metric key={table.name} label={table.name ?? '—'} value={table.error ? '—' : String(table.rows ?? 0)} />
                ))}
              </div>
              <div className="flex flex-wrap gap-2">
                <Badge tone="blue">{t('site.data.cache', { count: siteData?.cache_keys ?? 0 })}</Badge>
                <Badge tone="indigo">{t('site.data.mail', { count: siteData?.mail ?? 0 })}</Badge>
              </div>
            </TabsPanel>

            {/* 7. PHP & Node.js */}
            <TabsPanel className="flex flex-col gap-4">
              <p className="text-xs text-muted-foreground">{t('site.runtime.help')}</p>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm">PHP</CardTitle>
                  <CardDescription>
                    {t('site.runtime.source')}: {detail?.php_source || t('site.runtime.default')}
                    {detail?.php_file ? ` · ${detail.php_file}` : ''}
                  </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-wrap items-end gap-2">
                  <Select label={t('site.runtime.version')} value={phpVersion} options={phpOptions} onChange={setPhpVersion} className="min-w-40" />
                  <Button variant="outline" size="sm" disabled={!!state.busy || !phpVersion} onClick={() =>
                    run(t('site.runtime.saving', { tool: 'PHP' }), (c) => c.pin('php', phpVersion, root))
                  }>{t('site.runtime.pin', { tool: 'PHP' })}</Button>
                  <Button variant="outline" size="sm" disabled={!!state.busy || !phpVersion} onClick={() =>
                    run(t('site.runtime.saving', { tool: 'PHP' }), (c) => c.isolate('php', phpVersion, root))
                  }>{t('site.runtime.isolate', { tool: 'PHP' })}</Button>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm">Node.js</CardTitle>
                  <CardDescription>
                    {t('site.runtime.source')}: {detail?.node_source || t('site.runtime.default')}
                    {detail?.node_file ? ` · ${detail.node_file}` : ''}
                  </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-wrap items-end gap-2">
                  <Select label={t('site.runtime.version')} value={nodeVersion} options={nodeOptions} onChange={setNodeVersion} className="min-w-40" />
                  <Button variant="outline" size="sm" disabled={!!state.busy || !nodeVersion} onClick={() =>
                    run(t('site.runtime.saving', { tool: 'Node' }), (c) => c.pin('node', nodeVersion, root))
                  }>{t('site.runtime.pin', { tool: 'Node' })}</Button>
                  <Button variant="outline" size="sm" disabled={!!state.busy || !nodeVersion} onClick={() =>
                    run(t('site.runtime.saving', { tool: 'Node' }), (c) => c.isolate('node', nodeVersion, root))
                  }>{t('site.runtime.isolate', { tool: 'Node' })}</Button>
                </CardContent>
              </Card>
              <Metric label={t('site.runtime.file')} value={detail?.php_file || detail?.node_file || ''} />
            </TabsPanel>

            {/* 7. Logs — reuse LogViewer, filtered to this site */}
            <TabsPanel className="flex flex-col gap-4">
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs text-muted-foreground">{t('site.logs.help')}</p>
                <Button variant="outline" size="sm" disabled={!logs} onClick={() => setViewerOpen(true)}>
                  <ExternalLink className="size-3.5" aria-hidden="true" />
                  {t('site.logs.open')}
                </Button>
              </div>
              {loading ? (
                <p className="text-xs text-muted-foreground">{t('site.logs.loading')}</p>
              ) : logs ? (
                <pre className="max-h-64 overflow-auto rounded-lg border border-border bg-muted/40 p-3 text-xs whitespace-pre-wrap">{logs}</pre>
              ) : (
                <EmptyState icon={<FileText />} title={t('site.logs.empty')} />
              )}
            </TabsPanel>
          </TabsPanels>
        </Tabs>
      </Dialog>

      <Dialog
        open={vhostOpen}
        onClose={() => setVhostOpen(false)}
        title={t('site.vhost.title', { name })}
        description={t('site.vhost.description')}
        size="lg"
      >
        {vhostLoading ? (
          <p className="text-xs text-muted-foreground">{t('site.vhost.loading')}</p>
        ) : vhostError ? (
          <p role="alert" className="text-xs text-destructive">{vhostError}</p>
        ) : vhost ? (
          <CodeBlock text={vhost} />
        ) : (
          <p className="text-xs text-muted-foreground">{t('site.vhost.empty')}</p>
        )}
      </Dialog>

      {viewerOpen && (
        <div className="fixed inset-0 z-[60]">
          <LogViewer title={`${t('site.logs.title')} · ${name}`} text={logs} onClose={() => setViewerOpen(false)} />
        </div>
      )}
    </>
  );
}
