import { useState } from 'react';
import { ExternalLink, Globe2, Plus, RefreshCw, Settings2, Trash2 } from 'lucide-react';
import {
  ActionsMenu,
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CodeBlock,
  Dialog,
  EmptyState,
  Input,
  Select,
  Switch,
} from '../components';
import { useT } from '../i18n';
import type { SiteView, WebView } from '../api';
import { ProjectDialog } from './ProjectDialog';
import { SiteDetailDialog } from './SiteDetailDialog';
import { LairConfigDialog } from './LairConfigDialog';
import type { PaneProps } from './types';

type DialogKind = '' | 'project' | 'pin' | 'isolate';

interface SiteForm {
  php: string;
  node: string;
  database: string;
}

const DATABASES = ['sqlite', 'mysql', 'mariadb', 'pgsql', 'sqlsrv'];

function siteHost(site: SiteView): string {
  return site.name ?? site.host ?? '';
}

function siteRoot(site: SiteView): string {
  return site.root ?? site.path ?? '';
}

function siteURL(site: SiteView, web?: WebView): string {
  const host = siteHost(site);
  if (!host) return '';
  const secure = Boolean(site.secure && web?.https);
  const port = secure ? (web?.https_port ?? 8443) : (web?.http_port ?? 8080);
  const authority = port === 80 || port === 443 ? host : `${host}:${port}`;
  return `${secure ? 'https' : 'http'}://${authority}`;
}

/** Only the virtual host is shown; the real URL (with its port) stays in href. */
function siteLabel(site: SiteView): string {
  return siteHost(site) || '—';
}

function projectName(site: SiteView, domain: string): string {
  const host = siteHost(site);
  const suffix = `.${domain}`;
  if (domain && host.endsWith(suffix)) return host.slice(0, -suffix.length);
  return host.split('.')[0] || host || '—';
}

export function SitesPane({ state, onNavigate }: PaneProps) {
  const t = useT();
  const sites = state.status?.sites ?? [];
  const defaults = state.status?.defaults ?? {};
  const web = state.status?.web;
  const domain = web?.domain || 'test';
  const [dialog, setDialog] = useState<DialogKind>('');
  const [values, setValues] = useState<string[]>([]);
  const [configSite, setConfigSite] = useState<SiteView | null>(null);
  const [detailSite, setDetailSite] = useState<SiteView | null>(null);
  const [deleteSite, setDeleteSite] = useState<SiteView | null>(null);
  const [deleteFiles, setDeleteFiles] = useState(false);
  const [query, setQuery] = useState('');
  const [form, setForm] = useState<SiteForm>({ php: '', node: '', database: 'sqlite' });
  const [openError, setOpenError] = useState('');
  const [opening, setOpening] = useState('');
  const [valetOpen, setValetOpen] = useState(false);

  const openInBrowser = async (host: string) => {
    if (!state.client || !host) return;
    setOpening(host);
    setOpenError('');
    try {
      const result = await state.client.siteOpen(host);
      const url = result.url || result.output;
      if (!url) throw new Error(t('site.openFailed'));
      window.open(url, '_blank', 'noopener,noreferrer');
    } catch (cause) {
      setOpenError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setOpening('');
    }
  };

  const open = (kind: DialogKind, initial: string[] = []) => {
    setValues(initial);
    setDialog(kind);
  };
  const close = () => setDialog('');
  const run = (label: string, fn: Parameters<typeof state.act>[1]) => {
    void state.act(label, fn);
    close();
  };
  const field = (label: string, index: number, placeholder?: string) => (
    <label key={`${dialog}-${index}`} className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <Input
        autoFocus={index === 0}
        value={values[index] ?? ''}
        placeholder={placeholder}
        onChange={(event) => setValues((prev) => {
          const next = [...prev];
          next[index] = event.target.value;
          return next;
        })}
      />
    </label>
  );

  const titles: Record<DialogKind, string> = {
    '': '',
    project: t('project.newProject'),
    pin: t('dash.pin'),
    isolate: t('dash.isolate'),
  };

  const openConfig = (site: SiteView) => {
    setConfigSite(site);
    setForm({
      php: site.php ?? '',
      node: site.node ?? '',
      database: site.database ?? 'sqlite',
    });
  };

  const saveConfig = () => {
    if (!configSite) return;
    const host = siteHost(configSite);
    const root = siteRoot(configSite);
    void state.act(t('site.saved'), async (c) => {
      await c.isolate('php', form.php, root);
      await c.isolate('node', form.node, root);
      await c.projectSetDB(host, form.database);
    });
    setConfigSite(null);
  };

  const confirmDelete = () => {
    const host = siteHost(deleteSite ?? {});
    if (!host) return;
    void state.act(t('site.deleting', { name: host }), (c) => c.unlink(host, deleteFiles));
    setDeleteSite(null);
  };

  const needle = query.trim().toLowerCase();
  const visible = needle
    ? sites.filter((site) => [siteHost(site), siteRoot(site), site.database, site.php, site.node]
        .some((value) => (value ?? '').toLowerCase().includes(needle)))
    : sites;
  const secureCount = sites.filter((site) => Boolean(site.secure)).length;

  const phpOptions = [
    { value: '', label: t('site.defaultRuntime') },
    ...(state.status?.php ?? []).map((item) => ({
      value: item.minor ?? '',
      label: `PHP ${item.minor ?? '—'}`,
    })),
  ];
  const nodeOptions = [
    { value: '', label: t('site.defaultRuntime') },
    ...(state.status?.node ?? []).map((version) => ({ value: version, label: `Node ${version}` })),
  ];
  const databaseOptions = DATABASES.map((value) => ({ value, label: value }));

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap gap-2">
        <Button variant="primary" size="sm" title={t('project.newHelp')} disabled={!!state.busy} onClick={() => open('project')}>
          <Plus className="size-3.5" aria-hidden="true" />
          {t('project.newProject')}
        </Button>
        <Button variant="outline" size="sm" title={t('site.refreshProjectsHelp')} disabled={!!state.busy} onClick={() => void state.act(t('site.refreshProjects'), (c) => c.projectSync())}>
          <RefreshCw className="size-3.5" aria-hidden="true" />
          {t('site.refreshProjects')}
        </Button>
        <Button variant="outline" size="sm" title={t('lair.config.subtitle')} disabled={!!state.busy} onClick={() => setValetOpen(true)}>
          <Settings2 className="size-3.5" aria-hidden="true" />
          {t('lair.config.button')}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('sites.help')}</p>

      <div className="flex flex-wrap items-center gap-3">
        <label className="flex min-w-56 flex-1 flex-col gap-1.5">
          <Input
            type="search"
            aria-label={t('sites.search')}
            placeholder={t('sites.searchPlaceholder')}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone="blue">{t('sites.total', { count: sites.length })}</Badge>
          <Badge tone="green" dot>{t('sites.secureCount', { count: secureCount })}</Badge>
        </div>
      </div>

      {openError && (
        <p role="alert" className="whitespace-pre-wrap rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs text-amber-800 dark:text-amber-300">
          {openError}
        </p>
      )}

      {sites.length === 0 ? (
        <EmptyState icon={<Globe2 />} title={t('dash.noSites')} description={t('pane.sites.subtitle')} />
      ) : visible.length === 0 ? (
        <EmptyState icon={<Globe2 />} title={t('sites.noMatches')} />
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {visible.map((site) => {
            const host = siteHost(site);
            const root = siteRoot(site);
            const url = siteURL(site, web);
            const label = siteLabel(site);
            return (
              <Card key={host}>
                <CardHeader className="flex-row items-start justify-between space-y-0">
                  <div className="min-w-0">
                    <CardTitle className="truncate text-base">{projectName(site, domain)}</CardTitle>
                    <a
                      href={url || undefined}
                      target="_blank"
                      rel="noreferrer"
                      title={url}
                      onClick={(event) => { event.preventDefault(); void openInBrowser(host); }}
                      className="mt-1 inline-flex items-center gap-1 truncate text-xs font-medium text-primary hover:underline"
                    >
                      {label}
                      <ExternalLink className="size-3" aria-hidden="true" />
                    </a>
                    <p className="mt-1 truncate text-xs text-muted-foreground">{root || '—'}</p>
                  </div>
                  <div className="flex flex-wrap justify-end gap-1.5">
                    {site.secure && <Badge tone="green" dot>{t('dash.secure')}</Badge>}
                    {site.php && <Badge tone="indigo">{t('dash.phpShort', { version: site.php })}</Badge>}
                    {site.database && <Badge tone="orange">{site.database}</Badge>}
                    {site.node && <Badge tone="green">Node {site.node}</Badge>}
                  </div>
                </CardHeader>
                <CardContent className="flex flex-col gap-3">
                  <Switch
                    checked={Boolean(site.secure)}
                    disabled={!!state.busy || !host}
                    onChange={(on) => void state.act(t('site.https'), (c) => c.secure(host, on))}
                    label={t('site.https')}
                    description={t('site.httpsHelp')}
                  />
                  <div className="flex flex-wrap items-center gap-2">
                    <Button variant="outline" size="sm" disabled={!url || !!state.busy || opening === host} onClick={() => void openInBrowser(host)}>
                      <ExternalLink className="size-3.5" aria-hidden="true" />
                      {t('site.open')}
                    </Button>
                    <Button variant="outline" size="sm" disabled={!!state.busy} title={t('site.detailsHelp')} onClick={() => setDetailSite(site)}>
                      <Settings2 className="size-3.5" aria-hidden="true" />
                      {t('site.details')}
                    </Button>
                    <Button variant="ghost" size="sm" disabled={!!state.busy} onClick={() => openConfig(site)}>
                      <Settings2 className="size-3.5" aria-hidden="true" />
                      {t('site.configure')}
                    </Button>
                    <ActionsMenu
                      label={t('common.actions', { name: host })}
                      actions={[
                        {
                          label: t('dash.pin'),
                          disabled: !!state.busy,
                          onSelect: () => open('pin', [host, 'php', defaults.php ?? '']),
                        },
                        {
                          label: t('dash.isolate'),
                          disabled: !!state.busy,
                          onSelect: () => open('isolate', [host, 'php', defaults.php ?? '']),
                        },
                      ]}
                    />
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!!state.busy}
                      aria-label={`${t('site.delete')} ${host}`}
                      onClick={() => { setDeleteFiles(false); setDeleteSite(site); }}
                    >
                      <Trash2 className="size-3.5" aria-hidden="true" />
                      {t('site.delete')}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <Dialog
        open={dialog !== '' && dialog !== 'project'}
        onClose={close}
        title={titles[dialog]}
        description={dialog === 'pin' ? t('dash.pinHelp') : undefined}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={close}>{t('common.cancel')}</Button>
            <Button disabled={values.some((value) => !value.trim())} onClick={() => {
              if (dialog === 'pin') {
                const site = sites.find((item) => siteHost(item) === values[0]);
                run(t('dash.pin'), (c) => c.pin(values[1] ?? '', values[2] ?? '', siteRoot(site ?? {})));
              }
              if (dialog === 'isolate') {
                const site = sites.find((item) => siteHost(item) === values[0]);
                run(t('dash.isolate'), (c) => c.isolate(values[1] ?? '', values[2] ?? '', siteRoot(site ?? {})));
              }
            }}>{t('common.save')}</Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          {(dialog === 'pin' || dialog === 'isolate') && (
            <>
              {field(t('dash.siteName'), 0)}
              {field(t('dash.tool'), 1)}
              {field(t('dash.version'), 2)}
            </>
          )}
        </div>
      </Dialog>

      <Dialog
        open={configSite !== null}
        onClose={() => setConfigSite(null)}
        title={configSite ? `${t('site.configure')} · ${projectName(configSite, domain)}` : ''}
        description={t('site.configureHelp')}
        size="md"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfigSite(null)}>{t('common.cancel')}</Button>
            <Button onClick={saveConfig} disabled={!!state.busy}>{t('common.save')}</Button>
          </>
        }
      >
        {configSite && (
          <div className="flex flex-col gap-4">
            <div>
              <span className="text-xs font-medium text-muted-foreground">{t('site.host')}</span>
              <CodeBlock text={siteHost(configSite)} />
            </div>
            <div>
              <span className="text-xs font-medium text-muted-foreground">{t('site.url')}</span>
              <CodeBlock text={siteURL(configSite, web)} />
            </div>
            <div>
              <span className="text-xs font-medium text-muted-foreground">{t('site.root')}</span>
              <CodeBlock text={siteRoot(configSite)} />
            </div>
            <Select value={form.php} onChange={(value) => setForm((prev) => ({ ...prev, php: value }))} options={phpOptions} label={t('site.php')} />
            <Select value={form.node} onChange={(value) => setForm((prev) => ({ ...prev, node: value }))} options={nodeOptions} label={t('site.node')} />
            <Select value={form.database} onChange={(value) => setForm((prev) => ({ ...prev, database: value }))} options={databaseOptions} label={t('site.database')} />
          </div>
        )}
      </Dialog>

      <Dialog
        open={deleteSite !== null}
        onClose={() => setDeleteSite(null)}
        title={t('site.delete.title', { name: siteHost(deleteSite ?? {}) })}
        description={t('site.delete.description')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteSite(null)}>{t('common.cancel')}</Button>
            <Button variant="danger" disabled={!!state.busy} onClick={confirmDelete}>
              <Trash2 className="size-3.5" aria-hidden="true" />
              {t('site.delete.confirm')}
            </Button>
          </>
        }
      >
        <label className="flex items-start gap-3">
          <input
            type="checkbox"
            checked={deleteFiles}
            aria-label={t('site.delete.deleteFiles')}
            onChange={(event) => setDeleteFiles(event.target.checked)}
            className="mt-0.5 size-4 shrink-0 rounded border-input"
          />
          <span className="flex flex-col gap-0.5">
            <span className="text-sm font-medium">{t('site.delete.deleteFiles')}</span>
            <span className="text-xs text-muted-foreground">{t('site.delete.deleteFilesHelp')}</span>
          </span>
        </label>
      </Dialog>

      <SiteDetailDialog
        open={detailSite !== null}
        site={detailSite}
        state={state}
        onNavigate={onNavigate}
        onClose={() => setDetailSite(null)}
      />

      <ProjectDialog open={dialog === 'project'} onClose={close} state={state} />

      <LairConfigDialog state={state} open={valetOpen} onClose={() => setValetOpen(false)} />
    </div>
  );
}
