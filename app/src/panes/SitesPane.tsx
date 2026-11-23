import { useState } from 'react';
import { ExternalLink, Globe2, Plus, RefreshCw, Settings2, Trash2 } from 'lucide-react';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Dialog,
  EmptyState,
  Input,
  Switch,
} from '../components';
import { useT } from '../i18n';
import type { SiteView, WebView } from '../api';
import { ProjectDialog } from './ProjectDialog';
import { SiteDetailDialog } from './SiteDetailDialog';
import { LairConfigDialog } from './LairConfigDialog';
import type { PaneProps } from './types';

type DialogKind = '' | 'project';

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
  const web = state.status?.web;
  const domain = web?.domain || 'test';
  const [dialog, setDialog] = useState<DialogKind>('');
  const [detailSite, setDetailSite] = useState<SiteView | null>(null);
  const [deleteSite, setDeleteSite] = useState<SiteView | null>(null);
  const [deleteFiles, setDeleteFiles] = useState(false);
  const [query, setQuery] = useState('');
  const [openError, setOpenError] = useState('');
  const [valetOpen, setValetOpen] = useState(false);

  const openInBrowser = async (host: string) => {
    if (!state.client || !host) return;
    setOpenError('');
    try {
      const result = await state.client.siteOpen(host);
      const url = result.url || result.output;
      if (!url) throw new Error(t('site.openFailed'));
      window.open(url, '_blank', 'noopener,noreferrer');
    } catch (cause) {
      setOpenError(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const close = () => setDialog('');

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

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap gap-2">
        <Button variant="primary" size="sm" title={t('project.newHelp')} disabled={!!state.busy} onClick={() => setDialog('project')}>
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
                    <Button variant="outline" size="sm" disabled={!!state.busy} title={t('site.detailsHelp')} onClick={() => setDetailSite(site)}>
                      <Settings2 className="size-3.5" aria-hidden="true" />
                      {t('site.details')}
                    </Button>
                    <Button
                      variant="danger"
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
